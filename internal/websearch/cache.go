package websearch

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Cached membungkus Searcher dengan cache hasil per kueri. Mengurangi
// pemanggilan berulang (model sering mencari ulang kueri yang sama) dan
// menjaga fallback tanpa API key dari rate limit.
func Cached(s Searcher, ttl time.Duration, max int) Searcher {
	return &cached{inner: s, ttl: ttl, max: max, items: map[string]cacheItem{}}
}

type cacheItem struct {
	results []Result
	exp     time.Time
}

type cached struct {
	inner Searcher
	ttl   time.Duration
	max   int
	mu    sync.Mutex
	items map[string]cacheItem
}

func (c *cached) Name() string { return c.inner.Name() }

func (c *cached) Search(ctx context.Context, q string, n int) ([]Result, error) {
	key := strings.ToLower(strings.Join(strings.Fields(q), " "))
	now := time.Now()
	c.mu.Lock()
	if it, ok := c.items[key]; ok && now.Before(it.exp) && len(it.results) >= n {
		c.mu.Unlock()
		return it.results[:n:n], nil
	}
	c.mu.Unlock()

	rs, err := c.inner.Search(ctx, q, n)
	if err != nil || len(rs) == 0 {
		return rs, err // error & hasil kosong tidak di-cache
	}
	c.mu.Lock()
	if len(c.items) >= c.max {
		for k, it := range c.items { // buang yang kedaluwarsa; bila penuh, kosongkan
			if now.After(it.exp) {
				delete(c.items, k)
			}
		}
		if len(c.items) >= c.max {
			c.items = map[string]cacheItem{}
		}
	}
	c.items[key] = cacheItem{results: rs, exp: now.Add(c.ttl)}
	c.mu.Unlock()
	return rs, nil
}
