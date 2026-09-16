package loadbalancer

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type HealthCheck struct {
	Status    bool
	CheckedAt time.Time
	mu        sync.RWMutex
}

type Backend struct {
	URL         *url.URL
	Proxy       *httputil.ReverseProxy
	HealthCheck *HealthCheck
	cancel      context.CancelFunc
}

type LoadBalancer struct {
	active    map[string]*Backend
	counter   atomic.Uint64
	mu        sync.RWMutex
	stopCh    chan struct{}
	closeOnce sync.Once
}

func NewLoadBalancer() *LoadBalancer {
	lb := &LoadBalancer{
		active: make(map[string]*Backend),
		stopCh: make(chan struct{}),
	}
	go lb.startDiscovery(30*time.Second, lb.stopCh)
	return lb
}

// Close stops discovery and cancels all running health checks.
func (lb *LoadBalancer) Close() {
	lb.closeOnce.Do(func() {
		if lb.stopCh != nil {
			close(lb.stopCh)
		}

		lb.mu.Lock()
		defer lb.mu.Unlock()

		for ip, backend := range lb.active {
			if backend.cancel != nil {
				backend.cancel()
			}
			delete(lb.active, ip)
		}
	})
}

func newBackend(ip string) *Backend {
	rawURL := fmt.Sprintf("http://%s:8080", ip)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		log.Printf("Error parsing backend URL %s: %v", rawURL, err)
		return nil
	}

	proxy := httputil.NewSingleHostReverseProxy(parsed)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "Backend unavailable", http.StatusBadGateway)
	}

	return &Backend{
		URL:   parsed,
		Proxy: proxy,
		HealthCheck: &HealthCheck{
			Status: true,
		},
	}
}

func (lb *LoadBalancer) startDiscovery(interval time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lb.syncBackends()

	for {
		select {
		case <-ticker.C:
			lb.syncBackends()
		case <-stopCh:
			return
		}
	}
}

func (lb *LoadBalancer) syncBackends() {
	ips, err := net.LookupHost("backend")
	if err != nil {
		log.Printf("DNS lookup failed: %v", err)
		return
	}

	log.Printf("DNS resolved backend -> %v", ips)

	resolved := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		resolved[ip] = struct{}{}
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Add backends that are new in DNS.
	for ip := range resolved {
		if _, exists := lb.active[ip]; !exists {
			backend := newBackend(ip)
			if backend == nil {
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			backend.cancel = cancel
			lb.active[ip] = backend
			go lb.runHealthCheck(ctx, backend)
			log.Printf("Added backend %s", ip)
		}
	}

	// Remove backends that have disappeared from DNS.
	for ip, backend := range lb.active {
		if _, exists := resolved[ip]; !exists {
			backend.cancel()
			delete(lb.active, ip)
			log.Printf("Removed backend %s", ip)
		}
	}
}

// runHealthCheck polls a single backend until its context is cancelled.
func (lb *LoadBalancer) runHealthCheck(ctx context.Context, backend *Backend) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Check immediately on startup.
	lb.checkBackend(backend)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lb.checkBackend(backend)
		}
	}
}

func (lb *LoadBalancer) checkBackend(backend *Backend) {
	healthURL := fmt.Sprintf("http://%s/health", backend.URL.Host)
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(healthURL)

	backend.HealthCheck.mu.Lock()
	defer backend.HealthCheck.mu.Unlock()

	if err != nil {
		backend.HealthCheck.Status = false
		log.Printf("Health check failed for %s: %v", backend.URL.Host, err)
		return
	}
	defer resp.Body.Close()

	backend.HealthCheck.Status = resp.StatusCode >= 200 && resp.StatusCode < 300
	backend.HealthCheck.CheckedAt = time.Now()

	if !backend.HealthCheck.Status {
		log.Printf("Status %d for %s", resp.StatusCode, backend.URL.Host)
	}
}

func (lb *LoadBalancer) getNextHealthyBackend() *Backend {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if len(lb.active) == 0 {
		return nil
	}

	keys := make([]string, 0, len(lb.active))
	for key := range lb.active {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	idx := lb.counter.Add(1) - 1
	attempts := 0

	for attempts < len(keys) {
		backend := lb.active[keys[(idx+uint64(attempts))%uint64(len(keys))]]

		backend.HealthCheck.mu.RLock()
		healthy := backend.HealthCheck.Status
		backend.HealthCheck.mu.RUnlock()

		if healthy {
			return backend
		}

		attempts++
	}

	return nil
}

// ServeHTTP implements round-robin proxying .
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backend := lb.getNextHealthyBackend()
	if backend == nil {
		http.Error(w, "No backends available", http.StatusServiceUnavailable)
		return
	}
	backend.Proxy.ServeHTTP(w, r)
}
