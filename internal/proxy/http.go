package proxy

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (

	// Maximum time in milliseconds to read the request from the client.
	// (default: 0ms [no timeout])
	defaultHttpServerReadTimeoutMillis = 0

	// Maximum time in milliseconds to write the response to the client.
	// (default: 0ms [no timeout])
	defaultHttpServerWriteTimeoutMillis = 0

	// Disable keep alives on the server.
	// (default: false)
	defaultHttpServerDisableKeepAlives = false

	// Maximum time in milliseconds to wait for the entire backend request to complete,
	// including both connection and data transfer.
	// If the request takes longer than this timeout, it will be aborted.
	// (default: 0ms [no timeout])
	defaultHttpBackendRequestTimeoutMillis = 0

	// Maximum time in milliseconds to establish a connection with the backend.
	// If the dial takes longer than this timeout, it will be aborted. (default: 0s)
	// A timeout of 0 means no timeout.
	defaultHttpBackendDialTimeoutMillis = 0

	// Time between keep-alive messages on established connection to the backend
	// (default: 15s)
	defaultHttpBackendKeepAliveMillis = 15000

	// Disable keep alives to the backend.
	// (default: false)
	defaultHttpBackendDisableKeepAlives = false
)

// writeDirectResponse writes a static response.
// This is used to send errors to the client
func writeDirectResponse(w http.ResponseWriter, statusCode int, message string) {

	message = fmt.Sprintf("%d %s\n", statusCode, message)

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Length", strconv.Itoa(len(message)))

	w.WriteHeader(statusCode)
	w.Write([]byte(message))
}

// generateRandToken generates a random token to be used as a request ID
func generateRandToken() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// getConfiguredHttpServer returns an HTTP server already configured according to the proxy configuration
func (p *ProxyT) getConfiguredHttpServer(addr string, handler http.Handler) (server *http.Server) {

	server = &http.Server{}

	//
	readTimeout := defaultHttpServerReadTimeoutMillis * time.Millisecond
	if p.SelfConfig.Options.HttpServerReadTimeoutMillis > 0 {
		readTimeout = time.Duration(p.SelfConfig.Options.HttpServerReadTimeoutMillis) * time.Millisecond
	}

	//
	writeTimeout := defaultHttpServerWriteTimeoutMillis * time.Millisecond
	if p.SelfConfig.Options.HttpServerWriteTimeoutMillis > 0 {
		writeTimeout = time.Duration(p.SelfConfig.Options.HttpServerWriteTimeoutMillis) * time.Millisecond
	}

	//
	disableKeepAlives := defaultHttpServerDisableKeepAlives
	if p.SelfConfig.Options.HttpServerDisableKeepAlives {
		disableKeepAlives = true
	}

	//
	server.Addr = addr
	server.Handler = handler
	server.ReadTimeout = readTimeout
	server.WriteTimeout = writeTimeout
	server.SetKeepAlivesEnabled(!disableKeepAlives)

	return server
}

// getConfiguredHttpClient returns an HTTP client already configured according to the proxy configuration
func (p *ProxyT) getConfiguredHttpClient() *http.Client {

	requestTimeout := defaultHttpBackendRequestTimeoutMillis * time.Millisecond
	if p.SelfConfig.Options.HttpBackendRequestTimeoutMillis > 0 {
		requestTimeout = time.Duration(p.SelfConfig.Options.HttpBackendRequestTimeoutMillis) * time.Millisecond
	}

	dialTimeout := defaultHttpBackendDialTimeoutMillis * time.Millisecond
	if p.SelfConfig.Options.HttpBackendDialTimeoutMillis > 0 {
		dialTimeout = time.Duration(p.SelfConfig.Options.HttpBackendDialTimeoutMillis) * time.Millisecond
	}

	//
	keepAlive := defaultHttpBackendKeepAliveMillis * time.Millisecond
	if p.SelfConfig.Options.HttpBackendKeepAliveMillis > 0 {
		keepAlive = time.Duration(p.SelfConfig.Options.HttpBackendKeepAliveMillis) * time.Millisecond
	}

	//
	disableKeepAlives := defaultHttpBackendDisableKeepAlives
	if p.SelfConfig.Options.HttpBackendDisableKeepAlives {
		disableKeepAlives = true
	}

	return &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			DisableKeepAlives: disableKeepAlives,
			DialContext: (&net.Dialer{
				Timeout:   dialTimeout,
				KeepAlive: keepAlive,
			}).DialContext,
		},
	}
}

