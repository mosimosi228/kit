package blob

import (
	"os"
	"strings"
	"testing"
)

func TestOpenRequiresEndpointAndBucket(t *testing.T) {
	t.Parallel()
	if _, err := Open(t.Context(), Options{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Open(t.Context(), Options{Endpoint: "localhost:9000"}); err == nil {
		t.Fatal("expected bucket error")
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	t.Parallel()
	host, tls := normalizeEndpoint("https://s3.example.com", false)
	if host != "s3.example.com" || !tls {
		t.Fatalf("https: %s %v", host, tls)
	}
	host, tls = normalizeEndpoint("http://127.0.0.1:9000", true)
	if host != "127.0.0.1:9000" || tls {
		t.Fatalf("http loopback: %s %v", host, tls)
	}
	host, tls = normalizeEndpoint("s3.example.com", false)
	if host != "s3.example.com" || !tls {
		t.Fatalf("bare prod defaults TLS: %s %v", host, tls)
	}
	host, tls = normalizeEndpoint("127.0.0.1:9000", false)
	if host != "127.0.0.1:9000" || tls {
		t.Fatalf("bare loopback stays HTTP: %s %v", host, tls)
	}
}

func TestPublicURL(t *testing.T) {
	t.Parallel()
	c := &Client{publicBase: "https://cdn.example.com/"}
	if got := c.PublicURL("/p/x/a.mp4"); got != "https://cdn.example.com/p/x/a.mp4" {
		t.Fatalf("got %q", got)
	}
}

func TestOpenLive(t *testing.T) {
	endpoint := os.Getenv("KIT_BLOB_ENDPOINT")
	if endpoint == "" {
		t.Skip("KIT_BLOB_ENDPOINT not set")
	}
	c, err := Open(t.Context(), Options{
		Endpoint:  endpoint,
		Bucket:    os.Getenv("KIT_BLOB_BUCKET"),
		AccessKey: os.Getenv("KIT_BLOB_ACCESS_KEY"),
		SecretKey: os.Getenv("KIT_BLOB_SECRET_KEY"),
		Secure:    !strings.HasPrefix(endpoint, "http://"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
}
