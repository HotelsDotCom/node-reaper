/*
Copyright 2018 Expedia Inc.

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

package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PromMetrics implements the Instrumenter so the metrics can be managed by Prometheus
type PromMetrics struct {
	nodeDrainTotal prometheus.Counter
	nodeDrainFail  prometheus.Counter
	nodeReapTotal  prometheus.Counter
	nodeReapFail   prometheus.Counter

	registry prometheus.Registerer
	path     string
	mux      *http.ServeMux
}

// NewPrometheusMetrics returns a new PromMetrics object
func NewPrometheusMetrics(path string, mux *http.ServeMux) *PromMetrics {
	// Create metrics
	nodeDrainTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "node_reaper",
		Subsystem: "controller",
		Name:      "drain_total",
		Help:      "Number of nodes drained by the operator",
	})
	nodeDrainFail := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "node_reaper",
		Subsystem: "controller",
		Name:      "drain_fail_total",
		Help:      "Number of nodes unsuccessfully drained by the operator",
	})
	nodeReapTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "node_reaper",
		Subsystem: "controller",
		Name:      "reap_total",
		Help:      "Number of nodes reaped by the operator",
	})
	nodeReapFail := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "node_reaper",
		Subsystem: "controller",
		Name:      "reap_fail_total",
		Help:      "Number of nodes unsuccessfully reaped by the operator",
	})

	promReg := prometheus.NewRegistry()

	promMetrics := &PromMetrics{
		nodeDrainTotal: nodeDrainTotal,
		nodeDrainFail:  nodeDrainFail,
		nodeReapTotal:  nodeReapTotal,
		nodeReapFail:   nodeReapFail,

		registry: promReg,
		path:     path,
		mux:      mux,
	}

	promMetrics.register()

	handler := promhttp.HandlerFor(promReg, promhttp.HandlerOpts{})
	mux.Handle(path, handler)

	return promMetrics
}

// register will register all the required prometheus metrics on the Prometheus collector
func (p *PromMetrics) register() {
	p.registry.MustRegister(p.nodeDrainTotal)
	p.registry.MustRegister(p.nodeDrainFail)
	p.registry.MustRegister(p.nodeReapTotal)
	p.registry.MustRegister(p.nodeReapFail)
}

// IncNodeDrainTotal adds one to the nodeDrainTotal counter
func (p *PromMetrics) IncNodeDrainTotal() {
	p.nodeDrainTotal.Inc()
}

// IncNodeDrainFail adds one to the nodeDrainFail counter
func (p *PromMetrics) IncNodeDrainFail() {
	p.nodeDrainFail.Inc()
}

// IncNodeReapTotal adds one to the nodeReapTotal counter
func (p *PromMetrics) IncNodeReapTotal() {
	p.nodeReapTotal.Inc()
}

// IncNodeReapFail adds one to the nodeReapFail counter
func (p *PromMetrics) IncNodeReapFail() {
	p.nodeReapFail.Inc()
}
