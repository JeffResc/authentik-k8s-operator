package template

import "testing"

// FuzzRenderSecretData fuzzes the OAuth2 secret template rendering pipeline.
// The corpus is seeded with known templates from the existing test suite.
func FuzzRenderSecretData(f *testing.F) {
	// Seed with known templates
	f.Add(DefaultTemplate)
	f.Add("client-id: {{ .ClientID }}\nclient-secret: {{ .ClientSecret }}")
	f.Add("key: {{ .Slug }}")
	f.Add("{{ .Foo")          // invalid syntax
	f.Add("not valid yaml")   // valid template, invalid YAML output
	f.Add("key: {{ .Name }}") // valid
	f.Add("")                 // empty (uses default)

	data := SecretData{
		ClientID:        "fuzz-client-id",
		ClientSecret:    "fuzz-client-secret",
		IssuerURL:       "https://example.com/",
		AuthURL:         "https://example.com/authorize",
		TokenURL:        "https://example.com/token",
		UserInfoURL:     "https://example.com/userinfo",
		LogoutURL:       "https://example.com/logout",
		JWKSURL:         "https://example.com/jwks",
		ProviderInfoURL: "https://example.com/.well-known",
		Slug:            "fuzz-app",
		Name:            "Fuzz App",
	}

	f.Fuzz(func(t *testing.T, tmpl string) {
		// ValidateTemplate should never panic
		err := ValidateTemplate(tmpl)

		if err != nil {
			// If validation rejects the template, rendering should also fail or
			// the template is genuinely unparseable — either way, nothing more to check.
			return
		}

		// If validation passed, rendering should not panic.
		// It may still return an error (e.g. undefined field, invalid YAML output).
		result, err := RenderSecretData(tmpl, data)
		if err != nil {
			return
		}

		// If rendering succeeded, every value should be non-nil
		for k, v := range result {
			if v == nil {
				t.Errorf("nil value for key %q", k)
			}
		}
	})
}

// FuzzRenderSAMLSecretData fuzzes the SAML secret template rendering pipeline.
func FuzzRenderSAMLSecretData(f *testing.F) {
	f.Add(DefaultSAMLTemplate)
	f.Add("metadata: {{ .Metadata }}\nslug: {{ .Slug }}")
	f.Add("name: {{ .Name }}")
	f.Add("{{ .Foo") // invalid syntax
	f.Add("")        // empty (uses default)

	data := SAMLSecretData{
		Metadata: "<md:EntityDescriptor>fuzz</md:EntityDescriptor>",
		Slug:     "fuzz-saml-app",
		Name:     "Fuzz SAML App",
	}

	f.Fuzz(func(t *testing.T, tmpl string) {
		err := ValidateTemplate(tmpl)
		if err != nil {
			return
		}

		result, err := RenderSAMLSecretData(tmpl, data)
		if err != nil {
			return
		}

		for k, v := range result {
			if v == nil {
				t.Errorf("nil value for key %q", k)
			}
		}
	})
}
