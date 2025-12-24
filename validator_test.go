package yamlvalidator_test

import (
	"regexp"
	"strings"
	"testing"

	. "github.com/Yakwilik/go-yamlvalidator"
	keyv "github.com/Yakwilik/go-yamlvalidator/pkg/keyvalidator"
	valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
)

func TestBasicTypeValidation(t *testing.T) {
	tests := []struct {
		name       string
		schema     *FieldSchema
		yaml       string
		wantErrors int
	}{
		{
			name:       "valid string",
			schema:     &FieldSchema{Type: TypeString},
			yaml:       `"hello"`,
			wantErrors: 0,
		},
		{
			name:       "valid int",
			schema:     &FieldSchema{Type: TypeInt},
			yaml:       `42`,
			wantErrors: 0,
		},
		{
			name:       "valid float",
			schema:     &FieldSchema{Type: TypeFloat},
			yaml:       `3.14`,
			wantErrors: 0,
		},
		{
			name:       "int as float",
			schema:     &FieldSchema{Type: TypeFloat},
			yaml:       `42`,
			wantErrors: 0,
		},
		{
			name:       "valid bool",
			schema:     &FieldSchema{Type: TypeBool},
			yaml:       `true`,
			wantErrors: 0,
		},
		{
			name:       "type mismatch string vs int",
			schema:     &FieldSchema{Type: TypeInt},
			yaml:       `"hello"`,
			wantErrors: 1,
		},
		{
			name:       "type mismatch int vs string",
			schema:     &FieldSchema{Type: TypeString},
			yaml:       `42`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(tt.schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
				for _, err := range result.Collector.Errors() {
					t.Logf("  error: %s", err)
				}
			}
		})
	}
}

func TestNullableFields(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"required_string": {Type: TypeString, Required: true},
			"nullable_string": {Type: TypeString, Nullable: true},
			"normal_string":   {Type: TypeString},
		},
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name: "null in nullable field",
			yaml: `
required_string: "hello"
nullable_string: null
`,
			wantErrors: 0,
		},
		{
			name: "null in normal field",
			yaml: `
required_string: "hello"
normal_string: null
`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
				for _, err := range result.Collector.Errors() {
					t.Logf("  error: %s", err)
				}
			}
		})
	}
}

func TestRequiredFields(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name":     {Type: TypeString, Required: true},
			"optional": {Type: TypeString},
		},
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name:       "all required present",
			yaml:       `name: "test"`,
			wantErrors: 0,
		},
		{
			name:       "missing required",
			yaml:       `optional: "test"`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
			}
		})
	}
}

func TestUnknownKeys(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"known": {Type: TypeString},
		},
		UnknownKeyPolicy: UnknownKeyInherit,
	}

	yaml := `
known: "value"
unknown: "value"
`

	// With StrictKeys = false -> warning
	t.Run("unknown key as warning", func(t *testing.T) {
		v := NewValidator(schema)
		result := v.ValidateWithOptions([]byte(yaml), ValidationContext{StrictKeys: false})
		if len(result.Collector.Errors()) != 0 {
			t.Errorf("got %d errors, want 0", len(result.Collector.Errors()))
		}
		if len(result.Collector.Warnings()) != 1 {
			t.Errorf("got %d warnings, want 1", len(result.Collector.Warnings()))
		}
	})

	// With StrictKeys = true -> error
	t.Run("unknown key as error", func(t *testing.T) {
		v := NewValidator(schema)
		result := v.ValidateWithOptions([]byte(yaml), ValidationContext{StrictKeys: true})
		if len(result.Collector.Errors()) != 1 {
			t.Errorf("got %d errors, want 1", len(result.Collector.Errors()))
		}
	})
}

func TestAdditionalProperties(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name": {Type: TypeString},
		},
		AdditionalProperties: &FieldSchema{Type: TypeInt},
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name: "additional properties valid",
			yaml: `
name: "test"
count: 42
`,
			wantErrors: 0,
		},
		{
			name: "additional properties invalid type",
			yaml: `
name: "test"
count: "not an int"
`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
			}
		})
	}
}

