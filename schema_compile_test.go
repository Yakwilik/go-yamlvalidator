package yamlvalidator_test

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	. "github.com/Yakwilik/go-yamlvalidator"
	keyv "github.com/Yakwilik/go-yamlvalidator/pkg/keyvalidator"
	valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
	"gopkg.in/yaml.v3"
)

func TestCompileFieldSchemaValid(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name": {
				Type:     TypeString,
				Required: true,
				Validators: []ValueValidator{
					valv.RegexValidator{Pattern: regexp.MustCompile(`^[a-z]+$`)},
				},
			},
			"items": {
				Type:       TypeSequence,
				MinItems:   Ptr(1),
				ItemSchema: &FieldSchema{Type: TypeString},
			},
		},
		ExactlyOneOf: []string{"name", "items"},
		KeyValidators: []KeyValidator{
			keyv.LengthKeyValidator{Min: Ptr(1)},
		},
	}
	if _, err := CompileFieldSchema(schema); err != nil {
		t.Fatalf("compile valid schema: %v", err)
	}
}

func TestCompileFieldSchemaRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name   string
		schema *FieldSchema
		part   string
	}{
		{
			name:   "nil regex",
			schema: &FieldSchema{Type: TypeString, Validators: []ValueValidator{valv.RegexValidator{}}},
			part:   "regex pattern must not be nil",
		},
		{
			name:   "inverted range",
			schema: &FieldSchema{Type: TypeInt, Validators: []ValueValidator{valv.RangeValidator{Min: Ptr(10.0), Max: Ptr(1.0)}}},
			part:   "minimum must not exceed maximum",
		},
		{
			name:   "invalid URL scheme definition",
			schema: &FieldSchema{Type: TypeString, Validators: []ValueValidator{valv.URLValidator{AllowedSchemes: []string{"1http"}}}},
			part:   "invalid allowed URL scheme",
		},
		{
			name:   "empty enum definition",
			schema: &FieldSchema{Type: TypeString, Validators: []ValueValidator{valv.EnumValidator{}}},
			part:   "enum must contain at least one allowed value",
		},
		{
			name:   "duplicate enum definition",
			schema: &FieldSchema{Type: TypeString, Validators: []ValueValidator{valv.EnumValidator{Allowed: []string{"x", "x"}}}},
			part:   `duplicate enum value "x"`,
		},
		{
			name: "empty forbidden key definition",
			schema: &FieldSchema{
				Type:          TypeMap,
				KeyValidators: []KeyValidator{keyv.ForbiddenKeyValidator{}},
			},
			part: "forbidden key list must not be empty",
		},
		{
			name:   "inverted length",
			schema: &FieldSchema{Type: TypeString, Validators: []ValueValidator{valv.LengthValidator{Min: Ptr(3), Max: Ptr(2)}}},
			part:   "minimum length must not exceed maximum length",
		},
		{
			name:   "mapping constraints on scalar",
			schema: &FieldSchema{Type: TypeString, AllowedKeys: map[string]*FieldSchema{"x": {Type: TypeString}}},
			part:   "mapping constraints require map/any type",
		},
		{
			name:   "sequence constraints on map",
			schema: &FieldSchema{Type: TypeMap, MinItems: Ptr(1)},
			part:   "sequence constraints require sequence/any type",
		},
		{
			name: "unknown inter-field reference",
			schema: &FieldSchema{
				Type:         TypeMap,
				AllowedKeys:  map[string]*FieldSchema{"a": {Type: TypeString}},
				ExactlyOneOf: []string{"a", "b"},
			},
			part: `field "b" is not declared`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CompileFieldSchema(tt.schema)
			if err == nil || !strings.Contains(err.Error(), tt.part) {
				t.Fatalf("expected error containing %q, got %v", tt.part, err)
			}
		})
	}
}

func TestCompileFieldSchemaRejectsRecursiveGraph(t *testing.T) {
	schema := &FieldSchema{Type: TypeMap}
	schema.AdditionalProperties = schema
	_, err := CompileFieldSchema(schema)
	if err == nil || !strings.Contains(err.Error(), "recursive FieldSchema graph") {
		t.Fatalf("expected recursive graph error, got %v", err)
	}
}

func TestCompileFieldSchemaSnapshotsSchemaGraph(t *testing.T) {
	minItems := 1
	itemSchema := &FieldSchema{Type: TypeString}
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name": {
				Type:     TypeString,
				Required: true,
			},
			"items": {
				Type:       TypeSequence,
				MinItems:   &minItems,
				ItemSchema: itemSchema,
			},
		},
		ExactlyOneOf: []string{"name", "items"},
	}

	compiled, err := CompileFieldSchema(schema)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Mutate every container/pointer shape after compilation.
	schema.AllowedKeys["name"].Type = TypeInt
	schema.AllowedKeys["name"].Required = false
	schema.ExactlyOneOf[0] = "changed"
	*schema.AllowedKeys["items"].MinItems = 100
	itemSchema.Type = TypeInt
	delete(schema.AllowedKeys, "name")

	result := compiled.ValidateBytes([]byte("name: value\n"))
	if result.HasErrors() {
		t.Fatalf("compiled schema changed after source mutation: %v", result.Collector.Errors())
	}
}

