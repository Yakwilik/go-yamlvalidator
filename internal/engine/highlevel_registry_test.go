package yamlvalidator

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

type exactTextCheck struct{ want string }

func (c exactTextCheck) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	if node.Value != c.want {
		ctx.AddError(ValidationError{Level: LevelError, Code: "exact_text", Path: path, Message: "unexpected text"})
	}
}

func TestHighLevelRegistryFactoryReentrantCompile(t *testing.T) {
	type config struct {
		Value string `yaml:"value" yamlvalidate:"check={name=again}"`
	}
	var registry *Registry
	var err error
	registry, err = NewRegistry(RegistryConfig{ValueFactories: map[string]ValueValidatorFactory{"again": func(map[string]any) (ValueValidator, error) {
		_, err := compileHighLevel(reflect.TypeFor[config](), false, registry)
		if err == nil {
			return nil, fmt.Errorf("recursive compile was accepted")
		}
		return nil, err
	}}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		var out config
		done <- (UnmarshalOptions{Registry: registry}).Unmarshal([]byte("value: x"), &out)
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("reentrant compilation succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reentrant compilation deadlocked")
	}
}

type prefixKeyCheck struct{ prefix string }

func (c prefixKeyCheck) ValidateKey(key string, node *yaml.Node, path string, ctx *ValidationContext) {
	if len(key) < len(c.prefix) || key[:len(c.prefix)] != c.prefix {
		ctx.AddError(ValidationError{Level: LevelError, Code: "prefix_key", Path: path, Message: "unexpected key prefix"})
	}
}

func TestHighLevelRegistryFactoriesAndReferences(t *testing.T) {
	registry, err := NewRegistry(RegistryConfig{
		ValueFactories: map[string]ValueValidatorFactory{"equals": func(args map[string]any) (ValueValidator, error) {
			value, ok := args["value"].(json.Number)
			if len(args) != 4 || !ok || string(value) != "9007199254740993" || args["enabled"] != true {
				return nil, fmt.Errorf("bad structured args: %#v", args)
			}
			labels, ok := args["labels"].([]any)
			if !ok || len(labels) != 1 || labels[0] != "a,b" {
				return nil, fmt.Errorf("bad labels: %#v", args)
			}
			meta, ok := args["meta"].(map[string]any)
			if !ok || meta["tier"] != "prod" {
				return nil, fmt.Errorf("bad nested args: %#v", args)
			}
			return exactTextCheck{want: string(value)}, nil
		}},
		KeyFactories: map[string]KeyValidatorFactory{"prefix": func(args map[string]any) (KeyValidator, error) {
			value, ok := args["value"].(string)
			if len(args) != 1 || !ok {
				return nil, fmt.Errorf("bad key args: %#v", args)
			}
			return prefixKeyCheck{prefix: value}, nil
		}},
		Schemas: map[string]*FieldSchema{"name": {Type: TypeString, Validators: []ValueValidator{exactTextCheck{want: "9007199254740993"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	type config struct {
		Value   string            `yaml:"value" yamlvalidate:"check={name=equals,args={value=9007199254740993,enabled=true,labels=['a,b'],meta={tier=prod}}},ref=name"`
		Entries map[string]string `yaml:"entries" yamlvalidate:"keys={check={name=prefix,args={value='12'}}}"`
	}
	var got config
	opts := UnmarshalOptions{Registry: registry}
	if err := opts.Unmarshal([]byte("value: '9007199254740993'\nentries:\n  12key: ok\n"), &got); err != nil {
		t.Fatal(err)
	}
	if !diagnosticCode(opts.Unmarshal([]byte("value: wrong\nentries:\n  bad: ok\n"), &got), "prefix_key") {
		t.Fatal("key factory rule did not run")
	}
}

func TestHighLevelRegistryRejectsInvalidDefinitions(t *testing.T) {
	cases := []struct {
		name   string
		config RegistryConfig
	}{
		{"empty value name", RegistryConfig{ValueValidators: map[string]ValueValidator{"": exactTextCheck{}}}},
		{"nil value", RegistryConfig{ValueValidators: map[string]ValueValidator{"x": nil}}},
		{"empty key name", RegistryConfig{KeyValidators: map[string]KeyValidator{"": prefixKeyCheck{}}}},
		{"nil key", RegistryConfig{KeyValidators: map[string]KeyValidator{"x": nil}}},
		{"nil value factory", RegistryConfig{ValueFactories: map[string]ValueValidatorFactory{"x": nil}}},
		{"nil key factory", RegistryConfig{KeyFactories: map[string]KeyValidatorFactory{"x": nil}}},
		{"empty schema name", RegistryConfig{Schemas: map[string]*FieldSchema{"": {Type: TypeString}}}},
		{"invalid schema", RegistryConfig{Schemas: map[string]*FieldSchema{"x": {Type: NodeType(255)}}}},
		{"nil binding type", RegistryConfig{TypeBindings: map[reflect.Type]TypeBinding{nil: {}}}},
		{"invalid encode binding", RegistryConfig{TypeBindings: map[reflect.Type]TypeBinding{reflect.TypeFor[string](): {Encode: &FieldSchema{Type: NodeType(255)}}}}},
		{"invalid decode binding", RegistryConfig{TypeBindings: map[reflect.Type]TypeBinding{reflect.TypeFor[string](): {Decode: &FieldSchema{Type: NodeType(255)}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewRegistry(tc.config); err == nil {
				t.Fatal("invalid registry accepted")
			}
		})
	}
}
