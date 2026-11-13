package template

import (
	"fmt"

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

	out, err := execute(templateStr, data)
	if err != nil {
		return nil, err
	}

	var parsed map[string]string
	if err := yaml.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse rendered template as YAML: %w", err)
	}

	result := make(map[string][]byte, len(parsed))
	for k, v := range parsed {
		result[k] = []byte(v)
	}

	return result, nil
}
