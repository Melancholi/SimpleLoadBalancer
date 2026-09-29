package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	serverType := os.Getenv("SERVER_TYPE")
	serverName := os.Getenv("SERVER_NAME")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request from %s to %s", r.RemoteAddr, r.URL)
		fmt.Fprintf(w, "Connected to: %s\nServer type: %s", serverName, serverType)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","server":"%s", "type":"%s"}`, serverName, serverType)
	})

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("Backend failure: ", err)
	}
}
