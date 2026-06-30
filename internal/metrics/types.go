// SPDX-FileCopyrightText: 2026 Alby Hernández <hola@achetronic.com>
// SPDX-License-Identifier: Apache-2.0

package metrics

import "github.com/prometheus/client_golang/prometheus"

type PoolT struct {
	HttpRequestsTotal              *prometheus.CounterVec
	BackendConnectionFailuresTotal *prometheus.CounterVec
}
