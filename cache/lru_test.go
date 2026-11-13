package cache

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNewLRUUsesDefaultCapacity(t *testing.T) {
	c := NewLRU[string, int](0)
	if c.cap != 1000 {
		t.Fatalf("default capacity = %d, want 1000", c.cap)
	}
}

func TestLRUGetSetAndEviction(t *testing.T) {
	c := NewLRU[string, int](2)

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	if _, ok := c.Get("a"); ok {
		t.Fatal("expected key a to be evicted")
	}

	if val, ok := c.Get("b"); !ok || val != 2 {
		t.Fatalf("Get(b) = (%d, %v), want (2, true)", val, ok)
	}
	if val, ok := c.Get("c"); !ok || val != 3 {
		t.Fatalf("Get(c) = (%d, %v), want (3, true)", val, ok)
	}
}

func TestLRUSetUpdatesExistingKey(t *testing.T) {
	c := NewLRU[string, int](2)
	c.Set("a", 1)
	c.Set("a", 10)

	val, ok := c.Get("a")
	if !ok || val != 10 {
		t.Fatalf("Get(a) = (%d, %v), want (10, true)", val, ok)
	}
}

func TestLRUInvalidate(t *testing.T) {
	c := NewLRU[string, int](2)
	c.Set("a", 1)
	c.Invalidate("a")

	if _, ok := c.Get("a"); ok {
		t.Fatal("expected key a to be invalidated")
	}
}

func TestLRUInvalidateAll(t *testing.T) {
	c := NewLRU[string, int](2)
	c.Set("a", 1)
	c.Set("b", 2)
	c.InvalidateAll()

	if _, ok := c.Get("a"); ok {
		t.Fatal("expected cache to be empty after InvalidateAll")
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("expected cache to be empty after InvalidateAll")
	}
}

func TestLRUNilReceiver(t *testing.T) {
	var c *LRUCache[string, int]

	if _, ok := c.Get("a"); ok {
		t.Fatal("Get on nil cache should miss")
	}

	c.Set("a", 1)
	c.Invalidate("a")
	c.InvalidateAll()
}

func TestLRUGetOrLoad(t *testing.T) {
	c := NewLRU[string, int](10)

	calls := 0
	load := func() (int, error) {
		calls++
		return 5, nil
	}

	v, err := c.GetOrLoad("k", load)
	if err != nil || v != 5 || calls != 1 {
		t.Fatalf("first GetOrLoad = (%d, %v) calls=%d", v, err, calls)
	}

	v, err = c.GetOrLoad("k", load)
	if err != nil || v != 5 || calls != 1 {
		t.Fatalf("second GetOrLoad = (%d, %v) calls=%d, want load once", v, err, calls)
	}
}

func TestLRUGetOrLoadSingleflight(t *testing.T) {
	c := NewLRU[string, int](10)

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
			v, err := c.GetOrLoad("same-key", func() (int, error) {
				calls.Add(1)
				once.Do(func() { close(ready) })
				<-release
				return 42, nil
			})
			if err != nil || v != 42 {
				t.Errorf("GetOrLoad = (%d, %v), want (42, nil)", v, err)
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

func TestLRUGetOrLoadErrorNotCached(t *testing.T) {
	c := NewLRU[int, string](10)
	wantErr := errors.New("db down")

	if _, err := c.GetOrLoad(1, func() (string, error) {
		return "", wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("got err %v, want %v", err, wantErr)
	}

	v, err := c.GetOrLoad(1, func() (string, error) {
		return "ok", nil
	})
	if err != nil || v != "ok" {
		t.Fatalf("retry GetOrLoad = (%q, %v), want (\"ok\", nil)", v, err)
	}
}

func TestLRUGetOrLoadInvalidateDuringLoad(t *testing.T) {
	c := NewLRU[string, int](10)

	started := make(chan struct{})
	finishStale := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		v, err := c.GetOrLoad("k", func() (int, error) {
			close(started)
			<-finishStale
			return 1, nil // stale value
		})
		if err != nil || v != 1 {
			t.Errorf("first caller got (%d, %v), want (1, nil)", v, err)
		}
	}()

	<-started
	c.Invalidate("k")

	// Fresh caller after Invalidate must not join the stale flight.
	v, err := c.GetOrLoad("k", func() (int, error) {
		return 2, nil
	})
	if err != nil || v != 2 {
		t.Fatalf("fresh GetOrLoad = (%d, %v), want (2, nil)", v, err)
	}

	// Let slow stale loader finish; it must not overwrite 2 with 1.
	close(finishStale)
	wg.Wait()

	if cached, ok := c.Get("k"); !ok || cached != 2 {
		t.Fatalf("Get(k) after stale load finished = (%d, %v), want (2, true)", cached, ok)
	}
}
