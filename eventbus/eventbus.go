package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	defaultStream  = "EVENTBUS"
	defaultSubject = "eventbus.>"
	defaultMaxAge  = 7 * 24 * time.Hour
)

// Message is the envelope published on the bus.
type Message struct {
	Entity   string         `json:"entity"`
	EntityID any            `json:"entity_id,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
}

// Options configures a JetStream pull bus.
type Options struct {
	URL      string
	Name     string        // nats.Name
	Stream   string        // default EVENTBUS
	Subject  string        // default eventbus.>
	Consumer string        // required, unique per node
	MaxAge   time.Duration // stream max age, default 7d
}

// Bus is a JetStream event bus. The project holds the singleton, not this package.
type Bus struct {
	conn     *nats.Conn
	js       nats.JetStreamContext
	sub      *nats.Subscription
	mu       sync.RWMutex
	handlers map[string]func(Message)
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
	subject  string
	stream   string
}

// Open connects, ensures the stream, and starts a durable pull consumer.
func Open(ctx context.Context, opt Options) (*Bus, error) {
	if opt.URL == "" {
		return nil, fmt.Errorf("eventbus: url is required")
	}
	if strings.TrimSpace(opt.Consumer) == "" {
		return nil, fmt.Errorf("eventbus: consumer is required")
	}

	stream := opt.Stream
	if stream == "" {
		stream = defaultStream
	}
	subject := opt.Subject
	if subject == "" {
		subject = defaultSubject
	}
	maxAge := opt.MaxAge
	if maxAge <= 0 {
		maxAge = defaultMaxAge
	}

	nc, err := nats.Connect(opt.URL,
		nats.Name(opt.Name),
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Warn("nats disconnected", slog.Any("err", err))
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			slog.Info("nats reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("eventbus: connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		_ = nc.Drain()
		return nil, fmt.Errorf("eventbus: jetstream: %w", err)
	}

	if _, err := js.StreamInfo(stream); err != nil {
		if err != nats.ErrStreamNotFound {
			_ = nc.Drain()
			return nil, fmt.Errorf("eventbus: stream info: %w", err)
		}
		if _, err := js.AddStream(&nats.StreamConfig{
			Name:      stream,
			Subjects:  []string{subject},
			Retention: nats.LimitsPolicy,
			MaxAge:    maxAge,
			Storage:   nats.FileStorage,
			Replicas:  1,
		}); err != nil {
			_ = nc.Drain()
			return nil, fmt.Errorf("eventbus: add stream: %w", err)
		}
		slog.Info("nats stream created", slog.String("name", stream))
	}

	runCtx, cancel := context.WithCancel(context.Background())
	b := &Bus{
		conn:     nc,
		js:       js,
		handlers: make(map[string]func(Message)),
		ctx:      runCtx,
		cancel:   cancel,
		subject:  subject,
		stream:   stream,
	}

	sub, err := js.PullSubscribe(subject, opt.Consumer,
		nats.BindStream(stream),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(5),
		nats.DeliverNew(),
	)
	if err != nil {
		cancel()
		_ = nc.Drain()
		return nil, fmt.Errorf("eventbus: subscribe: %w", err)
	}
	b.sub = sub

	b.wg.Add(1)
	go b.worker()

	_ = ctx
	slog.Info("nats eventbus ready",
		slog.String("consumer", opt.Consumer),
		slog.String("stream", stream),
	)
	return b, nil
}

func (b *Bus) worker() {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			return
		default:
			ctx, cancel := context.WithTimeout(b.ctx, 3*time.Second)
			msgs, err := b.sub.Fetch(100, nats.Context(ctx))
			cancel()
			if err != nil {
				if err != nats.ErrTimeout && err != context.Canceled && err.Error() != "context deadline exceeded" {
					slog.Error("eventbus fetch failed", slog.Any("err", err))
				}
				continue
			}
			for _, msg := range msgs {
				go b.processMessage(msg)
			}
		}
	}
}

func (b *Bus) processMessage(m *nats.Msg) {
	var payload Message
	if err := json.Unmarshal(m.Data, &payload); err != nil {
		slog.Warn("eventbus invalid payload", slog.Any("err", err))
		_ = m.Nak()
		return
	}
	if err := b.dispatch(payload); err != nil {
		_ = m.Nak()
		return
	}
	_ = m.Ack()
}

func (b *Bus) dispatch(payload Message) (err error) {
	b.mu.RLock()
	handler, ok := b.handlers[payload.Entity]
	b.mu.RUnlock()
	if !ok {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Error("eventbus handler panic", slog.Any("recover", r), slog.String("entity", payload.Entity))
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()
	handler(payload)
	return nil
}

// Publish dispatches locally, then publishes to JetStream.
func (b *Bus) Publish(ctx context.Context, msg Message) error {
	_ = ctx
	if err := b.dispatch(msg); err != nil {
		slog.Error("eventbus local dispatch failed",
			slog.String("entity", msg.Entity),
			slog.Any("entity_id", msg.EntityID),
			slog.Any("err", err),
		)
	}
	return b.publishJS(msg)
}

// PublishRemote publishes to JetStream only. It does not run local handlers.
func (b *Bus) PublishRemote(ctx context.Context, msg Message) error {
	_ = ctx
	return b.publishJS(msg)
}

func (b *Bus) publishJS(msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("eventbus: marshal: %w", err)
	}
	if b == nil || b.js == nil {
		return fmt.Errorf("eventbus: jetstream is not connected")
	}
	subj := publishSubject(b.subject, msg.Entity)
	if _, err := b.js.Publish(subj, data); err != nil {
		return fmt.Errorf("eventbus: publish: %w", err)
	}
	return nil
}

// RegisterHandler sets the handler for an entity name.
func (b *Bus) RegisterHandler(entity string, handler func(Message)) {
	b.mu.Lock()
	b.handlers[entity] = handler
	b.mu.Unlock()
}

// Close stops the worker and drains the connection.
func (b *Bus) Close() error {
	if b == nil {
		return nil
	}
	b.cancel()
	b.wg.Wait()
	if b.sub != nil {
		_ = b.sub.Unsubscribe()
	}
	if b.conn != nil {
		_ = b.conn.Drain()
	}
	slog.Info("nats eventbus closed")
	return nil
}

func publishSubject(wildcard, entity string) string {
	switch {
	case strings.HasSuffix(wildcard, ".>"):
		return strings.TrimSuffix(wildcard, ".>") + "." + entity
	case strings.HasSuffix(wildcard, ".*"):
		return strings.TrimSuffix(wildcard, ".*") + "." + entity
	default:
		return wildcard
	}
}
