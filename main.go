package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/Melancholi/SimpleLoadBalancer/loadbalancer"
)

func main() {
	configPath := flag.String("config", "loadbalancer/config.toml", "path to the load balancer config file")
	flag.Parse()

	cfg := loadbalancer.LoadConfig(*configPath)

	lb := loadbalancer.NewLoadBalancer(loadbalancer.Options{
		Selection: &loadbalancer.RoundRobinSelection{},
		HealthChecker: &loadbalancer.HealthChecker{
			Interval: time.Duration(cfg.Server.HealthCheckInterval) * time.Second,
			Endpoint: cfg.Server.HealthEndpoint,
			Client:   &http.Client{Timeout: 5 * time.Second},
		},
		Discovery: loadbalancer.DNSDiscovery{Hostname: cfg.Server.Hostname, Interval: 30 * time.Second},
	}, cfg)
	defer lb.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Addr:    ":8080",
		Handler: lb,
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", lb.MetricsHandler())
	metricsServer := &http.Server{
		Addr:    ":9090",
		Handler: metricsMux,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("metrics shutdown failed: %v", err)
		}
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	go func() {
		log.Println("Metrics running on :9090")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	log.Println("Load balancer running on :8080")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("Loadbalancer error: ", err)
	}
}
