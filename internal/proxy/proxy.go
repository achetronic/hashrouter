package proxy

import (
	"hashrouter/api"
	"hashrouter/internal/globals"
	"hashrouter/internal/hashring"
	"net"
	"reflect"
	"slices"
	"strconv"
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
			// Arrancar una goroutine que sincronice el hashring de cuando en cuando
			go func() {

				syncDuration, err := time.ParseDuration(p.Config.Backends.Dns.Synchronization)
				if err != nil {
					globals.Application.Logger.Fatalf("error parsing synchronization duration: %s", err.Error())
				}

				serverPool := []string{}
				for {

					globals.Application.Logger.Infof("syncing hashring with DNS")

					discoveredIps, err := net.LookupIP(p.Config.Backends.Dns.Domain)
					if err != nil {
						globals.Application.Logger.Errorf("error looking up %s: %s", p.Config.Backends.Dns.Domain, err.Error())
					}

					// Add recently discovered servers to the hashring
					tmpDiscoveredIps := []string{}
					for _, discoveredIp := range discoveredIps {
						// Craft a human readable string list of discovered IPs
						tmpDiscoveredIps = append(tmpDiscoveredIps, discoveredIp.String())

						//
						server := discoveredIp.String() + ":" + strconv.Itoa(p.Config.Backends.Dns.Port)
						if IsIPv6(discoveredIp.String()) {
							server = "[" + server + "]" + ":" + strconv.Itoa(p.Config.Backends.Dns.Port)
						}
						if !slices.Contains(serverPool, server) {
							p.Hashring.AddServer(server)
						}
					}

					// Delete dead servers from hashring
					for _, server := range serverPool {
						if !slices.Contains(tmpDiscoveredIps, server) {
							p.Hashring.RemoveServer(server)
						}
					}

					// Update serverPool
					serverPool = tmpDiscoveredIps
					globals.Application.Logger.Infof("hashring updated: %v", serverPool)
					time.Sleep(syncDuration)
				}
			}()
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
