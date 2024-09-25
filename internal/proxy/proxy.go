package proxy

import (
	"sync"
	"time"

	"hashrouter/api"
	"hashrouter/internal/hashring"
	"hashrouter/internal/metrics"

	"go.uber.org/zap"
)

// ProxyStatusT represents the status of a proxy.
// It here as it's used by both 'proxy' and 'globals' packages.
type ProxyStatusT struct {
	sync.RWMutex

	IsHealthy bool
}

// ProxyStatusT represents a proxy.
// It here as it's used by both 'proxy' and 'globals' packages.
type ProxyT struct {
	// TODO: Avoid having global config here.
	// Pass global options in a different way.
	GlobalConfig api.ConfigT

	//
	Config api.ProxyT

	//
	Hashring *hashring.HashRing
	Status   *ProxyStatusT

	//
	Logger *zap.SugaredLogger
	Meter  *metrics.PoolT
}

// NewProxy return a new ProxyT instance
func NewProxy(globalConfig api.ConfigT, config api.ProxyT, log *zap.SugaredLogger, met *metrics.PoolT) (proxy *ProxyT) {

	proxy = &ProxyT{
		// TODO: Avoid having global config here.
		// Pass global options in a different way.
		GlobalConfig: globalConfig,

		//
		Config: config,

		Hashring: nil,
		Status:   &ProxyStatusT{},

		// TODO: These objects can be joined into a single 'InstrumentationT' struct
		Logger: log,
		Meter:  met,
	}

	return proxy
}

// Run launches the proxy and keeps it running.
// Intended to be run as a goroutine
func (p *ProxyT) Run(waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()

	//
	var err error

	for {

		// Run the proxy
		if p.Config.Options.Protocol == "http2" {
			err = p.RunHttp2()
		} else {
			err = p.RunHttp()
		}

		if err != nil {
			p.Logger.Errorf("error running proxy: %s", err.Error())

			p.Status.RWMutex.Lock()
			p.Status.IsHealthy = false
			p.Status.RWMutex.Unlock()
		}

		// If we reach this point, the proxy has been broken
		time.Sleep(2 * time.Second)
	}
}
