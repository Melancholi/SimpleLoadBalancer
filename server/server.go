package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
)

func main() {
	server_name := os.Getenv("SERVER_NAME")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		records, err := net.LookupHost(server_name)
		if err != nil {
			log.Printf("%s failed Records check: %s", err, server_name)
		} else {
			log.Printf("`{Records: %s}`", records)
		}
		fmt.Fprintf(w, "Connected to: %s\n", server_name)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		//Add randomness to test health check handlin
		res := rand.Intn(2)

		status := map[int]struct {
			status string
			code   int
		}{
			0: {"healthy", http.StatusOK},
			1: {"unhealthy", http.StatusInternalServerError},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status[res].code)

		fmt.Fprintf(w, `{"status":"%s","server":"%s"}`, status[res].status, server_name)
	})

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("Backend failure: ", err)
	}
}
