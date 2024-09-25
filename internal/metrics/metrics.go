package metrics

import (
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/exp/maps"
)

const (

	//
	MetricsPrefix = "hashrouter_"
)

// getProcessedLabels accept a list of strings representing an object's labels and return a map
// whose keys are the input labels, and the values are the same labels with a Prometheus-ready syntax
func getProcessedLabels(labelNames []string) (promLabelNames map[string]string, err error) {

	promLabelNames = make(map[string]string)

	// Make a regex to admit lower + uppercase letters, numbers and underscore
	reg, err := regexp.Compile("[^a-zA-Z0-9]+")
	if err != nil {
		return promLabelNames, err
	}

	promLabelNames = make(map[string]string)

	for _, labelName := range labelNames {
		processedlabelName := reg.ReplaceAllString(labelName, "_")
		promLabelNames[labelName] = processedlabelName
	}

	return promLabelNames, err
}

// RegisterMetrics register declared metrics with their labels on Prometheus SDK
func (p *PoolT) RegisterMetrics(extraLabelNames []string) {

	parsedLabelsMap, _ := getProcessedLabels(extraLabelNames) // TODO: Handle error
	parsedLabels := maps.Values(parsedLabelsMap)

	// Metric: http_requests_total
	httpRequestsTotalLabels := []string{"proxy_name", "status_code", "method"}
	httpRequestsTotalLabels = append(httpRequestsTotalLabels, parsedLabels...)

	p.HttpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: MetricsPrefix + "http_requests_total",
		Help: "total amount of requests by status code",
	}, httpRequestsTotalLabels)

	// Metric: backend_connection_failures_total
	backendConnectionFailuresTotalLabels := []string{"proxy_name", "method"}
	backendConnectionFailuresTotalLabels = append(backendConnectionFailuresTotalLabels, parsedLabels...)

	p.BackendConnectionFailuresTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: MetricsPrefix + "backend_connection_failures_total",
		Help: "total amount of requests that where tried against all the backends and failed",
	}, backendConnectionFailuresTotalLabels)

}
