// Package template handles rendering of Kubernetes secret data from Go templates.
package template

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// maxRenderedBytes caps the rendered output and every string that the
	// expanding builtins produce. 1 MiB is also the Kubernetes Secret size limit.
	maxRenderedBytes = 1 << 20
	// renderTimeout caps the wall-clock time of one template execution.
	renderTimeout = time.Second
	// guardFunc is the function that execute injects at the start of every
	// list in the parse tree to enforce renderTimeout.
	guardFunc = "_renderGuard"
	// maxFormatWidth is the largest width or precision that fmt accepts.
	maxFormatWidth = 1e6
)

var (
	errOutputLimit = fmt.Errorf("rendered output exceeds %d bytes", maxRenderedBytes)
	errValueLimit  = fmt.Errorf("template value exceeds %d bytes", maxRenderedBytes)
	errDeadline    = fmt.Errorf("template execution exceeded %s deadline", renderTimeout)
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

	out, err := execute(templateStr, data)
	if err != nil {
		return nil, err
	}

	// Parse the rendered template as YAML key: value pairs
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

// ValidateTemplate validates that a template string is valid
func ValidateTemplate(templateStr string) error {
	if templateStr == "" {
		return nil // Empty template uses default
	}

	_, err := template.New("secret").Option("missingkey=error").Parse(templateStr)
	if err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	return nil
}

// execute parses and runs templateStr with bounded output, memory, and time.
//
// text/template has no context support, so the deadline is enforced inside
// the template: a guard call is injected at the start of every list in the
// parse tree. Unbounded work needs a loop (range) or repeated template calls,
// and each iteration or call runs a list, so the guard runs before each one.
// The builtins that can make a string larger than their input are replaced
// with versions that reject results above maxRenderedBytes, because values
// held in variables never reach the output writer.
func execute(templateStr string, data any) ([]byte, error) {
	var deadline time.Time
	tmpl, err := template.New("secret").Option("missingkey=error").Funcs(limitedFuncs(&deadline)).Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	for _, t := range tmpl.Templates() {
		if t.Tree == nil {
			continue
		}
		guardLists(t.Root)
	}

	w := &limitedWriter{}
	deadline = time.Now().Add(renderTimeout)
	if err := tmpl.Execute(w, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}
	return w.buf.Bytes(), nil
}

// limitedWriter buffers output and fails once it would exceed maxRenderedBytes.
type limitedWriter struct {
	buf bytes.Buffer
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > maxRenderedBytes {
		return 0, errOutputLimit
	}
	return w.buf.Write(p)
}

// guardLists prepends a guardFunc call to list and to every list nested in it.
func guardLists(list *parse.ListNode) {
	if list == nil {
		return
	}
	for _, n := range list.Nodes {
		switch n := n.(type) {
		case *parse.IfNode:
			guardLists(n.List)
			guardLists(n.ElseList)
		case *parse.RangeNode:
			guardLists(n.List)
			guardLists(n.ElseList)
		case *parse.WithNode:
			guardLists(n.List)
			guardLists(n.ElseList)
		}
	}
	guard := &parse.ActionNode{
		NodeType: parse.NodeAction,
		Pos:      list.Pos,
		Pipe: &parse.PipeNode{
			NodeType: parse.NodePipe,
			Pos:      list.Pos,
			Cmds: []*parse.CommandNode{{
				NodeType: parse.NodeCommand,
				Pos:      list.Pos,
				Args:     []parse.Node{parse.NewIdentifier(guardFunc).SetPos(list.Pos)},
			}},
		},
	}
	list.Nodes = append([]parse.Node{guard}, list.Nodes...)
}

// limitedFuncs returns the deadline guard and size-limited replacements for
// the builtins that can produce a string larger than their input.
func limitedFuncs(deadline *time.Time) template.FuncMap {
	return template.FuncMap{
		guardFunc: func() (string, error) {
			if time.Now().After(*deadline) {
				return "", errDeadline
			}
			return "", nil
		},
		"print":    limitArgs(fmt.Sprint),
		"println":  limitArgs(fmt.Sprintln),
		"html":     limitArgs(template.HTMLEscaper),
		"js":       limitArgs(template.JSEscaper),
		"urlquery": limitArgs(template.URLQueryEscaper),
		"printf": func(format string, args ...any) (string, error) {
			total, largest, err := argSizes(args)
			if err != nil {
				return "", err
			}
			// Each verb prints at most one argument plus its padding, and
			// unused arguments are printed once in the %!(EXTRA ...) suffix.
			verbs, padding := scanFormat(format)
			if len(format)+padding+verbs*largest+total > maxRenderedBytes {
				return "", errValueLimit
			}
			return limitResult(fmt.Sprintf(format, args...))
		},
	}
}

// limitArgs wraps f so that it rejects inputs and results above maxRenderedBytes.
func limitArgs(f func(...any) string) func(...any) (string, error) {
	return func(args ...any) (string, error) {
		if _, _, err := argSizes(args); err != nil {
			return "", err
		}
		return limitResult(f(args...))
	}
}

// argSizes returns the total and the largest printed size of args.
func argSizes(args []any) (total, largest int, err error) {
	for _, a := range args {
		n := len(fmt.Sprint(a))
		total += n
		largest = max(largest, n)
		if total > maxRenderedBytes {
			return 0, 0, errValueLimit
		}
	}
	return total, largest, nil
}

func limitResult(s string) (string, error) {
	if len(s) > maxRenderedBytes {
		return "", errValueLimit
	}
	return s, nil
}

// scanFormat returns the number of verbs in format and an upper bound on the
// padding that their width and precision fields can add.
func scanFormat(format string) (verbs, padding int) {
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		num := 0
		for i++; i < len(format) && strings.IndexByte("+-# .*[]0123456789", format[i]) >= 0; i++ {
			c := format[i]
			switch {
			case c >= '0' && c <= '9':
				num = min(num*10+int(c-'0'), maxFormatWidth)
			case c == '*':
				padding += maxFormatWidth
			default:
				padding += num
				num = 0
			}
		}
		padding += num
		if i < len(format) && format[i] != '%' {
			verbs++
		}
	}
	return verbs, padding
}
