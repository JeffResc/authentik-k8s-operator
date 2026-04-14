package v1alpha1

import (
	"context"
	"testing"
)

func TestSAMLValidateCreate_EmptyTemplate(t *testing.T) {
	v := &AuthentikSAMLApplicationValidator{}
	app := &AuthentikSAMLApplication{}
	warnings, err := v.ValidateCreate(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestSAMLValidateCreate_ValidTemplate(t *testing.T) {
	v := &AuthentikSAMLApplicationValidator{}
	app := &AuthentikSAMLApplication{
		Spec: AuthentikSAMLApplicationSpec{
			Secret: SecretSpec{
				Template: "metadata: {{ .Metadata }}\nslug: {{ .Slug }}",
			},
		},
	}
	warnings, err := v.ValidateCreate(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestSAMLValidateCreate_InvalidSyntax(t *testing.T) {
	v := &AuthentikSAMLApplicationValidator{}
	app := &AuthentikSAMLApplication{
		Spec: AuthentikSAMLApplicationSpec{
			Secret: SecretSpec{
				Template: "{{ .Foo",
			},
		},
	}
	_, err := v.ValidateCreate(context.Background(), app)
	if err == nil {
		t.Fatal("expected error for invalid template syntax, got nil")
	}
}

func TestSAMLValidateCreate_UndefinedField(t *testing.T) {
	v := &AuthentikSAMLApplicationValidator{}
	app := &AuthentikSAMLApplication{
		Spec: AuthentikSAMLApplicationSpec{
			Secret: SecretSpec{
				Template: "key: {{ .ClientID }}",
			},
		},
	}
	_, err := v.ValidateCreate(context.Background(), app)
	if err == nil {
		t.Fatal("expected error for undefined field (ClientID is OAuth2, not SAML), got nil")
	}
}

func TestSAMLValidateCreate_InvalidYAML(t *testing.T) {
	v := &AuthentikSAMLApplicationValidator{}
	app := &AuthentikSAMLApplication{
		Spec: AuthentikSAMLApplicationSpec{
			Secret: SecretSpec{
				Template: "not valid yaml output",
			},
		},
	}
	_, err := v.ValidateCreate(context.Background(), app)
	if err == nil {
		t.Fatal("expected error for invalid YAML output, got nil")
	}
}

func TestSAMLValidateUpdate_ValidTemplate(t *testing.T) {
	v := &AuthentikSAMLApplicationValidator{}
	oldApp := &AuthentikSAMLApplication{}
	newApp := &AuthentikSAMLApplication{
		Spec: AuthentikSAMLApplicationSpec{
			Secret: SecretSpec{
				Template: "metadata: {{ .Metadata }}",
			},
		},
	}
	warnings, err := v.ValidateUpdate(context.Background(), oldApp, newApp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestSAMLValidateUpdate_InvalidTemplate(t *testing.T) {
	v := &AuthentikSAMLApplicationValidator{}
	oldApp := &AuthentikSAMLApplication{}
	newApp := &AuthentikSAMLApplication{
		Spec: AuthentikSAMLApplicationSpec{
			Secret: SecretSpec{
				Template: "{{ .BadField }}",
			},
		},
	}
	_, err := v.ValidateUpdate(context.Background(), oldApp, newApp)
	if err == nil {
		t.Fatal("expected error for invalid template, got nil")
	}
}

func TestSAMLValidateDelete_AlwaysAllowed(t *testing.T) {
	v := &AuthentikSAMLApplicationValidator{}
	app := &AuthentikSAMLApplication{}
	warnings, err := v.ValidateDelete(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestSAMLValidateCreate_AllFields(t *testing.T) {
	v := &AuthentikSAMLApplicationValidator{}
	app := &AuthentikSAMLApplication{
		Spec: AuthentikSAMLApplicationSpec{
			Secret: SecretSpec{
				Template: "metadata: {{ .Metadata }}\nslug: {{ .Slug }}\nname: {{ .Name }}",
			},
		},
	}
	warnings, err := v.ValidateCreate(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}
