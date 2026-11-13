package template

import (
	"testing"
	"time"
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

func TestRenderSAMLSecretData_ExecutionLimits(t *testing.T) {
	cases := map[string]string{
		"output limit": "k: '{{range 20000}}{{range 1000}}x{{end}}{{end}}'",
		"deadline":     "{{range 9223372036854775807}}{{end}}",
	}
	for name, tmpl := range cases {
		t.Run(name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() {
				_, err := RenderSAMLSecretData(tmpl, SAMLSecretData{})
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("expected error, got nil")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("RenderSAMLSecretData did not return within 5s")
			}
		})
	}
}
