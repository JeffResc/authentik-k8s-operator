package authentik

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimitedTransport_AllowsBurst(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Allow burst of 5 — all 5 requests should complete immediately
	transport := newRateLimitedTransport(http.DefaultTransport, 100, 5)

	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		resp.Body.Close()
	}

	if got := calls.Load(); got != 5 {
		t.Errorf("expected 5 calls, got %d", got)
	}
}

func TestRateLimitedTransport_RespectsContextCancellation(t *testing.T) {
	// Very low rate so the limiter will block
	transport := newRateLimitedTransport(http.DefaultTransport, 0.001, 1)

	// Consume the single burst token
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("first request: unexpected error: %v", err)
	}
	resp.Body.Close()

	// Next request should block; cancel the context quickly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	_, err = transport.RoundTrip(req)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestRateLimitedTransport_ThrottlesRequests(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 10 requests/sec, burst 1 — after the first, each request must wait ~100ms
	transport := newRateLimitedTransport(http.DefaultTransport, 10, 1)

	start := time.Now()
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		resp.Body.Close()
	}
	elapsed := time.Since(start)

	if got := calls.Load(); got != 3 {
		t.Errorf("expected 3 calls, got %d", got)
	}

	// 3 requests at 10/s with burst 1: first is immediate, 2nd and 3rd wait ~100ms each
	// Total should be >= 150ms (being conservative)
	if elapsed < 150*time.Millisecond {
		t.Errorf("expected throttling (>= 150ms), but completed in %v", elapsed)
	}
}
