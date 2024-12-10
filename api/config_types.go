/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import "time"

type ListenerT struct {
	Port    int    `yaml:"port"`
	Address string `yaml:"address"`
}

type HealthCheckT struct {
	Timeout time.Duration `yaml:"timeout"`
	Retries int           `yaml:"retries"`
	Path    string        `yaml:"path"`
}

type BackendsStaticT struct {
	Name        string       `yaml:"name"`
	Host        string       `yaml:"host"`
	HealthCheck HealthCheckT `yaml:"healthcheck,omitempty"`
}

type BackendsDnsT struct {
	Name        string       `yaml:"name"`
	Domain      string       `yaml:"domain"`
	Port        int          `yaml:"port"`
	HealthCheck HealthCheckT `yaml:"healthcheck,omitempty"`
}

type BackendsT struct {
	Synchronization string            `yaml:"synchronization"`
	Static          []BackendsStaticT `yaml:"static,omitempty"`
	Dns             BackendsDnsT      `yaml:"dns,omitempty"`
}

type HashKeyT struct {
	Pattern string `yaml:"pattern"`
}

// OptionsT defines TODO
type OptionsT struct {
	Protocol       string `yaml:"protocol"`
	TlsCertificate string `yaml:"tls_certificate,omitempty"`
	TlsKey         string `yaml:"tls_key,omitempty"`

	//
	HttpServerReadTimeoutMillis  int  `yaml:"http_server_read_timeout_ms,omitempty"`
	HttpServerWriteTimeoutMillis int  `yaml:"http_server_write_timeout_ms,omitempty"`
	HttpServerDisableKeepAlives  bool `yaml:"http_server_disable_keep_alives,omitempty"`

	//
	HttpBackendDialTimeoutMillis    int  `yaml:"http_backend_dial_timeout_ms,omitempty"`
	HttpBackendKeepAliveMillis      int  `yaml:"http_backend_keep_alive_ms,omitempty"`
	HttpBackendRequestTimeoutMillis int  `yaml:"http_backend_request_timeout_ms,omitempty"`
	HttpBackendDisableKeepAlives    bool `yaml:"http_backend_disable_keep_alives,omitempty"`

	//
	TryAnotherBackendOnFailure bool `yaml:"try_another_backend_on_failure,omitempty"`
}

// LogsT TODO
type LogsT struct {
	ShowAccessLogs   bool     `yaml:"show_access_logs"`
	AccessLogsFields []string `yaml:"access_logs_fields"`
}

// GlobalT TODO
type CommonT struct {
	Logs LogsT `yaml:"logs"`
}

// ProxyT TODO
type ProxyT struct {
	Name     string    `yaml:"name"`
	Listener ListenerT `yaml:"listener"`
	Backends BackendsT `yaml:"backends"`
	HashKey  HashKeyT  `yaml:"hash_key"`
	Options  OptionsT  `yaml:"options"`
}

// ConfigT TODO
type ConfigT struct {
	Common  CommonT  `yaml:"common"`
	Proxies []ProxyT `yaml:"proxies"`
}