func TestSequenceValidation(t *testing.T) {
	schema := &FieldSchema{
		Type:       TypeSequence,
		ItemSchema: &FieldSchema{Type: TypeString},
		MinItems:   Ptr[int](1),
		MaxItems:   Ptr[int](3),
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name:       "valid sequence",
			yaml:       `["a", "b"]`,
			wantErrors: 0,
		},
		{
			name:       "too few items",
			yaml:       `[]`,
			wantErrors: 1,
		},
		{
			name:       "too many items",
			yaml:       `["a", "b", "c", "d"]`,
			wantErrors: 1,
		},
		{
			name:       "invalid item type",
			yaml:       `["a", 42]`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
			}
		})
	}
}

func TestMutuallyExclusive(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"debug": {Type: TypeBool},
			"quiet": {Type: TypeBool},
			"name":  {Type: TypeString},
		},
		MutuallyExclusive: []string{"debug", "quiet"},
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name:       "neither present",
			yaml:       `name: "test"`,
			wantErrors: 0,
		},
		{
			name:       "one present",
			yaml:       `debug: true`,
			wantErrors: 0,
		},
		{
			name: "both present",
			yaml: `
debug: true
quiet: true
`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
			}
		})
	}
}

func TestExactlyOneOf(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"inline": {Type: TypeString},
			"file":   {Type: TypeString},
			"url":    {Type: TypeString},
		},
		ExactlyOneOf: []string{"inline", "file", "url"},
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name:       "exactly one present",
			yaml:       `file: "config.yaml"`,
			wantErrors: 0,
		},
		{
			name:       "none present",
			yaml:       `{}`,
			wantErrors: 1,
		},
		{
			name: "two present",
			yaml: `
file: "config.yaml"
url: "http://example.com"
`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
			}
		})
	}
}

func TestAnyOf(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"configFile": {Type: TypeString},
			"host":       {Type: TypeString},
			"port":       {Type: TypeInt},
		},
		AnyOf: [][]string{{"configFile"}, {"host", "port"}},
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name:       "first group present",
			yaml:       `configFile: "config.yaml"`,
			wantErrors: 0,
		},
		{
			name: "second group present",
			yaml: `
host: "localhost"
port: 8080
`,
			wantErrors: 0,
		},
		{
			name:       "partial second group",
			yaml:       `host: "localhost"`,
			wantErrors: 1,
		},
		{
			name:       "none present",
			yaml:       `{}`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
			}
		})
	}
}

func TestConditionalRules(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"type":    {Type: TypeString},
			"remote":  {Type: TypeString},
			"local":   {Type: TypeString},
			"service": {Type: TypeString},
		},
		Conditions: []ConditionalRule{
			{
				ConditionField: "type",
				ConditionValue: "external",
				ThenRequired:   []string{"remote"},
				ThenForbidden:  []string{"local"},
			},
		},
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name: "condition met with required",
			yaml: `
type: "external"
remote: "http://example.com"
`,
			wantErrors: 0,
		},
		{
			name: "condition met without required",
			yaml: `
type: "external"
`,
			wantErrors: 1,
		},
		{
			name: "condition met with forbidden",
			yaml: `
type: "external"
remote: "http://example.com"
local: "/path"
`,
			wantErrors: 1,
		},
		{
			name: "condition not met",
			yaml: `
type: "internal"
local: "/path"
`,
			wantErrors: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
				for _, err := range result.Collector.Errors() {
					t.Logf("  error: %s", err)
				}
			}
		})
	}
}

func TestEnumValidator(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeString,
		Validators: []ValueValidator{
			valv.EnumValidator{Allowed: []string{"v1", "v2", "v3"}},
		},
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name:       "valid enum",
			yaml:       `"v2"`,
			wantErrors: 0,
		},
		{
			name:       "invalid enum",
			yaml:       `"v4"`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
			}
		})
	}
}

func TestRegexValidator(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeString,
		Validators: []ValueValidator{
			valv.RegexValidator{
				Pattern: regexp.MustCompile(`^[a-z][a-z0-9-]*$`),
				Message: "must be lowercase DNS name",
			},
		},
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name:       "valid pattern",
			yaml:       `"my-app-123"`,
			wantErrors: 0,
		},
		{
			name:       "invalid pattern uppercase",
			yaml:       `"MyApp"`,
			wantErrors: 1,
		},
		{
			name:       "invalid pattern starts with number",
			yaml:       `"123-app"`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
			}
		})
	}
}

