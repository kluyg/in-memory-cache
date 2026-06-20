---
title: "Shard your locks: benchmarking 6 Go cache designs"
date: 2026-06-20
draft: false
tags: ["go", "caching", "performance", "concurrency", "benchmarking"]
summary: "I built the same in-memory cache six ways with the Go standard library and benchmarked them across read/write mixes and core counts. Lock striping wins by up to 8×, sync.RWMutex turns out to be a trap, and one design gets slower the more cores you add."
---

I built the same in-memory `string → string` cache six ways, using nothing but
the Go standard library, and benchmarked them under read-heavy, balanced, and
write-heavy load across 1 to 8 cores. The rankings flip depending on the
workload — and one of the "obvious" answers gets *slower* the more cores you
give it.

**TL;DR:** Shard your locks. A 256-way striped map (`sharded`) was the
all-around winner — up to **8× faster** than a single `sync.Mutex` at 8 cores —
and it's about 15 lines of code. `sync.RWMutex`, the reflexive fix for "reads
are contended," is a trap: it barely helps reads past two cores and is *slower
than a plain mutex* for writes.

## The contenders

| Cache | Idea | One-liner |
|---|---|---|
| `naive` | Plain `map`, no locking | Not thread-safe — concurrent writes crash the process. Baseline only. |
| `mutex` | One `sync.Mutex` | Simple, correct, doesn't scale. |
| `rwmutex` | One `sync.RWMutex` | Parallel reads, exclusive writes. |
| `syncmap` | `sync.Map` | The stdlib's own concurrent map. |
| `sharded` | 256 shards, one mutex each | Lock striping. Keys routed by hash. |
| `cow` | Copy-on-write via `atomic.Pointer` | Lock-free reads; every write copies the whole map. |

