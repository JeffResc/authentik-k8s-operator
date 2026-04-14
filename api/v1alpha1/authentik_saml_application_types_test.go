package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSAMLGetSlug_Default(t *testing.T) {
	app := &AuthentikSAMLApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "my-saml-app"},
	}
	if got := app.GetSlug(); got != "my-saml-app" {
		t.Errorf("expected %q, got %q", "my-saml-app", got)
	}
}

func TestSAMLGetSlug_Explicit(t *testing.T) {
	app := &AuthentikSAMLApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "my-saml-app"},
		Spec:       AuthentikSAMLApplicationSpec{ApplicationBaseSpec: ApplicationBaseSpec{Slug: "custom-slug"}},
	}
	if got := app.GetSlug(); got != "custom-slug" {
		t.Errorf("expected %q, got %q", "custom-slug", got)
	}
}

func TestSAMLGetSecretName_Default(t *testing.T) {
	app := &AuthentikSAMLApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "my-saml-app"},
	}
	if got := app.GetSecretName(); got != "my-saml-app-saml" {
		t.Errorf("expected %q, got %q", "my-saml-app-saml", got)
	}
}

func TestSAMLGetSecretName_Explicit(t *testing.T) {
	app := &AuthentikSAMLApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "my-saml-app"},
		Spec:       AuthentikSAMLApplicationSpec{ApplicationBaseSpec: ApplicationBaseSpec{Secret: SecretSpec{Name: "custom-secret"}}},
	}
	if got := app.GetSecretName(); got != "custom-secret" {
		t.Errorf("expected %q, got %q", "custom-secret", got)
	}
}

func TestSAMLGetProviderName(t *testing.T) {
	app := &AuthentikSAMLApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "my-saml-app"},
	}
	if got := app.GetProviderName(); got != "my-saml-app-provider" {
		t.Errorf("expected %q, got %q", "my-saml-app-provider", got)
	}
}
