package proxy

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (

	// (optional) Maximum time in milliseconds to wait for the entire backend request to complete,
	// including both connection and data transfer.
	// If the request takes longer than this timeout, it will be aborted. (default: 40ms)
	defaultHttpBackendRequestTimeoutMilliseconds = 40
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
	hashKey, err := ReplaceRequestTags(r, p.SelfConfig.HashKey.Pattern)
	if err != nil {
		p.Logger.Errorf("error calculating hash_key: %v", err.Error())

		httpRequestsTotalMetricLabels["delivered_status_code"] = strconv.Itoa(http.StatusInternalServerError)
		httpRequestsTotalMetricLabels["error"] = "hash_key_calculation_failed"

		writeDirectResponse(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	hashKey = strings.TrimSpace(hashKey)
	connectionExtraData.Hashkey = hashKey

	// get server
	dueBackend := p.Hashring.GetServer(hashKey)
	hashringServerPool := p.Hashring.GetServerList()
	dueBackendPoolIndex := slices.Index(hashringServerPool, dueBackend)

	var resp *http.Response
	for i := 0; i < len(hashringServerPool); i++ {

		// When the loop is in last item, start from the beginning
		indexToTry := (dueBackendPoolIndex + i) % len(hashringServerPool)

		currentSelectedBackend := hashringServerPool[indexToTry]

		url := fmt.Sprintf("http://%s%s", currentSelectedBackend, r.URL.Path+"?"+r.URL.RawQuery)
		req, err := http.NewRequest(r.Method, url, r.Body)
		if err != nil {
			p.Logger.Errorf("error creating request object: %s", err.Error())
			break
		}
		req.Header = r.Header

		//
		backendClientTimeout := defaultHttpBackendRequestTimeoutMilliseconds * time.Millisecond
		if p.SelfConfig.Options.HttpBackendRequestTimeoutMilliseconds > 0 {
			backendClientTimeout = time.Duration(p.SelfConfig.Options.HttpBackendRequestTimeoutMilliseconds) * time.Millisecond
		}

		// BackendCient represents the HTTP client to be used across concurrent requests
		backendCient := &http.Client{
			Timeout: backendClientTimeout,
		}

		//
		resp, err = backendCient.Do(req)
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
		logFields := GetRequestLogFields(r, connectionExtraData, p.CommonConfig.Logs.AccessLogsFields)
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

	err = http.ListenAndServe(
		p.SelfConfig.Listener.Address+":"+strconv.Itoa(p.SelfConfig.Listener.Port),
		http.HandlerFunc(p.HTTPHandleFunc))

	return err
}
