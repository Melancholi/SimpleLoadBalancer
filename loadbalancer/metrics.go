package loadbalancer

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"
)

type BackendMetrics struct {
	Address   string    `json:"address"`
	Healthy   bool      `json:"healthy"`
	CheckedAt time.Time `json:"checked_at"`
	Requests  uint64    `json:"requests"`
	Errors    uint64    `json:"errors"`
	InFlight  int64     `json:"in_flight"`
}

type Metrics struct {
	StartedAt       time.Time        `json:"started_at"`
	UptimeSeconds   float64          `json:"uptime_seconds"`
	TotalRequests   uint64           `json:"total_requests"`
	Unavailable     uint64           `json:"unavailable"`
	HealthyBackends int              `json:"healthy_backends"`
	Backends        []BackendMetrics `json:"backends"`
}

// Snapshot returns a point-in-time copy of the load balancer metrics.
func (lb *LoadBalancer) Snapshot() Metrics {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	m := Metrics{
		StartedAt:     lb.startedAt,
		TotalRequests: lb.totalRequests.Load(),
		Unavailable:   lb.unavailable.Load(),
		Backends:      make([]BackendMetrics, 0, len(lb.active)),
	}
	if !lb.startedAt.IsZero() {
		m.UptimeSeconds = time.Since(lb.startedAt).Seconds()
	}

	for ip, backend := range lb.active {
		healthy, checkedAt := backend.HealthCheck.get()

		if healthy {
			m.HealthyBackends++
		}

		m.Backends = append(m.Backends, BackendMetrics{
			Address:   ip,
			Healthy:   healthy,
			CheckedAt: checkedAt,
			Requests:  backend.requests.Load(),
			Errors:    backend.errors.Load(),
			InFlight:  backend.inFlight.Load(),
		})
	}

	sort.Slice(m.Backends, func(i, j int) bool {
		return m.Backends[i].Address < m.Backends[j].Address
	})

	return m
}

// MetricsHandler serves the metrics snapshot as JSON.
func (lb *LoadBalancer) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(lb.Snapshot())
	})
}
