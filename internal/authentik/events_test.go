package authentik

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	api "goauthentik.io/api/v3"
)

// eventTestHandler handles the various Authentik API endpoints needed for
// EnsureEventWebhookConfig and CleanupEventWebhookConfig tests.
type eventTestHandler struct {
	mu         sync.Mutex
	transports []api.NotificationTransport
	rules      []api.NotificationRule
	policies   []api.EventMatcherPolicy
	bindings   []api.PolicyBinding
}

func newEventTestHandler() *eventTestHandler {
	return &eventTestHandler{}
}

func (h *eventTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path

	switch {
	// Transport endpoints
	case strings.HasPrefix(path, "/api/v3/events/transports/") && r.Method == http.MethodPut:
		var req api.NotificationTransportRequest
		json.NewDecoder(r.Body).Decode(&req)
		t := api.NotificationTransport{Pk: "transport-1", Name: req.Name}
		if req.HasWebhookUrl() {
			url := req.GetWebhookUrl()
			t.WebhookUrl = &url
		}
		h.transports = []api.NotificationTransport{t}
		json.NewEncoder(w).Encode(t)

	case path == "/api/v3/events/transports/" && r.Method == http.MethodGet:
		nameFilter := r.URL.Query().Get("name")
		var filtered []api.NotificationTransport
		for _, t := range h.transports {
			if nameFilter == "" || t.Name == nameFilter {
				filtered = append(filtered, t)
			}
		}
		resp := api.PaginatedNotificationTransportList{
			Pagination: api.Pagination{Count: float32(len(filtered)), Current: 1, TotalPages: 1},
			Results:    filtered,
		}
		json.NewEncoder(w).Encode(resp)

	case path == "/api/v3/events/transports/" && r.Method == http.MethodPost:
		var req api.NotificationTransportRequest
		json.NewDecoder(r.Body).Decode(&req)
		t := api.NotificationTransport{Pk: "transport-1", Name: req.Name}
		if req.HasWebhookUrl() {
			url := req.GetWebhookUrl()
			t.WebhookUrl = &url
		}
		h.transports = append(h.transports, t)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(t)

	case strings.HasPrefix(path, "/api/v3/events/transports/") && r.Method == http.MethodDelete:
		h.transports = nil
		w.WriteHeader(http.StatusNoContent)

	// Rule endpoints
	case strings.HasPrefix(path, "/api/v3/events/rules/") && r.Method == http.MethodPut:
		var req api.NotificationRuleRequest
		json.NewDecoder(r.Body).Decode(&req)
		rule := api.NotificationRule{Pk: "rule-1", Name: req.Name, DestinationGroupObj: api.Group{Pk: "group-1", Name: "group"}}
		h.rules = []api.NotificationRule{rule}
		json.NewEncoder(w).Encode(rule)

	case path == "/api/v3/events/rules/" && r.Method == http.MethodGet:
		nameFilter := r.URL.Query().Get("name")
		var filtered []api.NotificationRule
		for _, rule := range h.rules {
			if nameFilter == "" || rule.Name == nameFilter {
				filtered = append(filtered, rule)
			}
		}
		resp := api.PaginatedNotificationRuleList{
			Pagination: api.Pagination{Count: float32(len(filtered)), Current: 1, TotalPages: 1},
			Results:    filtered,
		}
		json.NewEncoder(w).Encode(resp)

	case path == "/api/v3/events/rules/" && r.Method == http.MethodPost:
		var req api.NotificationRuleRequest
		json.NewDecoder(r.Body).Decode(&req)
		rule := api.NotificationRule{Pk: "rule-1", Name: req.Name, DestinationGroupObj: api.Group{Pk: "group-1", Name: "group"}}
		h.rules = append(h.rules, rule)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(rule)

	case strings.HasPrefix(path, "/api/v3/events/rules/") && r.Method == http.MethodDelete:
		h.rules = nil
		w.WriteHeader(http.StatusNoContent)

	// Policy endpoints
	case path == "/api/v3/policies/event_matcher/" && r.Method == http.MethodGet:
		nameFilter := r.URL.Query().Get("name")
		var filtered []api.EventMatcherPolicy
		for _, p := range h.policies {
			if nameFilter == "" || p.Name == nameFilter {
				filtered = append(filtered, p)
			}
		}
		resp := api.PaginatedEventMatcherPolicyList{
			Pagination: api.Pagination{Count: float32(len(filtered)), Current: 1, TotalPages: 1},
			Results:    filtered,
		}
		json.NewEncoder(w).Encode(resp)

	case path == "/api/v3/policies/event_matcher/" && r.Method == http.MethodPost:
		var req api.EventMatcherPolicyRequest
		json.NewDecoder(r.Body).Decode(&req)
		p := api.EventMatcherPolicy{
			Pk:                "policy-" + req.Name,
			Name:              req.Name,
			Component:         "ak-policy-event-matcher-form",
			VerboseName:       "Event Matcher Policy",
			VerboseNamePlural: "Event Matcher Policies",
			MetaModelName:     "authentik_events.eventmatcherpolicy",
		}
		h.policies = append(h.policies, p)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)

	case strings.HasPrefix(path, "/api/v3/policies/event_matcher/") && r.Method == http.MethodDelete:
		// Remove matching policy
		pk := strings.TrimSuffix(strings.TrimPrefix(path, "/api/v3/policies/event_matcher/"), "/")
		filtered := h.policies[:0]
		for _, p := range h.policies {
			if p.Pk != pk {
				filtered = append(filtered, p)
			}
		}
		h.policies = filtered
		w.WriteHeader(http.StatusNoContent)

	// Policy binding endpoints
	case path == "/api/v3/policies/bindings/" && r.Method == http.MethodGet:
		targetFilter := r.URL.Query().Get("target")
		var filtered []api.PolicyBinding
		for _, b := range h.bindings {
			if targetFilter == "" || b.Target == targetFilter {
				filtered = append(filtered, b)
			}
		}
		resp := api.PaginatedPolicyBindingList{
			Pagination: api.Pagination{Count: float32(len(filtered)), Current: 1, TotalPages: 1},
			Results:    filtered,
		}
		json.NewEncoder(w).Encode(resp)

	case path == "/api/v3/policies/bindings/" && r.Method == http.MethodPost:
		var req api.PolicyBindingRequest
		json.NewDecoder(r.Body).Decode(&req)
		b := api.PolicyBinding{
			Pk:     "binding-" + req.GetPolicy(),
			Target: req.Target,
			Order:  req.Order,
		}
		b.Policy.Set(req.Policy.Get())
		h.bindings = append(h.bindings, b)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(b)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestEnsureEventWebhookConfig_CreatesAll(t *testing.T) {
	handler := newEventTestHandler()
	server := httptest.NewServer(handler)
	defer server.Close()

	c := newTestClient(t, server)

	err := c.EnsureEventWebhookConfig(context.Background(), "http://operator:9443/webhook", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()

	if len(handler.transports) != 1 {
		t.Errorf("expected 1 transport, got %d", len(handler.transports))
	}
	if handler.transports[0].WebhookUrl == nil || *handler.transports[0].WebhookUrl != "http://operator:9443/webhook" {
		t.Errorf("unexpected webhook URL: %v", handler.transports[0].WebhookUrl)
	}

	if len(handler.rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(handler.rules))
	}

	if len(handler.policies) != 3 {
		t.Errorf("expected 3 policies (one per action), got %d", len(handler.policies))
	}

	if len(handler.bindings) != 3 {
		t.Errorf("expected 3 bindings, got %d", len(handler.bindings))
	}
}

func TestEnsureEventWebhookConfig_UpdatesExistingTransport(t *testing.T) {
	oldURL := "http://old-url:9443/webhook"
	handler := newEventTestHandler()
	handler.transports = []api.NotificationTransport{
		{Pk: "transport-1", Name: eventTransportName, WebhookUrl: &oldURL},
	}
	handler.rules = []api.NotificationRule{
		{Pk: "rule-1", Name: eventRuleName, DestinationGroupObj: api.Group{Pk: "group-1", Name: "group"}},
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	c := newTestClient(t, server)

	err := c.EnsureEventWebhookConfig(context.Background(), "http://new-url:9443/webhook", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()

	if handler.transports[0].WebhookUrl == nil || *handler.transports[0].WebhookUrl != "http://new-url:9443/webhook" {
		t.Errorf("expected updated URL, got %v", handler.transports[0].WebhookUrl)
	}
}

func TestCleanupEventWebhookConfig(t *testing.T) {
	url := "http://operator:9443/webhook"
	handler := newEventTestHandler()
	handler.transports = []api.NotificationTransport{
		{Pk: "transport-1", Name: eventTransportName, WebhookUrl: &url},
	}
	handler.rules = []api.NotificationRule{
		{Pk: "rule-1", Name: eventRuleName, DestinationGroupObj: api.Group{Pk: "group-1", Name: "group"}},
	}
	handler.policies = []api.EventMatcherPolicy{
		{Pk: "policy-authentik-k8s-operator-model_created", Name: "authentik-k8s-operator-model_created", Component: "ak-policy-event-matcher-form", VerboseName: "Event Matcher Policy", VerboseNamePlural: "Event Matcher Policies", MetaModelName: "authentik_events.eventmatcherpolicy"},
		{Pk: "policy-authentik-k8s-operator-model_updated", Name: "authentik-k8s-operator-model_updated", Component: "ak-policy-event-matcher-form", VerboseName: "Event Matcher Policy", VerboseNamePlural: "Event Matcher Policies", MetaModelName: "authentik_events.eventmatcherpolicy"},
		{Pk: "policy-authentik-k8s-operator-model_deleted", Name: "authentik-k8s-operator-model_deleted", Component: "ak-policy-event-matcher-form", VerboseName: "Event Matcher Policy", VerboseNamePlural: "Event Matcher Policies", MetaModelName: "authentik_events.eventmatcherpolicy"},
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	c := newTestClient(t, server)

	err := c.CleanupEventWebhookConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()

	if len(handler.transports) != 0 {
		t.Errorf("expected 0 transports after cleanup, got %d", len(handler.transports))
	}
	if len(handler.rules) != 0 {
		t.Errorf("expected 0 rules after cleanup, got %d", len(handler.rules))
	}
	if len(handler.policies) != 0 {
		t.Errorf("expected 0 policies after cleanup, got %d", len(handler.policies))
	}
}
