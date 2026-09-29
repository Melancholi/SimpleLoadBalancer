package loadbalancer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestBackend(t *testing.T, healthy bool, response string) *Backend {
	t.Helper()

	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, response)
	}))
	t.Cleanup(backendServer.Close)

	parsedURL, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatalf("parse backend URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(parsedURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "backend unavailable", http.StatusBadGateway)
	}

	return &Backend{
		URL:   parsedURL,
		Proxy: proxy,
		HealthCheck: &HealthCheck{
			Status: healthy,
		},
	}
}

func TestGetNextHealthyBackendUsesStableRoundRobinOrder(t *testing.T) {
	lb := &LoadBalancer{
		options: Options{Selection: &RoundRobinSelection{}},
		active: map[string]*Backend{
			"10.0.0.2": newTestBackend(t, true, "backend-2"),
			"10.0.0.1": newTestBackend(t, true, "backend-1"),
		},
	}

	first := lb.getNextBackend()
	second := lb.getNextBackend()

	if first == nil || second == nil {
		t.Fatalf("expected healthy backends, got first=%v second=%v", first, second)
	}

	if first.URL.Host != lb.active["10.0.0.1"].URL.Host {
		t.Fatalf("expected first backend %s, got %s", lb.active["10.0.0.1"].URL.Host, first.URL.Host)
	}

	if second.URL.Host != lb.active["10.0.0.2"].URL.Host {
		t.Fatalf("expected second backend %s, got %s", lb.active["10.0.0.2"].URL.Host, second.URL.Host)
	}
}

func TestGetNextHealthyBackendSkipsUnhealthyBackends(t *testing.T) {
	healthy := newTestBackend(t, true, "healthy")
	unhealthy := newTestBackend(t, false, "unhealthy")

	lb := &LoadBalancer{
		options: Options{Selection: &RoundRobinSelection{}},
		active: map[string]*Backend{
			"10.0.0.1": healthy,
			"10.0.0.2": unhealthy,
		},
	}

	backend := lb.getNextBackend()
	if backend == nil {
		t.Fatal("expected a healthy backend")
	}

	if backend.URL.Host != healthy.URL.Host {
		t.Fatalf("expected healthy backend %s, got %s", healthy.URL.Host, backend.URL.Host)
	}
}

