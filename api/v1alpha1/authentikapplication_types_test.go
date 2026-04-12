package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetSlug_Default(t *testing.T) {
	app := &AuthentikApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app"},
	}
	if got := app.GetSlug(); got != "my-app" {
		t.Errorf("expected %q, got %q", "my-app", got)
	}
}

func TestGetSlug_Explicit(t *testing.T) {
	app := &AuthentikApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app"},
		Spec: AuthentikApplicationSpec{
			Slug: "custom-slug",
		},
	}
	if got := app.GetSlug(); got != "custom-slug" {
		t.Errorf("expected %q, got %q", "custom-slug", got)
	}
}

func TestGetSecretName_Default(t *testing.T) {
	app := &AuthentikApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app"},
	}
	if got := app.GetSecretName(); got != "my-app-oauth" {
		t.Errorf("expected %q, got %q", "my-app-oauth", got)
	}
}

func TestGetSecretName_Explicit(t *testing.T) {
	app := &AuthentikApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app"},
		Spec: AuthentikApplicationSpec{
			Secret: SecretSpec{Name: "custom-secret"},
		},
	}
	if got := app.GetSecretName(); got != "custom-secret" {
		t.Errorf("expected %q, got %q", "custom-secret", got)
	}
}

func TestGetProviderName(t *testing.T) {
	app := &AuthentikApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app"},
	}
	if got := app.GetProviderName(); got != "my-app-provider" {
		t.Errorf("expected %q, got %q", "my-app-provider", got)
	}
}

func TestGetProviderName_WithSlug(t *testing.T) {
	app := &AuthentikApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app"},
		Spec: AuthentikApplicationSpec{
			Slug: "custom",
		},
	}
	if got := app.GetProviderName(); got != "custom-provider" {
		t.Errorf("expected %q, got %q", "custom-provider", got)
	}
}
