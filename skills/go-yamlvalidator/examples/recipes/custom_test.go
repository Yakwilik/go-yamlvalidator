package recipes_test

import (
	"fmt"
	"strings"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

// Application-defined function adapter, not an exported library type.
type valueCheck func(*yaml.Node, string, *v.ValidationContext)

func (f valueCheck) Validate(n *yaml.Node, path string, ctx *v.ValidationContext) { f(n, path, ctx) }

type reservedValues struct{ names []string }

func (r *reservedValues) ValidateDefinition() error {
	if len(r.names) == 0 {
		return fmt.Errorf("reserved values must be configured")
	}
	return nil
}
func (r *reservedValues) CloneValueValidator() v.ValueValidator {
	return &reservedValues{names: append([]string(nil), r.names...)}
}
func (r *reservedValues) Validate(n *yaml.Node, path string, ctx *v.ValidationContext) {
	if ctx.IsStopped() || n.Tag == "!!null" {
		return
	}
	for _, name := range r.names {
		if n.Value == name {
			ctx.AddError(v.ValidationError{Level: v.LevelError, Code: "reserved_name", Path: path, Line: n.Line, Column: n.Column, Message: "name is reserved"})
			return
		}
	}
}

type privateKeys struct{}

func (privateKeys) ValidateKey(key string, n *yaml.Node, path string, ctx *v.ValidationContext) {
	if strings.HasPrefix(key, "_") {
		ctx.AddError(v.ValidationError{Level: v.LevelError, Code: "private_key", Path: path, Line: n.Line, Column: n.Column, Message: "private key is not allowed"})
	}
}
func (privateKeys) CloneKeyValidator() v.KeyValidator { return privateKeys{} }

func TestCustomNativeCallbacks(t *testing.T) {
	original := &reservedValues{names: []string{"system"}}
	schema := &v.FieldSchema{
		Type: v.TypeMap, UnknownKeyPolicy: v.UnknownKeyError,
		KeyValidators: []v.KeyValidator{privateKeys{}},
		AllowedKeys: map[string]*v.FieldSchema{
			"name": {Type: v.TypeString, Nullable: true, Validators: []v.ValueValidator{original}},
			"note": {Type: v.TypeString, Validators: []v.ValueValidator{valueCheck(func(n *yaml.Node, path string, ctx *v.ValidationContext) {
				if strings.Contains(n.Value, "\n") {
					ctx.AddError(v.ValidationError{Level: v.LevelError, Code: "single_line", Path: path, Line: n.Line, Column: n.Column, Message: "note must be one line"})
				}
			})}},
		},
	}
	checked, err := v.CompileFieldSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	original.names[0] = "changed"
	checkInputs(t, checked, v.ValidationContext{}, []inputCase{
		{name: "valid", input: "name: api\nnote: short", valid: true},
		{name: "snapshot still rejects original", input: "name: system", code: "reserved_name"},
		{name: "nullable", input: "name: null", valid: true},
		{name: "private key", input: "_name: api", code: "private_key"},
		{name: "function", input: "note: |\n  two\n  lines\n", code: "single_line"},
	})
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "system", Line: 2, Column: 7}
	ctx := v.NewValidationContext()
	(&reservedValues{names: []string{"system"}}).Validate(node, "name", ctx)
	if !ctx.Collector().HasErrors() {
		t.Fatal("direct callback did not run")
	}
}
