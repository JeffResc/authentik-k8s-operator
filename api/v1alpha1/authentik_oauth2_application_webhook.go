package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/JeffResc/authentik-k8s-operator/internal/template"
)

// AuthentikOAuth2ApplicationValidator validates AuthentikOAuth2Application resources.
type AuthentikOAuth2ApplicationValidator struct{}

var _ admission.Validator[*AuthentikOAuth2Application] = &AuthentikOAuth2ApplicationValidator{}

// SetupWebhookWithManager registers the validating webhook with the manager.
func SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &AuthentikOAuth2Application{}).
		WithValidator(&AuthentikOAuth2ApplicationValidator{}).
		Complete()
}

// ValidateCreate validates the AuthentikOAuth2Application on creation.
func (v *AuthentikOAuth2ApplicationValidator) ValidateCreate(_ context.Context, obj *AuthentikOAuth2Application) (admission.Warnings, error) {
	return validateTemplate(obj)
}

// ValidateUpdate validates the AuthentikOAuth2Application on update.
func (v *AuthentikOAuth2ApplicationValidator) ValidateUpdate(_ context.Context, _, newObj *AuthentikOAuth2Application) (admission.Warnings, error) {
	return validateTemplate(newObj)
}

// ValidateDelete validates the AuthentikOAuth2Application on deletion (always allowed).
func (v *AuthentikOAuth2ApplicationValidator) ValidateDelete(_ context.Context, _ *AuthentikOAuth2Application) (admission.Warnings, error) {
	return nil, nil
}

// dummySecretData is used to test-render templates during validation.
var dummySecretData = template.SecretData{
	ClientID:        "validation-client-id",
	ClientSecret:    "validation-client-secret",
	IssuerURL:       "https://example.com/application/o/app/",
	AuthURL:         "https://example.com/application/o/authorize/",
	TokenURL:        "https://example.com/application/o/token/",
	UserInfoURL:     "https://example.com/application/o/userinfo/",
	LogoutURL:       "https://example.com/application/o/app/end-session/",
	JWKSURL:         "https://example.com/application/o/app/jwks/",
	ProviderInfoURL: "https://example.com/application/o/app/.well-known/openid-configuration",
	Slug:            "validation-app",
	Name:            "Validation App",
}

func validateTemplate(app *AuthentikOAuth2Application) (admission.Warnings, error) {
	tmpl := app.Spec.Secret.Template
	if tmpl == "" {
		return nil, nil
	}

	if err := template.ValidateTemplate(tmpl); err != nil {
		return nil, field.Invalid(
			field.NewPath("spec", "secret", "template"),
			tmpl,
			fmt.Sprintf("invalid Go template: %v", err),
		)
	}

	if _, err := template.RenderSecretData(tmpl, dummySecretData); err != nil {
		return nil, field.Invalid(
			field.NewPath("spec", "secret", "template"),
			tmpl,
			fmt.Sprintf("template renders invalid output: %v", err),
		)
	}

	return nil, nil
}

// +kubebuilder:webhook:path=/validate-goauthentik-io-v1alpha1-authentikoauth2application,mutating=false,failurePolicy=fail,sideEffects=None,groups=goauthentik.io,resources=authentikoauth2applications,verbs=create;update,versions=v1alpha1,name=vauthentikoauth2application.kb.io,admissionReviewVersions=v1

// NOTE: The above marker generates the ValidatingWebhookConfiguration manifest.
// To regenerate: make manifests
