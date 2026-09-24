package valuevalidator

import (
	"fmt"
	"net/url"
	"strings"

	v "github.com/Yakwilik/go-yamlvalidator"
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
		if !validURLScheme(normalized) {
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
	parsed, err := url.Parse(value)
	if err != nil {
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

	if vld.RequireScheme && parsed.Scheme == "" {
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

	if parsed.Scheme == "" || len(vld.AllowedSchemes) == 0 {
		return
	}
	for _, allowed := range vld.AllowedSchemes {
		if strings.EqualFold(parsed.Scheme, allowed) {
			return
		}
	}

	ctx.AddError(v.ValidationError{
		Level:    v.LevelError,
		Code:     "url_scheme",
		Path:     path,
		Line:     node.Line,
		Column:   node.Column,
		Message:  "URL scheme not allowed",
		Got:      parsed.Scheme,
		Expected: fmt.Sprintf("one of %v", vld.AllowedSchemes),
	})
}

func validURLScheme(scheme string) bool {
	if scheme == "" || !isASCIIAlpha(scheme[0]) {
		return false
	}
	for i := 1; i < len(scheme); i++ {
		ch := scheme[i]
		if isASCIIAlpha(ch) || ch >= '0' && ch <= '9' || ch == '+' || ch == '-' || ch == '.' {
			continue
		}
		return false
	}
	return true
}

func isASCIIAlpha(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}
