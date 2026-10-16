package eventbuslocal

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mosimosi228/kit/eventbus"
)

func TestSubmitRunsWorkOnce(t *testing.T) {
	b := Open(Options{Workers: 2})
	t.Cleanup(func() { _ = b.Close() })

	var calls atomic.Int32
	b.RegisterWork("user", func(_ context.Context, msg eventbus.Message) (any, error) {
		calls.Add(1)
		return msg.EntityID, nil
	})

	got, err := b.Submit(t.Context(), eventbus.Message{Entity: "user", EntityID: "42"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "42" {
		t.Fatalf("result = %#v", got)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestFanoutReachesEverySubscriber(t *testing.T) {
	b := Open(Options{Workers: 1})
	t.Cleanup(func() { _ = b.Close() })

	var work, a, c atomic.Int32
	b.RegisterWork("user", func(_ context.Context, msg eventbus.Message) (any, error) {
		work.Add(1)
		b.Fanout(msg)
		return "ok", nil
	})
	b.RegisterFanout("user", func(eventbus.Message) { a.Add(1) })
	b.RegisterFanout("user", func(eventbus.Message) { c.Add(1) })

	if _, err := b.Submit(t.Context(), eventbus.Message{Entity: "user", EntityID: "1"}); err != nil {
		t.Fatal(err)
	}
	if work.Load() != 1 || a.Load() != 1 || c.Load() != 1 {
		t.Fatalf("work=%d a=%d c=%d", work.Load(), a.Load(), c.Load())
	}
}

func TestWorkPanicReturnsError(t *testing.T) {
	b := Open(Options{Workers: 1})
	t.Cleanup(func() { _ = b.Close() })

	b.RegisterWork("user", func(context.Context, eventbus.Message) (any, error) {
		panic("boom")
	})
	_, err := b.Submit(t.Context(), eventbus.Message{Entity: "user"})
	if err == nil {
		t.Fatal("expected panic error")
	}
	_, err = b.Submit(t.Context(), eventbus.Message{Entity: "user", EntityID: "2"})
	if err == nil {
		t.Fatal("bus died after panic")
	}
}

func TestFanoutPanicDoesNotStopOthers(t *testing.T) {
	b := Open(Options{Workers: 1})
	defer b.Close()

	var second atomic.Int32
	b.RegisterFanout("user", func(eventbus.Message) { panic("fan") })
	b.RegisterFanout("user", func(eventbus.Message) { second.Add(1) })
	b.Fanout(eventbus.Message{Entity: "user"})
	if second.Load() != 1 {
		t.Fatalf("second = %d", second.Load())
	}
}

func TestSubmitRequiresHandler(t *testing.T) {
	b := Open(Options{Workers: 1})
	t.Cleanup(func() { _ = b.Close() })

	_, err := b.Submit(t.Context(), eventbus.Message{})
	if err == nil {
		t.Fatal("expected entity error")
	}
	_, err = b.Submit(t.Context(), eventbus.Message{Entity: "missing"})
	if err == nil {
		t.Fatal("expected handler error")
	}
}

func TestCloseRejectsNewWorkAndDoesNotHang(t *testing.T) {
	b := Open(Options{Workers: 1})

	started := make(chan struct{})
	release := make(chan struct{})
	b.RegisterWork("user", func(context.Context, eventbus.Message) (any, error) {
		close(started)
		<-release
		return "done", nil
	})

	go func() {
		_, _ = b.Submit(context.Background(), eventbus.Message{Entity: "user", EntityID: "1"})
	}()
	<-started

	done := make(chan error, 1)
	go func() { done <- b.Close() }()

	select {
	case err := <-done:
		t.Fatalf("Close returned before the taken job finished: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close hung")
	}

	_, err := b.Submit(context.Background(), eventbus.Message{Entity: "user", EntityID: "2"})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestQueuedWorkIsNotRunAfterClose(t *testing.T) {
	b := Open(Options{Workers: 1})

	started := make(chan struct{})
	release := make(chan struct{})
	var second atomic.Int32
	b.RegisterWork("user", func(_ context.Context, msg eventbus.Message) (any, error) {
		if msg.EntityID == "1" {
			close(started)
			<-release
			return "1", nil
		}
		second.Add(1)
		return "2", nil
	})

	go func() {
		_, _ = b.Submit(context.Background(), eventbus.Message{Entity: "user", EntityID: "1"})
	}()
	<-started

	secondDone := make(chan error, 1)
	go func() {
		_, err := b.Submit(context.Background(), eventbus.Message{Entity: "user", EntityID: "2"})
		secondDone <- err
	}()
	time.Sleep(20 * time.Millisecond)

	closeDone := make(chan struct{})
	go func() {
		_ = b.Close()
		close(closeDone)
	}()
	time.Sleep(20 * time.Millisecond)
	close(release)

	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close hung")
	}
	select {
	case err := <-secondDone:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("second submit err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second submit hung")
	}
	if second.Load() != 0 {
		t.Fatalf("second handler ran %d times", second.Load())
	}
}
