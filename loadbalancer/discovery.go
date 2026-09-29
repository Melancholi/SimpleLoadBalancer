package loadbalancer

import (
	"context"
	"log"
	"net"
	"time"
)

type DiscoveryStrategy interface {
	Watch(ctx context.Context) <-chan []string
}

type DNSDiscovery struct {
	Hostname string
	Interval time.Duration
}

func (d DNSDiscovery) Watch(ctx context.Context) <-chan []string {
	interval := d.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	out := make(chan []string)
	go func() {
		defer close(out)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			ips, err := net.DefaultResolver.LookupHost(lookupCtx, d.Hostname)
			cancel()

			if err != nil {
				// Skip this round rather than pushing an empty list, so a DNS
				// hiccup doesn't remove every backend.
				log.Printf("DNS lookup for %s failed: %v", d.Hostname, err)
			} else {
				select {
				case out <- ips:
				case <-ctx.Done():
					return
				}
			}

			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
