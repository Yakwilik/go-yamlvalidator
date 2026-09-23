package yamlvalidator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// JSONSchemaCompileOptions controls translation from JSON Schema semantics to
// FieldSchema semantics.
type JSONSchemaCompileOptions struct {
	// AdditionalPropertiesFalsePolicy overrides the default error severity used
	// for JSON Schema's additionalProperties: false. Set it to UnknownKeyWarn
	// when callers want schema-strict structure with warning-only unknown keys.
	AdditionalPropertiesFalsePolicy *UnknownKeyPolicy
}

// CompileJSONSchema compiles a supported JSON Schema document into FieldSchema.
// The supported subset intentionally covers EasyP's generated configuration
// schema. Unsupported schema keywords return an error instead of being ignored.
func CompileJSONSchema(data []byte) (*FieldSchema, error) {
	return CompileJSONSchemaWithOptions(data, JSONSchemaCompileOptions{})
}

// CompileJSONSchemaWithOptions is CompileJSONSchema with translation options.
func CompileJSONSchemaWithOptions(data []byte, opts JSONSchemaCompileOptions) (*FieldSchema, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode JSON Schema: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode JSON Schema: multiple JSON values")
		}
		return nil, fmt.Errorf("decode JSON Schema trailing data: %w", err)
	}

	compiler := jsonSchemaCompiler{opts: opts}
	return compiler.compile(raw, "$")
}

type jsonSchemaCompiler struct {
	opts JSONSchemaCompileOptions
}

var supportedJSONSchemaKeywords = map[string]struct{}{
	"$schema": {}, "$id": {}, "$comment": {},
	"title": {}, "description": {}, "default": {}, "examples": {}, "deprecated": {},
	"type": {}, "properties": {}, "additionalProperties": {}, "required": {},
	"items": {}, "minItems": {}, "maxItems": {},
	"oneOf": {}, "anyOf": {}, "not": {}, "dependentRequired": {},
}

func (c jsonSchemaCompiler) compile(raw any, path string) (*FieldSchema, error) {
	switch value := raw.(type) {
	case bool:
		if value {
			return &FieldSchema{Type: TypeAny}, nil
		}
		return &FieldSchema{Type: TypeAny, Validators: []ValueValidator{rejectJSONSchemaValidator{}}}, nil
	case map[string]any:
		return c.compileObject(value, path)
	default:
		return nil, fmt.Errorf("%s: schema must be an object or boolean", path)
	}
}

func (c jsonSchemaCompiler) compileObject(raw map[string]any, path string) (*FieldSchema, error) {
	for keyword := range raw {
		if _, ok := supportedJSONSchemaKeywords[keyword]; !ok {
			return nil, fmt.Errorf("%s: unsupported JSON Schema keyword %q", path, keyword)
		}
	}

	schema := &FieldSchema{Type: TypeAny}
	if description, ok := raw["description"].(string); ok {
		schema.Description = description
	}
	if value, ok := raw["default"]; ok {
		schema.Default = value
	}
	if deprecated, ok := raw["deprecated"].(bool); ok && deprecated {
		schema.Deprecated = "this field is deprecated"
	}

	if rawType, ok := raw["type"]; ok {
		switch value := rawType.(type) {
		case string:
			nodeType, err := nodeTypeFromJSONSchema(value)
			if err != nil {
				return nil, fmt.Errorf("%s.type: %w", path, err)
			}
			schema.Type = nodeType
		case []any:
			if len(value) == 0 {
				return nil, fmt.Errorf("%s.type: expected non-empty type array", path)
			}
			schema.Type = TypeAny
			for i, item := range value {
				typeName, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("%s.type[%d]: expected string", path, i)
				}
				nodeType, err := nodeTypeFromJSONSchema(typeName)
				if err != nil {
					return nil, fmt.Errorf("%s.type[%d]: %w", path, i, err)
				}
				schema.AllowedTypes = append(schema.AllowedTypes, nodeType)
			}
		default:
			return nil, fmt.Errorf("%s.type: expected string or array of strings", path)
		}
	}

	propertiesRaw, hasProperties := raw["properties"]
	if hasProperties {
		properties, ok := propertiesRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.properties: expected object", path)
		}
		schema.AllowedKeys = make(map[string]*FieldSchema, len(properties))
		for name, childRaw := range properties {
			child, err := c.compile(childRaw, path+".properties."+name)
			if err != nil {
				return nil, err
			}
			schema.AllowedKeys[name] = child
		}
	}

	if requiredRaw, ok := raw["required"]; ok {
		required, err := stringArray(requiredRaw, path+".required")
		if err != nil {
			return nil, err
		}
		for _, name := range required {
			child := schema.AllowedKeys[name]
			if child == nil {
				return nil, fmt.Errorf("%s.required: property %q is not declared in properties", path, name)
			}
			child.Required = true
		}
	}

	if additionalRaw, ok := raw["additionalProperties"]; ok {
		switch additional := additionalRaw.(type) {
		case bool:
			if additional {
				schema.AdditionalProperties = &FieldSchema{Type: TypeAny}
			} else {
				policy := UnknownKeyError
				if c.opts.AdditionalPropertiesFalsePolicy != nil {
					policy = *c.opts.AdditionalPropertiesFalsePolicy
				}
				schema.UnknownKeyPolicy = policy
			}
		case map[string]any:
			compiled, err := c.compile(additional, path+".additionalProperties")
			if err != nil {
				return nil, err
			}
			schema.AdditionalProperties = compiled
		default:
			return nil, fmt.Errorf("%s.additionalProperties: expected boolean or schema", path)
		}
	} else if schema.Type == TypeMap || containsNodeType(schema.AllowedTypes, TypeMap) || hasProperties {
		// JSON Schema allows additional properties by default.
		schema.AdditionalProperties = &FieldSchema{Type: TypeAny}
	}

	if itemsRaw, ok := raw["items"]; ok {
		itemSchema, err := c.compile(itemsRaw, path+".items")
		if err != nil {
			return nil, err
		}
		schema.ItemSchema = itemSchema
	}
	if minRaw, ok := raw["minItems"]; ok {
		value, err := nonNegativeInt(minRaw, path+".minItems")
		if err != nil {
			return nil, err
		}
		schema.MinItems = Ptr(value)
	}
	if maxRaw, ok := raw["maxItems"]; ok {
		value, err := nonNegativeInt(maxRaw, path+".maxItems")
		if err != nil {
			return nil, err
		}
		schema.MaxItems = Ptr(value)
	}
	if schema.MinItems != nil && schema.MaxItems != nil && *schema.MinItems > *schema.MaxItems {
		return nil, fmt.Errorf("%s: minItems must not exceed maxItems", path)
	}

	if oneOfRaw, ok := raw["oneOf"]; ok {
		branches, err := schemaArray(oneOfRaw, path+".oneOf")
		if err != nil {
			return nil, err
		}
		if groups, ok, err := requiredGroups(branches, path+".oneOf"); err != nil {
			return nil, err
		} else if ok {
			schema.OneOfRequired = groups
		} else {
			for i, branchRaw := range branches {
				branch, err := c.compile(branchRaw, fmt.Sprintf("%s.oneOf[%d]", path, i))
				if err != nil {
					return nil, err
				}
				schema.OneOfSchemas = append(schema.OneOfSchemas, branch)
			}
		}
	}

	if anyOfRaw, ok := raw["anyOf"]; ok {
		branches, err := schemaArray(anyOfRaw, path+".anyOf")
		if err != nil {
			return nil, err
		}
		if groups, ok, err := requiredGroups(branches, path+".anyOf"); err != nil {
			return nil, err
		} else if ok {
			schema.AnyOf = groups
		} else {
			for i, branchRaw := range branches {
				branch, err := c.compile(branchRaw, fmt.Sprintf("%s.anyOf[%d]", path, i))
				if err != nil {
					return nil, err
				}
				schema.AnyOfSchemas = append(schema.AnyOfSchemas, branch)
			}
		}
	}

	if notRaw, ok := raw["not"]; ok {
		group, ok, err := requiredOnlySchema(notRaw, path+".not")
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%s.not: only a schema containing required is currently supported", path)
		}
		schema.ForbiddenTogether = append(schema.ForbiddenTogether, group)
	}

	if dependentRaw, ok := raw["dependentRequired"]; ok {
		dependent, ok := dependentRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.dependentRequired: expected object", path)
		}
		schema.DependentRequired = make(map[string][]string, len(dependent))
		for trigger, requiredRaw := range dependent {
			required, err := stringArray(requiredRaw, path+".dependentRequired."+trigger)
			if err != nil {
				return nil, err
			}
			schema.DependentRequired[trigger] = required
		}
	}

	return schema, nil
}

