package controller

import (
	"context"
	"fmt"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	authentikv1alpha1 "github.com/JeffResc/authentik-k8s-operator/api/v1alpha1"
	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
)

// --- Group test helpers ---

func newGroup(name, namespace string) *authentikv1alpha1.AuthentikGroup {
	return &authentikv1alpha1.AuthentikGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func newGroupReconciler(s *runtime.Scheme, group *authentikv1alpha1.AuthentikGroup, mock *mockClient) *AuthentikGroupReconciler {
	objs := []runtime.Object{}
	if group != nil {
		objs = append(objs, group)
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&authentikv1alpha1.AuthentikGroup{}).
		Build()

	return &AuthentikGroupReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Recorder:       events.NewFakeRecorder(10),
		AuthentikURL:   "http://authentik.test",
		AuthentikToken: "test-token",
		RequeueDelay:   DefaultRequeueDelay,
		NewAuthentikClient: func(string, string) (authentik.Client, error) {
			return mock, nil
		},
	}
}

func getGroup(t *testing.T, r *AuthentikGroupReconciler, name string) *authentikv1alpha1.AuthentikGroup {
	t.Helper()
	g := &authentikv1alpha1.AuthentikGroup{}
	if err := r.Get(context.Background(), reqFor(name, "default").NamespacedName, g); err != nil {
		t.Fatalf("failed to get group: %v", err)
	}
	return g
}

// --- Group Tests ---

