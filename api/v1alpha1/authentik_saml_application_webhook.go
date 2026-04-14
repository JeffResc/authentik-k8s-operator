package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/JeffResc/authentik-k8s-operator/internal/template"
)

// AuthentikSAMLApplicationValidator validates AuthentikSAMLApplication resources.
type AuthentikSAMLApplicationValidator struct{}

var _ admission.Validator[*AuthentikSAMLApplication] = &AuthentikSAMLApplicationValidator{}

// SetupSAMLWebhookWithManager registers the SAML validating webhook with the manager.
func SetupSAMLWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &AuthentikSAMLApplication{}).
		WithValidator(&AuthentikSAMLApplicationValidator{}).
		Complete()
}

// ValidateCreate validates the AuthentikSAMLApplication on creation.
func (v *AuthentikSAMLApplicationValidator) ValidateCreate(_ context.Context, obj *AuthentikSAMLApplication) (admission.Warnings, error) {
	return validateSAMLTemplate(obj)
}

// ValidateUpdate validates the AuthentikSAMLApplication on update.
func (v *AuthentikSAMLApplicationValidator) ValidateUpdate(_ context.Context, _, newObj *AuthentikSAMLApplication) (admission.Warnings, error) {
	return validateSAMLTemplate(newObj)
}

// ValidateDelete validates the AuthentikSAMLApplication on deletion (always allowed).
func (v *AuthentikSAMLApplicationValidator) ValidateDelete(_ context.Context, _ *AuthentikSAMLApplication) (admission.Warnings, error) {
	return nil, nil
}

// dummySAMLSecretData is used to test-render templates during validation.
var dummySAMLSecretData = template.SAMLSecretData{
	Metadata: "<md:EntityDescriptor>...</md:EntityDescriptor>",
	Slug:     "validation-app",
	Name:     "Validation App",
}

func validateSAMLTemplate(app *AuthentikSAMLApplication) (admission.Warnings, error) {
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

	if _, err := template.RenderSAMLSecretData(tmpl, dummySAMLSecretData); err != nil {
		return nil, field.Invalid(
			field.NewPath("spec", "secret", "template"),
			tmpl,
			fmt.Sprintf("template renders invalid output: %v", err),
		)
	}

	return nil, nil
}

// +kubebuilder:webhook:path=/validate-goauthentik-io-v1alpha1-authentiksamlapplication,mutating=false,failurePolicy=fail,sideEffects=None,groups=goauthentik.io,resources=authentiksamlapplications,verbs=create;update,versions=v1alpha1,name=vauthentiksamlapplication.kb.io,admissionReviewVersions=v1

// NOTE: The above marker generates the ValidatingWebhookConfiguration manifest.
// To regenerate: make manifests
