package eventbus

import (
	"encoding/json"
	"testing"
)

func TestOpenRequiresURLAndConsumer(t *testing.T) {
	if _, err := Open(t.Context(), Options{Consumer: "c"}); err == nil {
		t.Fatal("expected url error")
	}
	if _, err := Open(t.Context(), Options{URL: "nats://127.0.0.1:4222"}); err == nil {
		t.Fatal("expected consumer error")
	}
}

func TestPublishSubject(t *testing.T) {
	if got := publishSubject("eventbus.>", "user"); got != "eventbus.user" {
		t.Fatalf("got %q", got)
	}
	if got := publishSubject("bus.*", "deal"); got != "bus.deal" {
		t.Fatalf("got %q", got)
	}
	if got := publishSubject("fixed", "x"); got != "fixed" {
		t.Fatalf("got %q", got)
	}
}

func TestMessageJSON(t *testing.T) {
	raw, err := json.Marshal(Message{Entity: "user", EntityID: "1", Meta: map[string]any{"op": "update"}})
	if err != nil {
		t.Fatal(err)
	}
	var got Message
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Entity != "user" {
		t.Fatalf("entity = %q", got.Entity)
	}
}

func TestPublishRemoteSkipsLocalDispatch(t *testing.T) {
	b := &Bus{handlers: map[string]func(Message){}, subject: "eventbus.>"}
	called := false
	b.RegisterHandler("source_ingest", func(Message) { called = true })
	err := b.PublishRemote(t.Context(), Message{Entity: "source_ingest", EntityID: "1"})
	if err == nil {
		t.Fatal("expected jetstream error")
	}
	if called {
		t.Fatal("PublishRemote must not dispatch locally")
	}
}

func TestLocalDispatch(t *testing.T) {
	b := &Bus{handlers: map[string]func(Message){}}
	var saw string
	b.RegisterHandler("user", func(m Message) { saw = m.Entity })
	if err := b.dispatch(Message{Entity: "user"}); err != nil {
		t.Fatal(err)
	}
	if saw != "user" {
		t.Fatalf("saw %q", saw)
	}
	if err := b.dispatch(Message{Entity: "missing"}); err != nil {
		t.Fatal(err)
	}
}
