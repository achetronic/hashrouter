package proxy

import (
	"fmt"
	"net"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"time"

	"hashrouter/api"
	"hashrouter/internal/hashring"
)

// TODO
type BackendT struct {
	Host   string
	Health api.HealthCheckT
}

// TODO
func (p *ProxyT) Synchronizer(syncTime time.Duration) {
	p.Hashring = hashring.NewHashRing(1000)

	for {
		tmpHostPool := []BackendT{}
		hostPool := []string{}

		// STATIC ---
		if !reflect.ValueOf(p.Config.Backends.Static).IsZero() {
			for _, backend := range p.Config.Backends.Static {
				tmpHostPool = append(tmpHostPool, BackendT{
					Host:   backend.Host,
					Health: backend.HealthCheck,
				})
			}
		}

		// DNS ---
		if !reflect.ValueOf(p.Config.Backends.Dns).IsZero() {

			p.Logger.Infof("syncing hashring with DNS")

			discoveredIps, err := net.LookupIP(p.Config.Backends.Dns.Domain)
			if err != nil {
				p.Logger.Errorf("error looking up %s: %s", p.Config.Backends.Dns.Domain, err.Error())
			}

			for _, discoveredIp := range discoveredIps {

				hostAddress := discoveredIp.String() + ":" + strconv.Itoa(p.Config.Backends.Dns.Port)
				if IsIPv6(discoveredIp.String()) {
					hostAddress = "[" + hostAddress + "]" + ":" + strconv.Itoa(p.Config.Backends.Dns.Port)
				}

				tmpHostPool = append(tmpHostPool, BackendT{
					Host:   hostAddress,
					Health: p.Config.Backends.Dns.HealthCheck,
				})

			}
		}

		//
		hClient := http.Client{}
		for _, backend := range tmpHostPool {
			if reflect.ValueOf(backend.Health).IsZero() {
				hostPool = append(hostPool, backend.Host)
				continue
			}

			//
			hClient.Timeout = backend.Health.Timeout
			for i := 0; i < backend.Health.Retries; i++ {
				resp, err := hClient.Get(fmt.Sprintf("http://%s%s", backend.Host, backend.Health.Path))
				if err == nil && resp.StatusCode == 200 {
					hostPool = append(hostPool, backend.Host)
					break
				}

				if err != nil {
					p.Logger.Errorf("unable to perform healthcheck on host '%s': %s", backend.Host, err.Error())
				}

				p.Logger.Errorf("healthcheck failed for host '%s' with status '%s'", backend.Host, resp.Status)
			}
		}

		currentServerList := p.Hashring.GetServerList()

		deleteServersList := []string{}
		for _, server := range currentServerList {
			if !slices.Contains(hostPool, server) {
				deleteServersList = append(deleteServersList, server)
			}
		}

		appendServersList := []string{}
		for _, server := range hostPool {
			if !slices.Contains(currentServerList, server) {
				appendServersList = append(appendServersList, server)
			}
		}

		for _, server := range appendServersList {
			p.Hashring.AddServer(server)
		}

		for _, server := range deleteServersList {
			p.Hashring.RemoveServer(server)
		}

		p.Logger.Infof("current hashring: %s", p.Hashring.String())

		time.Sleep(syncTime)
	}
}
