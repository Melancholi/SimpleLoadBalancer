package loadbalancer

import (
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
		active: map[string]*Backend{
			"10.0.0.2": newTestBackend(t, true, "backend-2"),
			"10.0.0.1": newTestBackend(t, true, "backend-1"),
		},
	}

	first := lb.getNextHealthyBackend()
	second := lb.getNextHealthyBackend()

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
		active: map[string]*Backend{
			"10.0.0.1": healthy,
			"10.0.0.2": unhealthy,
		},
	}

	backend := lb.getNextHealthyBackend()
	if backend == nil {
		t.Fatal("expected a healthy backend")
	}

	if backend.URL.Host != healthy.URL.Host {
		t.Fatalf("expected healthy backend %s, got %s", healthy.URL.Host, backend.URL.Host)
	}
}

func TestServeHTTPReturnsServiceUnavailableWhenNoBackendsAreAvailable(t *testing.T) {
	lb := &LoadBalancer{active: map[string]*Backend{}}
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

	var backendHits atomic.Int64
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendHits.Add(1)
		fmt.Fprint(w, "backend-ok")
	}))
	t.Cleanup(backendServer.Close)

	parsedURL, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatalf("parse backend URL: %v", err)
	}

	lb := &LoadBalancer{
		active: map[string]*Backend{
			"10.0.0.1": {
				URL:   parsedURL,
				Proxy: httputil.NewSingleHostReverseProxy(parsedURL),
				HealthCheck: &HealthCheck{
					Status: true,
				},
			},
		},
	}

	lbServer := httptest.NewServer(lb)
	t.Cleanup(lbServer.Close)

	requests := 1000
	concurrency := 50
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

	if backendHits.Load() == 0 {
		t.Fatal("expected hey traffic to reach the backend")
	}

	if !strings.Contains(string(output), "Status code distribution") {
		t.Fatalf("unexpected hey output:\n%s", string(output))
	}

	t.Logf("requests=%d concurrency=%d backend_hits=%d", requests, concurrency, backendHits.Load())
}
