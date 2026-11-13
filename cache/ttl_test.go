package cache

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheSetGetDel(t *testing.T) {
	c := NewCache[string, int]()
	defer c.Close()

	if ok := c.Set("key", 42, time.Minute); !ok {
		t.Fatal("Set() returned false")
	}

	val, ok := c.Get("key")
	if !ok || val != 42 {
		t.Fatalf("Get() = (%d, %v), want (42, true)", val, ok)
	}

	c.Del("key")
	if _, ok := c.Get("key"); ok {
		t.Fatal("expected key to be deleted")
	}
}

func TestCacheTTLExpiry(t *testing.T) {
	c := NewCache[string, int]()
	defer c.Close()

	if ok := c.Set("temp", 1, 20*time.Millisecond); !ok {
		t.Fatal("Set() returned false")
	}

	time.Sleep(50 * time.Millisecond)

	if _, ok := c.Get("temp"); ok {
		t.Fatal("expected key to expire by TTL")
	}
}

func TestCacheClearAndMetrics(t *testing.T) {
	c := NewCache[string, int]()
	defer c.Close()

	c.Set("a", 1, time.Minute)
	c.Get("a")
	c.Get("missing")

	hits, misses, _ := c.Metrics()
	if hits == 0 || misses == 0 {
		t.Fatalf("Metrics() = hits=%d misses=%d, want both > 0", hits, misses)
	}

	c.Clear()
	if _, ok := c.Get("a"); ok {
		t.Fatal("expected cache to be empty after Clear()")
	}
}

func TestCacheGetOrLoad(t *testing.T) {
	c := NewCache[string, int]()
	defer c.Close()

	calls := 0
	load := func() (int, error) {
		calls++
		return 7, nil
	}

	v, err := c.GetOrLoad("k", time.Minute, load)
	if err != nil || v != 7 || calls != 1 {
		t.Fatalf("first GetOrLoad = (%d, %v) calls=%d", v, err, calls)
	}

	v, err = c.GetOrLoad("k", time.Minute, load)
	if err != nil || v != 7 || calls != 1 {
		t.Fatalf("second GetOrLoad = (%d, %v) calls=%d, want load once", v, err, calls)
	}
}

func TestCacheGetOrLoadSingleflight(t *testing.T) {
	c := NewCache[string, int]()
	defer c.Close()

	const goroutines = 50
	var calls atomic.Int32
	ready := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			v, err := c.GetOrLoad("same-key", time.Minute, func() (int, error) {
				calls.Add(1)
				once.Do(func() { close(ready) })
				<-release
				return 99, nil
			})
			if err != nil || v != 99 {
				t.Errorf("GetOrLoad = (%d, %v), want (99, nil)", v, err)
			}
		}()
	}

	<-ready
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("load called %d times, want 1", got)
	}
}

func TestCacheGetOrLoadErrorNotCached(t *testing.T) {
	c := NewCache[int, string]()
	defer c.Close()

	wantErr := errors.New("boom")
	if _, err := c.GetOrLoad(1, time.Minute, func() (string, error) {
		return "", wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("got err %v, want %v", err, wantErr)
	}

	v, err := c.GetOrLoad(1, time.Minute, func() (string, error) {
		return "fresh", nil
	})
	if err != nil || v != "fresh" {
		t.Fatalf("retry GetOrLoad = (%q, %v), want (\"fresh\", nil)", v, err)
	}
}

func TestCacheGetOrLoadDelDuringLoad(t *testing.T) {
	c := NewCache[string, int]()
	defer c.Close()

	started := make(chan struct{})
	finishStale := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		v, err := c.GetOrLoad("k", time.Minute, func() (int, error) {
			close(started)
			<-finishStale
			return 1, nil // stale value
		})
		if err != nil || v != 1 {
			t.Errorf("first caller got (%d, %v), want (1, nil)", v, err)
		}
	}()

	<-started
	c.Del("k")

	v, err := c.GetOrLoad("k", time.Minute, func() (int, error) {
		return 2, nil
	})
	if err != nil || v != 2 {
		t.Fatalf("fresh GetOrLoad = (%d, %v), want (2, nil)", v, err)
	}

	close(finishStale)
	wg.Wait()

	if cached, ok := c.Get("k"); !ok || cached != 2 {
		t.Fatalf("Get(k) after stale load finished = (%d, %v), want (2, true)", cached, ok)
	}
}