func TestGroupReconcile_AddsFinalizer(t *testing.T) {
	s := newScheme(t)
	r := newGroupReconciler(s, newGroup("g", "default"), &mockClient{})

	result, err := r.Reconcile(context.Background(), reqFor("g", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == (ctrl.Result{}) {
		t.Error("expected requeue after adding finalizer")
	}
	if got := getGroup(t, r, "g").Finalizers; len(got) != 1 || got[0] != GroupFinalizerName {
		t.Errorf("expected finalizer %q, got %v", GroupFinalizerName, got)
	}
}

func TestGroupReconcile_CreatesGroup(t *testing.T) {
	s := newScheme(t)
	g := newGroup("g", "default")
	g.Finalizers = []string{GroupFinalizerName}
	superuser := true
	g.Spec.IsSuperuser = &superuser
	g.Spec.Parents = []string{"parent"}
	g.Spec.Roles = []string{"role"}
	g.Spec.Attributes = &apiextensionsv1.JSON{Raw: []byte(`{"team":"platform"}`)}
	mock := &mockClient{createGroupRes: &authentik.GroupInfo{UID: "uid-1", Name: "g"}}
	r := newGroupReconciler(s, g, mock)
	r.AllowPrivilegedGroupFields = true

	result, err := r.Reconcile(context.Background(), reqFor("g", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequeueAfter != DefaultRequeueDelay {
		t.Errorf("expected RequeueAfter=%v, got %v", DefaultRequeueDelay, result.RequeueAfter)
	}
	if mock.createGroupCalls != 1 || mock.updateGroupCalls != 0 {
		t.Errorf("expected 1 create and 0 updates, got %d and %d", mock.createGroupCalls, mock.updateGroupCalls)
	}

	opts := mock.lastGroupOpts
	if opts.Name != "g" || opts.Owner != "default/g" || !opts.IsSuperuser || opts.Parents[0] != "parent" || opts.Roles[0] != "role" || opts.Attributes["team"] != "platform" {
		t.Errorf("unexpected group options: %+v", opts)
	}

	updated := getGroup(t, r, "g")
	if updated.Status.GroupUID != "uid-1" {
		t.Errorf("expected GroupUID=uid-1, got %q", updated.Status.GroupUID)
	}
	if !meta.IsStatusConditionTrue(updated.Status.Conditions, authentikv1alpha1.ConditionTypeReady) {
		t.Error("expected Ready=True")
	}
}

func TestGroupReconcile_UpdatesOwnedGroupByName(t *testing.T) {
	s := newScheme(t)
	g := newGroup("g", "default")
	g.Finalizers = []string{GroupFinalizerName}
	g.Spec.Name = "Existing"
	mock := &mockClient{
		getGroupByNameRes: &authentik.GroupInfo{UID: "existing-uid", Name: "Existing", Owner: "default/g"},
		updateGroupRes:    &authentik.GroupInfo{UID: "existing-uid", Name: "Existing"},
	}
	r := newGroupReconciler(s, g, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("g", "default")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.createGroupCalls != 0 || mock.updateGroupCalls != 1 {
		t.Errorf("expected 0 creates and 1 update, got %d and %d", mock.createGroupCalls, mock.updateGroupCalls)
	}
	if mock.lastUpdatedGroupUID != "existing-uid" {
		t.Errorf("expected update of existing-uid, got %q", mock.lastUpdatedGroupUID)
	}
	if mock.lastGroupOpts.Name != "Existing" {
		t.Errorf("expected name Existing, got %q", mock.lastGroupOpts.Name)
	}
}

func TestGroupReconcile_RenameUsesStatusUID(t *testing.T) {
	s := newScheme(t)
	g := newGroup("g", "default")
	g.Finalizers = []string{GroupFinalizerName}
	g.Spec.Name = "New Name"
	g.Status.GroupUID = "uid-1"
	mock := &mockClient{
		// The new name does not exist yet, but the UID does.
		getGroupByUIDRes: &authentik.GroupInfo{UID: "uid-1", Name: "Old Name", Owner: "default/g"},
		updateGroupRes:   &authentik.GroupInfo{UID: "uid-1", Name: "New Name"},
	}
	r := newGroupReconciler(s, g, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("g", "default")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.createGroupCalls != 0 || mock.updateGroupCalls != 1 {
		t.Errorf("expected 0 creates and 1 update, got %d and %d", mock.createGroupCalls, mock.updateGroupCalls)
	}
	if mock.lastUpdatedGroupUID != "uid-1" {
		t.Errorf("expected update of uid-1, got %q", mock.lastUpdatedGroupUID)
	}
}

func TestGroupReconcile_AuthentikErrorSetsNotReady(t *testing.T) {
	s := newScheme(t)
	g := newGroup("g", "default")
	g.Finalizers = []string{GroupFinalizerName}
	g.Spec.Roles = []string{"missing"}
	mock := &mockClient{createGroupErr: fmt.Errorf(`role "missing" not found`)}
	r := newGroupReconciler(s, g, mock)
	r.AllowPrivilegedGroupFields = true

	result, err := r.Reconcile(context.Background(), reqFor("g", "default"))
	if err == nil {
		t.Fatal("expected error")
	}
	if result.RequeueAfter == 0 {
		t.Error("expected RequeueAfter on error")
	}

	cond := meta.FindStatusCondition(getGroup(t, r, "g").Status.Conditions, authentikv1alpha1.ConditionTypeReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != authentikv1alpha1.ReasonAuthentikError {
		t.Errorf("expected Ready=False with reason AuthentikError, got %+v", cond)
	}
}

func TestGroupReconcile_InvalidAttributesNoRequeue(t *testing.T) {
	s := newScheme(t)
	g := newGroup("g", "default")
	g.Finalizers = []string{GroupFinalizerName}
	g.Spec.Attributes = &apiextensionsv1.JSON{Raw: []byte(`["not","an","object"]`)}
	mock := &mockClient{}
	r := newGroupReconciler(s, g, mock)

	result, err := r.Reconcile(context.Background(), reqFor("g", "default"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != (ctrl.Result{}) {
		t.Errorf("expected no requeue, got %+v", result)
	}
	if mock.createGroupCalls != 0 {
		t.Error("expected no create call")
	}

	cond := meta.FindStatusCondition(getGroup(t, r, "g").Status.Conditions, authentikv1alpha1.ConditionTypeReady)
	if cond == nil || cond.Reason != authentikv1alpha1.ReasonValidationFailed {
		t.Errorf("expected reason ValidationFailed, got %+v", cond)
	}
}

func newDeletingGroup(policy string) *authentikv1alpha1.AuthentikGroup {
	g := newGroup("g", "default")
	g.Finalizers = []string{GroupFinalizerName}
	now := metav1.Now()
	g.DeletionTimestamp = &now
	g.Spec.DeletionPolicy = policy
	g.Status.GroupUID = "uid-1"
	return g
}

func TestGroupDeletion_DeletePolicy(t *testing.T) {
	s := newScheme(t)
	mock := &mockClient{getGroupByUIDRes: &authentik.GroupInfo{UID: "uid-1", Owner: "default/g"}}
	r := newGroupReconciler(s, newDeletingGroup(authentikv1alpha1.DeletionPolicyDelete), mock)

	if _, err := r.Reconcile(context.Background(), reqFor("g", "default")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.deleteGroupCalls != 1 {
		t.Errorf("expected 1 delete call, got %d", mock.deleteGroupCalls)
	}
	assertGroupGone(t, r)
}

func TestGroupDeletion_RetainPolicy(t *testing.T) {
	s := newScheme(t)
	mock := &mockClient{getGroupByUIDRes: &authentik.GroupInfo{UID: "uid-1"}}
	r := newGroupReconciler(s, newDeletingGroup(authentikv1alpha1.DeletionPolicyRetain), mock)

	if _, err := r.Reconcile(context.Background(), reqFor("g", "default")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.deleteGroupCalls != 0 {
		t.Errorf("expected 0 delete calls, got %d", mock.deleteGroupCalls)
	}
	assertGroupGone(t, r)
}

func TestGroupDeletion_AlreadyGoneInAuthentik(t *testing.T) {
	s := newScheme(t)
	mock := &mockClient{} // GetGroupByUID returns nil
	r := newGroupReconciler(s, newDeletingGroup(authentikv1alpha1.DeletionPolicyDelete), mock)

	if _, err := r.Reconcile(context.Background(), reqFor("g", "default")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.deleteGroupCalls != 0 {
		t.Errorf("expected 0 delete calls, got %d", mock.deleteGroupCalls)
	}
	assertGroupGone(t, r)
}

func TestGroupDeletion_DeleteErrorKeepsFinalizer(t *testing.T) {
	s := newScheme(t)
	mock := &mockClient{
		getGroupByUIDRes: &authentik.GroupInfo{UID: "uid-1", Owner: "default/g"},
		deleteGroupErr:   fmt.Errorf("boom"),
	}
	r := newGroupReconciler(s, newDeletingGroup(authentikv1alpha1.DeletionPolicyDelete), mock)

	if _, err := r.Reconcile(context.Background(), reqFor("g", "default")); err == nil {
		t.Fatal("expected error")
	}
	if got := getGroup(t, r, "g").Finalizers; len(got) != 1 {
		t.Errorf("expected finalizer to remain, got %v", got)
	}
}

func assertGroupGone(t *testing.T, r *AuthentikGroupReconciler) {
	t.Helper()
	err := r.Get(context.Background(), reqFor("g", "default").NamespacedName, &authentikv1alpha1.AuthentikGroup{})
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected group CR to be deleted after finalizer removal, got err=%v", err)
	}
}

// --- Ownership and privilege tests ---

func assertGroupNotReady(t *testing.T, r *AuthentikGroupReconciler, name, namespace, reason string) {
	t.Helper()
	g := &authentikv1alpha1.AuthentikGroup{}
	if err := r.Get(context.Background(), reqFor(name, namespace).NamespacedName, g); err != nil {
		t.Fatalf("failed to get group: %v", err)
	}
	if g.Status.GroupUID != "" {
		t.Errorf("expected empty GroupUID, got %q", g.Status.GroupUID)
	}
	cond := meta.FindStatusCondition(g.Status.Conditions, authentikv1alpha1.ConditionTypeReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != reason {
		t.Errorf("expected Ready=False with reason %s, got %+v", reason, cond)
	}
}

func TestGroupReconcile_RefusesUnmarkedGroupByName(t *testing.T) {
	s := newScheme(t)
	g := newGroup("g", "tenant-a")
	g.Finalizers = []string{GroupFinalizerName}
	g.Spec.Name = "authentik Admins"
	mock := &mockClient{
		getGroupByNameRes: &authentik.GroupInfo{UID: "admins-uid", Name: "authentik Admins"},
		updateGroupRes:    &authentik.GroupInfo{UID: "admins-uid"},
	}
	r := newGroupReconciler(s, g, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("g", "tenant-a")); err == nil {
		t.Fatal("expected error for unmarked group")
	}
	if mock.updateGroupCalls != 0 || mock.createGroupCalls != 0 {
		t.Errorf("expected no create or update, got %d creates and %d updates", mock.createGroupCalls, mock.updateGroupCalls)
	}
	assertGroupNotReady(t, r, "g", "tenant-a", authentikv1alpha1.ReasonAuthentikError)
}

func TestGroupReconcile_RefusesGroupOwnedByOtherNamespace(t *testing.T) {
	s := newScheme(t)
	g := newGroup("g", "tenant-b")
	g.Finalizers = []string{GroupFinalizerName}
	g.Spec.Name = "shared"
	mock := &mockClient{
		getGroupByNameRes: &authentik.GroupInfo{UID: "a-uid", Name: "shared", Owner: "tenant-a/g"},
	}
	r := newGroupReconciler(s, g, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("g", "tenant-b")); err == nil {
		t.Fatal("expected error for group owned by another CR")
	}
	if mock.updateGroupCalls != 0 || mock.createGroupCalls != 0 {
		t.Errorf("expected no create or update, got %d creates and %d updates", mock.createGroupCalls, mock.updateGroupCalls)
	}
}

func TestGroupReconcile_StatusUIDWithoutMarkerNotTrusted(t *testing.T) {
	s := newScheme(t)
	g := newGroup("g", "default")
	g.Finalizers = []string{GroupFinalizerName}
	g.Status.GroupUID = "admins-uid"
	mock := &mockClient{
		getGroupByUIDRes: &authentik.GroupInfo{UID: "admins-uid", Name: "authentik Admins"},
	}
	r := newGroupReconciler(s, g, mock)

	if _, err := r.Reconcile(context.Background(), reqFor("g", "default")); err == nil {
		t.Fatal("expected error for status UID that points to an unmarked group")
	}
	if mock.updateGroupCalls != 0 || mock.createGroupCalls != 0 {
		t.Errorf("expected no create or update, got %d creates and %d updates", mock.createGroupCalls, mock.updateGroupCalls)
	}
}

func TestGroupDeletion_SkipsUnmarkedGroup(t *testing.T) {
	s := newScheme(t)
	mock := &mockClient{getGroupByUIDRes: &authentik.GroupInfo{UID: "uid-1"}}
	r := newGroupReconciler(s, newDeletingGroup(authentikv1alpha1.DeletionPolicyDelete), mock)

	if _, err := r.Reconcile(context.Background(), reqFor("g", "default")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.deleteGroupCalls != 0 {
		t.Errorf("expected 0 delete calls for unmarked group, got %d", mock.deleteGroupCalls)
	}
	assertGroupGone(t, r)
}

func TestGroupDeletion_SkipsGroupOwnedByOtherCR(t *testing.T) {
	s := newScheme(t)
	mock := &mockClient{getGroupByUIDRes: &authentik.GroupInfo{UID: "uid-1", Owner: "other/g"}}
	r := newGroupReconciler(s, newDeletingGroup(authentikv1alpha1.DeletionPolicyDelete), mock)

	if _, err := r.Reconcile(context.Background(), reqFor("g", "default")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.deleteGroupCalls != 0 {
		t.Errorf("expected 0 delete calls for foreign group, got %d", mock.deleteGroupCalls)
	}
	assertGroupGone(t, r)
}

func TestGroupReconcile_PrivilegedFieldsRejectedByDefault(t *testing.T) {
	superuser := true
	cases := map[string]func(*authentikv1alpha1.AuthentikGroup){
		"isSuperuser": func(g *authentikv1alpha1.AuthentikGroup) { g.Spec.IsSuperuser = &superuser },
		"parents":     func(g *authentikv1alpha1.AuthentikGroup) { g.Spec.Parents = []string{"authentik Admins"} },
		"roles":       func(g *authentikv1alpha1.AuthentikGroup) { g.Spec.Roles = []string{"admin"} },
	}
	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			s := newScheme(t)
			g := newGroup("g", "default")
			g.Finalizers = []string{GroupFinalizerName}
			set(g)
			mock := &mockClient{createGroupRes: &authentik.GroupInfo{UID: "uid-1"}}
			r := newGroupReconciler(s, g, mock)

			result, err := r.Reconcile(context.Background(), reqFor("g", "default"))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != (ctrl.Result{}) {
				t.Errorf("expected no requeue, got %+v", result)
			}
			if mock.createGroupCalls != 0 || mock.updateGroupCalls != 0 {
				t.Errorf("expected no create or update, got %d creates and %d updates", mock.createGroupCalls, mock.updateGroupCalls)
			}
			assertGroupNotReady(t, r, "g", "default", authentikv1alpha1.ReasonValidationFailed)
		})
	}
}
