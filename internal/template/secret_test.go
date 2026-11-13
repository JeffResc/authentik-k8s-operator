package template

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestRenderSecretData_LinesWithoutColonsError(t *testing.T) {
	tmpl := `key1: value1
this line has no colon
key2: value2`

	_, err := RenderSecretData(tmpl, SecretData{})
	if err == nil {
		t.Fatal("expected error for invalid YAML (line without colon), got nil")
	}
}

func TestRenderSecretData_EmptyKeyError(t *testing.T) {
	tmpl := `: value-with-empty-key
key: value`

	_, err := RenderSecretData(tmpl, SecretData{})
	if err == nil {
		t.Fatal("expected error for invalid YAML (empty key), got nil")
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

// renderWithin fails the test if RenderSecretData does not return within d.
func renderWithin(t *testing.T, d time.Duration, tmpl string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := RenderSecretData(tmpl, SecretData{})
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("RenderSecretData did not return within %s", d)
		return nil
	}
}

func TestRenderSecretData_OutputLimit(t *testing.T) {
	// A 50-byte template that asks for 20 MB of output.
	err := renderWithin(t, 5*time.Second, "k: '{{range 20000}}{{range 1000}}x{{end}}{{end}}'")
	if err == nil {
		t.Fatal("expected error for output above the size limit, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size limit error, got: %v", err)
	}
}

func TestRenderSecretData_OutputAtLimitAllowed(t *testing.T) {
	// "k: " (3 bytes) + value + "\n" (1 byte) is exactly 1 MiB.
	n := 1<<20 - 4
	tmpl := "k: {{range " + strconv.Itoa(n) + "}}x{{end}}\n"
	result, err := RenderSecretData(tmpl, SecretData{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result["k"]) != n {
		t.Fatalf("expected %d bytes, got %d", n, len(result["k"]))
	}
}

func TestRenderSecretData_RangeWithoutOutputDeadline(t *testing.T) {
	err := renderWithin(t, 5*time.Second, "{{range 9223372036854775807}}{{end}}")
	if err == nil {
		t.Fatal("expected deadline error, got nil")
	}
	if !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("expected deadline error, got: %v", err)
	}
}

func TestRenderSecretData_NestedTemplateCallsDeadline(t *testing.T) {
	// Each level calls the level below it twice: 2^64 calls and no output.
	var b strings.Builder
	b.WriteString(`{{define "t0"}}{{end}}`)
	for i := 1; i <= 64; i++ {
		fmt.Fprintf(&b, `{{define "t%d"}}{{template "t%d"}}{{template "t%d"}}{{end}}`, i, i-1, i-1)
	}
	b.WriteString(`{{template "t64"}}`)

	err := renderWithin(t, 5*time.Second, b.String())
	if err == nil {
		t.Fatal("expected deadline error, got nil")
	}
	if !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("expected deadline error, got: %v", err)
	}
}

func TestRenderSecretData_IntermediateValueLimit(t *testing.T) {
	// Values held in variables are never written, so the output limit does
	// not see them. Without the limit, each case allocates only a few MB.
	cases := map[string]string{
		"printf width":     `{{$x := printf "%0600000d%0600000d" 0 0}}k: v`,
		"printf doubling":  `{{$x := printf "%0600000d" 0}}{{$x = printf "%s%s" $x $x}}k: v`,
		"printf arg index": `{{$x := printf "%0600000d" 0}}{{$x = printf "%[1]s%[1]s" $x}}k: v`,
		"print doubling":   `{{$x := printf "%0600000d" 0}}{{$x = print $x $x}}k: v`,
		"println":          `{{$x := printf "%0600000d" 0}}{{$x = println $x $x}}k: v`,
		"js expansion":     `{{$x := "<<<<<<<<"}}{{range 16}}{{$x = print $x $x}}{{end}}{{$x = js $x}}k: v`,
	}
	for name, tmpl := range cases {
		t.Run(name, func(t *testing.T) {
			err := renderWithin(t, 5*time.Second, tmpl)
			if err == nil {
				t.Fatal("expected size limit error, got nil")
			}
			if !strings.Contains(err.Error(), "exceeds") {
				t.Fatalf("expected size limit error, got: %v", err)
			}
		})
	}
}

func TestRenderSecretData_BuiltinsStillWork(t *testing.T) {
	tmpl := `a: {{printf "%s-%d" .Slug 3}}
b: {{print .Slug "x"}}
c: {{urlquery "a b"}}
d: '{{html "<"}}'
e: {{js "<"}}
f: {{range 3}}x{{end}}
g: {{if .Slug}}{{with .Name}}{{.}}{{else}}none{{end}}{{end}}`
	result, err := RenderSecretData(tmpl, SecretData{Slug: "app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]string{"a": "app-3", "b": "appx", "c": "a+b", "d": "&lt;", "e": `\u003C`, "f": "xxx", "g": "none"}
	for k, v := range want {
		if string(result[k]) != v {
			t.Errorf("key %q: expected %q, got %q", k, v, string(result[k]))
		}
	}
}
