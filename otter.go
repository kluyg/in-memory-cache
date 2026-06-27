package cache

import "github.com/maypok86/otter/v2"

// Otter wraps THE otter cache lib github.com/maypok86/otter

type Otter struct {
	o *otter.Cache[string, string]
}

func NewOtter() *Otter {
	cache := otter.Must(&otter.Options[string, string]{
		MaximumSize:     1 << 30,
		InitialCapacity: 1_000_000,
	})
	return &Otter{
		o: cache,
	}
}

func (c *Otter) Get(key string) (string, bool) {
	return c.o.GetIfPresent(key)
}

func (c *Otter) Set(key, value string) {
	c.o.Set(key, value)
}

func (c *Otter) Delete(key string) {
	_, _ = c.o.Invalidate(key)
}

func (c *Otter) Len() int {
	// use EstimatedSize because iterating and counting by hand would have similar problems
	//	like invalidation, todeleted items, inserted items in parallel, ...
	c.o.CleanUp()
	return int(c.o.EstimatedSize())
}
