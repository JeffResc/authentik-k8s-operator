package controller

import (
	"bytes"
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/JeffResc/authentik-k8s-operator/internal/authentik"
)

// reconcileApplication ensures the Authentik application exists and is configured correctly.
func reconcileApplication(ctx context.Context, akClient authentik.Client, slug, name string, providerID int32, opts *authentik.ApplicationOptions) (*authentik.ApplicationInfo, error) {
	logger := log.FromContext(ctx)

	existingApp, err := akClient.GetApplicationBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing application: %w", err)
	}

	if existingApp != nil {
		logger.Info("updating existing application", "slug", slug)
		return akClient.UpdateApplication(ctx, slug, name, providerID, opts)
	}

	logger.Info("creating new application", "slug", slug)
	return akClient.CreateApplication(ctx, slug, name, providerID, opts)
}

// deleteStaleSecret deletes a secret if the name has changed since the last reconciliation.
func deleteStaleSecret(ctx context.Context, c client.Client, namespace, oldName, newName string) {
	if oldName == "" || oldName == newName {
		return
	}
	logger := log.FromContext(ctx)
	oldSecret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: oldName, Namespace: namespace}, oldSecret); err == nil {
		if err := c.Delete(ctx, oldSecret); err != nil {
			logger.Error(err, "failed to delete stale secret", "name", oldName)
		} else {
			logger.Info("deleted stale secret after name change", "oldName", oldName, "newName", newName)
		}
	}
}

// reconcileSecretObject creates or updates a Kubernetes secret with the given data,
// labels, annotations, and owner reference.
func reconcileSecretObject(ctx context.Context, c client.Client, scheme *runtime.Scheme, owner client.Object, secretName, namespace string, secretData map[string][]byte, specLabels, specAnnotations map[string]string) error {
	logger := log.FromContext(ctx)

	// Check if existing secret already has the correct data
	existing := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, existing); err == nil {
		if secretDataEqual(existing.Data, secretData) {
			logger.V(1).Info("secret data unchanged, skipping update", "name", secretName)
			return nil
		}
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		secret.Labels["app.kubernetes.io/managed-by"] = "authentik-operator"
		secret.Labels["goauthentik.io/application"] = owner.GetName()
		for k, v := range specLabels {
			secret.Labels[k] = v
		}

		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		for k, v := range specAnnotations {
			secret.Annotations[k] = v
		}

		secret.Data = secretData
		secret.Type = corev1.SecretTypeOpaque

		return controllerutil.SetControllerReference(owner, secret, scheme)
	})

	if err != nil {
		return fmt.Errorf("failed to create or update secret: %w", err)
	}

	logger.Info("reconciled secret", "name", secretName, "operation", op)
	return nil
}

// secretDataEqual compares two secret data maps for byte-level equality.
func secretDataEqual(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !bytes.Equal(v, b[k]) {
			return false
		}
	}
	return true
}

// deleteAuthentikApplication deletes an Authentik application by slug if it exists.
func deleteAuthentikApplication(ctx context.Context, akClient authentik.Client, slug string) error {
	logger := log.FromContext(ctx)

	existingApp, err := akClient.GetApplicationBySlug(ctx, slug)
	if err != nil {
		return fmt.Errorf("failed to check if application exists: %w", err)
	}

	if existingApp != nil {
		if err := akClient.DeleteApplication(ctx, slug); err != nil {
			return fmt.Errorf("failed to delete application from Authentik: %w", err)
		}
		logger.Info("deleted application from Authentik", "slug", slug)
	}

	return nil
}