All six satisfy one interface, so a single harness drives them identically.
Code: [github.com/kluyg/in-memory-cache](https://github.com/kluyg/in-memory-cache).

## How I measured (the short version)

`testing.B` + `b.RunParallel`, 1,000,000 keys, GOMAXPROCS swept 1→8, on a
20-core i7-14700K. Each data point is the median of 10 runs summarized with
`benchstat`; variation was mostly ±0–3%. Throughput below is `1000 / (ns/op)`
in millions of ops/sec — higher is better. I measured the cache *in-process*,
not behind HTTP: net/http + JSON cost microseconds, which would bury the
nanosecond-scale differences I'm chasing.

The 14700K is a *hybrid* chip — 8 performance cores (with hyperthreading) plus
12 efficiency cores — so an unpinned sweep is a trap: as GOMAXPROCS rises, the
OS can spill goroutines onto E-cores or hyperthread siblings and migrate them
mid-run, which confounds the scaling curves. So the process is pinned to one
thread per physical P-core (affinity `0x5555`); each GOMAXPROCS step adds a real
P-core. Pinning shifted the absolute numbers by 10–25% in places but left every
ranking and curve shape unchanged.

One deliberate non-axis: **value size doesn't matter here.** Go strings are
immutable, so `Set` stores a 16-byte header and never touches the value bytes —
64 B and 16 KB benchmark identically (0 B/op). Value size affects memory and GC,
not op throughput.

## The results

![Throughput vs cores, by read/write mix, uniform distribution](throughput_by_mix_uniform.png)

Read the slopes, not just the heights:

- **`sharded` and `cow` climb; `mutex` is flat.** More cores, more throughput —
  unless you picked a single lock.
- **`cow` owns read-only** (87 Mops/s at 8 cores, fully lock-free reads) and
  **vanishes the moment writes appear** — it's pinned to ≈0 on the three write
  panels because every `Set` copies the entire million-entry map.
- **`sharded` is the only design that's near the top in *every* panel.**

### The obvious fix scales backwards

Normalize each design to its own single-core throughput and the story gets
sharper:

![Scaling efficiency, read-only](scaling_efficiency_r100_uniform.png)

- **`mutex` is *below* 1×** — at 8 cores it's 0.66× its single-core speed.
  Reads can't run in parallel, and the cache line holding the lock ping-pongs
  between cores. You added hardware and lost performance.
- **`rwmutex` plateaus around 2×.** The shared reader counter becomes the new
  contention point; it stops improving after ~4 cores.
- **`sharded` reaches 6.9×, while `cow` and `syncmap` track — even slightly
  exceed — the ideal 8× line** (lock-free reads get a bonus from the larger
  aggregate cache). Caveat: `syncmap`'s great *slope* flatters a poor baseline —
  it's still slower in absolute terms than `sharded`.

### Skew isn't simply "worse"

Real caches see Zipfian access — a few hot keys take most of the traffic. The
common assumption is that skew hurts. It's more interesting than that:

![Skew speedup at 8 cores](skew_speedup_8cores.png)

Above 1× means *faster* under skew. **Reads get faster almost everywhere** — the
hot keys stay in CPU cache (`mutex` reads speed up 1.6×, `syncmap` 1.9×). The
striking exception is **`sharded`'s balanced mix at 0.82× — skew makes it
*slower***: hot keys collide on a few shards, so those locks contend while the
rest sit idle. Skew helps locality and hurts lock striping; which wins depends
on your design and your write ratio.

### The numbers (8 cores, ns/op, lower is better)

Uniform:

| mix | mutex | rwmutex | syncmap | sharded | cow |
|---|--:|--:|--:|--:|--:|
| read-only | 168 | 53 | 30 | 21 | **11.5** |
| read-heavy | 168 | 259 | 37 | **22** | 12.0 ms |
| balanced | 190 | 282 | 57 | **24** | 46.5 ms |
| write-heavy | 208 | 222 | 73 | **25** | 82.5 ms |

Zipfian (s=1.1):

| mix | mutex | rwmutex | syncmap | sharded | cow |
|---|--:|--:|--:|--:|--:|
| read-only | 106 | 49 | 16 | 17 | **7** |
| read-heavy | 112 | 225 | 24 | **24** | 9.0 ms |
| balanced | 126 | 183 | 46 | **29** | 45.1 ms |
| write-heavy | 131 | 142 | 68 | **32** | 84.0 ms |

Those `cow` write cells are in *milliseconds*: copying a million-entry map per
write is ~10⁶× slower than the alternatives. It's the price of lock-free reads.

## The winner, in a few lines

`sharded` is just N independent maps, each behind its own lock. A key's hash
picks the shard, so operations on different keys almost never touch the same
lock — contention drops by roughly a factor of N:

```go
const shards = 256 // power of two, so we can mask instead of modulo

type part struct {
	mu sync.Mutex
	m  map[string]string
}
type Sharded struct{ parts [shards]*part }

func (c *Sharded) at(key string) *part {
	h := uint64(14695981039346656037) // FNV-1a
	for i := 0; i < len(key); i++ {
		h = (h ^ uint64(key[i])) * 1099511628211
	}
	return c.parts[h&(shards-1)]
}

func (c *Sharded) Get(key string) (string, bool) {
	p := c.at(key)
	p.mu.Lock()
	v, ok := p.m[key]
	p.mu.Unlock() // not defer: it has overhead, and this is a post about ns
	return v, ok
}

func (c *Sharded) Set(key, value string) {
	p := c.at(key)
	p.mu.Lock()
	p.m[key] = value
	p.mu.Unlock()
}
```

That's the whole idea. The [real version](https://github.com/kluyg/in-memory-cache/blob/main/sharded.go)
adds `Delete`/`Len` and pads each shard onto its own cache line (so locking one
shard doesn't bounce a neighbor's cache line), but the engine is right here.

## What to actually use

- **Default to `sharded`.** Best or near-best everywhere, scales with cores,
  trivial to write. This is the answer for most concurrent maps.
- **`cow` for read-mostly-to-read-only data** — config snapshots, routing
  tables, feature flags. Unbeatable reads, but only if writes are rare and
  batched. Never for write-heavy load.
- **`sync.Map` only in its niche** — stable keys written once and read forever,
  or goroutines touching disjoint key sets. Outside that it's mediocre and it
  *allocates* (interface boxing: 40–72 B/op).
- **`sync.RWMutex`: reach for it rarely.** It only wins in the narrow
  read-heavy, low-core corner, and it's worse than a plain mutex for writes.
- **A plain `mutex` is fine** when contention is low or you're at low core
  counts. Don't reach for cleverness you can't measure a need for.
- **Never the naive map across goroutines** — Go's runtime will deliberately
  crash the process with a `concurrent map writes` fatal error.

## Three things that surprised me

1. **More cores made the single-mutex cache slower.** Negative scaling is real,
   and it's just cache-line contention on the lock itself.
2. **`RWMutex` is a half-measure that backfires on writes.** The reader-counter
   bookkeeping costs more than it saves once writes enter the mix.
3. **Skew is a double-edged sword**, not a uniform penalty — it speeds up reads
   via cache locality while concentrating write contention.

The full code, raw `benchstat` output, and a one-command sweep to reproduce all
of this on your own hardware are in the
[repo](https://github.com/kluyg/in-memory-cache).
