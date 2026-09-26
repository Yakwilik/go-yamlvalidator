# Diagnostics and YAML behavior

Source: validator.go, diagnostic.go and the JSON Schema bridge. Tests:
[diagnostics_test.go](../examples/recipes/diagnostics_test.go).

## Use the actual result

~~~go
result := validator.ValidateBytes(data)
fmt.Print(result.FormatAll(true))
if result.HasErrors() {
    return fmt.Errorf("invalid configuration")
}
~~~

The fragment belongs in an error-returning function with fmt imported. FormatAll
includes source context. Do not invent a transcript such as "expected two more
spaces": syntax diagnostics come from yaml.v3 and cannot always identify the
intended nesting or an exact column. Fix syntax first, then rerun schema checks.

HasErrors counts errors only. Collector.Warnings(), Errors() and All() return
copies; All preserves insertion order. FormatAll(true) formats with positional
sorting, while SortByPosition mutates the result's collector order. Position
sorting is not a promise of fully deterministic tie order for all engine errors.

ValidationError fields are Level, Code, SchemaPath, Path, PathTokens, Line,
Column, Message, Got, Expected and Details. There is no Filename field; attach
application file identity outside the diagnostic. Missing-field errors point to
a parent node because no missing node exists. An alias-related error may point
to the anchor definition. Line and Column are 1-based, with 0 meaning unknown;
do not treat them as terminal display width or automatically as UTF-16 offsets.

For a single diagnostic, use <code>FormatErrorWithSource(diagnostic, sourceLines)</code>.
The lower-level <code>RenderLineWithCaret(line, byteCol)</code> returns the rendered
line, caret column and rendered length. Its input column is a 1-based byte
position; this helper is not a general terminal-width or UTF-16 converter.
<code>NewErrorCollector</code> is available when an application needs to collect
its own diagnostics separately. Use <code>NewValidationContext</code> when calling
a native validator directly so its collector is initialized.

## Structured handling

~~~go
for _, diagnostic := range result.Collector.All() {
    switch details := diagnostic.Details.(type) {
    case v.RequiredDetails:
        fmt.Println("missing:", details.Field)
    case v.UnknownKeyDetails:
        fmt.Println("unknown:", details.Key, "suggestion:", details.Suggestion)
    case v.TypeMismatchDetails:
        fmt.Println("expected:", details.Expected, "actual:", details.Actual)
    }
}
~~~

Other typed details are NumericRangeDetails, ItemCountDetails and
DependencyDetails. Details may be nil; do not assume every keyword supplies it.
Native type failures use type_mismatch; JSON Schema uses type. Native unknown
keys use unknown_key and may include a typo suggestion; JSON Schema reports
additionalProperties without promising the same suggestion. Syntax uses
yaml_syntax; duplicate keys use duplicate_key.

PathTokens distinguish property, index and document components. FormatPathTokens
and ParsePathTokens convert display paths. AppendPropertyPath handles names with
dots/brackets; AppendIndexPath returns an error for negative indexes. A leading
doc[N] prefix is reserved for subsequent documents; use the helpers instead of
concatenating or parsing paths with strings.Split.

## YAML cases to test

| Input characteristic | Relevant behavior |
| --- | --- |
| A shifted mapping member | May be a syntax error or syntactically valid but differently nested data; inspect both |
| Quoted "123" or "yes" | Remains a string |
| Plain yes/no/on/off | Strings by default; boolean scalar compatibility when YAML11Booleans is true |
| Empty value after key: | Null, not an empty string |
| Duplicate explicit keys | Diagnostic before schema-specific validation, including a bare TypeAny |
| Alias and merge | Alias targets resolved; explicit keys override merges; earlier mappings win in a merge sequence |
| Recursive alias | Rejected by safety checks; no supported cyclic JSON instance |
| Multiple documents | Each document validated against the same root schema; later paths prefixed doc[1], doc[2], etc. |
| Empty stream | No instance; set root Required true when a document must exist |
| JSON Schema with non-string keys or .nan/.inf | Conversion error, even for a permissive JSON Schema |

Native Default only announces the suggested default of an omitted optional
field. It never fills a struct or edits source bytes. JSON Schema default is
metadata and does not cause this native warning. Required is presence, not
minLength; add a separate nonempty constraint.

## Keep validation and decoding consistent

This library does not return a populated application config and does not format
or auto-fix YAML. A second decoder must be configured consistently and must
consume exactly the documents the application intends to accept. Do not turn a
multi-document validation success into silently decoding just its first document.
For direct native callbacks, account for raw node layout: mapping Content holds
alternating keys and values; built-in raw length checks are not a semantic merge
cardinality calculation.
