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

		// Launch a goroutine that synchronizes the hashring from time to time
		// when the DNS backend is used
		if !reflect.ValueOf(p.Config.Backends.Dns).IsZero() {
			go func() {

				syncDuration, err := time.ParseDuration(p.Config.Backends.Dns.Synchronization)
				if err != nil {
					globals.Application.Logger.Fatalf("error parsing synchronization duration: %s", err.Error())
				}

				hostPool := []string{}
				for {

					globals.Application.Logger.Infof("syncing hashring with DNS")

					discoveredIps, err := net.LookupIP(p.Config.Backends.Dns.Domain)
					if err != nil {
						globals.Application.Logger.Errorf("error looking up %s: %s", p.Config.Backends.Dns.Domain, err.Error())
					}

					// Add recently discovered hosts to the hashring
					tmpHostPool := []string{}
					for _, discoveredIp := range discoveredIps {

						hostAddress := discoveredIp.String() + ":" + strconv.Itoa(p.Config.Backends.Dns.Port)
						if IsIPv6(discoveredIp.String()) {
							hostAddress = "[" + hostAddress + "]" + ":" + strconv.Itoa(p.Config.Backends.Dns.Port)
						}

						// Craft a human readable string list of discovered IPs
						tmpHostPool = append(tmpHostPool, hostAddress)

						//
						if !slices.Contains(hostPool, hostAddress) {
							p.Hashring.AddServer(hostAddress)
						}
					}

					// Delete dead hosts from hashring
					for _, hostPoolItem := range hostPool {

						if !slices.Contains(tmpHostPool, hostPoolItem) {
							p.Hashring.RemoveServer(hostPoolItem)
						}
					}

					// Update hostPool
					slices.Sort(hostPool)
					slices.Sort(tmpHostPool)

					if !slices.Equal(hostPool, tmpHostPool) {
						hostPool = tmpHostPool
						globals.Application.Logger.Infof("hashring updated: %v", hostPool)
					}

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
