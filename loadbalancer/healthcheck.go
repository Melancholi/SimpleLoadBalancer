package loadbalancer

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type HealthCheck struct {
	Status    bool
	CheckedAt time.Time
	mu        sync.RWMutex
}

type HealthChecker struct {
	Interval time.Duration
	Endpoint string
	Client   *http.Client
}

func (h *HealthCheck) IsHealthy() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.Status
}

func (h *HealthCheck) setHealth(status bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Status = status
	h.CheckedAt = time.Now()
}

func (h *HealthCheck) get() (bool, time.Time) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.Status, h.CheckedAt
}

func (hc *HealthChecker) checkBackend(backend *Backend) {
	healthURL := fmt.Sprintf("http://%s%s", backend.URL.Host, hc.Endpoint)

	resp, err := hc.Client.Get(healthURL)
	if err != nil {
		backend.HealthCheck.setHealth(false)
		log.Printf("Health check failed for %s: %v", backend.URL.Host, err)
		return
	}
	defer resp.Body.Close()

	healthy := resp.StatusCode >= 200 && resp.StatusCode < 300
	backend.HealthCheck.setHealth(healthy)

	if !healthy {
		log.Printf("Status %d for %s", resp.StatusCode, backend.URL.Host)
	}
}

// RunHealthCheck polls a single backend until its context is cancelled.
func (hc *HealthChecker) RunHealthCheck(ctx context.Context, backend *Backend) {
	ticker := time.NewTicker(hc.Interval)
	defer ticker.Stop()

	// Check immediately on startup.
	hc.checkBackend(backend)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.checkBackend(backend)
		}
	}
}
