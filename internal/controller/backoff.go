package controller

import (
	"sync"
	"time"
)

const maxBackoffDelay = 10 * time.Minute

// errorTracker tracks consecutive error counts per resource for exponential backoff.
type errorTracker struct {
	mu     sync.Mutex
	counts map[string]int
}

func newErrorTracker() *errorTracker {
	return &errorTracker{counts: make(map[string]int)}
}

// recordError increments the error count for a resource key and returns the
// backoff delay: baseDelay * 2^(consecutiveErrors-1), capped at maxBackoffDelay.
func (t *errorTracker) recordError(key string, baseDelay time.Duration) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counts[key]++
	return backoffDelay(baseDelay, t.counts[key])
}

// recordSuccess resets the error count for a resource key.
func (t *errorTracker) recordSuccess(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.counts, key)
}

// backoffDelay computes min(baseDelay * 2^(failures-1), maxBackoffDelay).
func backoffDelay(baseDelay time.Duration, failures int) time.Duration {
	if failures <= 1 {
		return baseDelay
	}
	delay := baseDelay
	for i := 1; i < failures && delay < maxBackoffDelay; i++ {
		delay *= 2
	}
	if delay > maxBackoffDelay {
		delay = maxBackoffDelay
	}
	return delay
}
