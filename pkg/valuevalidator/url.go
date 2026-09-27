package valuevalidator

import (
	"fmt"
	"strings"

	v "github.com/Yakwilik/go-yamlvalidator"
	"github.com/Yakwilik/go-yamlvalidator/internal/checks"
	"gopkg.in/yaml.v3"
)

// URLValidator validates URL syntax and optionally constrains the scheme.
// Relative references are allowed unless RequireScheme is true.
type URLValidator struct {
	RequireScheme  bool
	AllowedSchemes []string // Empty means any syntactically valid scheme.
}

func (vld URLValidator) ValidateDefinition() error {
	seen := make(map[string]bool, len(vld.AllowedSchemes))
	for _, scheme := range vld.AllowedSchemes {
		normalized := strings.ToLower(scheme)
		if !checks.ValidURLScheme(normalized) {
			return fmt.Errorf("invalid allowed URL scheme %q", scheme)
		}
		if seen[normalized] {
			return fmt.Errorf("duplicate allowed URL scheme %q", scheme)
		}
		seen[normalized] = true
	}
	return nil
}

// Validate implements ValueValidator.
func (vld URLValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	value := node.Value
	issue, scheme := checks.URLIssue(value, vld.RequireScheme, vld.AllowedSchemes)
	if issue == "url" {
		ctx.AddError(v.ValidationError{
			Level:   v.LevelError,
			Code:    "url",
			Path:    path,
			Line:    node.Line,
			Column:  node.Column,
			Message: "invalid URL",
			Got:     value,
		})
		return
	}

	if issue == "url_scheme" && vld.RequireScheme && scheme == "" {
		ctx.AddError(v.ValidationError{
			Level:   v.LevelError,
			Code:    "url_scheme",
			Path:    path,
			Line:    node.Line,
			Column:  node.Column,
			Message: "URL must include scheme",
			Got:     value,
		})
		return
	}

	if issue != "url_scheme" {
		return
	}
	ctx.AddError(v.ValidationError{
		Level:    v.LevelError,
		Code:     "url_scheme",
		Path:     path,
		Line:     node.Line,
		Column:   node.Column,
		Message:  "URL scheme not allowed",
		Got:      scheme,
		Expected: fmt.Sprintf("one of %v", vld.AllowedSchemes),
	})
}

// Kept for package-internal compatibility tests; the calculation lives in checks.
func validURLScheme(scheme string) bool { return checks.ValidURLScheme(scheme) }
func isASCIIAlpha(ch byte) bool         { return checks.ValidURLScheme(string([]byte{ch})) }
