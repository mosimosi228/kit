package cache

import (
	"container/list"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"sync"

	"golang.org/x/sync/singleflight"
)

type entryLru[K comparable, V any] struct {
	key   K
	value V
}

type LRUCache[K comparable, V any] struct {
	mu        sync.RWMutex
	cap       int
	items     map[K]*list.Element
	evictList *list.List
	sf        singleflight.Group
	inflight  map[string]map[*byte]struct{}
}

func NewLRU[K comparable, V any](capacity int) *LRUCache[K, V] {
	if capacity <= 0 {
		capacity = 1000
	}
	return &LRUCache[K, V]{
		cap:       capacity,
		items:     make(map[K]*list.Element),
		evictList: list.New(),
		inflight:  make(map[string]map[*byte]struct{}),
	}
}

// Get returns the value for key and updates LRU order on hit.
// Miss: RLock only (minimal contention). Hit: brief Lock for double-check and MoveToFront
// to avoid a race with eviction (the element may be evicted in Set/Invalidate between RUnlock and Lock).
func (c *LRUCache[K, V]) Get(key K) (V, bool) {
	if c == nil {
		var zero V
		return zero, false
	}
	c.mu.RLock()
	elem, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		var zero V
		return zero, false
	}
	c.mu.Lock()
	// Element may have been evicted between RUnlock and Lock — recheck.
	if elem2, stillPresent := c.items[key]; !stillPresent || elem2 != elem {
		c.mu.Unlock()
		var zero V
		return zero, false
	}
	c.evictList.MoveToFront(elem)
	value := elem.Value.(*entryLru[K, V]).value
	c.mu.Unlock()
	return value, true
}

func (c *LRUCache[K, V]) Set(key K, value V) {
	if c == nil {
		return
	}
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
	c.setLocked(key, value)
	c.mu.Unlock()
	if hadInflight {
		c.sf.Forget(skey)
	}
}

func (c *LRUCache[K, V]) setLocked(key K, value V) {
	if elem, ok := c.items[key]; ok {
		c.evictList.MoveToFront(elem)
		elem.Value.(*entryLru[K, V]).value = value
		return
	}
	e := &entryLru[K, V]{key: key, value: value}
	elem := c.evictList.PushFront(e)
	c.items[key] = elem
	if c.evictList.Len() > c.cap {
		if back := c.evictList.Back(); back != nil {
			e := c.evictList.Remove(back).(*entryLru[K, V])
			delete(c.items, e.key)
			slog.Debug("evicted from cache", "key", e.key)
		}
	}
}

func (c *LRUCache[K, V]) Invalidate(key K) {
	if c == nil {
		return
	}
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
	if elem, ok := c.items[key]; ok {
		c.evictList.Remove(elem)
		delete(c.items, key)
		slog.Debug("invalidated cache entry", "key", key)
	}
	c.mu.Unlock()
	if hadInflight {
		c.sf.Forget(skey)
	}
}

// InvalidateAll removes all entries from the cache. Use when a single entity affects all keys (e.g. deal with no endpoint).
func (c *LRUCache[K, V]) InvalidateAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	keys := make([]string, 0, len(c.inflight))
	for k := range c.inflight {
		keys = append(keys, k)
	}
	clear(c.inflight)
	c.items = make(map[K]*list.Element)
	c.evictList = list.New()
	slog.Debug("invalidated entire cache")
	c.mu.Unlock()
	for _, k := range keys {
		c.sf.Forget(k)
	}
}

// GetOrLoad reads from the cache first; on miss it coalesces concurrent loads
// for the same key via singleflight, stores the result, and returns it.
func (c *LRUCache[K, V]) GetOrLoad(key K, load func() (V, error)) (V, error) {
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
		if elem, ok := c.items[key]; ok {
			c.evictList.MoveToFront(elem)
			v := elem.Value.(*entryLru[K, V]).value
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
		if removeInflight(c.inflight, skey, tok) {
			c.setLocked(key, v)
		}
		c.mu.Unlock()

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

func addInflight(inflight *map[string]map[*byte]struct{}, skey string, tok *byte) {
	if *inflight == nil {
		*inflight = make(map[string]map[*byte]struct{})
	}
	set := (*inflight)[skey]
	if set == nil {
		set = make(map[*byte]struct{}, 1)
		(*inflight)[skey] = set
	}
	set[tok] = struct{}{}
}

func removeInflight(inflight map[string]map[*byte]struct{}, skey string, tok *byte) bool {
	set, ok := inflight[skey]
	if !ok {
		return false
	}
	if _, exists := set[tok]; !exists {
		return false
	}
	delete(set, tok)
	if len(set) == 0 {
		delete(inflight, skey)
	}
	return true
}

func sfKey[K comparable](key K) string {
	var zero K
	switch any(zero).(type) {
	case string:
		return any(key).(string)
	case int:
		return strconv.Itoa(any(key).(int))
	case int64:
		return strconv.FormatInt(any(key).(int64), 10)
	case uint64:
		return strconv.FormatUint(any(key).(uint64), 10)
	case int32:
		return strconv.FormatInt(int64(any(key).(int32)), 10)
	case uint32:
		return strconv.FormatUint(uint64(any(key).(uint32)), 10)
	}

	rv := reflect.ValueOf(key)
	if !rv.IsValid() {
		return "nil"
	}
	switch rv.Kind() {
	case reflect.String:
		return rv.Type().String() + ":" + rv.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Type().String() + ":" + strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Type().String() + ":" + strconv.FormatUint(rv.Uint(), 10)
	case reflect.Pointer, reflect.Chan, reflect.UnsafePointer:
		return fmt.Sprintf("%s:0x%x", rv.Type(), rv.Pointer())
	default:
		return fmt.Sprintf("%T:%#v", key, key)
	}
}
