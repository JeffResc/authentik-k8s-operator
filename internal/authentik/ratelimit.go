package authentik

import (
	"net/http"

	"golang.org/x/time/rate"
)

// rateLimitedTransport is an http.RoundTripper that enforces a client-side
// rate limit on outgoing requests using a token-bucket limiter.
type rateLimitedTransport struct {
	next    http.RoundTripper
	limiter *rate.Limiter
}

func newRateLimitedTransport(next http.RoundTripper, rps float64, burst int) *rateLimitedTransport {
	return &rateLimitedTransport{
		next:    next,
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
	}
}

func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(req)
}