func TestRangeValidator(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeInt,
		Validators: []ValueValidator{
			valv.RangeValidator{Min: Ptr[float64](1), Max: Ptr[float64](100)},
		},
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name:       "in range",
			yaml:       `50`,
			wantErrors: 0,
		},
		{
			name:       "at min",
			yaml:       `1`,
			wantErrors: 0,
		},
		{
			name:       "at max",
			yaml:       `100`,
			wantErrors: 0,
		},
		{
			name:       "below min",
			yaml:       `0`,
			wantErrors: 1,
		},
		{
			name:       "above max",
			yaml:       `101`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
			}
		})
	}
}

func TestRangeValidatorYAMLNumbers(t *testing.T) {
	t.Run("hex int", func(t *testing.T) {
		schema := &FieldSchema{
			Type: TypeInt,
			Validators: []ValueValidator{
				valv.RangeValidator{Min: Ptr[float64](0), Max: Ptr[float64](100)},
			},
		}
		v := NewValidator(schema)
		result := v.ValidateBytes([]byte("0x10"))
		if len(result.Collector.Errors()) != 0 {
			t.Fatalf("expected hex int accepted, got errors: %v", result.Collector.Errors())
		}
	})

	t.Run("inf float", func(t *testing.T) {
		schema := &FieldSchema{
			Type: TypeFloat,
			Validators: []ValueValidator{
				valv.RangeValidator{},
			},
		}
		v := NewValidator(schema)
		result := v.ValidateBytes([]byte(".inf"))
		if len(result.Collector.Errors()) != 0 {
			t.Fatalf("expected .inf accepted, got errors: %v", result.Collector.Errors())
		}
	})
}

func TestLengthValidatorUnicode(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeString,
		Validators: []ValueValidator{
			valv.LengthValidator{Max: Ptr[int](6)},
		},
	}

	yaml := `"привет"` // 6 runes, 12 bytes

	v := NewValidator(schema)
	result := v.ValidateBytes([]byte(yaml))
	if len(result.Collector.Errors()) != 0 {
		t.Fatalf("expected unicode string accepted by LengthValidator, got %v", result.Collector.Errors())
	}
}

func TestYAML11Booleans(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"value": {Type: TypeBool},
		},
	}

	yaml := []byte("value: yes")

	t.Run("disabled by default", func(t *testing.T) {
		res := NewValidator(schema).ValidateWithOptions(yaml, ValidationContext{StrictKeys: true})
		if len(res.Collector.Errors()) != 1 {
			t.Fatalf("expected 1 error when YAML 1.1 booleans disabled, got %d", len(res.Collector.Errors()))
		}
	})

	t.Run("enabled", func(t *testing.T) {
		values := []string{"yes", "YES", "On", "off", "Y", "N"}
		for _, val := range values {
			val := val
			t.Run(val, func(t *testing.T) {
				res := NewValidator(schema).ValidateWithOptions([]byte("value: "+val), ValidationContext{YAML11Booleans: true, StrictKeys: true})
				if len(res.Collector.Errors()) != 0 {
					t.Fatalf("expected no errors when YAML 1.1 booleans enabled for %q, got %d: %v", val, len(res.Collector.Errors()), res.Collector.Errors())
				}
			})
		}
	})

	t.Run("quoted literals", func(t *testing.T) {
		values := []string{`"yes"`, `'No'`, `"ON"`, "'off'"}
		for _, val := range values {
			val := val
			t.Run(val, func(t *testing.T) {
				res := NewValidator(schema).ValidateWithOptions([]byte("value: "+val), ValidationContext{YAML11Booleans: true, StrictKeys: true})
				if len(res.Collector.Errors()) != 0 {
					t.Fatalf("expected no errors for quoted YAML 1.1 boolean %q when enabled, got %d: %v", val, len(res.Collector.Errors()), res.Collector.Errors())
				}
			})
		}
	})
}

