package authentik

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
	api "goauthentik.io/api/v3"
)

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "UUID in path",
			input:    "/api/v3/core/applications/550e8400-e29b-41d4-a716-446655440000/",
			expected: "/api/v3/core/applications/:id/",
		},
		{
			name:     "numeric ID in path",
			input:    "/api/v3/providers/oauth2/42/",
			expected: "/api/v3/providers/oauth2/:id/",
		},
		{
			name:     "no IDs",
			input:    "/api/v3/core/brands/current/",
			expected: "/api/v3/core/brands/current/",
		},
		{
			name:     "multiple IDs",
			input:    "/api/v3/providers/oauth2/123/used_by/",
			expected: "/api/v3/providers/oauth2/:id/used_by/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeEndpoint(tc.input)
			if got != tc.expected {
				t.Errorf("normalizeEndpoint(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestInstrumentedTransport_RecordsMetrics(t *testing.T) {
	// Reset metrics before test
	apiRequestDuration.Reset()
	apiErrorsTotal.Reset()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		brand := api.CurrentBrand{
			MatchedDomain: "example.com",
			BrandingTitle: "Test",
			UiTheme:       api.UITHEMEENUM_AUTOMATIC,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(brand)
	}))
	defer server.Close()

	cfg := api.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = server.Listener.Addr().String()
	cfg.AddDefaultHeader("Authorization", "Bearer test-token")
	cfg.HTTPClient = &http.Client{
		Transport: &instrumentedTransport{next: http.DefaultTransport},
	}

	client := &APIClient{
		api:     api.NewAPIClient(cfg),
		baseURL: server.URL,
	}

	err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify duration histogram was recorded
	ch := make(chan prometheus.Metric, 10)
	apiRequestDuration.Collect(ch)
	close(ch)

	found := false
	for m := range ch {
		var metric io_prometheus_client.Metric
		if err := m.Write(&metric); err != nil {
			t.Fatalf("failed to write metric: %v", err)
		}
		if metric.Histogram != nil && metric.Histogram.GetSampleCount() > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected duration histogram to have observations")
	}
}

func TestInstrumentedTransport_RecordsErrors(t *testing.T) {
	apiRequestDuration.Reset()
	apiErrorsTotal.Reset()

	// Use a server that immediately closes connections
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("server does not support hijacking")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer server.Close()

	cfg := api.NewConfiguration()
	cfg.Scheme = "http"
	cfg.Host = server.Listener.Addr().String()
	cfg.AddDefaultHeader("Authorization", "Bearer test-token")
	cfg.HTTPClient = &http.Client{
		Transport: &instrumentedTransport{next: http.DefaultTransport},
	}

	client := &APIClient{
		api:     api.NewAPIClient(cfg),
		baseURL: server.URL,
	}

	_ = client.HealthCheck(context.Background())

	// Verify error counter was incremented
	ch := make(chan prometheus.Metric, 10)
	apiErrorsTotal.Collect(ch)
	close(ch)

	found := false
	for m := range ch {
		var metric io_prometheus_client.Metric
		if err := m.Write(&metric); err != nil {
			t.Fatalf("failed to write metric: %v", err)
		}
		if metric.Counter != nil && metric.Counter.GetValue() > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected error counter to be incremented")
	}
}
