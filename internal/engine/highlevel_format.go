package yamlvalidator

import (
	"fmt"
	"net/netip"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

type nativeFormatRule struct {
	name     string
	validate func(string) error
}

func (v nativeFormatRule) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if isNullNode(node) || node.Kind != yaml.ScalarNode {
		return
	}
	if err := v.validate(node.Value); err != nil {
		addHighLevelError(ctx, node, path, "format", fmt.Sprintf("invalid %s: %v", v.name, err))
	}
}

type nativeKeyFormatRule struct{ nativeFormatRule }

func (v nativeKeyFormatRule) ValidateKey(key string, node *yaml.Node, path string, ctx *ValidationContext) {
	if err := v.validate(key); err != nil {
		addHighLevelError(ctx, node, path, "key_format", fmt.Sprintf("invalid %s key: %v", v.name, err))
	}
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func nativeFormat(name string) (nativeFormatRule, error) {
	wrap := func(fn func(any) error) func(string) error { return func(s string) error { return fn(s) } }
	var fn func(string) error
	switch name {
	case "hostname":
		fn = wrap(validateJSONSchemaHostname)
	case "email":
		fn = wrap(validateJSONSchemaEmail)
	case "idn-hostname":
		fn = wrap(validateJSONSchemaIDNHostname)
	case "idn-email":
		fn = wrap(validateJSONSchemaIDNEmail)
	case "ipv4":
		fn = wrap(validateJSONSchemaIPv4)
	case "ipv6":
		fn = func(s string) error {
			a, e := netip.ParseAddr(s)
			if e != nil || !a.Is6() {
				return fmt.Errorf("invalid IPv6 address")
			}
			return nil
		}
	case "uri":
		fn = wrap(validateJSONSchemaURI)
	case "uri-reference":
		fn = wrap(validateJSONSchemaURIReference)
	case "uri-template":
		fn = wrap(validateJSONSchemaURITemplate)
	case "duration":
		fn = wrap(validateJSONSchemaDuration)
	case "uuid":
		fn = func(s string) error {
			if !uuidPattern.MatchString(s) {
				return fmt.Errorf("invalid UUID")
			}
			return nil
		}
	case "date-time":
		fn = func(s string) error { _, e := time.Parse(time.RFC3339Nano, s); return e }
	case "date":
		fn = func(s string) error { _, e := time.Parse("2006-01-02", s); return e }
	case "time":
		fn = func(s string) error { _, e := time.Parse("15:04:05Z07:00", s); return e }
	default:
		return nativeFormatRule{}, fmt.Errorf("unknown format %q", name)
	}
	return nativeFormatRule{name: name, validate: fn}, nil
}
