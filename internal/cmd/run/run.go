package run

import (
	"fmt"
	"hashrouter/internal/config"
	"hashrouter/internal/globals"
	"hashrouter/internal/metrics"
	"hashrouter/internal/proxy"
	"log"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

const (
	descriptionShort = `Execute router process`

	descriptionLong = `
	Run execute router process`

	//
	ConfigFlagErrorMessage       = "impossible to get flag --config: %s"
	ConfigNotParsedErrorMessage  = "impossible to parse config file: %s"
	LogLevelFlagErrorMessage     = "impossible to get flag --log-level: %s"
	DisableTraceFlagErrorMessage = "impossible to get flag --disable-trace: %s"
	MetricsPortFlagErrorMessage  = "impossible to get flag --metrics-port: %s"
	MetricsHostFlagErrorMessage  = "impossible to get flag --metrics-host: %s"
	MetricsWebserverErrorMessage = "imposible to launch metrics webserver: %s"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "run",
		DisableFlagsInUseLine: true,
		Short:                 descriptionShort,
		Long:                  descriptionLong,

		Run: RunCommand,
	}

	//
	cmd.Flags().String("log-level", "info", "Verbosity level for logs")
	cmd.Flags().Bool("disable-trace", true, "Disable showing traces in logs")

	cmd.Flags().String("metrics-port", "2112", "Port where metrics web-server will run")
	cmd.Flags().String("metrics-host", "0.0.0.0", "Host where metrics web-server will run")

	cmd.Flags().String("config", "hashrouter.yaml", "Path to the YAML config file")

	return cmd
}

// RunCommand TODO
// Ref: https://pkg.go.dev/github.com/spf13/pflag#StringSlice
func RunCommand(cmd *cobra.Command, args []string) {

	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		log.Fatalf(ConfigFlagErrorMessage, err)
	}

	// Init the logger and store the level into the context
	logLevelFlag, err := cmd.Flags().GetString("log-level")
	if err != nil {
		log.Fatalf(LogLevelFlagErrorMessage, err)
	}

	disableTraceFlag, err := cmd.Flags().GetBool("disable-trace")
	if err != nil {
		log.Fatalf(DisableTraceFlagErrorMessage, err)
	}

	// TODO
	metricsPortFlag, err := cmd.Flags().GetString("metrics-port")
	if err != nil {
		log.Fatalf(MetricsPortFlagErrorMessage, err)
	}

	metricsHostFlag, err := cmd.Flags().GetString("metrics-host")
	if err != nil {
		log.Fatalf(MetricsHostFlagErrorMessage, err)
	}

	/////////////////////////////
	// EXECUTION FLOW RELATED
	/////////////////////////////

	//
	logger, err := globals.GetLogger(logLevelFlag, disableTraceFlag)
	if err != nil {
		log.Fatal(err)
	}

	logger.Infof("starting hashrouter. Getting ready to route some targets")

	// Parse and store the config
	configContent, err := config.ReadFile(configPath)
	if err != nil {
		logger.Fatalf(fmt.Sprintf(ConfigNotParsedErrorMessage, err))
	}
	globals.Application.Config = configContent

	// Register metrics into Prometheus Registry
	meter := metrics.PoolT{}
	meter.RegisterMetrics([]string{})

	// Start a webserver for exposing metrics endpoint in the background
	go func() {
		metricsHost := metricsHostFlag + ":" + metricsPortFlag

		http.Handle("/metrics", promhttp.Handler())
		logger.Infof("starting metrics endpoint on host '%s' and path '/metrics'", metricsHost)

		http.HandleFunc("GET /{name}/health", proxyHealthHandleFunc)
		logger.Infof("starting health endpoint on host '%s' and path '/{proxy-name}/health'", metricsHost)

		err = http.ListenAndServe(metricsHost, nil)
		if err != nil {
			logger.Fatalf(MetricsWebserverErrorMessage, err)
		}
	}()

	//
	var waitGroup sync.WaitGroup
	for _, proxyConfig := range globals.Application.Config.Proxies {

		proxyObj := proxy.NewProxy(configContent, proxyConfig, logger, &meter)

		// Register the proxy in the global pool.
		// This will allow access to its properties everywhere
		globals.Application.ProxyPool[proxyConfig.Name] = proxyObj

		if reflect.ValueOf(proxyObj.Config.Backends.Dns).IsZero() && reflect.ValueOf(proxyObj.Config.Backends.Static).IsZero() {
			logger.Errorf("backends not defined for proxy '%s'", proxyObj.Config.Name)
			continue
		}

		if !reflect.ValueOf(proxyObj.Config.Backends.Dns).IsZero() && !reflect.ValueOf(proxyObj.Config.Backends.Static).IsZero() {
			logger.Errorf("failed to load backends: static and dns are mutually exclusive for proxy '%s'",
				proxyObj.Config.Name)
			continue
		}

		syncTime, err := time.ParseDuration(proxyObj.Config.Backends.Synchronization)
		if err != nil {
			logger.Errorf("error parsing backend synchronization time for proxy '%s': %s",
				proxyObj.Config.Name, err.Error())
			continue
		}

		waitGroup.Add(1)
		go proxyObj.Synchronizer(syncTime)
		go proxyObj.Run(&waitGroup)

		time.Sleep(2 * time.Second) // TODO: unhardcode this
	}

	waitGroup.Wait()
}

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
