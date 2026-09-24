package yamlvalidator

import (
	"fmt"
	"reflect"
)

// DefinitionValidator may be implemented by value/key validators that can
// validate their own configuration during FieldSchema compilation.
type DefinitionValidator interface {
	ValidateDefinition() error
}

// CompileFieldSchema validates a native FieldSchema before constructing a
// Validator. New code should prefer this over NewValidator when schemas are
// assembled dynamically or loaded from configuration.
func CompileFieldSchema(schema *FieldSchema) (*Validator, error) {
	if err := ValidateFieldSchema(schema); err != nil {
		return nil, err
	}
	// Compile takes an immutable snapshot of the FieldSchema graph so callers
	// may safely reuse or mutate the source schema after compilation. Built-in
	// validators and custom validators implementing the cloner interfaces are
	// snapshotted too; other custom validators retain their interface value and
	// must manage their own mutable state/concurrency.
	snapshot := cloneFieldSchema(schema, make(map[*FieldSchema]*FieldSchema))
	return NewValidator(snapshot), nil
}

// ValidateFieldSchema checks FieldSchema invariants, including nested schemas,
// invalid constraints, recursive schema graphs, and validator definitions.
func ValidateFieldSchema(schema *FieldSchema) error {
	if schema == nil {
		return fmt.Errorf("schema is nil")
	}
	return validateFieldSchema(schema, "$", make(map[*FieldSchema]bool), make(map[*FieldSchema]bool))
}

