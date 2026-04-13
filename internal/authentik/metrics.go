package authentik

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	apiRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "authentik_api_request_duration_seconds",
			Help:    "Duration of Authentik API requests in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint", "status_code"},
	)

	apiErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "authentik_api_errors_total",
			Help: "Total number of Authentik API request errors (transport-level failures).",
		},
		[]string{"method", "endpoint"},
	)

	// uuidPattern matches UUIDs in URL paths
	uuidPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

	// numericIDPattern matches purely numeric path segments (e.g., provider IDs)
	numericIDPattern = regexp.MustCompile(`/(\d+)(/|$)`)
)

func init() {
	metrics.Registry.MustRegister(apiRequestDuration, apiErrorsTotal)
}

// normalizeEndpoint strips UUIDs and numeric IDs from a URL path to prevent
// high-cardinality metric labels.
func normalizeEndpoint(path string) string {
	path = uuidPattern.ReplaceAllString(path, ":id")
	path = numericIDPattern.ReplaceAllString(path, "/:id$2")
	return path
}

// instrumentedTransport is an http.RoundTripper that records Prometheus metrics
// for every HTTP request.
type instrumentedTransport struct {
	next http.RoundTripper
}

func (t *instrumentedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	endpoint := normalizeEndpoint(req.URL.Path)
	method := req.Method

	start := time.Now()
	resp, err := t.next.RoundTrip(req)
	duration := time.Since(start).Seconds()

	if err != nil {
		apiErrorsTotal.WithLabelValues(method, endpoint).Inc()
		apiRequestDuration.WithLabelValues(method, endpoint, "0").Observe(duration)
		return nil, err
	}

	apiRequestDuration.WithLabelValues(method, endpoint, strconv.Itoa(resp.StatusCode)).Observe(duration)
	return resp, nil
}
