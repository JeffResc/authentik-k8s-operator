package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/JeffResc/authentik-k8s-operator/internal/template"
)

// AuthentikApplicationValidator validates AuthentikApplication resources.
type AuthentikApplicationValidator struct{}

var _ admission.Validator[*AuthentikApplication] = &AuthentikApplicationValidator{}

// SetupWebhookWithManager registers the validating webhook with the manager.
func SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &AuthentikApplication{}).
		WithValidator(&AuthentikApplicationValidator{}).
		Complete()
}

// ValidateCreate validates the AuthentikApplication on creation.
func (v *AuthentikApplicationValidator) ValidateCreate(_ context.Context, obj *AuthentikApplication) (admission.Warnings, error) {
	return validateTemplate(obj)
}

// ValidateUpdate validates the AuthentikApplication on update.
func (v *AuthentikApplicationValidator) ValidateUpdate(_ context.Context, _, newObj *AuthentikApplication) (admission.Warnings, error) {
	return validateTemplate(newObj)
}

// ValidateDelete validates the AuthentikApplication on deletion (always allowed).
func (v *AuthentikApplicationValidator) ValidateDelete(_ context.Context, _ *AuthentikApplication) (admission.Warnings, error) {
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

func validateTemplate(app *AuthentikApplication) (admission.Warnings, error) {
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

// +kubebuilder:webhook:path=/validate-goauthentik-io-v1alpha1-authentikapplication,mutating=false,failurePolicy=fail,sideEffects=None,groups=goauthentik.io,resources=authentikapplications,verbs=create;update,versions=v1alpha1,name=vauthentikapplication.kb.io,admissionReviewVersions=v1

// NOTE: The above marker generates the ValidatingWebhookConfiguration manifest.
// To regenerate: make manifests
