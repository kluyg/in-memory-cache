package cache

import "github.com/puzpuzpuz/xsync/v4"

// SyncXMap wraps puzpuzpuz/xsync map
//
// A Map is like a concurrent hash table-based map. It follows the interface of
// sync.Map with a number of valuable extensions like Compute or Size.
// Map uses a modified version of Cache-Line Hash Table (CLHT) data structure
//
// CLHT is built around the idea of organizing the hash table in
// cache-line-sized buckets, so that on all modern CPUs update operations
// complete with minimal cache-line transfer. Also, Get operations are
// obstruction-free and involve no writes to shared memory, hence no mutexes or
// any other sort of locks. Due to this design, in all considered scenarios Map
// outperforms sync.Map. Map also uses cooperative parallel rehashing: this
// means that the goroutines executing write operations may participate in a
// concurrent rehashing instead of waiting for it to finish.
//
// Apart from CLHT, Map borrows ideas from Java's j.u.c.ConcurrentHashMap
// (immutable K/V pair structs instead of atomic snapshots) and C++'s
// absl::flat_hash_map a.k.a. SwissTable (meta memory and SWAR-based lookups).
//
// Map uses the built-in Golang's hash function which has DDOS protection. It
// uses maphash.Comparable as the default hash function. This means that each
// map instance gets its own seed number and the hash function uses that seed
// for hash code calculation.

type SyncXMap struct {
	m *xsync.MapOf[string, string]
}

func NewSyncXMap() *SyncXMap {
	return &SyncXMap{
		m: xsync.NewMapOf[string, string](),
	}
}

func (c *SyncXMap) Get(key string) (string, bool) {
	v, ok := c.m.Load(key)
	if !ok {
		return "", false
	}
	return v, true
}

func (c *SyncXMap) Set(key, value string) {
	c.m.Store(key, value)
}

func (c *SyncXMap) Delete(key string) {
	c.m.Delete(key)
}

func (c *SyncXMap) Len() int {
	n := 0
	c.m.Range(func(_, _ string) bool {
		n++
		return true
	})
	return n
}
