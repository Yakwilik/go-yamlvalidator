package checks

import (
	"net/url"
	"strings"
)

// URLIssue reports a stable reason code and parsed scheme for a URL rule.
func URLIssue(value string, requireScheme bool, allowed []string) (string, string) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "url", ""
	}
	if requireScheme && parsed.Scheme == "" {
		return "url_scheme", ""
	}
	if parsed.Scheme == "" || len(allowed) == 0 {
		return "", parsed.Scheme
	}
	for _, scheme := range allowed {
		if strings.EqualFold(parsed.Scheme, scheme) {
			return "", parsed.Scheme
		}
	}
	return "url_scheme", parsed.Scheme
}

// ValidURLScheme checks the grammar used for configured allowed schemes.
func ValidURLScheme(scheme string) bool {
	if scheme == "" || !asciiAlpha(scheme[0]) {
		return false
	}
	for i := 1; i < len(scheme); i++ {
		ch := scheme[i]
		if asciiAlpha(ch) || ch >= '0' && ch <= '9' || ch == '+' || ch == '-' || ch == '.' {
			continue
		}
		return false
	}
	return true
}
func asciiAlpha(ch byte) bool { return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' }

// ContainsScalarText implements the native enum's textual comparison.
func ContainsScalarText(allowed []string, value string) bool {
	for _, item := range allowed {
		if item == value {
			return true
		}
	}
	return false
}
