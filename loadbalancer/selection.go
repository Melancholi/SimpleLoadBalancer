package loadbalancer

import "sync/atomic"

type SelectionStrategy interface {
	GetNextBackend(backends []*Backend) *Backend
}

type RoundRobinSelection struct {
	counter atomic.Uint64
}

// Backends passed to this function MUST be healthy
func (rr *RoundRobinSelection) GetNextBackend(backends []*Backend) *Backend {
	//technically, this should not happen (checks before passing to this), but keep it just in case
	if len(backends) == 0 {
		return nil
	}
	idx := rr.counter.Add(1) - 1
	return backends[idx%uint64(len(backends))]
}
