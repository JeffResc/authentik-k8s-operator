// Package webhook provides an HTTP handler that receives Authentik event
// notifications and triggers reconciliation of the affected resources.
package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
)

// Receiver handles incoming Authentik event webhook POSTs and triggers
// reconciliation by sending events to the provided channel.
type Receiver struct {
	k8sClient client.Reader
	channels  Channels
	secret    string
}

// Channels holds one event channel per controller. Each controller consumes
// only its own channel, so every resource kind must be sent to the channel of
// the controller that reconciles it.
type Channels struct {
	OAuth2 chan<- event.GenericEvent
	SAML   chan<- event.GenericEvent
	Group  chan<- event.GenericEvent
}

// NewReceiver creates a new webhook Receiver. If secret is non-empty,
// requests must include an Authorization: Bearer <secret> header.
func NewReceiver(k8sClient client.Reader, channels Channels, secret string) *Receiver {
	return &Receiver{
		k8sClient: k8sClient,
		channels:  channels,
		secret:    secret,
	}
}

// authentikWebhookPayload represents the subset of an Authentik notification
// webhook payload we need.
type authentikWebhookPayload struct {
	Body     string `json:"body"`
	Severity string `json:"severity,omitempty"`
}

// ServeHTTP handles the webhook POST request.
func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	logger := log.FromContext(ctx)

	if r.secret != "" {
		auth := req.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")
		if auth == "" || token == auth || subtle.ConstantTimeCompare([]byte(token), []byte(r.secret)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20)) // 1 MB limit
	if err != nil {
		logger.Error(err, "failed to read request body")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var payload authentikWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		logger.Error(err, "failed to parse event payload")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	logger.Info("received Authentik event webhook", "severity", payload.Severity)

	// Enqueue all managed resources for reconciliation.
	// We can't map an Authentik model event to a specific CR because the
	// event payload doesn't carry the CR name.
	r.enqueueAll(ctx)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (r *Receiver) enqueueAll(ctx context.Context) {
	r.enqueueOAuth2Apps(ctx)
	r.enqueueSAMLApps(ctx)
	r.enqueueGroups(ctx)
}

func (r *Receiver) enqueueOAuth2Apps(ctx context.Context) {
	logger := log.FromContext(ctx)

	var list authentikv1alpha1.AuthentikOAuth2ApplicationList
	if err := r.k8sClient.List(ctx, &list); err != nil {
		logger.Error(err, "failed to list AuthentikOAuth2Applications for event webhook reconcile")
		return
	}

	for i := range list.Items {
		r.trySend(r.channels.OAuth2, event.GenericEvent{Object: &list.Items[i]})
	}

	logger.Info("enqueued AuthentikOAuth2Applications for reconciliation", "count", len(list.Items))
}

func (r *Receiver) enqueueSAMLApps(ctx context.Context) {
	logger := log.FromContext(ctx)

	var list authentikv1alpha1.AuthentikSAMLApplicationList
	if err := r.k8sClient.List(ctx, &list); err != nil {
		logger.Error(err, "failed to list AuthentikSAMLApplications for event webhook reconcile")
		return
	}

	for i := range list.Items {
		r.trySend(r.channels.SAML, event.GenericEvent{Object: &list.Items[i]})
	}

	logger.Info("enqueued AuthentikSAMLApplications for reconciliation", "count", len(list.Items))
}

func (r *Receiver) enqueueGroups(ctx context.Context) {
	logger := log.FromContext(ctx)

	var list authentikv1alpha1.AuthentikGroupList
	if err := r.k8sClient.List(ctx, &list); err != nil {
		logger.Error(err, "failed to list AuthentikGroups for event webhook reconcile")
		return
	}

	for i := range list.Items {
		r.trySend(r.channels.Group, event.GenericEvent{Object: &list.Items[i]})
	}

	logger.Info("enqueued AuthentikGroups for reconciliation", "count", len(list.Items))
}

func (r *Receiver) trySend(ch chan<- event.GenericEvent, evt event.GenericEvent) {
	select {
	case ch <- evt:
	default:
		// Channel full — reconciliation already pending
	}
}
