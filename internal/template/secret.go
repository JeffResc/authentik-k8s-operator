package template

import (
	"bytes"
	"fmt"
	"text/template"
)

// SecretData contains the data available for secret templates
type SecretData struct {
	// ClientID is the OAuth2 client ID
	ClientID string
	// ClientSecret is the OAuth2 client secret
	ClientSecret string
	// IssuerURL is the OIDC issuer URL
	IssuerURL string
	// AuthURL is the OIDC authorization endpoint
	AuthURL string
	// TokenURL is the OIDC token endpoint
	TokenURL string
	// UserInfoURL is the OIDC userinfo endpoint
	UserInfoURL string
	// LogoutURL is the OIDC end session endpoint
	LogoutURL string
	// JWKSURL is the OIDC JWKS endpoint
	JWKSURL string
	// ProviderInfoURL is the .well-known/openid-configuration URL
	ProviderInfoURL string
	// Slug is the application slug
	Slug string
	// Name is the application display name
	Name string
}

// DefaultTemplate is the default secret template
const DefaultTemplate = `client-id: {{ .ClientID }}
client-secret: {{ .ClientSecret }}
issuer-url: {{ .IssuerURL }}
authorization-url: {{ .AuthURL }}
token-url: {{ .TokenURL }}
userinfo-url: {{ .UserInfoURL }}`

// RenderSecretData renders a template string with the given secret data
// Returns a map of key-value pairs for the secret data
func RenderSecretData(templateStr string, data SecretData) (map[string][]byte, error) {
	if templateStr == "" {
		templateStr = DefaultTemplate
	}

	tmpl, err := template.New("secret").Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	// Parse the rendered template as YAML-like key: value pairs
	result := make(map[string][]byte)
	lines := bytes.Split(buf.Bytes(), []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		// Split on first colon
		idx := bytes.IndexByte(line, ':')
		if idx == -1 {
			continue
		}

		key := string(bytes.TrimSpace(line[:idx]))
		value := bytes.TrimSpace(line[idx+1:])

		// Remove surrounding quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		if key != "" {
			result[key] = value
		}
	}

	return result, nil
}

// ValidateTemplate validates that a template string is valid
func ValidateTemplate(templateStr string) error {
	if templateStr == "" {
		return nil // Empty template uses default
	}

	_, err := template.New("secret").Parse(templateStr)
	if err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	return nil
}
