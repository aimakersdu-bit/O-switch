package proxy

import (
	"net/http"
	"testing"

	"baixin-switch/internal/config"
)

func TestNewUpstreamClientUsesHighConcurrencyTransport(t *testing.T) {
	client := newUpstreamClient(config.Config{
		RequestTimeout:        0,
		MaxConcurrentRequests: 1000,
		MaxConcurrentStreams:  1000,
	}.WithDefaults())

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if transport.MaxIdleConns < 1000 {
		t.Fatalf("expected MaxIdleConns >= 1000, got %d", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost < 1000 {
		t.Fatalf("expected MaxIdleConnsPerHost >= 1000, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost < 1000 {
		t.Fatalf("expected MaxConnsPerHost >= 1000, got %d", transport.MaxConnsPerHost)
	}
}
