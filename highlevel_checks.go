package yamlvalidator

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Yakwilik/go-yamlvalidator/internal/checks"
	"gopkg.in/yaml.v3"
)

func scalarText(v tagValue) (string, error) { return tagScalar(v) }
func addHighLevelError(ctx *ValidationContext, node *yaml.Node, path, code, message string) {
	ctx.AddError(ValidationError{Level: LevelError, Code: code, Path: path, Line: node.Line, Column: node.Column, Message: message})
}

func isNullNode(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Tag == "!!null"
}

type mappingShapeRule struct{}

func (mappingShapeRule) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if !isNullNode(node) && node.Kind != yaml.MappingNode {
		addHighLevelError(ctx, node, path, "type_mismatch", "object group requires a mapping")
	}
}

type byteRepresentationValidator struct{}

func (byteRepresentationValidator) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if isNullNode(node) || node.Kind == yaml.SequenceNode || node.Kind == yaml.ScalarNode && node.Tag == "!!binary" {
		return
	}
	addHighLevelError(ctx, node, path, "bytes_representation", "byte slice requires a sequence or !!binary scalar")
}

type representableValidator struct {
	kind reflect.Kind
	bits int
}

func (v representableValidator) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if isNullNode(node) {
		return
	}
	text := strings.ReplaceAll(node.Value, "_", "")
	var err error
	switch v.kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		_, err = strconv.ParseInt(text, 0, v.bits)
		if err != nil && strings.HasPrefix(text, "0o") {
			_, err = strconv.ParseInt(text[2:], 8, v.bits)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		_, err = strconv.ParseUint(text, 0, v.bits)
		if err != nil && strings.HasPrefix(text, "0o") {
			_, err = strconv.ParseUint(text[2:], 8, v.bits)
		}
	case reflect.Float32, reflect.Float64:
		lower := strings.ToLower(text)
		if lower == ".inf" || lower == "+.inf" || lower == "-.inf" || lower == ".nan" {
			break
		}
		var f float64
		if node.Tag == "!!int" {
			var exact *big.Rat
			exact, _, err = checks.ParseFiniteNumber(text)
			if err == nil {
				f, _ = exact.Float64()
			}
		} else {
			f, err = strconv.ParseFloat(text, v.bits)
		}
		if err == nil && (math.IsInf(f, 0) || v.bits == 32 && math.Abs(f) > math.MaxFloat32) {
			err = fmt.Errorf("float overflow")
		}
	}
	if err != nil {
		addHighLevelError(ctx, node, path, "representability", fmt.Sprintf("value %q is not representable as %s", node.Value, v.kind))
	}
}

type rangeRule struct {
	bound   *big.Rat
	minimum bool
}

func (v rangeRule) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if isNullNode(node) {
		return
	}
	value, err := parseExactBound(tagValue{kind: tagAtom, text: node.Value})
	if err != nil {
		if node.Tag == "!!float" {
			code := "max"
			if v.minimum {
				code = "min"
			}
			addHighLevelError(ctx, node, path, code, "non-finite number cannot satisfy a numeric bound")
		}
		return
	}
	comparison := value.Cmp(v.bound)
	if v.minimum && comparison < 0 {
		addHighLevelError(ctx, node, path, "min", "number below minimum")
	}
	if !v.minimum && comparison > 0 {
		addHighLevelError(ctx, node, path, "max", "number above maximum")
	}
}

func parseExactBound(value tagValue) (*big.Rat, error) {
	if value.kind != tagAtom {
		return nil, fmt.Errorf("expected exact numeric atom")
	}
	number, _, err := checks.ParseFiniteNumber(value.text)
	return number, err
}

type lengthRule struct {
	bound               int
	minimum, properties bool
}

func (v lengthRule) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if isNullNode(node) {
		return
	}
	length := 0
	if v.properties {
		if node.Kind != yaml.MappingNode {
			return
		}
		length = len(expandMappingWithMergesBounded(node, ctx))
		if ctx.IsStopped() {
			return
		}
	} else {
		if node.Kind != yaml.ScalarNode {
			return
		}
		length = checks.RuneLength(node.Value)
	}
	if v.minimum && length < v.bound {
		addHighLevelError(ctx, node, path, "min_length", fmt.Sprintf("length %d is below %d", length, v.bound))
	}
	if !v.minimum && length > v.bound {
		addHighLevelError(ctx, node, path, "max_length", fmt.Sprintf("length %d exceeds %d", length, v.bound))
	}
}

