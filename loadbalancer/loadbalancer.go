package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
)

type LoadBalancer struct {
	urls    []*httputil.ReverseProxy
	counter uint64
}

func NewLoadBalancer(addrs []string) *LoadBalancer {
	var urls []*httputil.ReverseProxy
	for _, u := range addrs {
		parsed, err := url.Parse(u)
		if err != nil {
			log.Fatal("LoadBalancer Initialisation error: ", err)
		}

		proxy := httputil.NewSingleHostReverseProxy(parsed)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "Backend has not responded", http.StatusBadGateway)
		}

		urls = append(urls, proxy)
	}
	return &LoadBalancer{
		urls: urls,
	}
}

// Round-robin
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	/**
	Receive request
	proxy to next addrss <-- getNextBackend
	Continue request in new proxy
	**/
	idx := atomic.AddUint64(&lb.counter, 1)
	lb.urls[idx%uint64(len(lb.urls))].ServeHTTP(w, r)
}

func main() {
	list := []string{
		"http://backend1:8080",
		"http://backend2:8080",
		"http://backend3:8080",
	}
	lb := NewLoadBalancer(list)

	log.Println("Load balancer running on :8080")
	err := http.ListenAndServe(":8080", lb)
	if err != nil {
		log.Fatal("Loadbalancer error: ", err)
	}
}
