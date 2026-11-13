package cache

import (
	"log/slog"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"
	"golang.org/x/sync/singleflight"
)

type Cache[K comparable, V any] struct {
	cache    *ristretto.Cache
	sf       singleflight.Group
	mu       sync.Mutex
	inflight map[string]map[*byte]struct{}
}

// NewCache is tuned for RTB bids (~500k–1M active bids).
func NewCache[K comparable, V any]() *Cache[K, V] {
	c, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 10e7,       // 100M counters — accuracy for 10M keys
		MaxCost:     8 << 30,    // 4GB cost (cost=1 per item → 4B items max, limited by RAM in practice)
		BufferItems: 128 * 1024, // large buffer for QPS >10k
		Metrics:     true,
		OnEvict: func(item *ristretto.Item) {
			slog.Debug("bid cache evicted", slog.Any("key", item.Key))
		},
		OnReject: func(item *ristretto.Item) {
			slog.Debug("bid cache rejected (full)", slog.Any("key", item.Key))
		},
	})
	if err != nil {
		panic(err)
	}
	slog.Info("ristretto bid cache initialized")
	return &Cache[K, V]{
		cache:    c,
		inflight: make(map[string]map[*byte]struct{}),
	}
}

func (c *Cache[K, V]) Set(key K, value V, ttl time.Duration) bool {
	var skey string
	var hadInflight bool
	c.mu.Lock()
	if len(c.inflight) > 0 {
		skey = sfKey(key)
		if _, ok := c.inflight[skey]; ok {
			delete(c.inflight, skey)
			hadInflight = true
		}
	}
	ok := c.cache.SetWithTTL(key, value, 1, ttl)
	c.mu.Unlock()
	if hadInflight {
		c.sf.Forget(skey)
	}
	c.cache.Wait()
	return ok
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	val, found := c.cache.Get(key)
	if !found {
		var zero V
		return zero, false
	}
	if val == nil {
		var zero V
		return zero, true
	}
	return val.(V), true
}

func (c *Cache[K, V]) Del(key K) {
	var skey string
	var hadInflight bool
	c.mu.Lock()
	if len(c.inflight) > 0 {
		skey = sfKey(key)
		if _, ok := c.inflight[skey]; ok {
			delete(c.inflight, skey)
			hadInflight = true
		}
	}
	c.cache.Del(key)
	c.mu.Unlock()
	if hadInflight {
		c.sf.Forget(skey)
	}
}

// Clear wipes the entire cache. Use after batch DB operations (e.g. worker resetting limit
// counters) when invalidating each key individually is too expensive.
func (c *Cache[K, V]) Clear() {
	c.mu.Lock()
	keys := make([]string, 0, len(c.inflight))
	for k := range c.inflight {
		keys = append(keys, k)
	}
	clear(c.inflight)
	c.cache.Clear()
	c.mu.Unlock()
	for _, k := range keys {
		c.sf.Forget(k)
	}
}

func (c *Cache[K, V]) Close() {
	c.cache.Close()
}

func (c *Cache[K, V]) Metrics() (hits, misses, drops uint64) {
	return c.cache.Metrics.Hits(), c.cache.Metrics.Misses(), c.cache.Metrics.GetsDropped()
}

// GetOrLoad reads from the cache first; on miss it coalesces concurrent loads
// for the same key via singleflight, stores the result, and returns it.
func (c *Cache[K, V]) GetOrLoad(key K, ttl time.Duration, load func() (V, error)) (V, error) {
	if c == nil {
		return load()
	}
	if v, ok := c.Get(key); ok {
		return v, nil
	}

	skey := sfKey(key)
	val, err, _ := c.sf.Do(skey, func() (any, error) {
		tok := new(byte)
		c.mu.Lock()
		if v, ok := c.Get(key); ok {
			c.mu.Unlock()
			return v, nil
		}
		addInflight(&c.inflight, skey, tok)
		c.mu.Unlock()

		defer func() {
			c.mu.Lock()
			removeInflight(c.inflight, skey, tok)
			c.mu.Unlock()
		}()

		v, err := load()
		if err != nil {
			var zero V
			return zero, err
		}

		c.mu.Lock()
		stored := false
		if removeInflight(c.inflight, skey, tok) {
			stored = c.cache.SetWithTTL(key, v, 1, ttl)
		}
		c.mu.Unlock()
		if stored {
			c.cache.Wait()
		}

		return v, nil
	})
	if err != nil {
		var zero V
		return zero, err
	}
	if val == nil {
		var zero V
		return zero, nil
	}
	return val.(V), nil
}
