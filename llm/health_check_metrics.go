package llm

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	llmProviderHealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "llm_provider_healthy",
			Help: "LLM provider health status (1 healthy, 0 unhealthy).",
		},
		[]string{"provider_id"},
	)
	llmProviderHealthCheckLatencyMs = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "llm_provider_health_check_latency_ms",
			Help:    "LLM provider health check latency in milliseconds.",
			Buckets: []float64{10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
		},
		[]string{"provider_id"},
	)
	llmProviderHealthCheckFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llm_provider_health_check_failures_total",
			Help: "Total LLM provider health check failures.",
		},
		[]string{"provider_id"},
	)
)

func init() {
	prometheus.MustRegister(
		llmProviderHealthy,
		llmProviderHealthCheckLatencyMs,
		llmProviderHealthCheckFailuresTotal,
	)
}

func observeProviderHealthCheck(providerID string, healthy bool, latency time.Duration, err error) {
	if providerID == "" {
		providerID = "unknown"
	}
	if healthy {
		llmProviderHealthy.WithLabelValues(providerID).Set(1)
	} else {
		llmProviderHealthy.WithLabelValues(providerID).Set(0)
	}
	if latency > 0 {
		llmProviderHealthCheckLatencyMs.WithLabelValues(providerID).Observe(float64(latency.Milliseconds()))
	}
	if err != nil {
		llmProviderHealthCheckFailuresTotal.WithLabelValues(providerID).Inc()
	}
}