type nonemptyRule struct{}

func (nonemptyRule) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if isNullNode(node) {
		addHighLevelError(ctx, node, path, "nonempty", "value must not be null or empty")
		return
	}
	if node.Kind == yaml.MappingNode {
		pairs := expandMappingWithMergesBounded(node, ctx)
		if ctx.IsStopped() {
			return
		}
		if len(pairs) == 0 {
			addHighLevelError(ctx, node, path, "nonempty", "value must not be empty")
		}
		return
	}
	if node.Kind == yaml.ScalarNode && node.Value == "" || node.Kind == yaml.SequenceNode && len(node.Content) == 0 {
		addHighLevelError(ctx, node, path, "nonempty", "value must not be empty")
	}
}

type enumRule []string

func (v enumRule) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if node.Kind == yaml.ScalarNode && checks.ContainsScalarText(v, node.Value) {
		return
	}
	addHighLevelError(ctx, node, path, "enum", "value is not in enum")
}

type patternRule struct{ re *regexp.Regexp }

func (v patternRule) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if isNullNode(node) {
		return
	}
	if node.Kind == yaml.ScalarNode && !v.re.MatchString(node.Value) {
		addHighLevelError(ctx, node, path, "pattern", "string does not match pattern")
	}
}

type urlRule struct {
	requireScheme bool
	schemes       []string
}

func parseURLRule(value tagValue, hasValue bool) (urlRule, error) {
	out := urlRule{}
	if !hasValue {
		return out, nil
	}
	if value.kind != tagObject {
		return out, fmt.Errorf("url requires a rule object")
	}
	seen := map[string]bool{}
	for _, r := range value.rules {
		if seen[r.key] {
			return out, fmt.Errorf("duplicate url option %q", r.key)
		}
		seen[r.key] = true
		switch r.key {
		case "requireScheme":
			s, e := scalarText(r.value)
			if e != nil {
				return out, e
			}
			if s != "true" && s != "false" {
				return out, fmt.Errorf("requireScheme must be true or false")
			}
			out.requireScheme = s == "true"
		case "schemes":
			s, e := stringList(r.value)
			if e != nil {
				return out, e
			}
			out.schemes = s
		default:
			return out, fmt.Errorf("unknown url option %q", r.key)
		}
	}
	for _, scheme := range out.schemes {
		if !checks.ValidURLScheme(scheme) {
			return out, fmt.Errorf("invalid URL scheme %q", scheme)
		}
	}
	return out, nil
}
func (v urlRule) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if isNullNode(node) {
		return
	}
	if node.Kind != yaml.ScalarNode {
		return
	}
	code, scheme := checks.URLIssue(node.Value, v.requireScheme, v.schemes)
	if code == "url" {
		addHighLevelError(ctx, node, path, "url", "invalid URL")
		return
	}
	if code == "url_scheme" && v.requireScheme && scheme == "" {
		addHighLevelError(ctx, node, path, "url_scheme", "URL scheme is required")
		return
	}
	if code == "url_scheme" {
		addHighLevelError(ctx, node, path, "url_scheme", "URL scheme is not allowed")
	}
}

type uniqueItemsRule struct{}

func (uniqueItemsRule) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if node.Kind != yaml.SequenceNode {
		return
	}
	seen := map[string]bool{}
	for i, item := range node.Content {
		value, err := canonicalNode(item, map[*yaml.Node]bool{}, ctx)
		if err != nil {
			if !ctx.IsStopped() {
				addHighLevelError(ctx, item, fmt.Sprintf("%s[%d]", path, i), "unique_items", err.Error())
			}
			return
		}
		if seen[value] {
			addHighLevelError(ctx, item, fmt.Sprintf("%s[%d]", path, i), "unique_items", "sequence item is duplicated")
			return
		}
		seen[value] = true
	}
}

