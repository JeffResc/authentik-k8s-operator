package v1alpha1

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AuthentikGroupValidator validates AuthentikGroup resources.
type AuthentikGroupValidator struct{}

var _ admission.Validator[*AuthentikGroup] = &AuthentikGroupValidator{}

// SetupGroupWebhookWithManager registers the group validating webhook with the manager.
func SetupGroupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &AuthentikGroup{}).
		WithValidator(&AuthentikGroupValidator{}).
		Complete()
}

// ValidateCreate validates the AuthentikGroup on creation.
func (v *AuthentikGroupValidator) ValidateCreate(_ context.Context, obj *AuthentikGroup) (admission.Warnings, error) {
	return nil, validateGroup(obj)
}

// ValidateUpdate validates the AuthentikGroup on update.
func (v *AuthentikGroupValidator) ValidateUpdate(_ context.Context, _, newObj *AuthentikGroup) (admission.Warnings, error) {
	return nil, validateGroup(newObj)
}

// ValidateDelete validates the AuthentikGroup on deletion (always allowed).
func (v *AuthentikGroupValidator) ValidateDelete(_ context.Context, _ *AuthentikGroup) (admission.Warnings, error) {
	return nil, nil
}

func validateGroup(g *AuthentikGroup) error {
	var errs field.ErrorList
	specPath := field.NewPath("spec")

	if g.Spec.Attributes != nil && len(g.Spec.Attributes.Raw) > 0 {
		var obj map[string]interface{}
		if err := json.Unmarshal(g.Spec.Attributes.Raw, &obj); err != nil {
			errs = append(errs, field.Invalid(specPath.Child("attributes"), string(g.Spec.Attributes.Raw), "must be a JSON object"))
		}
	}

	name := g.GetGroupName()
	for i, p := range g.Spec.Parents {
		if p == name {
			errs = append(errs, field.Invalid(specPath.Child("parents").Index(i), p, "a group cannot be its own parent"))
		}
	}

	errs = append(errs, validateNoDuplicates(specPath.Child("parents"), g.Spec.Parents)...)
	errs = append(errs, validateNoDuplicates(specPath.Child("roles"), g.Spec.Roles)...)

	return errs.ToAggregate()
}

func validateNoDuplicates(path *field.Path, values []string) field.ErrorList {
	var errs field.ErrorList
	seen := make(map[string]struct{}, len(values))
	for i, v := range values {
		if _, ok := seen[v]; ok {
			errs = append(errs, field.Duplicate(path.Index(i), v))
			continue
		}
		seen[v] = struct{}{}
	}
	return errs
}

// +kubebuilder:webhook:path=/validate-goauthentik-io-v1alpha1-authentikgroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=goauthentik.io,resources=authentikgroups,verbs=create;update,versions=v1alpha1,name=vauthentikgroup.kb.io,admissionReviewVersions=v1

// NOTE: The above marker generates the ValidatingWebhookConfiguration manifest.
// To regenerate: make manifests
