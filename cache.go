// Package cache contains several in-memory string->string cache
// implementations that differ only in how they handle concurrent access.
// They all satisfy the same Cache interface so a benchmark harness can
// compare them under identical workloads.
//
// All implementations use only the Go standard library.
package cache

import "fmt"

// Cache is the common interface implemented by every variant.
//
// The contract is deliberately minimal: a concurrent map with no eviction
// and no bounds. This keeps the focus on synchronization cost rather than
// cache-replacement policy.
type Cache interface {
	// Get returns the value for key and whether it was present.
	Get(key string) (value string, ok bool)
	// Set stores value under key, overwriting any previous value.
	Set(key, value string)
	// Delete removes key if present.
	Delete(key string)
	// Len returns the number of entries currently stored.
	Len() int
}

// BulkLoader is an optional fast path for prefilling a cache before a
// benchmark. It matters for copy-on-write implementations, where calling
// Set n times to load n keys is O(n^2). When a Cache implements BulkLoader
// the harness uses it instead of a Set loop.
type BulkLoader interface {
	// Load replaces the cache contents with items. The caller must not
	// mutate items afterwards; ownership is transferred to the cache.
	Load(items map[string]string)
}

// Impl pairs an implementation name with a constructor.
type Impl struct {
	Name string
	New  func() Cache
}

// Concurrent lists the implementations that are safe to drive from many
// goroutines at once. These are the ones the parallel benchmark exercises.
var Concurrent = []Impl{
	{"mutex", func() Cache { return NewMutex() }},
	{"rwmutex", func() Cache { return NewRWMutex() }},
	{"syncmap", func() Cache { return NewSyncMap() }},
	{"syncXmap", func() Cache { return NewSyncXMap() }},
	{"otter", func() Cache { return NewOtter() }},
	{"sharded", func() Cache { return NewSharded() }},
	{"cow", func() Cache { return NewCOW() }},
	{"cowmap", func() Cache { return NewCowMap() }},
}

// All includes the naive (unsynchronized) implementation in addition to the
// concurrent ones. naive is only safe single-threaded; it exists as a
// sequential baseline and to demonstrate Go's data-race behavior.
var All = append([]Impl{
	{"naive", func() Cache { return NewNaive() }},
}, Concurrent...)

// New constructs the named implementation, or returns an error if the name
// is unknown. Used by the HTTP demo server.
func New(name string) (Cache, error) {
	for _, impl := range All {
		if impl.Name == name {
			return impl.New(), nil
		}
	}
	return nil, fmt.Errorf("unknown cache implementation %q", name)
}
