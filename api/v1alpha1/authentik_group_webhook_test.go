package v1alpha1

import (
	"context"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGroupValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    AuthentikGroupSpec
		wantErr bool
	}{
		{name: "empty spec", spec: AuthentikGroupSpec{}},
		{
			name: "valid spec",
			spec: AuthentikGroupSpec{
				Parents:    []string{"a", "b"},
				Roles:      []string{"r"},
				Attributes: &apiextensionsv1.JSON{Raw: []byte(`{"k":"v"}`)},
			},
		},
		{name: "attributes array", spec: AuthentikGroupSpec{Attributes: &apiextensionsv1.JSON{Raw: []byte(`[1]`)}}, wantErr: true},
		{name: "attributes scalar", spec: AuthentikGroupSpec{Attributes: &apiextensionsv1.JSON{Raw: []byte(`"x"`)}}, wantErr: true},
		{name: "self parent via metadata.name", spec: AuthentikGroupSpec{Parents: []string{"my-group"}}, wantErr: true},
		{name: "self parent via spec.name", spec: AuthentikGroupSpec{Name: "Custom", Parents: []string{"Custom"}}, wantErr: true},
		{name: "duplicate parents", spec: AuthentikGroupSpec{Parents: []string{"a", "a"}}, wantErr: true},
		{name: "duplicate roles", spec: AuthentikGroupSpec{Roles: []string{"r", "r"}}, wantErr: true},
	}

	v := &AuthentikGroupValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &AuthentikGroup{ObjectMeta: metav1.ObjectMeta{Name: "my-group"}, Spec: tt.spec}

			_, createErr := v.ValidateCreate(context.Background(), g)
			_, updateErr := v.ValidateUpdate(context.Background(), g, g)

			if (createErr != nil) != tt.wantErr {
				t.Errorf("ValidateCreate error = %v, wantErr %v", createErr, tt.wantErr)
			}
			if (updateErr != nil) != tt.wantErr {
				t.Errorf("ValidateUpdate error = %v, wantErr %v", updateErr, tt.wantErr)
			}
		})
	}
}

func TestGroupValidateDelete(t *testing.T) {
	v := &AuthentikGroupValidator{}
	if _, err := v.ValidateDelete(context.Background(), &AuthentikGroup{}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