func TestServeHTTPReturnsServiceUnavailableWhenNoBackendsAreAvailable(t *testing.T) {
	lb := &LoadBalancer{active: map[string]*Backend{}, options: Options{Selection: &RoundRobinSelection{}}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	lb.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestServeHTTPProxiesToHealthyBackend(t *testing.T) {
	backend := newTestBackend(t, true, "connected")
	lb := &LoadBalancer{
		options: Options{Selection: &RoundRobinSelection{}},
		active: map[string]*Backend{
			"10.0.0.1": backend,
		},
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	lb.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	body, err := io.ReadAll(recorder.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if string(body) != "connected" {
		t.Fatalf("expected proxied response %q, got %q", "connected", string(body))
	}
}

func TestLoadBalancerUnderPressureWithHey(t *testing.T) {
	if _, err := exec.LookPath("hey"); err != nil {
		t.Skip("hey is not installed")
	}

	const backendCount = 5
	var backendHits [backendCount]atomic.Int64
	active := make(map[string]*Backend, backendCount)

	for i := 0; i < backendCount; i++ {
		index := i
		backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			backendHits[index].Add(1)
			fmt.Fprintf(w, "backend-%d", index+1)
		}))
		t.Cleanup(backendServer.Close)

		parsedURL, err := url.Parse(backendServer.URL)
		if err != nil {
			t.Fatalf("parse backend %d URL: %v", index+1, err)
		}

		active[fmt.Sprintf("10.0.0.%d", index+1)] = &Backend{
			URL:   parsedURL,
			Proxy: httputil.NewSingleHostReverseProxy(parsedURL),
			HealthCheck: &HealthCheck{
				Status: true,
			},
		}
	}

	lb := &LoadBalancer{active: active, options: Options{Selection: &RoundRobinSelection{}}}

	lbServer := httptest.NewServer(lb)
	t.Cleanup(lbServer.Close)

	requests := 10000
	concurrency := 100
	output, err := exec.Command(
		"hey",
		"-n", strconv.Itoa(requests),
		"-c", strconv.Itoa(concurrency),
		lbServer.URL,
	).CombinedOutput()

	t.Logf("Hey output:\n%s", string(output))
	if err != nil {
		t.Fatalf("hey failed: %v\noutput:\n%s", err, string(output))
	}

	var totalHits int64
	for i := range backendHits {
		hits := backendHits[i].Load()
		if hits == 0 {
			t.Fatalf("expected hey traffic to reach backend %d", i+1)
		}
		totalHits += hits
	}

	if !strings.Contains(string(output), "Status code distribution") {
		t.Fatalf("unexpected hey output:\n%s", string(output))
	}

	if totalHits != int64(requests) {
		t.Fatalf("expected %d total backend hits, got %d", requests, totalHits)
	}

	t.Logf("requests=%d concurrency=%d backend_hits=%v", requests, concurrency, []int64{
		backendHits[0].Load(),
		backendHits[1].Load(),
		backendHits[2].Load(),
		backendHits[3].Load(),
		backendHits[4].Load(),
	})
}

func TestSnapshotCountsRequestsPerBackend(t *testing.T) {
	lb := &LoadBalancer{
		options: Options{Selection: &RoundRobinSelection{}},
		active: map[string]*Backend{
			"10.0.0.1": newTestBackend(t, true, "backend-1"),
			"10.0.0.2": newTestBackend(t, true, "backend-2"),
		},
	}

	for range 4 {
		lb.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}

	m := lb.Snapshot()
	if m.TotalRequests != 4 {
		t.Fatalf("expected 4 total requests, got %d", m.TotalRequests)
	}
	if m.HealthyBackends != 2 || len(m.Backends) != 2 {
		t.Fatalf("expected 2 healthy backends, got %d of %d", m.HealthyBackends, len(m.Backends))
	}
	for _, b := range m.Backends {
		if b.Requests != 2 {
			t.Fatalf("expected backend %s to serve 2 requests, got %d", b.Address, b.Requests)
		}
	}

	empty := &LoadBalancer{active: map[string]*Backend{}, options: Options{Selection: &RoundRobinSelection{}}}
	empty.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if got := empty.Snapshot().Unavailable; got != 1 {
		t.Fatalf("expected 1 unavailable request, got %d", got)
	}
}

func TestSyncBackendsAddsAndRemovesBackends(t *testing.T) {
	lb := &LoadBalancer{
		active: map[string]*Backend{},
		options: Options{
			Selection: &RoundRobinSelection{},
			// Health checks will fail against these fake IPs; keep them fast and rare.
			HealthChecker: &HealthChecker{
				Interval: time.Hour,
				Endpoint: "/health",
				Client:   &http.Client{Timeout: 10 * time.Millisecond},
			},
		},
	}
	t.Cleanup(lb.Close)

	lb.syncBackends([]string{"10.0.0.1", "10.0.0.2"})
	if len(lb.active) != 2 {
		t.Fatalf("expected 2 backends after first sync, got %d", len(lb.active))
	}

	lb.syncBackends([]string{"10.0.0.2"})
	if len(lb.active) != 1 {
		t.Fatalf("expected 1 backend after second sync, got %d", len(lb.active))
	}
	if _, ok := lb.active["10.0.0.2"]; !ok {
		t.Fatalf("expected 10.0.0.2 to remain, got %v", lb.active)
	}
}

func TestDNSDiscoveryPushesAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	updates := DNSDiscovery{Hostname: "localhost", Interval: time.Hour}.Watch(ctx)

	select {
	case ips := <-updates:
		if len(ips) == 0 {
			t.Fatal("expected localhost to resolve to at least one address")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for discovery")
	}

	cancel()
	select {
	case _, open := <-updates:
		if open {
			t.Fatal("expected channel to be closed after cancel")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("discovery did not stop after cancel")
	}
}
