package valuevalidator

import (
	"fmt"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// URLValidator validates that a string is a valid URL (basic checks).
type URLValidator struct {
	RequireScheme  bool     // Require scheme (http/https)
	AllowedSchemes []string // Allowed schemes (empty = any)
}

// Validate implements ValueValidator.
func (vld URLValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	val := node.Value

	// Simple URL validation
	hasScheme := false
	scheme := ""
	if idx := findSchemeEnd(val); idx > 0 {
		hasScheme = true
		scheme = val[:idx]
	}

	if vld.RequireScheme && !hasScheme {
		ctx.AddError(v.ValidationError{
			Level:   v.LevelError,
			Path:    path,
			Line:    node.Line,
			Column:  node.Column,
			Message: "URL must include scheme",
			Got:     val,
		})
		return
	}

	if hasScheme && len(vld.AllowedSchemes) > 0 {
		allowed := false
		for _, s := range vld.AllowedSchemes {
			if scheme == s {
				allowed = true
				break
			}
		}
		if !allowed {
			ctx.AddError(v.ValidationError{
				Level:    v.LevelError,
				Path:     path,
				Line:     node.Line,
				Column:   node.Column,
				Message:  "URL scheme not allowed",
				Got:      scheme,
				Expected: fmt.Sprintf("one of %v", vld.AllowedSchemes),
			})
		}
	}
}

func findSchemeEnd(s string) int {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ':' {
			if i > 0 && i+2 < len(s) && s[i+1] == '/' && s[i+2] == '/' {
				return i
			}
			return -1
		}
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(i > 0 && ((c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'))) {
			return -1
		}
	}
	return -1
}
