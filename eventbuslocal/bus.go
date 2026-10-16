package eventbuslocal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"

	"github.com/mosimosi228/kit/eventbus"
)

// ErrClosed is returned when Submit runs after Close.
var ErrClosed = errors.New("eventbuslocal: closed")

// Options sets the in-process worker pool.
type Options struct {
	Workers int // default GOMAXPROCS
}

// Bus runs writes on one worker and fans cache events out locally.
// The project holds the instance. This package has no global.
type Bus struct {
	workers int
	queue   chan *job
	stop    chan struct{}

	mu       sync.Mutex
	closed   bool
	work     map[string]func(context.Context, eventbus.Message) (any, error)
	fanout   map[string][]func(eventbus.Message)
	wg       sync.WaitGroup
	inflight sync.WaitGroup
}

type job struct {
	ctx  context.Context
	msg  eventbus.Message
	fn   func(context.Context, eventbus.Message) (any, error)
	done chan result
}

type result struct {
	val any
	err error
}

// Open starts the pool. It does not dial NATS.
func Open(opt Options) *Bus {
	n := opt.Workers
	if n <= 0 {
		n = runtime.GOMAXPROCS(0)
	}
	b := &Bus{
		workers: n,
		queue:   make(chan *job),
		stop:    make(chan struct{}),
		work:    make(map[string]func(context.Context, eventbus.Message) (any, error)),
		fanout:  make(map[string][]func(eventbus.Message)),
	}
	for i := 0; i < n; i++ {
		b.wg.Add(1)
		go b.worker()
	}
	return b
}

// RegisterWork sets the single write handler for an entity.
func (b *Bus) RegisterWork(entity string, fn func(context.Context, eventbus.Message) (any, error)) {
	if b == nil || entity == "" || fn == nil {
		return
	}
	b.mu.Lock()
	b.work[entity] = fn
	b.mu.Unlock()
}

// RegisterFanout adds a local subscriber for an entity.
func (b *Bus) RegisterFanout(entity string, fn func(eventbus.Message)) {
	if b == nil || entity == "" || fn == nil {
		return
	}
	b.mu.Lock()
	b.fanout[entity] = append(b.fanout[entity], fn)
	b.mu.Unlock()
}

// Submit runs the work handler once and waits for its result.
func (b *Bus) Submit(ctx context.Context, msg eventbus.Message) (any, error) {
	if b == nil {
		return nil, ErrClosed
	}
	if msg.Entity == "" {
		return nil, fmt.Errorf("eventbuslocal: entity is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	b.mu.Lock()
	fn, ok := b.work[msg.Entity]
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return nil, ErrClosed
	}
	if !ok {
		return nil, fmt.Errorf("eventbuslocal: no work handler for %q", msg.Entity)
	}

	j := &job{
		ctx:  ctx,
		msg:  msg,
		fn:   fn,
		done: make(chan result, 1),
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.stop:
		return nil, ErrClosed
	case b.queue <- j:
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-j.done:
		return r.val, r.err
	}
}

// Fanout delivers msg to every local subscriber and does not run work.
func (b *Bus) Fanout(msg eventbus.Message) {
	if b == nil || msg.Entity == "" {
		return
	}
	b.mu.Lock()
	hs := append([]func(eventbus.Message){}, b.fanout[msg.Entity]...)
	b.mu.Unlock()
	for _, h := range hs {
		callFanout(h, msg)
	}
}

// Close waits for taken jobs and rejects new ones.
func (b *Bus) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	close(b.stop)
	b.mu.Unlock()

	b.inflight.Wait()
	b.wg.Wait()
	return nil
}

func (b *Bus) worker() {
	defer b.wg.Done()
	for {
		select {
		case <-b.stop:
			b.rejectQueued()
			return
		case j := <-b.queue:
			b.mu.Lock()
			closed := b.closed
			if !closed {
				b.inflight.Add(1)
			}
			b.mu.Unlock()
			if closed {
				j.done <- result{err: ErrClosed}
				continue
			}
			b.execute(j)
			b.inflight.Done()
		}
	}
}

func (b *Bus) rejectQueued() {
	for {
		select {
		case j := <-b.queue:
			j.done <- result{err: ErrClosed}
		default:
			return
		}
	}
}

func (b *Bus) execute(j *job) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("eventbuslocal work panic", slog.Any("recover", r), slog.String("entity", j.msg.Entity))
			j.done <- result{err: fmt.Errorf("eventbuslocal: panic: %v", r)}
		}
	}()
	if err := j.ctx.Err(); err != nil {
		j.done <- result{err: err}
		return
	}
	val, err := j.fn(j.ctx, j.msg)
	j.done <- result{val: val, err: err}
}

func callFanout(h func(eventbus.Message), msg eventbus.Message) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("eventbuslocal fanout panic", slog.Any("recover", r), slog.String("entity", msg.Entity))
		}
	}()
	h(msg)
}
