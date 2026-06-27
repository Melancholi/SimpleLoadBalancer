package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type HealthCheck struct {
	Status    bool
	CheckedAt time.Time
	Endpoint  string
	mu        sync.RWMutex
}

type Backend struct {
	URL         *url.URL
	Proxy       *httputil.ReverseProxy
	HealthCheck *HealthCheck
	mu          sync.RWMutex
}

type LoadBalancer struct {
	backends []*Backend
	counter  uint64
	mu       sync.RWMutex
}

func NewLoadBalancer(servers []ServerConfig) *LoadBalancer {
	var backends []*Backend

	for _, s := range servers {
		parsed, err := url.Parse(s.URL)
		if err != nil {
			log.Printf("Error parsing url %s: %v", s.URL, err)
		}

		proxy := httputil.NewSingleHostReverseProxy(parsed)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "Backend unavailable", http.StatusBadGateway)
		}

		backend := &Backend{
			URL:   parsed,
			Proxy: proxy,
			HealthCheck: &HealthCheck{
				Status:   true,
				Endpoint: s.HealthEndpoint,
			},
		}
		backends = append(backends, backend)
	}
	lb := &LoadBalancer{backends: backends}

	// Start health checks in the background
	go lb.startHealthChecks(10 * time.Second)

	return lb
}

func (lb *LoadBalancer) startHealthChecks(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lb.checkAllBackends()

	for range ticker.C {
		lb.checkAllBackends()
	}
}

func (lb *LoadBalancer) checkAllBackends() {
	lb.mu.RLock()
	backends := lb.backends

	lb.mu.RUnlock()

	for _, backend := range backends {
		go lb.checkBackend(backend)
	}
}

func (lb *LoadBalancer) checkBackend(backend *Backend) {
	endpoint := backend.HealthCheck.Endpoint

	if endpoint == "" {
		endpoint = "/health"
	}

	healthURL := fmt.Sprintf("http://%s%s", backend.URL.Host, endpoint)

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
		log.Printf("Health check status %d for %s", resp.StatusCode, backend.URL.Host)
	}
}

func (lb *LoadBalancer) getNextHealthyBackend() *Backend {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	if len(lb.backends) == 0 {
		return nil
	}

	idx := atomic.AddUint64(&lb.counter, 1)
	attempts := 0

	for attempts < len(lb.backends) {
		backend := lb.backends[idx%uint64(len(lb.backends))]

		backend.HealthCheck.mu.RLock()
		isHealthy := backend.HealthCheck.Status
		backend.HealthCheck.mu.RUnlock()

		if isHealthy {
			return backend
		}

		idx++
		attempts++
	}

	return lb.backends[0]
}

// Round-robin
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	/**
	Receive request
	proxy to next addrss <-- getNextBackend
	Continue request in new proxy
	**/
	backend := lb.getNextHealthyBackend()
	if backend == nil {
		http.Error(w, "No backends available", http.StatusServiceUnavailable)
		return
	}
	addr, err := net.LookupHost(backend.URL.Hostname())
	if err != nil {
		log.Printf("Failed to lookup backend records: %s\n", err)
	} else {
		log.Printf("Backend records: %s\n", addr)
	}
	backend.Proxy.ServeHTTP(w, r)
}

func main() {
	config := LoadConfig("config.toml")

	lb := NewLoadBalancer(config.Server.ServerList)

	http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		/*
			add the url of the ip that hit this endpoint
		*/
		fmt.Fprintf(w, "Register not yet implemented\n")
	})

	http.HandleFunc("/unregister", func(w http.ResponseWriter, r *http.Request) {
		/*
			remove the url of the ip that hit this endpoint
		*/
		fmt.Fprintf(w, "Unregister not yet implemented\n")
	})

	log.Println("Load balancer running on :8080")
	err := http.ListenAndServe(":8080", lb)
	if err != nil {
		log.Fatal("Loadbalancer error: ", err)
	}
}
