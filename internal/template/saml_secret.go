package template

import (
	"bytes"
	"fmt"
	"text/template"

	"gopkg.in/yaml.v3"
)

// SAMLSecretData contains the data available for SAML secret templates
type SAMLSecretData struct {
	// Metadata is the IdP SAML metadata XML
	Metadata string
	// Slug is the application slug
	Slug string
	// Name is the application display name
	Name string
}

// DefaultSAMLTemplate is the default secret template for SAML providers
const DefaultSAMLTemplate = `metadata: {{ .Metadata }}`

// RenderSAMLSecretData renders a template string with the given SAML secret data
func RenderSAMLSecretData(templateStr string, data SAMLSecretData) (map[string][]byte, error) {
	if templateStr == "" {
		templateStr = DefaultSAMLTemplate
	}

	tmpl, err := template.New("secret").Option("missingkey=error").Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	var parsed map[string]string
	if err := yaml.Unmarshal(buf.Bytes(), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse rendered template as YAML: %w", err)
	}

	result := make(map[string][]byte, len(parsed))
	for k, v := range parsed {
		result[k] = []byte(v)
	}

	return result, nil
}