func TestKeyValidator(t *testing.T) {
	schema := &FieldSchema{
		Type:                 TypeMap,
		AdditionalProperties: &FieldSchema{Type: TypeString},
		KeyValidators: []KeyValidator{
			keyv.RegexKeyValidator{
				Pattern: regexp.MustCompile(`^[a-z][a-z0-9._-]*$`),
				Message: "invalid label key",
			},
		},
	}

	tests := []struct {
		name       string
		yaml       string
		wantErrors int
	}{
		{
			name: "valid keys",
			yaml: `
app: "nginx"
version: "1.0"
`,
			wantErrors: 0,
		},
		{
			name: "invalid key uppercase",
			yaml: `
App: "nginx"
`,
			wantErrors: 1,
		},
		{
			name: "invalid key starts with number",
			yaml: `
123-app: "nginx"
`,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(schema)
			result := v.ValidateBytes([]byte(tt.yaml))
			if len(result.Collector.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d", len(result.Collector.Errors()), tt.wantErrors)
			}
		})
	}
}

func TestLengthKeyValidatorUnicode(t *testing.T) {
	schema := &FieldSchema{
		Type:                 TypeMap,
		AdditionalProperties: &FieldSchema{Type: TypeString},
		KeyValidators: []KeyValidator{
			keyv.LengthKeyValidator{Min: Ptr[int](2), Max: Ptr[int](3)},
		},
	}

	yaml := `
ключ: "value"
`
	v := NewValidator(schema)
	result := v.ValidateBytes([]byte(yaml))
	if len(result.Collector.Errors()) != 1 {
		t.Fatalf("expected length error for unicode key, got %v", result.Collector.Errors())
	}
	if got := result.Collector.Errors()[0].Got; got != "4 characters" {
		t.Fatalf("expected rune count in error, got %q", got)
	}
}

func TestMultiDocument(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name": {Type: TypeString, Required: true},
		},
	}

	yaml := `
name: "first"
---
name: "second"
---
missing: "third"
`

	v := NewValidator(schema)
	result := v.ValidateBytes([]byte(yaml))

	if len(result.Collector.Errors()) != 1 {
		t.Errorf("got %d errors, want 1", len(result.Collector.Errors()))
	}
	if !strings.Contains(result.Collector.Errors()[0].Path, "doc[2]") {
		t.Errorf("error should reference doc[2], got: %s", result.Collector.Errors()[0].Path)
	}
}

func TestMultiDocumentPathFormatting(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name": {Type: TypeString, Required: true},
		},
	}

	yaml := `
name: "first"
---
{}
`
	v := NewValidator(schema)
	result := v.ValidateBytes([]byte(yaml))

	if len(result.Collector.Errors()) != 1 {
		t.Fatalf("got %d errors, want 1", len(result.Collector.Errors()))
	}
	if got := result.Collector.Errors()[0].Path; got != "doc[1].name" {
		t.Fatalf("expected path doc[1].name, got %s", got)
	}
}

func TestYAMLAlias(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"defaults": {
				Type: TypeMap,
				AllowedKeys: map[string]*FieldSchema{
					"timeout": {Type: TypeInt},
				},
			},
			"server": {
				Type: TypeMap,
				AllowedKeys: map[string]*FieldSchema{
					"timeout": {Type: TypeInt},
				},
			},
		},
	}

	yaml := `
defaults: &defaults
  timeout: 30
server:
  <<: *defaults
`

	v := NewValidator(schema)
	result := v.ValidateBytes([]byte(yaml))

	if len(result.Collector.Errors()) != 0 {
		t.Errorf("got %d errors, want 0", len(result.Collector.Errors()))
		for _, err := range result.Collector.Errors() {
			t.Logf("  error: %s", err)
		}
	}
}

func TestDeprecatedField(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"newField": {Type: TypeString},
			"oldField": {Type: TypeString, Deprecated: "use newField instead"},
		},
	}

	yaml := `oldField: "value"`

	v := NewValidator(schema)
	result := v.ValidateBytes([]byte(yaml))

	if len(result.Collector.Errors()) != 0 {
		t.Errorf("got %d errors, want 0", len(result.Collector.Errors()))
	}
	if len(result.Collector.Warnings()) != 1 {
		t.Errorf("got %d warnings, want 1", len(result.Collector.Warnings()))
	}
	if !strings.Contains(result.Collector.Warnings()[0].Message, "newField") {
		t.Errorf("warning should mention newField")
	}
}

