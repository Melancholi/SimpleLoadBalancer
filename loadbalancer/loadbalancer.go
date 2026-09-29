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
type Options struct {
	Selection     SelectionStrategy
	HealthChecker *HealthChecker
	Discovery     DiscoveryStrategy
	//maybe logger
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

func (lb *LoadBalancer) getNextBackend() *Backend {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if len(lb.active) == 0 {
		return nil
	}

	//get all active backends
	keys := make([]string, 0, len(lb.active))
	for key := range lb.active {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	//check which is healthy
	healthy := make([]*Backend, 0, len(lb.active))
	for _, key := range keys {
		if lb.active[key].HealthCheck.IsHealthy() {
			healthy = append(healthy, lb.active[key])
		}
	}

	if len(healthy) == 0 {
	return nil
	}
	return lb.options.Selection.GetNextBackend(healthy)
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
