package loadbalancer

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Backend struct {
	URL         *url.URL
	Proxy       *httputil.ReverseProxy
	HealthCheck *HealthCheck
	cancel      context.CancelFunc

	requests atomic.Uint64
	errors   atomic.Uint64
	inFlight atomic.Int64
}

// Options are the parts plugged into the load balancer. Any left empty fall
// back to the defaults applied in NewLoadBalancer.
type Options struct {
	Selection     SelectionStrategy
	HealthChecker *HealthChecker
	Discovery     DiscoveryStrategy
	backendPort   string
	//maybe logger
}

type LoadBalancer struct {
	active        map[string]*Backend
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	closeOnce     sync.Once
	options       Options
	startedAt     time.Time
	totalRequests atomic.Uint64
	unavailable   atomic.Uint64
}

func NewLoadBalancer(opts Options, Config Config) *LoadBalancer {
	if opts.Selection == nil {
		opts.Selection = &RoundRobinSelection{}
	}
	if opts.HealthChecker == nil {
		opts.HealthChecker = &HealthChecker{}
	}
	if opts.HealthChecker.Interval <= 0 {
		opts.HealthChecker.Interval = 10 * time.Second
	}
	if opts.HealthChecker.Endpoint == "" {
		opts.HealthChecker.Endpoint = "/health"
	}
	if opts.HealthChecker.Client == nil {
		opts.HealthChecker.Client = &http.Client{Timeout: 5 * time.Second}
	}
	if opts.Discovery == nil {
		opts.Discovery = DNSDiscovery{Hostname: "backend", Interval: 30 * time.Second}
	}

	ctx, cancel := context.WithCancel(context.Background())
	lb := &LoadBalancer{
		active:    make(map[string]*Backend),
		ctx:       ctx,
		cancel:    cancel,
		options:   opts,
		startedAt: time.Now(),
	}
	go lb.startDiscovery()
	return lb
}

// Close stops discovery and cancels all running health checks.
func (lb *LoadBalancer) Close() {
	lb.closeOnce.Do(func() {
		if lb.cancel != nil {
			lb.cancel()
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

func newBackend(ip string, backendPort string) *Backend {
	rawURL := fmt.Sprintf("http://%s:%s", ip, backendPort)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		log.Printf("Error parsing backend URL %s: %v", rawURL, err)
		return nil
	}

	backend := &Backend{
		URL:   parsed,
		Proxy: httputil.NewSingleHostReverseProxy(parsed),
		HealthCheck: &HealthCheck{
			Status: true,
		},
	}
	backend.Proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		backend.errors.Add(1)
		http.Error(w, "Backend unavailable", http.StatusBadGateway)
	}

	return backend
}

// startDiscovery applies every backend list the discovery strategy pushes,
// until the load balancer is closed and the strategy closes its channel.
func (lb *LoadBalancer) startDiscovery() {
	for ips := range lb.options.Discovery.Watch(lb.ctx) {
		lb.syncBackends(ips)
	}
}

// syncBackends adds newly discovered backends and removes missing ones,
// starting and stopping their health checks along the way.
func (lb *LoadBalancer) syncBackends(ips []string) {
	log.Printf("Discovered backends -> %v", ips)

	resolved := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		resolved[ip] = struct{}{}
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	// A list can arrive just as Close runs; don't start health checks after it.
	if lb.ctx != nil && lb.ctx.Err() != nil {
		return
	}

	// Add backends that were newly discovered.
	for ip := range resolved {
		if _, exists := lb.active[ip]; !exists {
			backend := newBackend(ip, lb.options.backendPort)
			if backend == nil {
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			backend.cancel = cancel
			lb.active[ip] = backend
			go lb.options.HealthChecker.RunHealthCheck(ctx, backend)
			log.Printf("Added backend %s", ip)
		}
	}

	// Remove backends that are no longer discovered.
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

// ServeHTTP proxies the request to the backend picked by the selection strategy.
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lb.totalRequests.Add(1)

	backend := lb.getNextBackend()
	if backend == nil {
		lb.unavailable.Add(1)
		http.Error(w, "No backends available", http.StatusServiceUnavailable)
		return
	}

	backend.requests.Add(1)
	backend.inFlight.Add(1)
	defer backend.inFlight.Add(-1)

	backend.Proxy.ServeHTTP(w, r)
}
