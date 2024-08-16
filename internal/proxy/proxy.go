package proxy

import (
	"hashrouter/api"
	"hashrouter/internal/globals"
	"hashrouter/internal/hashring"
	"net"
	"reflect"
	"sync"
	"time"
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

		p.Hashring = hashring.NewHashRing(1000)

		// Add bakends to hashring
		if reflect.ValueOf(p.Config.Backends.Dns).IsZero() && reflect.ValueOf(p.Config.Backends.Static).IsZero() {
			globals.Application.Logger.Error("backends not defined")
			goto waitNextLoop
		}

		if !reflect.ValueOf(p.Config.Backends.Dns).IsZero() && !reflect.ValueOf(p.Config.Backends.Static).IsZero() {
			globals.Application.Logger.Error("failed to load backends: static and dns are mutually exclusive")
			goto waitNextLoop
		}

		if !reflect.ValueOf(p.Config.Backends.Static).IsZero() {
			for _, backend := range p.Config.Backends.Static {
				p.Hashring.AddServer(backend.Host)
			}
		}

		if !reflect.ValueOf(p.Config.Backends.Dns).IsZero() {
			ips, err := net.LookupIP(p.Config.Backends.Dns.Domain)

			if err != nil {
				globals.Application.Logger.Errorf("error looking up %s: %s", p.Config.Backends.Dns.Domain, err.Error())
				goto waitNextLoop
			}

			for _, ip := range ips {
				p.Hashring.AddServer(ip.String())
			}
		}

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
	waitNextLoop:
		globals.Application.Logger.Errorf("a proxy broke, it will be restarted. Please wait...")
		time.Sleep(2 * time.Second)
	}
}
