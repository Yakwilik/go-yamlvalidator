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

// JSONSchemaKeyword defines one custom JSON Schema keyword. Set exactly one of
// Validate or Compile.
type JSONSchemaKeyword struct {
	Name     string
	Validate JSONSchemaKeywordFunc
	Compile  JSONSchemaKeywordCompileFunc
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
		customCtx := &JSONSchemaKeywordValidationContext{}
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
		if keyword.Validate == nil && keyword.Compile == nil {
			return fmt.Errorf("JSON Schema keyword %q: validate/compile function is nil", keyword.Name)
		}
		if keyword.Validate != nil && keyword.Compile != nil {
			return fmt.Errorf("JSON Schema keyword %q: set either Validate or Compile, not both", keyword.Name)
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
		URL: vocabulary.URL,
		Compile: func(_ *jsonschema.CompilerContext, obj map[string]any) (jsonschema.SchemaExt, error) {
			compiled := make([]compiledFunctionalKeyword, 0, len(vocabulary.Keywords))
			for _, keyword := range vocabulary.Keywords {
				value, ok := obj[keyword.Name]
				if !ok {
					continue
				}

				var validate JSONSchemaKeywordValidateFunc
				if keyword.Compile != nil {
					var err error
					validate, err = keyword.Compile(value)
					if err != nil {
						return nil, fmt.Errorf("compile JSON Schema keyword %q: %w", keyword.Name, err)
					}
					if validate == nil {
						return nil, fmt.Errorf("compile JSON Schema keyword %q: returned nil validator", keyword.Name)
					}
				} else {
					keywordValue := value
					validate = func(instance any, ctx *JSONSchemaKeywordValidationContext) {
						keyword.Validate(keywordValue, instance, ctx)
					}
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
