package yamlvalidator

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// PathTokenKind identifies one component of a structured validation path.
type PathTokenKind string

const (
	PathTokenProperty PathTokenKind = "property"
	PathTokenIndex    PathTokenKind = "index"
	// PathTokenDocument represents the reserved leading doc[N] prefix used for
	// multi-document YAML diagnostics.
	PathTokenDocument PathTokenKind = "document"
)

// PathToken is one typed component of ValidationError.Path.
type PathToken struct {
	Kind  PathTokenKind
	Name  string
	Index int
}

// RequiredDetails describes a missing required field/property.
type RequiredDetails struct {
	Field string
}

// UnknownKeyDetails describes an unknown mapping key and optional suggestion.
type UnknownKeyDetails struct {
	Key        string
	Suggestion string
}

// TypeMismatchDetails describes a type validation failure.
type TypeMismatchDetails struct {
	Expected []string
	Actual   string
}

// NumericRangeDetails describes a numeric bound failure.
type NumericRangeDetails struct {
	Value string
	Bound string
}

// ItemCountDetails describes a sequence length failure.
type ItemCountDetails struct {
	Actual int
	Bound  int
}

// DependencyDetails describes a field required by another field.
type DependencyDetails struct {
	Field   string
	Trigger string
}

// ParsePathTokens parses the library's display path syntax into typed tokens.
func ParsePathTokens(path string) ([]PathToken, error) {
	if path == "" {
		return nil, nil
	}

	var tokens []PathToken
	i := 0
	if next, index, ok := parseDocumentPrefix(path); ok {
		tokens = append(tokens, PathToken{Kind: PathTokenDocument, Index: index})
		i = next
	}

	for i < len(path) {
		if path[i] == '.' {
			i++
			if i == len(path) {
				return nil, fmt.Errorf("invalid path %q: trailing dot", path)
			}
			continue
		}

		if path[i] == '[' {
			token, next, err := parseBracketPathToken(path, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			i = next
			continue
		}

		start := i
		for i < len(path) && path[i] != '.' && path[i] != '[' {
			i++
		}
		if start == i {
			return nil, fmt.Errorf("invalid path %q at byte %d", path, i)
		}
		tokens = append(tokens, PathToken{
			Kind: PathTokenProperty,
			Name: path[start:i],
		})
	}
	return tokens, nil
}

func parseDocumentPrefix(path string) (next, index int, ok bool) {
	if len(path) < 6 || path[:4] != "doc[" {
		return 0, 0, false
	}
	end := 4
	for end < len(path) && path[end] >= '0' && path[end] <= '9' {
		end++
	}
	if end == 4 || end >= len(path) || path[end] != ']' {
		return 0, 0, false
	}
	value, err := strconv.Atoi(path[4:end])
	if err != nil {
		return 0, 0, false
	}
	return end + 1, value, true
}

func parseBracketPathToken(path string, start int) (PathToken, int, error) {
	if start+1 >= len(path) {
		return PathToken{}, 0, fmt.Errorf("invalid path %q: unterminated bracket", path)
	}

	if strings.HasPrefix(path[start+1:], "doc:") {
		end := start + len("[doc:")
		for end < len(path) && path[end] >= '0' && path[end] <= '9' {
			end++
		}
		if end == start+len("[doc:") || end >= len(path) || path[end] != ']' {
			return PathToken{}, 0, fmt.Errorf("invalid path %q: expected document index", path)
		}
		index, err := strconv.Atoi(path[start+len("[doc:") : end])
		if err != nil {
			return PathToken{}, 0, fmt.Errorf("invalid path %q: %w", path, err)
		}
		return PathToken{Kind: PathTokenDocument, Index: index}, end + 1, nil
	}

	if path[start+1] == '"' {
		end, err := findJSONStringEnd(path, start+1)
		if err != nil {
			return PathToken{}, 0, err
		}
		if end+1 >= len(path) || path[end+1] != ']' {
			return PathToken{}, 0, fmt.Errorf("invalid path %q: expected closing bracket", path)
		}
		var name string
		if err := json.Unmarshal([]byte(path[start+1:end+1]), &name); err != nil {
			return PathToken{}, 0, fmt.Errorf("invalid path %q: %w", path, err)
		}
		return PathToken{Kind: PathTokenProperty, Name: name}, end + 2, nil
	}

	end := start + 1
	for end < len(path) && path[end] >= '0' && path[end] <= '9' {
		end++
	}
	if end == start+1 || end >= len(path) || path[end] != ']' {
		return PathToken{}, 0, fmt.Errorf("invalid path %q: expected numeric index", path)
	}
	index, err := strconv.Atoi(path[start+1 : end])
	if err != nil {
		return PathToken{}, 0, fmt.Errorf("invalid path %q: %w", path, err)
	}
	return PathToken{Kind: PathTokenIndex, Index: index}, end + 1, nil
}

func findJSONStringEnd(path string, quote int) (int, error) {
	escaped := false
	for i := quote + 1; i < len(path); i++ {
		switch {
		case escaped:
			escaped = false
		case path[i] == '\\':
			escaped = true
		case path[i] == '"':
			return i, nil
		}
	}
	return 0, fmt.Errorf("invalid path %q: unterminated quoted property", path)
}

// FormatPathTokens formats typed path tokens using the library's stable display syntax.
func FormatPathTokens(tokens []PathToken) string {
	var result string
	for _, token := range tokens {
		switch token.Kind {
		case PathTokenDocument:
			if result == "" {
				result = fmt.Sprintf("doc[%d]", token.Index)
			} else {
				result += fmt.Sprintf("[doc:%d]", token.Index)
			}
		case PathTokenIndex:
			result += fmt.Sprintf("[%d]", token.Index)
		case PathTokenProperty:
			if isSimplePathSegment(token.Name) {
				if result != "" {
					result += "."
				}
				result += token.Name
				continue
			}
			encoded, _ := json.Marshal(token.Name)
			result += "[" + string(encoded) + "]"
		}
	}
	return result
}

func normalizeDiagnosticPath(err *ValidationError) {
	if len(err.PathTokens) == 0 && err.Path != "" {
		if tokens, parseErr := ParsePathTokens(err.Path); parseErr == nil {
			err.PathTokens = tokens
		}
	}
	if err.Path == "" && len(err.PathTokens) > 0 {
		err.Path = FormatPathTokens(err.PathTokens)
	}
}

func cloneDiagnostics(source []ValidationError) []ValidationError {
	if source == nil {
		return nil
	}
	result := make([]ValidationError, len(source))
	for i, diagnostic := range source {
		result[i] = diagnostic
		result[i].PathTokens = append([]PathToken(nil), diagnostic.PathTokens...)
		switch details := diagnostic.Details.(type) {
		case TypeMismatchDetails:
			details.Expected = append([]string(nil), details.Expected...)
			result[i].Details = details
		}
	}
	return result
}
