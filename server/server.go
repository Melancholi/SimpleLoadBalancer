package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
)

func main() {
	serverType := os.Getenv("SERVER_TYPE")
	serverName := os.Getenv("SERVER_NAME")
	healthFailRate := 0
	// Health Fail Rate is to test how LB deals with failing servers
	if rawRate := os.Getenv("HEALTH_FAIL_RATE"); rawRate != "" {
		parsedRate, err := strconv.Atoi(rawRate)
		if err != nil || parsedRate < 0 || parsedRate > 100 {
			log.Fatalf("Invalid HEALTH_FAIL_RATE value %q: must be between 0 and 100", rawRate)
		}
		healthFailRate = parsedRate
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request from %s to %s", r.RemoteAddr, r.URL)
		fmt.Fprintf(w, "Connected to: %s\nServer type: %s", serverName, serverType)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if healthFailRate > 0 && rand.Intn(100) < healthFailRate {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"status":"unhealthy","server":"%s", "type":"%s"}`, serverName, serverType)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","server":"%s", "type":"%s"}`, serverName, serverType)
	})

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("Backend failure: ", err)
	}
}