func TestEmptyStringIsNotNull(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name": {Type: TypeString},
		},
	}

	// Empty quoted string should be valid string, not null
	yaml := `name: ""`

	v := NewValidator(schema)
	result := v.ValidateBytes([]byte(yaml))

	if len(result.Collector.Errors()) != 0 {
		t.Errorf("empty string should be valid, got errors: %v", result.Collector.Errors())
	}
}

func TestRenderLineWithCaret(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		byteCol    int
		wantLine   string
		wantVisual int
	}{
		{
			name:       "no tabs",
			line:       "hello world",
			byteCol:    7,
			wantLine:   "hello world",
			wantVisual: 7,
		},
		{
			name:       "tab at start",
			line:       "\thello",
			byteCol:    2,
			wantLine:   "    hello",
			wantVisual: 5,
		},
		{
			name:       "tab after 2 chars",
			line:       "ab\tcd",
			byteCol:    4,
			wantLine:   "ab  cd",
			wantVisual: 5,
		},
		{
			name:       "tab after 3 chars",
			line:       "abc\td",
			byteCol:    5,
			wantLine:   "abc d",
			wantVisual: 5,
		},
		{
			name:       "unicode cyrillic",
			line:       "привет мир",
			byteCol:    14,
			wantLine:   "привет мир",
			wantVisual: 8,
		},
		{
			name:       "emoji",
			line:       "hello 🎉 world",
			byteCol:    11,
			wantLine:   "hello 🎉 world",
			wantVisual: 8,
		},
		{
			name:       "mixed tabs and unicode",
			line:       "тест\tvalue",
			byteCol:    10,
			wantLine:   "тест    value",
			wantVisual: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLine, gotVisual, _ := RenderLineWithCaret(tt.line, tt.byteCol)
			if gotLine != tt.wantLine {
				t.Fatalf("line mismatch:\n  got:  %q\n  want: %q", gotLine, tt.wantLine)
			}
			if gotVisual != tt.wantVisual {
				t.Fatalf("visual column mismatch: got %d, want %d", gotVisual, tt.wantVisual)
			}
		})
	}
}

func TestStopOnFirstError(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"a": {Type: TypeInt},
			"b": {Type: TypeInt},
			"c": {Type: TypeInt},
		},
	}

	yaml := `
a: "not int"
b: "not int"
c: "not int"
`

	v := NewValidator(schema)
	result := v.ValidateWithOptions([]byte(yaml), ValidationContext{StopOnFirst: true})

	if len(result.Collector.Errors()) != 1 {
		t.Errorf("got %d errors, want 1 (stop on first)", len(result.Collector.Errors()))
	}
}

func TestSortByPositionInterleaved(t *testing.T) {
	collector := NewErrorCollector()
	collector.Add(ValidationError{Level: LevelWarning, Line: 1, Column: 1, Message: "warn first"})
	collector.Add(ValidationError{Level: LevelError, Line: 2, Column: 1, Message: "error second"})
	result := ValidationResult{
		Collector:   collector,
		SourceLines: []string{"line1", "line2"},
	}
	out := result.FormatAll(true)
	firstWarn := strings.Index(out, "warn first")
	firstErr := strings.Index(out, "error second")
	if firstWarn == -1 || firstErr == -1 || firstWarn > firstErr {
		t.Fatalf("expected warning before error after position sort, got output: %s", out)
	}
}

func TestMergeKeysSupported(t *testing.T) {
	serverSchema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"timeout": {Type: TypeInt, Required: true},
			"host":    {Type: TypeString, Required: true},
		},
		UnknownKeyPolicy: UnknownKeyIgnore,
	}
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"defaults": {Type: TypeMap, UnknownKeyPolicy: UnknownKeyIgnore, AdditionalProperties: &FieldSchema{Type: TypeAny}},
			"server":   serverSchema,
		},
		UnknownKeyPolicy: UnknownKeyIgnore,
		AdditionalProperties: &FieldSchema{
			Type: TypeAny,
		},
	}

	yaml := `
defaults: &defaults
  timeout: 30
server:
  <<: *defaults
  host: example.com
`
	v := NewValidator(schema)
	result := v.ValidateBytes([]byte(yaml))
	if len(result.Collector.Errors()) != 0 {
		t.Fatalf("expected merge keys to be honored, got errors: %v", result.Collector.Errors())
	}
}
