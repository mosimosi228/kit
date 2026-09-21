package redis

import "testing"

func TestOpenRequiresHost(t *testing.T) {
	if _, err := Open(t.Context(), Options{}); err == nil {
		t.Fatal("expected error")
	}
}
