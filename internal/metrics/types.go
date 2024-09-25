package metrics

import "github.com/prometheus/client_golang/prometheus"

type PoolT struct {
	HttpRequestsTotal              *prometheus.CounterVec
	BackendConnectionFailuresTotal *prometheus.CounterVec
}
