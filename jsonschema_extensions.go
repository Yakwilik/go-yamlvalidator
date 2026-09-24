package yamlvalidator

import (
	"fmt"
	"net/url"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/message"
)

// JSONSchemaFormat registers a custom JSON Schema format without exposing the
// underlying JSON Schema engine API.
type JSONSchemaFormat struct {
	Name     string
	Validate func(value any) error
}

// JSONSchemaKeywordIssue is a diagnostic produced by a custom JSON Schema
// keyword. Path is relative to the instance value to which the keyword applies.
type JSONSchemaKeywordIssue struct {
	Path    []string
	Message string
}

// JSONSchemaKeywordValidationContext collects custom keyword diagnostics and
// evaluation annotations during one keyword validation call.
type JSONSchemaKeywordValidationContext struct {
	issues              []JSONSchemaKeywordIssue
	evaluatedProperties []string
	evaluatedItems      []int
	engine              *jsonschema.ValidatorContext
}

// JSONSchemaSubschema is a compiled nested schema used by custom keywords.
// Its implementation is intentionally opaque so callers do not depend on the
// underlying JSON Schema engine.
type JSONSchemaSubschema struct {
	schema *jsonschema.Schema
}

// JSONSchemaKeywordCompileContext exposes high-level helpers needed by custom
// keywords which contain nested schemas.
type JSONSchemaKeywordCompileContext struct {
	keyword string
	engine  *jsonschema.CompilerContext
}

// Subschema compiles a nested schema located at path relative to the custom
// keyword value. Path components are JSON object keys or array indexes encoded
// as decimal strings.
func (ctx *JSONSchemaKeywordCompileContext) Subschema(path ...string) *JSONSchemaSubschema {
	fullPath := make([]string, 0, len(path)+1)
	fullPath = append(fullPath, ctx.keyword)
	fullPath = append(fullPath, path...)
	return &JSONSchemaSubschema{schema: ctx.engine.Enqueue(fullPath)}
}

// Reference compiles a schema reference using the current schema resource.
func (ctx *JSONSchemaKeywordCompileContext) Reference(ref string) (*JSONSchemaSubschema, error) {
	schema, err := ctx.engine.EnqueueRef(ref)
	if err != nil {
		return nil, err
	}
	return &JSONSchemaSubschema{schema: schema}, nil
}

// ValidateSubschema validates instance with a nested schema and reports any
// resulting JSON Schema errors through the current validation context.
// relativePath is relative to the current instance value.
func (ctx *JSONSchemaKeywordValidationContext) ValidateSubschema(
	schema *JSONSchemaSubschema,
	instance any,
	relativePath []string,
) bool {
	if schema == nil || schema.schema == nil {
		ctx.AddError("custom keyword attempted to validate with a nil subschema")
		return false
	}
	if ctx.engine == nil {
		ctx.AddError("subschema validation is unavailable in this context")
		return false
	}
	err := ctx.engine.Validate(schema.schema, instance, relativePath)
	if err == nil {
		return true
	}
	ctx.engine.AddErr(err)
	return false
}

// JSONSchemaSubschemaPosition identifies one possible nested-schema location
// inside a custom keyword value. Use the constructor functions below.
type JSONSchemaSubschemaPosition struct {
	kind     jsonSchemaSubschemaPositionKind
	property string
	index    int
}

type jsonSchemaSubschemaPositionKind uint8

const (
	jsonSchemaSubschemaProperty jsonSchemaSubschemaPositionKind = iota + 1
	jsonSchemaSubschemaAllProperties
	jsonSchemaSubschemaItem
	jsonSchemaSubschemaAllItems
)

// JSONSchemaSubschemaPath declares a nested schema location relative to a
// custom keyword value.
type JSONSchemaSubschemaPath []JSONSchemaSubschemaPosition

// JSONSchemaSubschemaProperty selects one object property.
func JSONSchemaSubschemaProperty(name string) JSONSchemaSubschemaPosition {
	return JSONSchemaSubschemaPosition{
		kind:     jsonSchemaSubschemaProperty,
		property: name,
	}
}