func validateFieldSchema(schema *FieldSchema, path string, visiting, validated map[*FieldSchema]bool) error {
	if schema == nil {
		return fmt.Errorf("%s: schema is nil", path)
	}
	if validated[schema] {
		return nil
	}
	if visiting[schema] {
		return fmt.Errorf("%s: recursive FieldSchema graph is not supported", path)
	}
	visiting[schema] = true
	defer delete(visiting, schema)

	if !validNodeType(schema.Type) {
		return fmt.Errorf("%s.type: invalid node type %d", path, schema.Type)
	}
	seenTypes := make(map[NodeType]bool, len(schema.AllowedTypes))
	for i, nodeType := range schema.AllowedTypes {
		if !validNodeType(nodeType) {
			return fmt.Errorf("%s.allowedTypes[%d]: invalid node type %d", path, i, nodeType)
		}
		if seenTypes[nodeType] {
			return fmt.Errorf("%s.allowedTypes[%d]: duplicate node type %s", path, i, nodeType)
		}
		seenTypes[nodeType] = true
	}
	if schema.UnknownKeyPolicy < UnknownKeyInherit || schema.UnknownKeyPolicy > UnknownKeyIgnore {
		return fmt.Errorf("%s.unknownKeyPolicy: invalid policy %d", path, schema.UnknownKeyPolicy)
	}
	if schema.MinItems != nil && *schema.MinItems < 0 {
		return fmt.Errorf("%s.minItems: must be non-negative", path)
	}
	if schema.MaxItems != nil && *schema.MaxItems < 0 {
		return fmt.Errorf("%s.maxItems: must be non-negative", path)
	}
	if schema.MinItems != nil && schema.MaxItems != nil && *schema.MinItems > *schema.MaxItems {
		return fmt.Errorf("%s: minItems must not exceed maxItems", path)
	}

	if hasMappingConstraints(schema) && !schemaCanHaveType(schema, TypeMap) {
		return fmt.Errorf("%s: mapping constraints require map/any type", path)
	}
	if hasSequenceConstraints(schema) && !schemaCanHaveType(schema, TypeSequence) {
		return fmt.Errorf("%s: sequence constraints require sequence/any type", path)
	}

	for i, validator := range schema.Validators {
		if nilInterface(validator) {
			return fmt.Errorf("%s.validators[%d]: validator is nil", path, i)
		}
		if definition, ok := validator.(DefinitionValidator); ok {
			if err := definition.ValidateDefinition(); err != nil {
				return fmt.Errorf("%s.validators[%d]: %w", path, i, err)
			}
		}
	}
	for i, validator := range schema.KeyValidators {
		if nilInterface(validator) {
			return fmt.Errorf("%s.keyValidators[%d]: validator is nil", path, i)
		}
		if definition, ok := validator.(DefinitionValidator); ok {
			if err := definition.ValidateDefinition(); err != nil {
				return fmt.Errorf("%s.keyValidators[%d]: %w", path, i, err)
			}
		}
	}

	for name, child := range schema.AllowedKeys {
		if child == nil {
			return fmt.Errorf("%s.allowedKeys[%q]: schema is nil", path, name)
		}
		if err := validateFieldSchema(child, path+".allowedKeys["+fmt.Sprintf("%q", name)+"]", visiting, validated); err != nil {
			return err
		}
	}
	if schema.AdditionalProperties != nil {
		if err := validateFieldSchema(schema.AdditionalProperties, path+".additionalProperties", visiting, validated); err != nil {
			return err
		}
	}
	if schema.ItemSchema != nil {
		if err := validateFieldSchema(schema.ItemSchema, path+".itemSchema", visiting, validated); err != nil {
			return err
		}
	}
	for i, child := range schema.OneOfSchemas {
		if child == nil {
			return fmt.Errorf("%s.oneOfSchemas[%d]: schema is nil", path, i)
		}
		if err := validateFieldSchema(child, fmt.Sprintf("%s.oneOfSchemas[%d]", path, i), visiting, validated); err != nil {
			return err
		}
	}
	for i, child := range schema.AnyOfSchemas {
		if child == nil {
			return fmt.Errorf("%s.anyOfSchemas[%d]: schema is nil", path, i)
		}
		if err := validateFieldSchema(child, fmt.Sprintf("%s.anyOfSchemas[%d]", path, i), visiting, validated); err != nil {
			return err
		}
	}

	if err := validateFieldGroups(path+".anyOf", schema.AnyOf, schema.AllowedKeys); err != nil {
		return err
	}
	if err := validateFieldList(path+".exactlyOneOf", schema.ExactlyOneOf, schema.AllowedKeys); err != nil {
		return err
	}
	if err := validateFieldList(path+".mutuallyExclusive", schema.MutuallyExclusive, schema.AllowedKeys); err != nil {
		return err
	}
	if err := validateFieldGroups(path+".oneOfRequired", schema.OneOfRequired, schema.AllowedKeys); err != nil {
		return err
	}
	if err := validateFieldGroups(path+".forbiddenTogether", schema.ForbiddenTogether, schema.AllowedKeys); err != nil {
		return err
	}
	for trigger, required := range schema.DependentRequired {
		if trigger == "" {
			return fmt.Errorf("%s.dependentRequired: empty trigger field", path)
		}
		if schema.AllowedKeys != nil {
			if _, ok := schema.AllowedKeys[trigger]; !ok {
				return fmt.Errorf("%s.dependentRequired[%q]: trigger is not declared in allowedKeys", path, trigger)
			}
		}
		if err := validateFieldList(path+".dependentRequired["+fmt.Sprintf("%q", trigger)+"]", required, schema.AllowedKeys); err != nil {
			return err
		}
	}
	for i, condition := range schema.Conditions {
		conditionPath := fmt.Sprintf("%s.conditions[%d]", path, i)
		if condition.ConditionField == "" {
			return fmt.Errorf("%s.conditionField: must not be empty", conditionPath)
		}
		if schema.AllowedKeys != nil {
			if _, ok := schema.AllowedKeys[condition.ConditionField]; !ok {
				return fmt.Errorf("%s.conditionField: %q is not declared in allowedKeys", conditionPath, condition.ConditionField)
			}
		}
		if err := validateFieldList(conditionPath+".thenRequired", condition.ThenRequired, schema.AllowedKeys); err != nil {
			return err
		}
		if err := validateFieldList(conditionPath+".thenForbidden", condition.ThenForbidden, schema.AllowedKeys); err != nil {
			return err
		}
	}

	validated[schema] = true
	return nil
}

func validNodeType(nodeType NodeType) bool {
	return nodeType >= TypeAny && nodeType <= TypeSequence
}