func containsNodeType(types []NodeType, target NodeType) bool {
	for _, nodeType := range types {
		if nodeType == target {
			return true
		}
	}
	return false
}

func nodeTypeFromJSONSchema(value string) (NodeType, error) {
	switch strings.ToLower(value) {
	case "null":
		return TypeNull, nil
	case "string":
		return TypeString, nil
	case "integer":
		return TypeInt, nil
	case "number":
		return TypeFloat, nil
	case "boolean":
		return TypeBool, nil
	case "object":
		return TypeMap, nil
	case "array":
		return TypeSequence, nil
	default:
		return TypeAny, fmt.Errorf("unsupported JSON Schema type %q", value)
	}
}

func schemaArray(raw any, path string) ([]any, error) {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 {
		return nil, fmt.Errorf("%s: expected non-empty array", path)
	}
	return values, nil
}

func requiredGroups(branches []any, path string) ([][]string, bool, error) {
	groups := make([][]string, 0, len(branches))
	for i, branch := range branches {
		group, ok, err := requiredOnlySchema(branch, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, nil
		}
		groups = append(groups, group)
	}
	return groups, true, nil
}

func requiredOnlySchema(raw any, path string) ([]string, bool, error) {
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, false, nil
	}
	if len(object) != 1 {
		return nil, false, nil
	}
	requiredRaw, ok := object["required"]
	if !ok {
		return nil, false, nil
	}
	required, err := stringArray(requiredRaw, path+".required")
	if err != nil {
		return nil, false, err
	}
	if len(required) == 0 {
		return nil, false, fmt.Errorf("%s.required: expected non-empty array", path)
	}
	return required, true, nil
}

func stringArray(raw any, path string) ([]string, error) {
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected array", path)
	}
	out := make([]string, 0, len(values))
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: expected string", path, i)
		}
		out = append(out, text)
	}
	return out, nil
}

func nonNegativeInt(raw any, path string) (int, error) {
	number, ok := raw.(json.Number)
	if !ok {
		return 0, fmt.Errorf("%s: expected integer", path)
	}
	value, err := number.Int64()
	if err != nil || value < 0 || int64(int(value)) != value {
		return 0, fmt.Errorf("%s: expected non-negative integer", path)
	}
	return int(value), nil
}

type rejectJSONSchemaValidator struct{}

func (rejectJSONSchemaValidator) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
	ctx.AddError(ValidationError{
		Level:   LevelError,
		Path:    path,
		Line:    node.Line,
		Column:  node.Column,
		Message: "value is rejected by boolean false JSON Schema",
	})
}
