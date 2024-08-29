package proxy

import (
	"crypto/rand"
	"fmt"
	"hashrouter/internal/globals"
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
func (p *Proxy) HTTPHandleFunc(w http.ResponseWriter, r *http.Request) {

	var err error

	connectionExtraData := ConnectionExtraData{}

	requestId := generateRandToken()
	connectionExtraData.RequestId = requestId

	// calculate hashkey
	hashKey, err := ReplaceRequestTags(r, p.Config.HashKey.Pattern)
	if err != nil {
		globals.Application.Logger.Errorf("error calculating hash_key: %v", err.Error())
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
			globals.Application.Logger.Errorf("error creating request object: %s", err.Error())
			break
		}
		req.Header = r.Header

		//
		http.DefaultClient.Timeout = defaultBackendConnectTimeoutMilliseconds * time.Millisecond
		if p.Config.Options.BackendConnectTimeoutMilliseconds > 0 {
			http.DefaultClient.Timeout = time.Duration(p.Config.Options.BackendConnectTimeoutMilliseconds) * time.Millisecond
		}

		//
		resp, err = http.DefaultClient.Do(req)
		if err == nil {
			connectionExtraData.Backend = hashringServerPool[indexToTry]
			break
		}

		// TODO: Discuss this message usefulness with more people
		globals.Application.Logger.Debugf("failed connecting to server '%s': %v", hashringServerPool[indexToTry], err.Error())

		// There is an error but user does not want to try another backend
		if !p.Config.Options.TryAnotherBackendOnFailure {
			globals.Application.Logger.Infof("'options.try_another_backend_on_failure' is disabled, skip trying another backend.")
			break
		}
	}

	if err != nil {
		globals.Application.Logger.Errorf("failed connecting to all backend servers: %v", err.Error())
		connectionExtraData.Backend = "none"

		writeDirectResponse(w, http.StatusServiceUnavailable, "Service Unavailable")
		return
	}

	if len(hashringServerPool) == 0 {
		globals.Application.Logger.Errorf("failed connecting to all backend servers: no backends found")
		connectionExtraData.Backend = "none"

		writeDirectResponse(w, http.StatusServiceUnavailable, "Service Unavailable")
		return
	}

	// Throw request log as early as possible
	if globals.Application.Config.Logs.ShowAccessLogs {
		logFields := GetRequestLogFields(r, connectionExtraData, globals.Application.Config.Logs.AccessLogsFields)
		globals.Application.Logger.Infow("request", logFields...)
	}

	// Clone the headers
	for k, v := range resp.Header {
		for _, headV := range v {
			w.Header().Set(k, headV)
		}
	}

	// Copy the data without trully reading it
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		globals.Application.Logger.Errorf("failed sending body to the backends: %s", err.Error())
	}

	//
	if globals.Application.Config.Logs.ShowAccessLogs {
		logFields := GetResponseLogFields(resp, connectionExtraData, globals.Application.Config.Logs.AccessLogsFields)
		globals.Application.Logger.Infow("response", logFields...)
	}
}

// TODO
func (p *Proxy) RunHttp() (err error) {

	err = http.ListenAndServe(p.Config.Listener.Address+":"+strconv.Itoa(p.Config.Listener.Port), http.HandlerFunc(p.HTTPHandleFunc))
	if err != nil {
		return fmt.Errorf("error launching the listener: %v", err.Error())
	}

	return nil
}
