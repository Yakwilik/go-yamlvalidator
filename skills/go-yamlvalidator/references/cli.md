# CLI and serialized schemas

Source: cmd/yamlvalidator/main.go and schema_loader.go. The two CLI schema
formats are distinct. Do not pass a JSON Schema document with the default field
format, or assume a JSON filename automatically selects JSON Schema mode.

~~~bash
go install github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator@v1.0.0
yamlvalidator -schema schema.json -schema-format jsonschema -file config.yaml
~~~

JSON Schema file mode creates a file resolver rooted in the schema's directory
and sets the root retrieval URI. Relative refs inside that directory work.
Go callbacks cannot be installed into this CLI by putting function names in a
schema file; use a Go integration with registration for custom extensions.

## Native serialized format

Use [assets/native-schema.yaml](../assets/native-schema.yaml) with
[assets/valid.yaml](../assets/valid.yaml):

~~~bash
yamlvalidator -schema native-schema.yaml -schema-format field -file valid.yaml
~~~

Only the fields supported by schema_loader.go exist in this format:

~~~yaml
type: map
required: true
unknownKeyPolicy: error
allowedKeys:
  name:
    type: string
    required: true
    validators:
      - name: nonempty
  port:
    type: integer
    validators:
      - name: range
        min: 1
        max: 65535
~~~

Available top-level/nested field names: type, required, nullable, deprecated,
description, default, allowedKeys, additionalProperties, unknownKeyPolicy,
keyValidators, itemSchema, minItems, maxItems, validators, anyOf, exactlyOneOf,
mutuallyExclusive, conditions. Conditions use conditionField, conditionValue,
thenRequired, thenForbidden.

This loader is not a lossless serializer of the full Go FieldSchema. It does not
expose AllowedTypes, OneOfSchemas, AnyOfSchemas, OneOfRequired, ForbiddenTogether,
DependentRequired, exact numeric bounds or arbitrary callbacks. Do not invent
oneOfRequired or minExact YAML fields. Use the Go API or standard JSON Schema
for those features. Unknown serialized fields are rejected.

Type spellings: any, null, string, int/integer, float/number, bool/boolean,
map/object, sequence/array. Policy spellings: inherit, error, warn, ignore.

| Serialized value validator name | Options |
| --- | --- |
| enum | allowed (array of strings) |
| regex | pattern, message |
| range | min, max (float64 bounds) |
| nonempty | none |
| length | minLength, maxLength |
| url | requireScheme, allowedSchemes |
| oneoftype | types (array of type names) |

Key validators: regex uses pattern/message; forbidden uses forbidden; length
uses minLength/maxLength (or min/max aliases). Do not advertise message overrides
for fields that the loader does not propagate.

## Flags and exit handling

| Flag | Default / role |
| --- | --- |
| -schema | Required filename |
| -schema-format | field; also fieldschema, jsonschema, json-schema |
| -file | Empty reads stdin |
| -strict-keys | false; inherited native unknown-key errors when true |
| -stop-on-first | false |
| -strict-types | false; native inference |
| -yaml11-bools | true in CLI, unlike the library default |
| -sort | true |
| -json-schema-draft | 2020-12; explicit $schema still wins |
| -assert-format, -assert-content, -assert-vocabs | false |
| -regexp-timeout | 0; duration such as 200ms |
| -max-bytes, -max-documents, -max-depth, -max-diagnostics | 0 = unlimited |

Flags must precede positional arguments as appropriate for Go's flag parser.
Use <code>-yaml11-bools=false</code> to match default library scalar behavior.
There is no CLI -format json or public LoadSchemaFromFile API. The loader is
internal to the command. Exit 0 means no error-level validation failures
(warnings can still be printed); exit 1 means invalid YAML/config; exit 2 means
schema, argument or input-read failure. A warning-only truncated run can exit 0;
use a Go wrapper when incomplete checks must be rejected explicitly.