// JSONSchemaSubschemaAllProperties selects all object property values.
func JSONSchemaSubschemaAllProperties() JSONSchemaSubschemaPosition {
	return JSONSchemaSubschemaPosition{kind: jsonSchemaSubschemaAllProperties}
}

// JSONSchemaSubschemaItem selects one array item.
func JSONSchemaSubschemaItem(index int) JSONSchemaSubschemaPosition {
	return JSONSchemaSubschemaPosition{
		kind:  jsonSchemaSubschemaItem,
		index: index,
	}
}

// JSONSchemaSubschemaAllItems selects all array items.
func JSONSchemaSubschemaAllItems() JSONSchemaSubschemaPosition {
	return JSONSchemaSubschemaPosition{kind: jsonSchemaSubschemaAllItems}
}

// AddError reports an error for the current instance value.
func (ctx *JSONSchemaKeywordValidationContext) AddError(message string) {
	ctx.AddErrorAt(nil, message)
}

// AddErrorAt reports an error at a path relative to the current instance value.
func (ctx *JSONSchemaKeywordValidationContext) AddErrorAt(path []string, message string) {
	ctx.issues = append(ctx.issues, JSONSchemaKeywordIssue{
		Path:    append([]string(nil), path...),
		Message: message,
	})
}

// MarkPropertyEvaluated marks an object property as evaluated for
// unevaluatedProperties semantics.
func (ctx *JSONSchemaKeywordValidationContext) MarkPropertyEvaluated(name string) {
	ctx.evaluatedProperties = append(ctx.evaluatedProperties, name)
}

// MarkItemEvaluated marks an array item as evaluated for unevaluatedItems
// semantics.
func (ctx *JSONSchemaKeywordValidationContext) MarkItemEvaluated(index int) {
	ctx.evaluatedItems = append(ctx.evaluatedItems, index)
}

// JSONSchemaKeywordValidateFunc validates one JSON instance value.
type JSONSchemaKeywordValidateFunc func(
	instance any,
	ctx *JSONSchemaKeywordValidationContext,
)

// JSONSchemaKeywordFunc is the simple custom-keyword form. The schema keyword
// value and current JSON instance value are supplied on each validation call.
type JSONSchemaKeywordFunc func(
	keywordValue any,
	instance any,
	ctx *JSONSchemaKeywordValidationContext,
)

// JSONSchemaKeywordCompileFunc compiles a keyword value from the schema into a
// runtime validation function. It is useful for parsing/validating keyword
// configuration once instead of on every validation call.
type JSONSchemaKeywordCompileFunc func(
	keywordValue any,
) (JSONSchemaKeywordValidateFunc, error)

// JSONSchemaKeywordCompileContextFunc is the advanced high-level compile form
// for keywords that contain nested schemas or schema references.
type JSONSchemaKeywordCompileContextFunc func(
	ctx *JSONSchemaKeywordCompileContext,
	keywordValue any,
) (JSONSchemaKeywordValidateFunc, error)

// JSONSchemaKeyword defines one custom JSON Schema keyword. Set exactly one of
// Validate, Compile, or CompileWithContext. Subschemas declares locations of
// nested schemas relative to the keyword value.
type JSONSchemaKeyword struct {
	Name               string
	Validate           JSONSchemaKeywordFunc
	Compile            JSONSchemaKeywordCompileFunc
	CompileWithContext JSONSchemaKeywordCompileContextFunc
	Subschemas         []JSONSchemaSubschemaPath
}

// JSONSchemaVocabulary groups custom keywords under a vocabulary URL.
// Vocabularies registered through JSONSchemaCompileOptions are activated for
// that compilation. For declaration-only/advanced vocabulary semantics use
// ConfigureCompiler.
type JSONSchemaVocabulary struct {
	URL      string
	Keywords []JSONSchemaKeyword
}

const defaultFunctionalVocabularyURL = "urn:go-yamlvalidator:custom-keywords"

type compiledFunctionalKeyword struct {
	name     string
	validate JSONSchemaKeywordValidateFunc
}

type functionalVocabularyExt struct {
	keywords []compiledFunctionalKeyword
}

