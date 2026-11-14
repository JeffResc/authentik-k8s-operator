package main

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestManagerCacheOptionsScopeSecrets(t *testing.T) {
	cacheOpts, clientOpts := managerCacheOptions()

	var secretSelector labels.Selector
	for obj, byObj := range cacheOpts.ByObject {
		if _, ok := obj.(*corev1.Secret); ok {
			secretSelector = byObj.Label
		}
	}
	if secretSelector == nil {
		t.Fatal("expected a label selector for the Secret informer, got none")
	}
	if !secretSelector.Matches(labels.Set{"app.kubernetes.io/managed-by": "authentik-operator"}) {
		t.Error("Secret selector must match Secrets that the operator manages")
	}
	if secretSelector.Matches(labels.Set{}) {
		t.Error("Secret selector must not match unlabelled Secrets")
	}
	if secretSelector.Matches(labels.Set{"app.kubernetes.io/managed-by": "helm"}) {
		t.Error("Secret selector must not match Secrets managed by another tool")
	}

	if clientOpts.Cache == nil {
		t.Fatal("expected client cache options, got nil")
	}
	disabled := false
	for _, obj := range clientOpts.Cache.DisableFor {
		if _, ok := obj.(*corev1.Secret); ok {
			disabled = true
		}
	}
	if !disabled {
		t.Error("client reads of Secrets must bypass the label-scoped cache so that unlabelled Secrets with the same name are found")
	}
}
