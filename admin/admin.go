package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed static
var static embed.FS

func main() {
	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")
	if username == "" || password == "" {
		log.Fatal("ADMIN_USERNAME and ADMIN_PASSWORD must be set")
	}

	metricsURL := os.Getenv("METRICS_URL")
	if metricsURL == "" {
		metricsURL = "http://loadbalancer:9090/metrics"
	}

	staticFS, err := fs.Sub(static, "static")
	if err != nil {
		log.Fatal("Failed to load static files: ", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(staticFS))
	mux.Handle("GET /api/metrics", metricsProxy(metricsURL))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Addr:              ":8081",
		Handler:           basicAuth(username, password, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	log.Println("Admin panel running on :8081")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("Admin panel error: ", err)
	}
}

// basicAuth guards every route with HTTP Basic Auth. Credentials are hashed
// before comparing so the comparison is constant time regardless of length.
func basicAuth(username, password string, next http.Handler) http.Handler {
	wantUser := sha256.Sum256([]byte(username))
	wantPass := sha256.Sum256([]byte(password))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		gotUser := sha256.Sum256([]byte(user))
		gotPass := sha256.Sum256([]byte(pass))

		//Constant Time avoids timing attacks
		userMatch := subtle.ConstantTimeCompare(gotUser[:], wantUser[:]) == 1
		passMatch := subtle.ConstantTimeCompare(gotPass[:], wantPass[:]) == 1

		if !ok || !userMatch || !passMatch {
			w.Header().Set("WWW-Authenticate", `Basic realm="loadbalancer-admin", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// metricsProxy fetches the load balancer metrics from its internal port.
func metricsProxy(metricsURL string) http.Handler {
	client := &http.Client{Timeout: 3 * time.Second}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, err := client.Get(metricsURL)
		if err != nil {
			log.Printf("Failed to fetch metrics: %v", err)
			http.Error(w, "Load balancer unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}