func (ext *functionalVocabularyExt) Validate(ctx *jsonschema.ValidatorContext, instance any) {
	for _, keyword := range ext.keywords {
		customCtx := &JSONSchemaKeywordValidationContext{engine: ctx}
		keyword.validate(instance, customCtx)

		for _, property := range customCtx.evaluatedProperties {
			ctx.EvaluatedProp(property)
		}
		for _, index := range customCtx.evaluatedItems {
			ctx.EvaluatedItem(index)
		}
		for _, issue := range customCtx.issues {
			ctx.AddError(&functionalKeywordError{
				keyword:      keyword.name,
				message:      issue.Message,
				relativePath: append([]string(nil), issue.Path...),
			})
		}
	}
}

type functionalKeywordError struct {
	keyword      string
	message      string
	relativePath []string
}

func (e *functionalKeywordError) KeywordPath() []string {
	return []string{e.keyword}
}

func (e *functionalKeywordError) LocalizedString(*message.Printer) string {
	return e.message
}

func registerFunctionalJSONSchemaExtensions(
	compiler *jsonschema.Compiler,
	formats []JSONSchemaFormat,
	keywords []JSONSchemaKeyword,
	vocabularies []JSONSchemaVocabulary,
) error {
	if err := registerFunctionalFormats(compiler, formats); err != nil {
		return err
	}

	all := make([]JSONSchemaVocabulary, 0, len(vocabularies)+1)
	if len(keywords) > 0 {
		all = append(all, JSONSchemaVocabulary{
			URL:      defaultFunctionalVocabularyURL,
			Keywords: keywords,
		})
	}
	all = append(all, vocabularies...)
	if len(all) == 0 {
		return nil
	}

	seenURLs := make(map[string]bool, len(all))
	seenKeywords := make(map[string]string)
	for _, vocabulary := range all {
		if err := validateVocabularyDefinition(vocabulary, seenURLs, seenKeywords); err != nil {
			return err
		}
		compiler.RegisterVocabulary(buildFunctionalVocabulary(vocabulary))
	}

	// First-class functional vocabularies are intended to be active immediately.
	// Users who need dialect-controlled activation can use ConfigureCompiler.
	compiler.AssertVocabs()
	return nil
}

func registerFunctionalFormats(compiler *jsonschema.Compiler, formats []JSONSchemaFormat) error {
	seen := make(map[string]bool, len(formats))
	for i, format := range formats {
		if format.Name == "" {
			return fmt.Errorf("JSON Schema format[%d]: name must not be empty", i)
		}
		if format.Validate == nil {
			return fmt.Errorf("JSON Schema format %q: validate function is nil", format.Name)
		}
		if seen[format.Name] {
			return fmt.Errorf("JSON Schema format %q registered more than once", format.Name)
		}
		seen[format.Name] = true
		compiler.RegisterFormat(&jsonschema.Format{
			Name:     format.Name,
			Validate: format.Validate,
		})
	}
	return nil
}