func TestCompiledSchemaIndependentOfConcurrentSourceMutation(t *testing.T) {
	child := &FieldSchema{Type: TypeString, Required: true}
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"value": child,
		},
		UnknownKeyPolicy: UnknownKeyError,
	}
	compiled, err := CompileFieldSchema(schema)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			child.Type = TypeInt
			child.Type = TypeString
			schema.UnknownKeyPolicy = UnknownKeyIgnore
			schema.UnknownKeyPolicy = UnknownKeyError
			schema.AllowedKeys["extra"] = &FieldSchema{Type: TypeAny}
			delete(schema.AllowedKeys, "extra")
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			result := compiled.ValidateBytes([]byte(`value: ok`))
			if result.HasErrors() {
				select {
				case errCh <- fmt.Errorf("compiled validator changed: %v", result.Collector.Errors()):
				default:
				}
				return
			}
		}
	}()

	wg.Wait()
	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}
}

func TestCompiledValidatorConcurrentUse(t *testing.T) {
	compiled, err := CompileFieldSchema(&FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name":  {Type: TypeString, Required: true},
			"count": {Type: TypeInt},
		},
		UnknownKeyPolicy: UnknownKeyError,
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	const workers = 32
	const iterations = 200
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	wg.Add(workers)

	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				result := compiled.ValidateBytes([]byte(`name: app
count: 3
`))
				if result.HasErrors() {
					errCh <- fmt.Errorf("concurrent validation failed: %v", result.Collector.Errors())
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestCompileFieldSchemaSnapshotsDefaultContainers(t *testing.T) {
	defaultValue := map[string]any{
		"labels": []any{"one", "two"},
	}
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"settings": {
				Type:    TypeMap,
				Default: defaultValue,
			},
		},
	}
	compiled, err := CompileFieldSchema(schema)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	defaultValue["labels"].([]any)[0] = "changed"
	defaultValue["extra"] = true

	result := compiled.ValidateBytes([]byte("{}"))
	if len(result.Collector.Warnings()) != 1 {
		t.Fatalf("expected one default warning, got %v", result.Collector.All())
	}
	message := result.Collector.Warnings()[0].Message
	if strings.Contains(message, "changed") || strings.Contains(message, "extra") {
		t.Fatalf("compiled default changed after source mutation: %s", message)
	}
}

func TestCompileFieldSchemaSnapshotsBuiltInValidators(t *testing.T) {
	allowed := []string{"ok"}
	minimum := float64(1)
	exactMaximum := valv.MustExactNumber("10")
	forbidden := []string{"bad"}

	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"name": {
				Type: TypeString,
				Validators: []ValueValidator{
					valv.EnumValidator{Allowed: allowed},
				},
			},
			"count": {
				Type: TypeInt,
				Validators: []ValueValidator{
					valv.RangeValidator{Min: &minimum, MaxExact: exactMaximum},
				},
			},
			"bad": {Type: TypeString},
		},
		KeyValidators: []KeyValidator{
			keyv.ForbiddenKeyValidator{Forbidden: forbidden},
		},
	}

	compiled, err := CompileFieldSchema(schema)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	allowed[0] = "changed"
	minimum = 100
	*exactMaximum = *valv.MustExactNumber("2")
	forbidden[0] = "other"

	result := compiled.ValidateBytes([]byte("name: ok\ncount: 10\nbad: value\n"))
	errors := result.Collector.Errors()
	if len(errors) != 1 || errors[0].Code != "forbidden_key" {
		t.Fatalf("built-in validator configuration was not snapshotted: %v", errors)
	}
}

type cloneableValueValidator struct {
	allowed *string
}

func (validator cloneableValueValidator) Validate(
	node *yaml.Node,
	path string,
	ctx *ValidationContext,
) {
	if node.Value != *validator.allowed {
		ctx.AddError(ValidationError{Level: LevelError, Code: "cloneable", Path: path})
	}
}

func (validator cloneableValueValidator) CloneValueValidator() ValueValidator {
	allowed := *validator.allowed
	return cloneableValueValidator{allowed: &allowed}
}

func TestCompileFieldSchemaUsesCustomValidatorCloner(t *testing.T) {
	allowed := "before"
	schema := &FieldSchema{
		Type:       TypeString,
		Validators: []ValueValidator{cloneableValueValidator{allowed: &allowed}},
	}
	compiled, err := CompileFieldSchema(schema)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	allowed = "after"
	if result := compiled.ValidateBytes([]byte("before")); result.HasErrors() {
		t.Fatalf("custom validator cloner was ignored: %v", result.Collector.Errors())
	}
}
