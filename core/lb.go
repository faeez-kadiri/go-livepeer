package core

import (
	"errors"
	"math"
	"sync"
)

type loadBalancer struct {

	// Accesses to all fields need to be protected by this mutex.
	mu *sync.Mutex

	// Holds current load per GPU.
	gpus []int

	// Maps existing sessions to GPUs (as a bitmap).
	// Each session may exist on one or more GPUs.
	sessions map[string]uint64
}

func NewGPULoadBalancer(nbGPUs int) *loadBalancer {
	return &loadBalancer{
		mu:       &sync.Mutex{},
		gpus:     make([]int, nbGPUs),
		sessions: make(map[string]uint64),
	}
}

var errNoGPUs = errors.New("GPU list must not be empty")

func penalize(c int) int {
	// 30% increase
	return int(math.Ceil(float64(c) * 1.3))
}

func (lb *loadBalancer) minGPU(sess string, cost int) (int, int, error) {
	if len(lb.gpus) <= 0 {
		return 0, 0, errNoGPUs
	}
	min, sz, idx, actualC := math.MaxInt64, len(lb.gpus), 0, 0
	for i := 0; i < sz; i++ {
		c := cost
		if exists := lb.sessions[sess]&(1<<uint64(i)) > 0; !exists {
			c = penalize(c) // creating a new session is expensive
		}
		if lb.gpus[i]+c < min {
			min = lb.gpus[i] + c
			idx = i
			actualC = c
		}
	}
	return idx, actualC, nil
}

func (lb *loadBalancer) choose(sess string, cost int) (int, int, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	i, c, err := lb.minGPU(sess, cost)
	if err != nil {
		return 0, 0, err
	}
	if c != cost {
		// Creating a new session
		lb.sessions[sess] |= (1 << uint64(i))
	}
	lb.gpus[i] += c
	return i, c, nil
}

func (lb *loadBalancer) complete(i int, cost int) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.gpus[i] -= cost
}

func (lb *loadBalancer) terminate(sess string, gpu int) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	v, exists := lb.sessions[sess]
	if !exists {
		return
	}
	v &= ^(1 << uint64(gpu))
	if v == 0 {
		delete(lb.sessions, sess)
	} else {
		lb.sessions[sess] = v
	}
}
