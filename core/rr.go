package core

import (
	"sync"
)

type roundRobin struct {
	mu       *sync.Mutex
	sessions map[string]int
	nb       int
	idx      int
}

func NewRRLoadBalancer(nbGPUs int) *roundRobin {
	return &roundRobin{
		mu:       &sync.Mutex{},
		sessions: make(map[string]int),
		nb:       nbGPUs,
	}
}

func (rr *roundRobin) choose(sess string, cost int) (int, int, error) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	gpu, exists := rr.sessions[sess]
	if !exists {
		gpu = rr.idx
		rr.idx = (rr.idx + 1) % rr.nb
		rr.sessions[sess] = gpu
	}
	return gpu, cost, nil
}

func (rr *roundRobin) complete(i int, cost int) {
}

func (rr *roundRobin) terminate(sess string, gpu int) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	delete(rr.sessions, sess)
}