func validateVocabularyDefinition(
	vocabulary JSONSchemaVocabulary,
	seenURLs map[string]bool,
	seenKeywords map[string]string,
) error {
	if vocabulary.URL == "" {
		return fmt.Errorf("JSON Schema vocabulary URL must not be empty")
	}
	parsed, err := url.Parse(vocabulary.URL)
	if err != nil || !parsed.IsAbs() {
		return fmt.Errorf("JSON Schema vocabulary URL %q must be absolute", vocabulary.URL)
	}
	if seenURLs[vocabulary.URL] {
		return fmt.Errorf("JSON Schema vocabulary %q registered more than once", vocabulary.URL)
	}
	seenURLs[vocabulary.URL] = true
	if len(vocabulary.Keywords) == 0 {
		return fmt.Errorf("JSON Schema vocabulary %q has no keywords", vocabulary.URL)
	}

	for i, keyword := range vocabulary.Keywords {
		if keyword.Name == "" {
			return fmt.Errorf("JSON Schema vocabulary %q keyword[%d]: name must not be empty", vocabulary.URL, i)
		}
		callbacks := 0
		if keyword.Validate != nil {
			callbacks++
		}
		if keyword.Compile != nil {
			callbacks++
		}
		if keyword.CompileWithContext != nil {
			callbacks++
		}
		if callbacks == 0 {
			return fmt.Errorf(
				"JSON Schema keyword %q: validate/compile function is nil",
				keyword.Name,
			)
		}
		if callbacks > 1 {
			return fmt.Errorf(
				"JSON Schema keyword %q: set exactly one of Validate, Compile, or CompileWithContext",
				keyword.Name,
			)
		}
		for pathIndex, path := range keyword.Subschemas {
			for positionIndex, position := range path {
				switch position.kind {
				case jsonSchemaSubschemaProperty, jsonSchemaSubschemaAllProperties,
					jsonSchemaSubschemaAllItems:
				// valid
				case jsonSchemaSubschemaItem:
					if position.index < 0 {
						return fmt.Errorf(
							"JSON Schema keyword %q subschemas[%d][%d]: item index must be non-negative",
							keyword.Name, pathIndex, positionIndex,
						)
					}
				default:
					return fmt.Errorf(
						"JSON Schema keyword %q subschemas[%d][%d]: invalid subschema position",
						keyword.Name, pathIndex, positionIndex,
					)
				}
			}
		}
		if previous, ok := seenKeywords[keyword.Name]; ok {
			return fmt.Errorf(
				"JSON Schema keyword %q is registered by both %q and %q",
				keyword.Name, previous, vocabulary.URL,
			)
		}
		seenKeywords[keyword.Name] = vocabulary.URL
	}
	return nil
}

func buildFunctionalVocabulary(vocabulary JSONSchemaVocabulary) *jsonschema.Vocabulary {
	return &jsonschema.Vocabulary{
		URL:        vocabulary.URL,
		Subschemas: buildFunctionalSubschemaPaths(vocabulary),
		Compile: func(engineCtx *jsonschema.CompilerContext, obj map[string]any) (jsonschema.SchemaExt, error) {
			compiled := make([]compiledFunctionalKeyword, 0, len(vocabulary.Keywords))
			for _, keyword := range vocabulary.Keywords {
				value, ok := obj[keyword.Name]
				if !ok {
					continue
				}

				var validate JSONSchemaKeywordValidateFunc
				var err error
				switch {
				case keyword.CompileWithContext != nil:
					validate, err = keyword.CompileWithContext(
						&JSONSchemaKeywordCompileContext{
							keyword: keyword.Name,
							engine:  engineCtx,
						},
						value,
					)
				case keyword.Compile != nil:
					validate, err = keyword.Compile(value)
				default:
					keywordValue := value
					validate = func(instance any, ctx *JSONSchemaKeywordValidationContext) {
						keyword.Validate(keywordValue, instance, ctx)
					}
				}
				if err != nil {
					return nil, fmt.Errorf("compile JSON Schema keyword %q: %w", keyword.Name, err)
				}
				if validate == nil {
					return nil, fmt.Errorf("compile JSON Schema keyword %q: returned nil validator", keyword.Name)
				}

				compiled = append(compiled, compiledFunctionalKeyword{
					name:     keyword.Name,
					validate: validate,
				})
			}
			if len(compiled) == 0 {
				return nil, nil
			}
			return &functionalVocabularyExt{keywords: compiled}, nil
		},
	}
}

func buildFunctionalSubschemaPaths(vocabulary JSONSchemaVocabulary) []jsonschema.SchemaPath {
	var result []jsonschema.SchemaPath
	for _, keyword := range vocabulary.Keywords {
		for _, path := range keyword.Subschemas {
			enginePath := jsonschema.SchemaPath{jsonschema.Prop(keyword.Name)}
			for _, position := range path {
				switch position.kind {
				case jsonSchemaSubschemaProperty:
					enginePath = append(enginePath, jsonschema.Prop(position.property))
				case jsonSchemaSubschemaAllProperties:
					enginePath = append(enginePath, jsonschema.AllProp{})
				case jsonSchemaSubschemaItem:
					enginePath = append(enginePath, jsonschema.Item(position.index))
				case jsonSchemaSubschemaAllItems:
					enginePath = append(enginePath, jsonschema.AllItem{})
				}
			}
			result = append(result, enginePath)
		}
	}
	return result
}
