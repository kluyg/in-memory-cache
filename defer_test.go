package cache

import (
	"sync"
	"testing"
)

// Micro-benchmark isolating the cost of `defer mu.Unlock()` versus an explicit
// Unlock in a mutex-guarded leaf read — the pattern in Sharded.Get/Set. Since
// Go 1.14's open-coded defers, a single defer in a leaf function is nearly free;
// this quantifies it on the current toolchain. Run single-threaded so the lock
// is uncontended and the defer cost is what's left:
//
//	go test -bench=BenchmarkDefer -cpu=1 -count=10

type deferBox struct {
	mu sync.Mutex
	m  map[string]string
}

func newDeferBox() *deferBox {
	return &deferBox{m: map[string]string{"k": "v"}}
}

func (b *deferBox) getExplicit(k string) (string, bool) {
	b.mu.Lock()
	v, ok := b.m[k]
	b.mu.Unlock()
	return v, ok
}

func (b *deferBox) getDefer(k string) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.m[k], true
}

// sink defeats dead-code elimination of the benchmarked calls.
var sink string

func BenchmarkDeferExplicit(b *testing.B) {
	box := newDeferBox()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, _ := box.getExplicit("k")
		sink = v
	}
}

func BenchmarkDeferDeferred(b *testing.B) {
	box := newDeferBox()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		v, _ := box.getDefer("k")
		sink = v
	}
}
