// Package metrics exposes crawler runtime metrics in Prometheus format:
// pages fetched (success/failure, per domain — from which rates are
// derived via PromQL), queue depth, and per-domain success/failure counts.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the crawler's Prometheus collectors.
type Metrics struct {
	registry   *prometheus.Registry
	pagesTotal *prometheus.CounterVec
}

// New creates a Metrics instance with its own registry (not the global
// default one, so multiple instances in tests don't collide).
func New() *Metrics {
	registry := prometheus.NewRegistry()

	pagesTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "crawler_pages_total",
		Help: "Total number of page fetch attempts, by domain and result.",
	}, []string{"domain", "result"})

	registry.MustRegister(pagesTotal)
	registry.MustRegister(collectors.NewGoCollector())
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	return &Metrics{registry: registry, pagesTotal: pagesTotal}
}

// RecordFetch records the outcome of one fetch attempt for domain.
// pages/sec and error rates are derived from this counter via PromQL
// (e.g. rate(crawler_pages_total{result="success"}[1m])).
func (m *Metrics) RecordFetch(domain string, success bool) {
	result := "success"
	if !success {
		result = "failure"
	}
	m.pagesTotal.WithLabelValues(domain, result).Inc()
}

// SetQueueDepthFunc registers a gauge that calls fn on every scrape to
// report the current frontier queue depth.
func (m *Metrics) SetQueueDepthFunc(fn func() float64) {
	gauge := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "crawler_queue_depth",
		Help: "Current number of URLs queued in the frontier.",
	}, fn)
	m.registry.MustRegister(gauge)
}

// Handler returns an http.Handler that serves metrics in the Prometheus
// text exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
