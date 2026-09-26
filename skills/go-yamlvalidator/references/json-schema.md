# JSON Schema validation of YAML

Reference implementation: jsonschema.go, jsonschema_formats.go, jsonschema_test.go,
and jsonschema_conformance_test.go at the revision in [api-index.md](api-index.md).
Runnable [JSON Schema cases](../examples/recipes/jsonschema_test.go).

## Compile once, validate YAML repeatedly

~~~go
schema, err := v.CompileJSONSchema(schemaData)
if err != nil {
    return err
}
validator := v.NewValidator(schema)
result := validator.ValidateBytes(yamlData)
if result.HasErrors() {
    fmt.Print(result.FormatAll(true))
}
~~~

schemaData must contain JSON, not a native FieldSchema YAML file. Syntax or
metaschema errors are compilation errors. Invalid YAML or instance constraints
produce ValidationResult diagnostics. The compiler returns a FieldSchema adapter;
there is no public compiled.Validate method on that returned value.

When the application requires an actual document, set <code>schema.Required =
true</code> before creating the validator. An empty YAML stream otherwise has no
instance to validate. A document containing null is a different case, evaluated
against the JSON Schema as usual.

## Dialect selection

| DefaultDraft constant | Dialect without an explicit $schema |
| --- | --- |
| JSONSchemaDraft4 | draft-04 |
| JSONSchemaDraft6 | draft-06 |
| JSONSchemaDraft7 | draft-07 |
| JSONSchemaDraft2019 | 2019-09 |
| JSONSchemaDraft2020, or zero value | 2020-12 |

An explicit $schema takes precedence. Match tuple keywords to the dialect:
2020-12 uses prefixItems with items for the remainder; older drafts use an items
array and additionalItems. An unsupported custom dialect needs its resources
and applicable vocabulary; do not promise arbitrary unknown vocabularies work.

## Standard rules

Use ordinary JSON Schema keywords rather than adding custom rules for behavior
already expressible in the schema:

| Need | Keywords |
| --- | --- |
| Scalar types and null union | type (string or array of strings) |
| Fixed values | const, enum |
| Strings | minLength, maxLength, pattern, format |
| Numbers | minimum, maximum, exclusiveMinimum, exclusiveMaximum, multipleOf |
| Objects | properties, patternProperties, propertyNames, required, additionalProperties, minProperties, maxProperties |
| Dependencies | dependentRequired, dependentSchemas; dependencies in older drafts |
| Arrays | items, prefixItems, minItems, maxItems, uniqueItems, contains, minContains, maxContains |
| Composition | allOf, anyOf, oneOf, not, if/then/else |
| Evaluated members | unevaluatedProperties, unevaluatedItems in supporting drafts |
| References | $id, $ref, $defs/definitions, $anchor, $dynamicRef/$dynamicAnchor, $recursiveRef in the corresponding dialect |

Boolean schemas true and false are supported where the dialect permits them.
Standard constraints that only apply to objects do not imply type: object.
For example, required alone does not reject a scalar. Specify types explicitly.

~~~json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "name": {"type": "string", "minLength": 1},
    "workers": {
      "type": "array",
      "minItems": 1,
      "items": {"type": "string"},
      "uniqueItems": true
    },
    "comment": {"type": ["string", "null"]}
  },
  "required": ["name"],
  "additionalProperties": false
}
~~~

JSON Schema defaults and metadata do not write values into the YAML. Unknown
extension keywords may be ignored by the engine; native CLI KnownFields is a
different mechanism. Register a keyword or declare a recognized required
vocabulary when its validation must execute.

## Options and warnings

| JSONSchemaCompileOptions field | Use |
| --- | --- |
| SchemaURL | Retrieval URI/base for the root, especially with relative refs |
| DefaultDraft | Default only when $schema is absent |
| AssertFormat | Enforce format even when the dialect normally treats it as annotation |
| AssertContent | Opt into contentEncoding/contentMediaType/contentSchema assertions |
| AssertVocabs | Enable engine vocabulary assertions; registered functional vocabularies also activate themselves |
| RegexpTimeout | Limit an individual ECMAScript match; zero is unlimited |
| Resources | Explicit URI-to-JSON-bytes resources |
| Resolver | Explicit external resource access; none by default |
| Formats | Custom format functions |
| Keywords, Vocabularies | Functional extension registration |
| ContentEncodings, ContentMediaTypes | Custom content handlers |
| AdditionalPropertiesFalsePolicy | Change direct additionalProperties:false diagnostics to warning/ignore |

The last option does not rewrite oneOf/not/if matching or unevaluatedProperties.
If a branch fails because of additionalProperties:false, suppressing the leaf
diagnostic does not turn the branch into a match. StrictKeys configures native
schema unknown-key policies, not JSON Schema additionalProperties.

~~~go
policy := v.UnknownKeyWarn
schema, err := v.CompileJSONSchemaWithOptions(schemaData, v.JSONSchemaCompileOptions{
    AssertFormat: true,
    AdditionalPropertiesFalsePolicy: &policy,
})
~~~

Do not silently downgrade validation to warnings. Use the policy only when the
caller explicitly needs permissive diagnostics, such as an editor preview.

## Data-model boundaries

The bridge converts yaml.Node into map[string]any, []any, string, bool, nil and
json.Number without serializing through float64. Object keys must be strings.
Non-finite YAML numbers (.nan/.inf) are not JSON values and fail conversion, even
under JSON Schema true. YAML timestamps and custom-tagged scalars are represented
as scalar text. Keep tag/style checks in native callbacks.

Quoted numeric and boolean-looking strings remain strings. YAML11Booleans is a
separate input option; it applies to plain scalar values, not arbitrary key
coercion. Native TypeInt and JSON Schema integer differ for decimal spellings
such as 1.0: test this explicitly when moving rules between surfaces.

The repository has an official conformance harness. Do not infer a universal
compatibility guarantee from its test count: identify the tested suite revision,
optional exclusions and the actual target dialect when reporting coverage.
