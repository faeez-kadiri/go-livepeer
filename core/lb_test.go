package core

import (
	"strconv"
	"testing"

	"github.com/flyingmutant/rapid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLB_Choose(t *testing.T) {
	assert := assert.New(t)
	lb := NewGPULoadBalancer(8)
	isPenalized := func(orig, cost int) bool {
		return cost == penalize(orig)
	}
	rapid.Check(t, func(t *rapid.T) {
		sessInt := rapid.IntsRange(1, 100).Draw(t, "sess").(int)
		sess := strconv.Itoa(sessInt)
		hasGPU := make([]bool, len(lb.gpus))

		// Check existing sessions map in LB and compare before-and-after choosing
		for i := 0; i < len(hasGPU); i++ {
			v, exists := lb.sessions[sess]
			hasGPU[i] = exists && (v&(1<<uint64(i)) > 0)
		}
		origCost := sessInt // assume cost == sess for now
		idx, cost, err := lb.choose(sess, origCost)
		require.Nil(t, err)

		if hasGPU[idx] {
			assert.Equal(cost, origCost, "Did not expect cost to change if no insert was done")
		} else {
			// New session should have been added
			assert.Contains(lb.sessions, sess)
			assert.Equal(uint64(1<<uint64(idx)), lb.sessions[sess]&(1<<uint64(idx)),
				"Did not find session at index?")
			assert.True(isPenalized(origCost, cost), "Did not have expected penalty")
		}
		// Check actual load balance
		// this gpu should not be more loaded than any other gpu by `cost`
		for i := 0; i < len(lb.gpus); i++ {
			if i == idx {
				continue
			}
			c := cost
			if !hasGPU[i] && !isPenalized(origCost, c) {
				// account for penalty if would have been a new session at i
				// (but don't re-penalize if current cost is already penalzied)
				c = penalize(c)
			}
			if lb.gpus[idx] > lb.gpus[i] && lb.gpus[idx]-lb.gpus[i] > c {
				t.Error("Did not have expected load : ", idx, i, c, lb.gpus)
			}
		}
	})
}