// TODO
func (p *ProxyT) HTTPHandleFunc(w http.ResponseWriter, r *http.Request) {

	//
	httpRequestsTotalMetricLabels := map[string]string{
		"proxy_name": p.SelfConfig.Name,
		"method":     r.Method,
	}

	defer func() {
		p.Meter.HttpRequestsTotal.With(httpRequestsTotalMetricLabels).Add(1)
	}()

	var err error

	// The variable 'lastErr' is used to store the last error that occurred while trying to connect to a backend.
	// You should be wondering why we are using this variable... Well, there is a 'kind of' race condition where
	// the we could error a panic in runtime using directly 'err' during the loop you will observe soon.
	var lastErr error

	connectionExtraData := ConnectionExtraData{}

	requestId := generateRandToken()
	connectionExtraData.RequestId = requestId

	// calculate hashkey
	hashKey := ReplaceRequestTags(r, p.SelfConfig.HashKey.Pattern)
	hashKey = ReplaceRequestHeaderTags(r, hashKey)
	hashKey = strings.TrimSpace(hashKey)
	if len(hashKey) == 0 {
		p.Logger.Error("error calculating hash_key: can not be empty")

		httpRequestsTotalMetricLabels["delivered_status_code"] = strconv.Itoa(http.StatusInternalServerError)
		httpRequestsTotalMetricLabels["error"] = "hash_key_calculation_failed"

		writeDirectResponse(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	connectionExtraData.Hashkey = hashKey

	// get server
	dueBackend := p.Hashring.GetServer(hashKey)
	hashringServerPool := p.Hashring.GetServerList()
	dueBackendPoolIndex := slices.Index(hashringServerPool, dueBackend)

	var resp *http.Response
	requestBodyContent := &bytes.Buffer{}
	for i := 0; i < len(hashringServerPool); i++ {

		// When the loop is in last item, start from the beginning
		indexToTry := (dueBackendPoolIndex + i) % len(hashringServerPool)

		currentSelectedBackend := hashringServerPool[indexToTry]

		url := fmt.Sprintf("http://%s%s", currentSelectedBackend, r.URL.Path+"?"+r.URL.RawQuery)

		// The following is a trick to read the request body content
		// using streaming techniques instead of wasting memory
		// this way we can copy it keeping the memory consumption plain
		// teeReader = (r.Body -> pipeWriter -> pipeReader -> sink)
		pipeReader, pipeWriter := io.Pipe()
		teeReader := io.TeeReader(r.Body, pipeWriter)

		// Following goroutine is just for consuming from the pipe,
		// storing or discarding the content, just because pipes are blocking
		var wg sync.WaitGroup
		requestBodyContent.Reset()

		wg.Add(1)
		go func() {
			defer wg.Done()

			if p.CommonConfig.Logs.EnableRequestBodyLogs {
				io.Copy(requestBodyContent, pipeReader)
				return
			}
			io.Copy(io.Discard, pipeReader)
		}()

		//req, err := http.NewRequest(r.Method, url, r.Body)
		req, err := http.NewRequest(r.Method, url, teeReader)
		if err != nil {
			p.Logger.Errorf("error creating request object: %s", err.Error())
			break
		}
		req.Header = r.Header

		// BackendCient represents the HTTP client to be used across concurrent requests
		backendCient := p.getConfiguredHttpClient()

		//
		resp, err = backendCient.Do(req)

		// After .Do call finish, force closing body-stalker goroutine
		// and wait before using the result
		pipeWriter.Close()
		wg.Wait()

		if err == nil {
			connectionExtraData.Backend = hashringServerPool[indexToTry]
			lastErr = nil
			break
		}
		lastErr = err

		// TODO: Discuss this message usefulness with more people
		p.Logger.Debugf("failed connecting to server '%s': %s", hashringServerPool[indexToTry], err.Error())

		// There is an error but user does not want to try another backend
		if !p.SelfConfig.Options.TryAnotherBackendOnFailure {
			p.Logger.Infof("'options.try_another_backend_on_failure' is disabled, skip trying another backend.")
			break
		}
	}

	if lastErr != nil {
		p.Logger.Errorf("failed connecting to all backend servers: %s", lastErr.Error())
		connectionExtraData.Backend = "none"

		httpRequestsTotalMetricLabels["delivered_status_code"] = strconv.Itoa(http.StatusServiceUnavailable)
		httpRequestsTotalMetricLabels["error"] = "all_backends_failed"

		writeDirectResponse(w, http.StatusServiceUnavailable, "Service Unavailable")
		return
	}

	if len(hashringServerPool) == 0 {
		p.Logger.Errorf("failed connecting to all backend servers: no backends found")
		connectionExtraData.Backend = "none"

		httpRequestsTotalMetricLabels["delivered_status_code"] = strconv.Itoa(http.StatusServiceUnavailable)
		httpRequestsTotalMetricLabels["error"] = "no_backends_found"

		writeDirectResponse(w, http.StatusServiceUnavailable, "Service Unavailable")
		return
	}

	// Throw request log as early as possible
	if p.CommonConfig.Logs.ShowAccessLogs {
		logFields := GetRequestLogFields(r, connectionExtraData, p.CommonConfig.Logs.AccessLogsFields, requestBodyContent,
			p.CommonConfig.Logs.EnableRequestBodyLogsJsonParsing)
		p.Logger.Infow("request", logFields...)
	}

	// Clone the headers
	for k, v := range resp.Header {
		for _, headV := range v {
			w.Header().Set(k, headV)
		}
	}

	// set status code of the response
	w.WriteHeader(resp.StatusCode)

	// Copy the data without trully reading it
	httpRequestsTotalMetricLabels["delivered_status_code"] = strconv.Itoa(resp.StatusCode)
	httpRequestsTotalMetricLabels["error"] = "none"

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		p.Logger.Errorf("failed copying body to the frontend: %s", err.Error())

		httpRequestsTotalMetricLabels["error"] = "body_copy_failed"
	}

	//
	if p.CommonConfig.Logs.ShowAccessLogs {
		logFields := GetResponseLogFields(resp, connectionExtraData, p.CommonConfig.Logs.AccessLogsFields)
		p.Logger.Infow("response", logFields...)
	}
}

// TODO
func (p *ProxyT) RunHttp() (err error) {

	p.Status.RWMutex.Lock()
	p.Status.IsHealthy = true
	p.Status.RWMutex.Unlock()

	httpServer := p.getConfiguredHttpServer(
		p.SelfConfig.Listener.Address+":"+strconv.Itoa(p.SelfConfig.Listener.Port),
		http.HandlerFunc(p.HTTPHandleFunc))

	err = httpServer.ListenAndServe()

	return err
}
