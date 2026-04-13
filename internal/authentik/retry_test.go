package authentik

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryTransport_SuccessNoRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := newRetryTransport(http.DefaultTransport)
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 call, got %d", got)
	}
}

func TestRetryTransport_RetriesOnServerError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := &retryTransport{
		next:       http.DefaultTransport,
		maxRetries: 3,
		baseDelay:  10 * time.Millisecond, // fast for tests
	}

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", got)
	}
}

func TestRetryTransport_NoRetryOn4xx(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	transport := &retryTransport{
		next:       http.DefaultTransport,
		maxRetries: 3,
		baseDelay:  10 * time.Millisecond,
	}

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 call (no retry on 400), got %d", got)
	}
}

func TestRetryTransport_ExhaustsRetries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	transport := &retryTransport{
		next:       http.DefaultTransport,
		maxRetries: 3,
		baseDelay:  10 * time.Millisecond,
	}

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", resp.StatusCode)
	}
	// 1 initial + 3 retries = 4
	if got := calls.Load(); got != 4 {
		t.Errorf("expected 4 calls (1 + 3 retries), got %d", got)
	}
}

func TestRetryTransport_RespectsContextCancellation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	transport := &retryTransport{
		next:       http.DefaultTransport,
		maxRetries: 3,
		baseDelay:  1 * time.Second, // long delay so cancellation fires first
	}

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)

	// Cancel after the first attempt completes
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := transport.RoundTrip(req)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 call before cancellation, got %d", got)
	}
}

func TestRetryTransport_RetriesOn429(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := &retryTransport{
		next:       http.DefaultTransport,
		maxRetries: 3,
		baseDelay:  10 * time.Millisecond,
	}

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("expected 2 calls, got %d", got)
	}
}
