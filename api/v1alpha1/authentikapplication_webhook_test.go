package v1alpha1

import (
	"context"
	"testing"
)

func TestValidateCreate_EmptyTemplate(t *testing.T) {
	v := &AuthentikApplicationValidator{}
	app := &AuthentikApplication{}
	warnings, err := v.ValidateCreate(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestValidateCreate_ValidTemplate(t *testing.T) {
	v := &AuthentikApplicationValidator{}
	app := &AuthentikApplication{
		Spec: AuthentikApplicationSpec{
			Secret: SecretSpec{
				Template: "client-id: {{ .ClientID }}\nclient-secret: {{ .ClientSecret }}",
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

func TestValidateCreate_InvalidSyntax(t *testing.T) {
	v := &AuthentikApplicationValidator{}
	app := &AuthentikApplication{
		Spec: AuthentikApplicationSpec{
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

func TestValidateCreate_UndefinedField(t *testing.T) {
	v := &AuthentikApplicationValidator{}
	app := &AuthentikApplication{
		Spec: AuthentikApplicationSpec{
			Secret: SecretSpec{
				Template: "key: {{ .NonExistent }}",
			},
		},
	}
	_, err := v.ValidateCreate(context.Background(), app)
	if err == nil {
		t.Fatal("expected error for undefined field, got nil")
	}
}

func TestValidateCreate_InvalidYAML(t *testing.T) {
	v := &AuthentikApplicationValidator{}
	app := &AuthentikApplication{
		Spec: AuthentikApplicationSpec{
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

func TestValidateUpdate_ValidTemplate(t *testing.T) {
	v := &AuthentikApplicationValidator{}
	oldApp := &AuthentikApplication{}
	newApp := &AuthentikApplication{
		Spec: AuthentikApplicationSpec{
			Secret: SecretSpec{
				Template: "id: {{ .ClientID }}",
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

func TestValidateUpdate_InvalidTemplate(t *testing.T) {
	v := &AuthentikApplicationValidator{}
	oldApp := &AuthentikApplication{}
	newApp := &AuthentikApplication{
		Spec: AuthentikApplicationSpec{
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

func TestValidateDelete_AlwaysAllowed(t *testing.T) {
	v := &AuthentikApplicationValidator{}
	app := &AuthentikApplication{}
	warnings, err := v.ValidateDelete(context.Background(), app)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestValidateCreate_AllFields(t *testing.T) {
	v := &AuthentikApplicationValidator{}
	app := &AuthentikApplication{
		Spec: AuthentikApplicationSpec{
			Secret: SecretSpec{
				Template: `client-id: {{ .ClientID }}
client-secret: {{ .ClientSecret }}
issuer: {{ .IssuerURL }}
auth: {{ .AuthURL }}
token: {{ .TokenURL }}
userinfo: {{ .UserInfoURL }}
logout: {{ .LogoutURL }}
jwks: {{ .JWKSURL }}
provider-info: {{ .ProviderInfoURL }}
slug: {{ .Slug }}
name: {{ .Name }}`,
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
