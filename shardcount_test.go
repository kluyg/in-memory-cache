package cache

import (
	"fmt"
	"math/rand"
	"testing"
)

// shardedN is the sharded cache with a configurable, runtime-set shard count
// (the production Sharded fixes it at 256 via a const + array). It reuses the
// same shard type and fnv1a hash, so only N differs — exactly what a
// shard-count sensitivity sweep needs.
type shardedN struct {
	mask  uint64
	parts []*shard
}

func newShardedN(n int) *shardedN { // n must be a power of two
	parts := make([]*shard, n)
	for i := range parts {
		parts[i] = &shard{m: make(map[string]string)}
	}
	return &shardedN{mask: uint64(n - 1), parts: parts}
}

func (c *shardedN) get(k string) (string, bool) {
	p := c.parts[fnv1a(k)&c.mask]
	p.mu.Lock()
	v, ok := p.m[k]
	p.mu.Unlock()
	return v, ok
}

func (c *shardedN) set(k, v string) {
	p := c.parts[fnv1a(k)&c.mask]
	p.mu.Lock()
	p.m[k] = v
	p.mu.Unlock()
}

// BenchmarkShardCount sweeps the shard count to show where the contention curve
// flattens — i.e. why 256 is a reasonable default. Balanced 50/50 mix over
// uniform keys; run at a high core count to expose lock contention:
//
//	INMEMCACHE_AFFINITY=0x5555 go test -bench=BenchmarkShardCount -cpu=8 -count=10 -keys=1000000
func BenchmarkShardCount(b *testing.B) {
	n := *numKeysFlag
	keys := makeKeys(n, *keyLenFlag)
	val := makeValue(benchValueBytes)

	for _, shards := range []int{1, 16, 64, 256, 1024, 4096} {
		c := newShardedN(shards)
		for _, k := range keys { // prefill
			c.set(k, val)
		}
		b.Run(fmt.Sprintf("shards=%d", shards), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				r := rand.New(rand.NewSource(nextSeed()))
				for pb.Next() {
					k := keys[r.Intn(n)]
					if r.Float64() < 0.5 {
						c.get(k)
					} else {
						c.set(k, val)
					}
				}
			})
		})
	}
}
