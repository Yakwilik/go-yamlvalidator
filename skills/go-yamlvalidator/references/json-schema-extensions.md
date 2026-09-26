# Functional JSON Schema extensions

Source: jsonschema_extensions.go and jsonschema_content.go. Complete executable
examples are in [extensions_test.go](../examples/recipes/extensions_test.go) and
[content_test.go](../examples/recipes/content_test.go). Load the relevant recipe,
not the entire extension system, for a simple validation task.

## Select the smallest extension

| Requirement | Mechanism |
| --- | --- |
| Specialized scalar check | JSONSchemaFormat |
| Check every object key | propertyNames with a format or standard schema |
| Arbitrary local rule with schema configuration | JSONSchemaKeyword.Validate |
| Parse/check rule configuration once | JSONSchemaKeyword.Compile |
| Rule contains subschemas or refs | CompileWithContext plus Subschemas |
| Named set of rules | JSONSchemaVocabulary |
| Decode or validate string-encoded content | ContentEncodings and ContentMediaTypes |

Callbacks receive JSON data, never yaml.Node. Numbers are json.Number. Guard
value types; do not compare interfaces containing maps/slices with ==. Do not
mutate keyword configuration or instances. Compiled callbacks can run concurrently.
A JSONSchemaKeywordValidationContext is not a native ValidationContext: it has
no Context(), Code field, warning API or source node.

## Custom format, including key validation

~~~go
format := v.JSONSchemaFormat{
    Name: "lowercase-name",
    Validate: func(value any) error {
        name, ok := value.(string)
        if !ok {
            return nil
        }
        if name != strings.ToLower(name) {
            return fmt.Errorf("name must be lowercase")
        }
        return nil
    },
}
schema, err := v.CompileJSONSchemaWithOptions(schemaData, v.JSONSchemaCompileOptions{
    AssertFormat: true,
    Formats: []v.JSONSchemaFormat{format},
})
~~~

The fragment requires fmt, strings and v. The schema can use
<code>{"type":"string","format":"lowercase-name"}</code> for a value, or
<code>{"type":"object","propertyNames":{"format":"lowercase-name"}}</code>
for keys. Registration alone does not enable format assertions in 2020-12.
Use a separate minLength rule if empty strings are invalid.

## Simple keyword

Set exactly one of Validate, Compile or CompileWithContext.

~~~go
keyword := v.JSONSchemaKeyword{
    Name: "x-forbidden-value",
    Validate: func(raw, instance any, ctx *v.JSONSchemaKeywordValidationContext) {
        forbidden, ok := raw.(string)
        if !ok {
            ctx.AddError("x-forbidden-value expects a string")
            return
        }
        value, ok := instance.(string)
        if ok && value == forbidden {
            ctx.AddError("value is reserved")
        }
    },
}
~~~

Register in JSONSchemaCompileOptions.Keywords. The schema uses, for example,
<code>{"type":"string","x-forbidden-value":"system"}</code>. This is a
library-specific keyword, not standard JSON Schema; another validator must
register it too. A Validate callback detects a malformed raw value at instance
validation time. Prefer Compile or Vocabulary.Schema to reject it earlier.

## Compile rule configuration once

~~~go
keyword := v.JSONSchemaKeyword{
    Name: "x-forbidden-value",
    Compile: func(raw any) (v.JSONSchemaKeywordValidateFunc, error) {
        forbidden, ok := raw.(string)
        if !ok {
            return nil, fmt.Errorf("expected a string")
        }
        return func(instance any, ctx *v.JSONSchemaKeywordValidationContext) {
            if value, ok := instance.(string); ok && value == forbidden {
                ctx.AddError("value is reserved")
            }
        }, nil
    },
}
~~~

Returning nil without a validator is a compile error. Configuration is compiled
per schema location, not once globally per keyword name.

## Paths and evaluated members

AddError reports on the current instance. AddErrorAt accepts relative raw path
segments: <code>[]string{"workers", "0", "name"}</code>. Do not pass a dotted
path or JSON-Pointer-escaped names. The bridge maps segments to source positions;
a missing field uses the closest existing ancestor. The diagnostic Code is the
keyword name. Standard nested-subschema failures keep their standard codes.

Use MarkPropertyEvaluated(name) and MarkItemEvaluated(index) only for members the
keyword actually handles. These affect unevaluatedProperties/unevaluatedItems;
they neither validate the member nor insert it. Inspect the instance and avoid
marking nonexistent members or invalid indexes. Do not mark unrelated members
to make an unevaluated error disappear.

## Subschema-bearing keywords

Declare schema locations relative to the keyword value in Subschemas. Each
JSONSchemaSubschemaPath is a sequence of selectors:

| Selector | Meaning |
| --- | --- |
| JSONSchemaSubschemaProperty(name) | Specific property value |
| JSONSchemaSubschemaAllProperties() | Every property value |
| JSONSchemaSubschemaItem(index) | Specific nonnegative array index |
| JSONSchemaSubschemaAllItems() | Every array item |
| Empty path | The keyword value itself is a schema |

Use CompileWithContext to enqueue them with ctx.Subschema(path...) or resolve
ctx.Reference(ref). Subschema returns an opaque *JSONSchemaSubschema, not an
error pair. Reference returns a schema and error. Paths passed to Subschema
start inside the keyword; do not prefix the keyword name again.

At runtime call validation.ValidateSubschema(branch, instance, relativePath).
It returns bool and adds failures to the current engine context. A nil path
validates the current value; a child path validates a child value. This helper
is not a side-effect-free probe: a false return has already reported errors.
For branch selection, choose the branch first rather than testing several
alternatives and hoping to discard failures. The tested dispatch recipe shows
both inline branches and $ref branches without importing the engine.

## Named vocabulary and its own schema

~~~go
vocab := v.JSONSchemaVocabulary{
    URL: "https://schemas.example.test/vocab/names",
    Schema: []byte("{\"properties\":{\"x-forbidden-value\":{\"type\":\"string\"}}}"),
    Keywords: []v.JSONSchemaKeyword{keyword},
}
~~~

Register with JSONSchemaCompileOptions.Vocabularies. URLs must be absolute;
keyword names must not collide across the registered sets. The library activates
registered functional vocabularies for this compilation; declaring a URL alone
does not discover or download Go callbacks. Vocabulary.Schema validates keyword
declarations. Its external references use preloaded Resources or Resolver.
Do not register the same keyword both directly and inside a vocabulary.

## Encoded content

JSONSchemaContentEncoding has Name and Decode func(string) ([]byte, error).
JSONSchemaContentMediaType has Name, Validate func([]byte) error and optional
UnmarshalJSON func([]byte) (any, error). Enable AssertContent explicitly.
For contentSchema to inspect decoded content, supply UnmarshalJSON and return
a JSON-compatible value graph. When decoding JSON, use Decoder.UseNumber to
avoid loss of numeric precision. The recipe uses a hex decoder and a JSON media
type, then validates the decoded object's required fields.

Custom format and content handlers can intentionally override a built-in name;
do not do that accidentally. The supported public extension API does not require
ConfigureCompiler, RegisterFormat methods, or an external compiler type.
