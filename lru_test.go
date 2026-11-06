package rocketwind

import (
	"container/list"
	"slices"
	"strings"
	"testing"
)

// Tailwind uses LRUDefaultMap only in its canonicalization tests. Keep our
// corresponding fixture cache in test code too, bounded across theme variations.
type fixtureCache[K comparable, V any] struct {
	limit   int
	factory func(K) V
	entries map[K]*list.Element
	order   list.List
}

type fixtureEntry[K comparable, V any] struct {
	key   K
	value V
}

func (c *fixtureCache[K, V]) Get(key K) V {
	if entry := c.entries[key]; entry != nil {
		c.order.MoveToBack(entry)
		return entry.Value.(fixtureEntry[K, V]).value
	}
	value := c.factory(key)
	c.Set(key, value)
	return value
}

func (c *fixtureCache[K, V]) Set(key K, value V) {
	if c.entries == nil {
		c.entries = make(map[K]*list.Element)
	}
	if old := c.entries[key]; old != nil {
		c.order.Remove(old)
	}
	c.entries[key] = c.order.PushBack(fixtureEntry[K, V]{key, value})
	if len(c.entries) > c.limit {
		old := c.order.Front()
		delete(c.entries, old.Value.(fixtureEntry[K, V]).key)
		c.order.Remove(old)
	}
}

func (c *fixtureCache[K, V]) Keys() []K {
	keys := make([]K, 0, len(c.entries))
	for entry := c.order.Front(); entry != nil; entry = entry.Next() {
		keys = append(keys, entry.Value.(fixtureEntry[K, V]).key)
	}
	return keys
}

// Direct ports of all seven tests in utils/lru-default-map.test.ts (v4.3.4).
func TestUpstreamLRUDefaultMap(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		cache := fixtureCache[string, string]{limit: 2, factory: strings.ToUpper}
		if cache.Get("a") != "A" || cache.Get("b") != "B" {
			t.Fatal("wrong default")
		}
	})
	t.Run("factory once per cached key", func(t *testing.T) {
		calls := 0
		cache := fixtureCache[string, string]{limit: 2, factory: func(key string) string { calls++; return strings.ToUpper(key) }}
		cache.Get("a")
		cache.Get("a")
		cache.Get("a")
		if calls != 1 {
			t.Fatal(calls)
		}
	})
	t.Run("evict oldest key", func(t *testing.T) {
		cache := fixtureCache[string, string]{limit: 2, factory: func(key string) string { return key }}
		cache.Get("a")
		cache.Get("b")
		cache.Get("c")
		if !slices.Equal(cache.Keys(), []string{"b", "c"}) {
			t.Fatal(cache.Keys())
		}
	})
	t.Run("keep recently used key", func(t *testing.T) {
		cache := fixtureCache[string, string]{limit: 2, factory: func(key string) string { return key }}
		cache.Get("a")
		cache.Get("b")
		cache.Get("a")
		cache.Get("c")
		if !slices.Equal(cache.Keys(), []string{"a", "c"}) {
			t.Fatal(cache.Keys())
		}
	})
	t.Run("recreate evicted key", func(t *testing.T) {
		calls := 0
		cache := fixtureCache[string, string]{limit: 1, factory: func(key string) string { calls++; return key }}
		cache.Get("a")
		cache.Get("b")
		cache.Get("a")
		if calls != 3 {
			t.Fatal(calls)
		}
	})
	t.Run("respect limit", func(t *testing.T) {
		cache := fixtureCache[int, int]{limit: 3, factory: func(key int) int { return key }}
		for i := 0; i < 100; i++ {
			cache.Get(i % 7)
		}
		if len(cache.entries) > 3 {
			t.Fatal(cache.Keys())
		}
	})
	t.Run("evict on set", func(t *testing.T) {
		cache := fixtureCache[string, string]{limit: 2}
		cache.Set("a", "a")
		cache.Set("b", "b")
		cache.Set("c", "c")
		if !slices.Equal(cache.Keys(), []string{"b", "c"}) {
			t.Fatal(cache.Keys())
		}
	})
}
