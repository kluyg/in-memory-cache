package cache

import "github.com/gebi/cowmap"

type CowMap struct {
	m *cowmap.Map[string, string]
}

func NewCowMap() *CowMap {
	return &CowMap{
		m: cowmap.New[string, string](nil),
	}
}

func (c *CowMap) Get(key string) (string, bool) {
	return c.m.Get(key)
}

func (c *CowMap) Set(key, value string) {
	c.m.Insert(key, value)
}

func (c *CowMap) Delete(key string) {
	_ = c.m.Delete(key)
}

func (c *CowMap) Len() int {
	n := 0
	for _, _ = range c.m.All() {
		n++
	}
	return n
}

// Note: CowMap intentionally does NOT implement BulkLoader. There is no way
// to bulk-assign a sync.Map, so the harness prefills it with a Store loop,
// which is O(n) and fine.
