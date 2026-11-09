package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/prometheus/client_golang/prometheus"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
)

const (
	// GroupFinalizerName is the finalizer name for AuthentikGroup resources
	GroupFinalizerName = "goauthentik.io/group-finalizer"
)

// AuthentikGroupReconciler reconciles AuthentikGroup objects
type AuthentikGroupReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	Recorder           events.EventRecorder
	AuthentikURL       string
	AuthentikToken     string
	NewAuthentikClient NewAuthentikClientFunc
	RequeueDelay       time.Duration

	// EventChannel receives external events (e.g. from the Authentik webhook
	// receiver) that should trigger reconciliation.
	EventChannel <-chan event.GenericEvent

	errors     *errorTracker
	errorsOnce sync.Once
}

// ensureErrorTracker lazily initializes the error tracker.
func (r *AuthentikGroupReconciler) ensureErrorTracker() {
	r.errorsOnce.Do(func() {
		r.errors = newErrorTracker()
	})
}

// +kubebuilder:rbac:groups=goauthentik.io,resources=authentikgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=goauthentik.io,resources=authentikgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=goauthentik.io,resources=authentikgroups/finalizers,verbs=update

// Reconcile reconciles an AuthentikGroup resource.
func (r *AuthentikGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	logger := log.FromContext(ctx)
	timer := prometheus.NewTimer(reconcileDuration.WithLabelValues("AuthentikGroup"))
	defer func() {
		timer.ObserveDuration()
		res := "success"
		if retErr != nil {
			res = "error"
		}
		reconcileTotal.WithLabelValues("AuthentikGroup", res).Inc()
	}()
	r.ensureErrorTracker()
	key := req.NamespacedName.String() //nolint:staticcheck // QF1008 suggestion doesn't apply to ctrl.Request

	group := &authentikv1alpha1.AuthentikGroup{}
	if err := r.Get(ctx, req.NamespacedName, group); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch AuthentikGroup")
		return ctrl.Result{}, err
	}

	akClient, err := r.NewAuthentikClient(r.AuthentikURL, r.AuthentikToken)
	if err != nil {
		logger.Error(err, "failed to create Authentik client")
		r.Recorder.Eventf(group, nil, corev1.EventTypeWarning, "AuthentikError", "Reconcile", "Failed to create Authentik client: %v", err)
		if condErr := r.setCondition(ctx, group, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to create Authentik client: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, fmt.Errorf("failed to create Authentik client: %w", err)
	}

	if !group.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, group, akClient)
	}

	if !controllerutil.ContainsFinalizer(group, GroupFinalizerName) {
		controllerutil.AddFinalizer(group, GroupFinalizerName)
		if err := r.Update(ctx, group); err != nil {
			logger.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	opts, err := groupOptionsFromSpec(group)
	if err != nil {
		logger.Error(err, "invalid group spec")
		r.Recorder.Eventf(group, nil, corev1.EventTypeWarning, "ValidationFailed", "Reconcile", "Invalid group spec: %v", err)
		if condErr := r.setCondition(ctx, group, metav1.ConditionFalse,
			authentikv1alpha1.ReasonValidationFailed, fmt.Sprintf("Invalid group spec: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		// User error — don't requeue until CR is updated
		return ctrl.Result{}, nil
	}

	info, err := r.reconcileGroup(ctx, group, akClient, opts)
	if err != nil {
		logger.Error(err, "failed to reconcile group")
		r.Recorder.Eventf(group, nil, corev1.EventTypeWarning, "GroupError", "Reconcile", "Failed to reconcile group: %v", err)
		if condErr := r.setCondition(ctx, group, metav1.ConditionFalse,
			authentikv1alpha1.ReasonAuthentikError, fmt.Sprintf("Failed to reconcile group: %v", err)); condErr != nil {
			logger.Error(condErr, "failed to update status condition")
		}
		return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, fmt.Errorf("failed to reconcile group: %w", err)
	}

	generationChanged := group.Status.ObservedGeneration != group.Generation

	group.Status.GroupUID = info.UID
	group.Status.ObservedGeneration = group.Generation

	if err := r.setCondition(ctx, group, metav1.ConditionTrue,
		authentikv1alpha1.ReasonSucceeded, "Group synced to Authentik"); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	if generationChanged {
		r.Recorder.Eventf(group, nil, corev1.EventTypeNormal, "Synced", "Reconcile",
			"Group synced to Authentik (uid=%s)", info.UID)
	}

	r.errors.recordSuccess(key)

	logger.Info("successfully reconciled AuthentikGroup", "groupUID", info.UID)

	return ctrl.Result{RequeueAfter: r.RequeueDelay}, nil
}

// reconcileGroup finds the group by status UID, then by name, and updates it.
// Looking up by UID first means a spec.name change renames the same group.
func (r *AuthentikGroupReconciler) reconcileGroup(ctx context.Context, group *authentikv1alpha1.AuthentikGroup, akClient authentik.Client, opts *authentik.GroupOptions) (*authentik.GroupInfo, error) {
	logger := log.FromContext(ctx)

	existing, err := r.findGroup(ctx, group, akClient)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		logger.Info("updating existing group", "name", opts.Name, "uid", existing.UID)
		return akClient.UpdateGroup(ctx, existing.UID, opts)
	}

	logger.Info("creating new group", "name", opts.Name)
	return akClient.CreateGroup(ctx, opts)
}

func (r *AuthentikGroupReconciler) findGroup(ctx context.Context, group *authentikv1alpha1.AuthentikGroup, akClient authentik.Client) (*authentik.GroupInfo, error) {
	if group.Status.GroupUID != "" {
		existing, err := akClient.GetGroupByUID(ctx, group.Status.GroupUID)
		if err != nil {
			return nil, fmt.Errorf("failed to check for existing group: %w", err)
		}
		if existing != nil {
			return existing, nil
		}
	}

	existing, err := akClient.GetGroupByName(ctx, group.GetGroupName())
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing group: %w", err)
	}
	return existing, nil
}

func (r *AuthentikGroupReconciler) handleDeletion(ctx context.Context, group *authentikv1alpha1.AuthentikGroup, akClient authentik.Client) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	key := client.ObjectKeyFromObject(group).String()

	if !controllerutil.ContainsFinalizer(group, GroupFinalizerName) {
		return ctrl.Result{}, nil
	}

	logger.Info("handling deletion of AuthentikGroup")

	if group.Spec.DeletionPolicy == authentikv1alpha1.DeletionPolicyRetain {
		logger.Info("deletion policy is Retain, keeping group in Authentik", "uid", group.Status.GroupUID)
		r.Recorder.Eventf(group, nil, corev1.EventTypeNormal, "Retained", "Delete", "Authentik group retained (deletionPolicy=Retain)")
	} else if group.Status.GroupUID != "" {
		existing, err := akClient.GetGroupByUID(ctx, group.Status.GroupUID)
		if err != nil {
			return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, fmt.Errorf("failed to check if group exists: %w", err)
		}
		if existing != nil {
			if err := akClient.DeleteGroup(ctx, group.Status.GroupUID); err != nil {
				return ctrl.Result{RequeueAfter: r.errors.recordError(key, r.RequeueDelay)}, fmt.Errorf("failed to delete group from Authentik: %w", err)
			}
			logger.Info("deleted group from Authentik", "uid", group.Status.GroupUID)
		}
		r.Recorder.Eventf(group, nil, corev1.EventTypeNormal, "Deleted", "Delete", "Authentik group cleaned up")
	}

	controllerutil.RemoveFinalizer(group, GroupFinalizerName)
	if err := r.Update(ctx, group); err != nil {
		logger.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// groupOptionsFromSpec converts the CR spec into Authentik client options.
func groupOptionsFromSpec(group *authentikv1alpha1.AuthentikGroup) (*authentik.GroupOptions, error) {
	opts := &authentik.GroupOptions{
		Name:        group.GetGroupName(),
		IsSuperuser: group.Spec.IsSuperuser != nil && *group.Spec.IsSuperuser,
		Parents:     group.Spec.Parents,
		Roles:       group.Spec.Roles,
	}

	if group.Spec.Attributes != nil && len(group.Spec.Attributes.Raw) > 0 {
		if err := json.Unmarshal(group.Spec.Attributes.Raw, &opts.Attributes); err != nil {
			return nil, fmt.Errorf("attributes must be a JSON object: %w", err)
		}
	}

	return opts, nil
}

func (r *AuthentikGroupReconciler) setCondition(ctx context.Context, group *authentikv1alpha1.AuthentikGroup, status metav1.ConditionStatus, reason, message string) error {
	condition := metav1.Condition{
		Type:               authentikv1alpha1.ConditionTypeReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: group.Generation,
		LastTransitionTime: metav1.Now(),
	}
	meta.SetStatusCondition(&group.Status.Conditions, condition)

	if err := r.Status().Update(ctx, group); err != nil {
		return fmt.Errorf("failed to update status condition: %w", err)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AuthentikGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := ctrl.NewControllerManagedBy(mgr).
		For(&authentikv1alpha1.AuthentikGroup{}).
		WithOptions(ctrlcontroller.Options{MaxConcurrentReconciles: 2})

	if r.EventChannel != nil {
		b = b.WatchesRawSource(source.Channel(r.EventChannel, &handler.EnqueueRequestForObject{}))
	}

	return b.Complete(r)
}