// canonicalNode compares YAML representation, including explicit tags, while
// normalizing standard numeric scalars and effective merged mappings.
func canonicalNode(node *yaml.Node, visiting map[*yaml.Node]bool, ctx *ValidationContext) (string, error) {
	if ctx.IsStopped() || !ctx.visitNode(node, "") {
		return "", fmt.Errorf("YAML node visit limit reached")
	}
	if node.Kind == yaml.AliasNode {
		if node.Alias == nil {
			return "", fmt.Errorf("unresolved alias")
		}
		node = node.Alias
	}
	if visiting[node] {
		return "", fmt.Errorf("cyclic alias")
	}
	visiting[node] = true
	defer delete(visiting, node)
	prefix := strconv.Quote(node.Tag) + ":"
	switch node.Kind {
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!null":
			return prefix + "null", nil
		case "!!bool":
			return prefix + strings.ToLower(node.Value), nil
		case "!!int":
			if number, err := normalizeYAMLInteger(node.Value); err == nil {
				return "number:" + number, nil
			}
		case "!!float":
			if number, err := parseExactBound(tagValue{kind: tagAtom, text: node.Value}); err == nil {
				return "number:" + number.RatString(), nil
			}
			lower := strings.ToLower(node.Value)
			if strings.Contains(lower, ".nan") {
				return prefix + "nan", nil
			}
			if strings.Contains(lower, ".inf") {
				if strings.HasPrefix(lower, "+") {
					lower = lower[1:]
				}
				return prefix + lower, nil
			}
		}
		return prefix + strconv.Quote(node.Value), nil
	case yaml.SequenceNode:
		var b strings.Builder
		b.WriteString(prefix)
		b.WriteByte('[')
		for _, child := range node.Content {
			value, err := canonicalNode(child, visiting, ctx)
			if err != nil {
				return "", err
			}
			b.WriteString(strconv.Quote(value))
			b.WriteByte(',')
		}
		b.WriteByte(']')
		return b.String(), nil
	case yaml.MappingNode:
		type kv struct{ key, value string }
		var parts []kv
		for _, pair := range expandMappingWithMergesBounded(node, ctx) {
			key, err := canonicalNode(pair.key, visiting, ctx)
			if err != nil {
				return "", err
			}
			value, err := canonicalNode(pair.value, visiting, ctx)
			if err != nil {
				return "", err
			}
			parts = append(parts, kv{key, value})
		}
		if ctx.IsStopped() {
			return "", fmt.Errorf("YAML node visit limit reached")
		}
		sort.Slice(parts, func(i, j int) bool {
			if parts[i].key == parts[j].key {
				return parts[i].value < parts[j].value
			}
			return parts[i].key < parts[j].key
		})
		var b strings.Builder
		b.WriteString(prefix)
		b.WriteByte('{')
		for _, pair := range parts {
			b.WriteString(strconv.Quote(pair.key))
			b.WriteByte(':')
			b.WriteString(strconv.Quote(pair.value))
			b.WriteByte(',')
		}
		b.WriteByte('}')
		return b.String(), nil
	}
	return "", fmt.Errorf("unsupported YAML node in uniqueItems")
}

type keyPatternRule struct{ re *regexp.Regexp }

func (v keyPatternRule) ValidateKey(key string, node *yaml.Node, path string, ctx *ValidationContext) {
	if !v.re.MatchString(key) {
		addHighLevelError(ctx, node, path, "key_pattern", "key does not match pattern")
	}
}

type keyLengthRule struct {
	bound   int
	minimum bool
}

func (v keyLengthRule) ValidateKey(key string, node *yaml.Node, path string, ctx *ValidationContext) {
	length := checks.RuneLength(key)
	if v.minimum && length < v.bound {
		addHighLevelError(ctx, node, path, "key_min_length", "key is too short")
	}
	if !v.minimum && length > v.bound {
		addHighLevelError(ctx, node, path, "key_max_length", "key is too long")
	}
}

func (c *highLevelCompiler) valueCheck(value tagValue) (ValueValidator, error) {
	if value.kind == tagObject {
		name, args, err := parseFactoryDeclaration(value)
		if err != nil {
			return nil, err
		}
		if c.registry == nil || c.registry.valueFactories[name] == nil {
			return nil, fmt.Errorf("unknown value factory %q", name)
		}
		validator, err := c.registry.valueFactories[name](args)
		if err != nil {
			return nil, err
		}
		if nilInterface(validator) {
			return nil, fmt.Errorf("factory %q returned nil", name)
		}
		if d, ok := validator.(DefinitionValidator); ok {
			if err := d.ValidateDefinition(); err != nil {
				return nil, fmt.Errorf("factory %q: %w", name, err)
			}
		}
		return validator, nil
	}
	name, err := scalarText(value)
	if err != nil {
		return nil, err
	}
	if c.registry == nil || c.registry.values[name] == nil {
		return nil, fmt.Errorf("unknown value validator %q", name)
	}
	return c.registry.values[name], nil
}
