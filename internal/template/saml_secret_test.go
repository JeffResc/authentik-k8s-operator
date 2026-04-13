package template

import (
	"testing"
)

func TestRenderSAMLSecretData_Default(t *testing.T) {
	data := SAMLSecretData{
		Metadata: "<xml>test</xml>",
		Slug:     "my-app",
		Name:     "My App",
	}

	result, err := RenderSAMLSecretData("", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(result["metadata"]) != "<xml>test</xml>" {
		t.Errorf("expected metadata value, got %q", string(result["metadata"]))
	}
}

func TestRenderSAMLSecretData_CustomTemplate(t *testing.T) {
	data := SAMLSecretData{
		Metadata: "<xml>saml</xml>",
		Slug:     "my-app",
		Name:     "My App",
	}

	tmpl := "idp-metadata: {{ .Metadata }}\napp-slug: {{ .Slug }}\napp-name: {{ .Name }}"
	result, err := RenderSAMLSecretData(tmpl, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(result["idp-metadata"]) != "<xml>saml</xml>" {
		t.Errorf("expected metadata, got %q", string(result["idp-metadata"]))
	}
	if string(result["app-slug"]) != "my-app" {
		t.Errorf("expected slug, got %q", string(result["app-slug"]))
	}
	if string(result["app-name"]) != "My App" {
		t.Errorf("expected name, got %q", string(result["app-name"]))
	}
}

func TestRenderSAMLSecretData_InvalidTemplate(t *testing.T) {
	data := SAMLSecretData{}
	_, err := RenderSAMLSecretData("{{ .Invalid", data)
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
}

func TestRenderSAMLSecretData_UndefinedField(t *testing.T) {
	data := SAMLSecretData{}
	_, err := RenderSAMLSecretData("key: {{ .NonExistent }}", data)
	if err == nil {
		t.Fatal("expected error for undefined field")
	}
}
