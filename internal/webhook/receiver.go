// Package webhook provides an HTTP handler that receives Authentik event
// notifications and triggers reconciliation of the affected resources.
package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
)

// Receiver handles incoming Authentik event webhook POSTs and triggers
// reconciliation by sending events to the provided channel.
type Receiver struct {
	k8sClient client.Reader
	eventChan chan<- event.GenericEvent
}

// NewReceiver creates a new webhook Receiver.
func NewReceiver(k8sClient client.Reader, eventChan chan<- event.GenericEvent) *Receiver {
	return &Receiver{
		k8sClient: k8sClient,
		eventChan: eventChan,
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

	// Enqueue all AuthentikApplication resources for reconciliation.
	// We can't map an Authentik model event to a specific CR because the
	// event payload doesn't carry the CR name.
	r.enqueueAll(ctx)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (r *Receiver) enqueueAll(ctx context.Context) {
	logger := log.FromContext(ctx)

	var list authentikv1alpha1.AuthentikApplicationList
	if err := r.k8sClient.List(ctx, &list); err != nil {
		logger.Error(err, "failed to list AuthentikApplications for event webhook reconcile")
		return
	}

	for i := range list.Items {
		evt := event.GenericEvent{Object: &list.Items[i]}
		select {
		case r.eventChan <- evt:
		default:
			// Channel full — reconciliation already pending
		}
	}

	logger.Info("enqueued AuthentikApplications for reconciliation", "count", len(list.Items))
}
