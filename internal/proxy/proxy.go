package proxy

import (
	"sync"
	"time"

	"hashrouter/api"
	"hashrouter/internal/globals"
	"hashrouter/internal/hashring"
)

type Proxy struct {
	Config   api.ProxyT
	Hashring *hashring.HashRing
}

func NewProxy(config api.ProxyT) (proxy *Proxy) {

	return &Proxy{
		Config:   config,
		Hashring: nil,
	}
}

// TODO
// Intended to be run as a goroutine
func (p *Proxy) Run(waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()

	var err error

	for {

		// Run the proxy
		if p.Config.Options.Protocol == "http2" {
			err = p.RunHttp2()
		} else {
			err = p.RunHttp()
		}

		if err != nil {
			globals.Application.Logger.Errorf("error running proxy: %s", err.Error())
		}

		// If we reach this point, the proxy has been broken
		time.Sleep(2 * time.Second)
	}
}
