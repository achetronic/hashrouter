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

type ListenerT struct {
	Port    int    `yaml:"port"`
	Address string `yaml:"address"`
}

type BackendsStaticT struct {
	Name string `yaml:"name"`
	Host string `yaml:"host"`
}

type BackendsDnsT struct {
	Name   string `yaml:"name"`
	Domain string `yaml:"domain"`
}

type BackendsT struct {
	Static []BackendsStaticT `yaml:"static,omitempty"`
	Dns    BackendsDnsT      `yaml:"dns,omitempty"`
}

type HashKeyT struct {
	Pattern string `yaml:"pattern"`
}

// OptionsT defines TODO
type OptionsT struct {
	Protocol       string `yaml:"protocol"`
	TlsCertificate string `yaml:"tls_certificate"`
	TlsKey         string `yaml:"tls_key"`
}

// LogsT TODO
type LogsT struct {
	ShowAccessLogs   bool     `yaml:"show_access_logs"`
	AccessLogsFields []string `yaml:"access_logs_fields"`
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
	Logs    LogsT    `yaml:"logs"`
	Proxies []ProxyT `yaml:"proxies"`
}
