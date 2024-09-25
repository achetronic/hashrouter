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
	// defaultBackendConnectTimeoutMilliseconds is the default timeout for connecting to a backend
	defaultBackendConnectTimeoutMilliseconds = 40
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

	var err error

	// The variable 'lastErr' is used to store the last error that occurred while trying to connect to a backend.
	// You should be wondering why we are using this variable... Well, there is a 'kind of' race condition where
	// the we could cause a panic in runtime using directly 'err' during the loop you will observe soon.
	var lastErr error

	connectionExtraData := ConnectionExtraData{}

	requestId := generateRandToken()
	connectionExtraData.RequestId = requestId

	// calculate hashkey
	hashKey, err := ReplaceRequestTags(r, p.Config.HashKey.Pattern)
	if err != nil {
		p.Logger.Errorf("error calculating hash_key: %v", err.Error())
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
		httpTimeout := defaultBackendConnectTimeoutMilliseconds * time.Millisecond
		if p.Config.Options.BackendConnectTimeoutMilliseconds > 0 {
			httpTimeout = time.Duration(p.Config.Options.BackendConnectTimeoutMilliseconds) * time.Millisecond
		}

		var backendCient = &http.Client{
			Timeout: httpTimeout,
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
		p.Logger.Debugf("failed connecting to server '%s': %v", hashringServerPool[indexToTry], err.Error())

		// There is an error but user does not want to try another backend
		if !p.Config.Options.TryAnotherBackendOnFailure {
			p.Logger.Infof("'options.try_another_backend_on_failure' is disabled, skip trying another backend.")
			break
		}
	}

	if lastErr != nil {
		p.Logger.Errorf("failed connecting to all backend servers: %v", lastErr.Error())
		connectionExtraData.Backend = "none"

		writeDirectResponse(w, http.StatusServiceUnavailable, "Service Unavailable")
		return
	}

	if len(hashringServerPool) == 0 {
		p.Logger.Errorf("failed connecting to all backend servers: no backends found")
		connectionExtraData.Backend = "none"

		writeDirectResponse(w, http.StatusServiceUnavailable, "Service Unavailable")
		return
	}

	// Throw request log as early as possible
	if p.GlobalConfig.Logs.ShowAccessLogs {
		logFields := GetRequestLogFields(r, connectionExtraData, p.GlobalConfig.Logs.AccessLogsFields)
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
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		p.Logger.Errorf("failed sending body to the backends: %s", err.Error())
	}

	//
	if p.GlobalConfig.Logs.ShowAccessLogs {
		logFields := GetResponseLogFields(resp, connectionExtraData, p.GlobalConfig.Logs.AccessLogsFields)
		p.Logger.Infow("response", logFields...)
	}
}

// TODO
func (p *ProxyT) RunHttp() (err error) {

	p.Status.RWMutex.Lock()
	p.Status.IsHealthy = true
	p.Status.RWMutex.Unlock()

	err = http.ListenAndServe(
		p.Config.Listener.Address+":"+strconv.Itoa(p.Config.Listener.Port),
		http.HandlerFunc(p.HTTPHandleFunc))

	return err
}
