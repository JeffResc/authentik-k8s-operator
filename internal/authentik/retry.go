package authentik

import (
	"io"
	"net/http"
	"time"
)

// retryableStatusCodes defines HTTP status codes that warrant a retry.
var retryableStatusCodes = map[int]bool{
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

// retryTransport is an http.RoundTripper that retries requests on transient
// errors using exponential backoff.
type retryTransport struct {
	next       http.RoundTripper
	maxRetries int
	baseDelay  time.Duration
}

func newRetryTransport(next http.RoundTripper) *retryTransport {
	return &retryTransport{
		next:       next,
		maxRetries: 3,
		baseDelay:  1 * time.Second,
	}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		if attempt > 0 {
			delay := t.baseDelay << (attempt - 1) // 1s, 2s, 4s
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(delay):
			}
		}

		resp, err = t.next.RoundTrip(req)
		if err != nil {
			// Transport-level error — retry
			continue
		}

		if !retryableStatusCodes[resp.StatusCode] {
			return resp, nil
		}

		// Drain and close the body before retrying to release the connection.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	// Return the last response/error after exhausting retries.
	return resp, err
}