func schemaCanHaveType(schema *FieldSchema, wanted NodeType) bool {
	if len(schema.AllowedTypes) > 0 {
		for _, nodeType := range schema.AllowedTypes {
			if nodeType == TypeAny || nodeType == wanted {
				return true
			}
		}
		return false
	}
	return schema.Type == TypeAny || schema.Type == wanted
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func validateFieldGroups(path string, groups [][]string, allowed map[string]*FieldSchema) error {
	for i, group := range groups {
		if len(group) == 0 {
			return fmt.Errorf("%s[%d]: group must not be empty", path, i)
		}
		if err := validateFieldList(fmt.Sprintf("%s[%d]", path, i), group, allowed); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldList(path string, fields []string, allowed map[string]*FieldSchema) error {
	seen := make(map[string]bool, len(fields))
	for i, field := range fields {
		if field == "" {
			return fmt.Errorf("%s[%d]: field must not be empty", path, i)
		}
		if seen[field] {
			return fmt.Errorf("%s[%d]: duplicate field %q", path, i, field)
		}
		seen[field] = true
		if allowed != nil {
			if _, ok := allowed[field]; !ok {
				return fmt.Errorf("%s[%d]: field %q is not declared in allowedKeys", path, i, field)
			}
		}
	}
	return nil
}

func cloneFieldSchema(schema *FieldSchema, memo map[*FieldSchema]*FieldSchema) *FieldSchema {
	if schema == nil {
		return nil
	}
	if cloned := memo[schema]; cloned != nil {
		return cloned
	}

	cloned := *schema
	memo[schema] = &cloned

	cloned.AllowedTypes = append([]NodeType(nil), schema.AllowedTypes...)
	cloned.Validators = cloneValueValidators(schema.Validators)
	cloned.KeyValidators = cloneKeyValidators(schema.KeyValidators)
	cloned.ExactlyOneOf = append([]string(nil), schema.ExactlyOneOf...)
	cloned.MutuallyExclusive = append([]string(nil), schema.MutuallyExclusive...)
	cloned.AnyOf = cloneStringGroups(schema.AnyOf)
	cloned.OneOfRequired = cloneStringGroups(schema.OneOfRequired)
	cloned.ForbiddenTogether = cloneStringGroups(schema.ForbiddenTogether)

	if schema.MinItems != nil {
		value := *schema.MinItems
		cloned.MinItems = &value
	}
	if schema.MaxItems != nil {
		value := *schema.MaxItems
		cloned.MaxItems = &value
	}

	cloned.AllowedKeys = cloneSchemaMap(schema.AllowedKeys, memo)
	cloned.AdditionalProperties = cloneFieldSchema(schema.AdditionalProperties, memo)
	cloned.ItemSchema = cloneFieldSchema(schema.ItemSchema, memo)

	cloned.OneOfSchemas = make([]*FieldSchema, len(schema.OneOfSchemas))
	for i, child := range schema.OneOfSchemas {
		cloned.OneOfSchemas[i] = cloneFieldSchema(child, memo)
	}
	cloned.AnyOfSchemas = make([]*FieldSchema, len(schema.AnyOfSchemas))
	for i, child := range schema.AnyOfSchemas {
		cloned.AnyOfSchemas[i] = cloneFieldSchema(child, memo)
	}

	cloned.Conditions = make([]ConditionalRule, len(schema.Conditions))
	for i, condition := range schema.Conditions {
		cloned.Conditions[i] = condition
		cloned.Conditions[i].ThenRequired = append([]string(nil), condition.ThenRequired...)
		cloned.Conditions[i].ThenForbidden = append([]string(nil), condition.ThenForbidden...)
	}

	if schema.DependentRequired != nil {
		cloned.DependentRequired = make(map[string][]string, len(schema.DependentRequired))
		for trigger, required := range schema.DependentRequired {
			cloned.DependentRequired[trigger] = append([]string(nil), required...)
		}
	}

	cloned.Default = cloneSchemaDefault(schema.Default)
	return &cloned
}

func cloneSchemaMap(source map[string]*FieldSchema, memo map[*FieldSchema]*FieldSchema) map[string]*FieldSchema {
	if source == nil {
		return nil
	}
	result := make(map[string]*FieldSchema, len(source))
	for key, child := range source {
		result[key] = cloneFieldSchema(child, memo)
	}
	return result
}

func cloneStringGroups(groups [][]string) [][]string {
	if groups == nil {
		return nil
	}
	result := make([][]string, len(groups))
	for i, group := range groups {
		result[i] = append([]string(nil), group...)
	}
	return result
}

func cloneSchemaDefault(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = cloneSchemaDefault(item)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = cloneSchemaDefault(item)
		}
		return result
	case map[any]any:
		result := make(map[any]any, len(typed))
		for key, item := range typed {
			result[key] = cloneSchemaDefault(item)
		}
		return result
	default:
		// Scalar values and opaque application-defined defaults are copied by
		// value. Callers that store mutable opaque values in Default remain
		// responsible for their own synchronization.
		return value
	}
}

func cloneValueValidators(source []ValueValidator) []ValueValidator {
	if source == nil {
		return nil
	}
	result := make([]ValueValidator, len(source))
	for i, validator := range source {
		if cloner, ok := validator.(ValueValidatorCloner); ok {
			result[i] = cloner.CloneValueValidator()
		} else {
			result[i] = validator
		}
	}
	return result
}

func cloneKeyValidators(source []KeyValidator) []KeyValidator {
	if source == nil {
		return nil
	}
	result := make([]KeyValidator, len(source))
	for i, validator := range source {
		if cloner, ok := validator.(KeyValidatorCloner); ok {
			result[i] = cloner.CloneKeyValidator()
		} else {
			result[i] = validator
		}
	}
	return result
}
