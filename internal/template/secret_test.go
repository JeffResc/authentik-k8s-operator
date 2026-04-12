package template

import (
	"testing"
)

func TestRenderSecretData_DefaultTemplate(t *testing.T) {
	data := SecretData{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		IssuerURL:    "https://auth.example.com/application/o/test-app/",
		AuthURL:      "https://auth.example.com/application/o/authorize/",
		TokenURL:     "https://auth.example.com/application/o/token/",
		UserInfoURL:  "https://auth.example.com/application/o/userinfo/",
	}

	result, err := RenderSecretData("", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]string{
		"client-id":         "test-client-id",
		"client-secret":     "test-client-secret",
		"issuer-url":        "https://auth.example.com/application/o/test-app/",
		"authorization-url": "https://auth.example.com/application/o/authorize/",
		"token-url":         "https://auth.example.com/application/o/token/",
		"userinfo-url":      "https://auth.example.com/application/o/userinfo/",
	}

	if len(result) != len(expected) {
		t.Fatalf("expected %d keys, got %d: %v", len(expected), len(result), result)
	}

	for key, want := range expected {
		got, ok := result[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}
		if string(got) != want {
			t.Errorf("key %q: expected %q, got %q", key, want, string(got))
		}
	}
}

func TestRenderSecretData_CustomTemplate(t *testing.T) {
	data := SecretData{
		ClientID: "my-id",
		Slug:     "my-app",
		Name:     "My App",
	}

	tmpl := `id: {{ .ClientID }}
slug: {{ .Slug }}
name: {{ .Name }}`

	result, err := RenderSecretData(tmpl, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(result["id"]) != "my-id" {
		t.Errorf("expected id %q, got %q", "my-id", string(result["id"]))
	}
	if string(result["slug"]) != "my-app" {
		t.Errorf("expected slug %q, got %q", "my-app", string(result["slug"]))
	}
	if string(result["name"]) != "My App" {
		t.Errorf("expected name %q, got %q", "My App", string(result["name"]))
	}
}

func TestRenderSecretData_EmptyTemplateUsesDefault(t *testing.T) {
	data := SecretData{
		ClientID:     "id",
		ClientSecret: "secret",
		IssuerURL:    "https://issuer",
		AuthURL:      "https://auth",
		TokenURL:     "https://token",
		UserInfoURL:  "https://userinfo",
	}

	result, err := RenderSecretData("", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default template produces 6 keys
	if len(result) != 6 {
		t.Fatalf("expected 6 keys from default template, got %d: %v", len(result), result)
	}

	for _, key := range []string{"client-id", "client-secret", "issuer-url", "authorization-url", "token-url", "userinfo-url"} {
		if _, ok := result[key]; !ok {
			t.Errorf("missing expected default key %q", key)
		}
	}
}

func TestRenderSecretData_InvalidTemplate(t *testing.T) {
	_, err := RenderSecretData("{{ .Foo", SecretData{})
	if err == nil {
		t.Fatal("expected error for invalid template, got nil")
	}
}

func TestRenderSecretData_InvalidField(t *testing.T) {
	// Go templates error on fields that don't exist in the struct
	_, err := RenderSecretData("key: {{ .NonExistent }}", SecretData{})
	if err == nil {
		t.Fatal("expected error for invalid field reference, got nil")
	}
}

func TestRenderSecretData_ValuesContainingColons(t *testing.T) {
	data := SecretData{
		IssuerURL: "https://auth.example.com:8443/app",
	}

	result, err := RenderSecretData("issuer: {{ .IssuerURL }}", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := string(result["issuer"])
	if got != "https://auth.example.com:8443/app" {
		t.Errorf("expected URL with port, got %q", got)
	}
}

func TestRenderSecretData_EmptyLinesSkipped(t *testing.T) {
	tmpl := `key1: value1

key2: value2

`
	result, err := RenderSecretData(tmpl, SecretData{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(result), result)
	}
}

func TestRenderSecretData_QuotedValues(t *testing.T) {
	tmpl := `double: "hello world"
single: 'hello world'
unquoted: hello world`

	result, err := RenderSecretData(tmpl, SecretData{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(result["double"]) != "hello world" {
		t.Errorf("double-quoted: expected %q, got %q", "hello world", string(result["double"]))
	}
	if string(result["single"]) != "hello world" {
		t.Errorf("single-quoted: expected %q, got %q", "hello world", string(result["single"]))
	}
	if string(result["unquoted"]) != "hello world" {
		t.Errorf("unquoted: expected %q, got %q", "hello world", string(result["unquoted"]))
	}
}

func TestRenderSecretData_LinesWithoutColonsSkipped(t *testing.T) {
	tmpl := `key1: value1
this line has no colon
key2: value2`

	result, err := RenderSecretData(tmpl, SecretData{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(result), result)
	}
}

func TestRenderSecretData_EmptyKeySkipped(t *testing.T) {
	tmpl := `: value-with-empty-key
key: value`

	result, err := RenderSecretData(tmpl, SecretData{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 key, got %d: %v", len(result), result)
	}
}

func TestValidateTemplate_Valid(t *testing.T) {
	err := ValidateTemplate("key: {{ .ClientID }}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTemplate_Invalid(t *testing.T) {
	err := ValidateTemplate("{{ .Foo")
	if err == nil {
		t.Fatal("expected error for invalid template, got nil")
	}
}

func TestValidateTemplate_Empty(t *testing.T) {
	err := ValidateTemplate("")
	if err != nil {
		t.Fatalf("unexpected error for empty template: %v", err)
	}
}
