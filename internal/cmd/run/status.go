package run

import (
	"hashrouter/internal/globals"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// proxyHealthHandleFunc is an HTTP HandleFunc to check
// the health of a proxy and writes it as HTTP response
func proxyHealthHandleFunc(res http.ResponseWriter, req *http.Request) {
	var status int
	var message []byte

	var isHealthy bool

	proxyName := req.PathValue("name")

	_, proxyFound := globals.Application.ProxyPool[proxyName]

	// Proxy not found
	if !proxyFound {
		status = http.StatusNotFound
		message = []byte("NOT FOUND")
		goto sendResponse
	}

	// Proxy is not healthy
	globals.Application.ProxyPool[proxyName].Status.RWMutex.RLock()
	isHealthy = globals.Application.ProxyPool[proxyName].Status.IsHealthy
	globals.Application.ProxyPool[proxyName].Status.RWMutex.RUnlock()

	if !isHealthy {
		status = http.StatusServiceUnavailable
		message = []byte("SERVICE UNAVAILABLE")
		goto sendResponse
	}

	status = http.StatusOK
	message = []byte("OK")

	//
sendResponse:
	res.WriteHeader(status)
	res.Write(message)
}

// Start a webserver for exposing metrics endpoint in the background
func RunStatusWebserver(logger *zap.SugaredLogger, host string, port string) {

	var err error

	metricsHost := host + ":" + port

	http.Handle("/metrics", promhttp.Handler())
	logger.Infof("starting metrics endpoint on host '%s' and path '/metrics'", metricsHost)

	http.HandleFunc("GET /{name}/health", proxyHealthHandleFunc)
	logger.Infof("starting health endpoint on host '%s' and path '/{proxy-name}/health'", metricsHost)

	err = http.ListenAndServe(metricsHost, nil)
	if err != nil {
		logger.Fatalf(MetricsWebserverErrorMessage, err)
	}
}
