package main

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	})
}

func TestBasicAuthAllowsValidCredentials(t *testing.T) {
	handler := basicAuth("admin", "secret", okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected body %q, got %q", "ok", rec.Body.String())
	}
}

func TestBasicAuthRejectsInvalidCredentials(t *testing.T) {
	tests := []struct {
		name     string
		setAuth  bool
		username string
		password string
	}{
		{name: "missing credentials", setAuth: false},
		{name: "wrong username", setAuth: true, username: "root", password: "secret"},
		{name: "wrong password", setAuth: true, username: "admin", password: "nope"},
		{name: "both wrong", setAuth: true, username: "root", password: "nope"},
		{name: "empty credentials", setAuth: true, username: "", password: ""},
		{name: "password prefix", setAuth: true, username: "admin", password: "secre"},
	}

	handler := basicAuth("admin", "secret", okHandler())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.setAuth {
				req.SetBasicAuth(tt.username, tt.password)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected status 401, got %d", rec.Code)
			}
			if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Basic ") {
				t.Fatalf("expected Basic WWW-Authenticate challenge, got %q", got)
			}
			if strings.Contains(rec.Body.String(), "ok") {
				t.Fatalf("protected handler should not have run, body=%q", rec.Body.String())
			}
		})
	}
}

func TestBasicAuthRejectsMalformedHeader(t *testing.T) {
	handler := basicAuth("admin", "secret", okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer admin:secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestMetricsProxyForwardsBody(t *testing.T) {
	const payload = `{"backends":[{"url":"http://10.0.0.1","healthy":true}]}`

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			t.Errorf("expected upstream path /metrics, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, payload)
	}))
	defer upstream.Close()

	rec := httptest.NewRecorder()
	metricsProxy(upstream.URL+"/metrics").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != payload {
		t.Fatalf("expected body %q, got %q", payload, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", got)
	}
}

func TestMetricsProxyPreservesUpstreamStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	rec := httptest.NewRecorder()
	metricsProxy(upstream.URL).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestMetricsProxyReturnsBadGatewayWhenUnreachable(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	unreachableURL := upstream.URL
	upstream.Close()

	rec := httptest.NewRecorder()
	metricsProxy(unreachableURL).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", rec.Code)
	}
}

func TestEmbeddedStaticServesIndex(t *testing.T) {
	staticFS, err := fs.Sub(static, "static")
	if err != nil {
		t.Fatalf("sub static FS: %v", err)
	}

	rec := httptest.NewRecorder()
	http.FileServerFS(staticFS).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "<html") {
		t.Fatalf("expected index.html content, got %q", rec.Body.String())
	}
}

// Wires the handlers the same way main does, to check auth sits in front of
// both the static files and the metrics endpoint.
func TestAdminRoutesRequireAuth(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	staticFS, err := fs.Sub(static, "static")
	if err != nil {
		t.Fatalf("sub static FS: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(staticFS))
	mux.Handle("GET /api/metrics", metricsProxy(upstream.URL))

	server := httptest.NewServer(basicAuth("admin", "secret", mux))
	defer server.Close()

	for _, path := range []string{"/", "/api/metrics"} {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(server.URL + path)
			if err != nil {
				t.Fatalf("unauthenticated request: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401 without credentials, got %d", resp.StatusCode)
			}

			req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
			req.SetBasicAuth("admin", "secret")
			resp, err = http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("authenticated request: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200 with credentials, got %d", resp.StatusCode)
			}
		})
	}
}
