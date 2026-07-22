package main

import (
	"log"
	"net/http"

	"github.com/Melancholi/SimpleLoadBalancer/loadbalancer"
)

func main() {
	lb := loadbalancer.NewLoadBalancer()

	log.Println("Load balancer running on :8080")
	err := http.ListenAndServe(":8080", lb)
	if err != nil {
		log.Fatal("Loadbalancer error: ", err)
	}
}
