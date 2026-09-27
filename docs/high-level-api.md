# High-level YAML API

The high-level API validates one YAML document when encoding or decoding a Go value. Package Marshal and Unmarshal use zero-value options; validation is enabled by default. Standard gopkg.in/yaml.v3 Marshal and Unmarshal do not interpret yamlvalidate tags. They continue to use caller-written YAML hooks when present. A typed generated codec is not implemented.

~~~go
var config Config
err := yamlvalidator.Unmarshal([]byte("mode: prod\nport: 8443\n"), &config)
data, err := yamlvalidator.Marshal(config)
~~~

Unmarshal compiles the destination type, parses one document into a yaml.Node, validates that parsed node, then decodes once. Parse and validation failures leave the destination unchanged. A late codec error may partly update it. Marshal compiles the encode type, checks ordinary Go cycles, encodes once, parses and validates the actual output, then returns those bytes. Custom hooks run once. An invalid output returns nil bytes. Omitempty affects the validated output exactly as it affects yaml.v3 encoding.

## Tags and scopes

The yaml tag controls field names, inline flattening, and codec flags. The yamlvalidate tag declares validation. A blank _ field with yamlvalidate is a SchemaError, as is yamljsonschema. Unexported and YAML-excluded fields cannot carry validation declarations. Unknown directives, including jsonschema, fail compilation.

~~~text
rules  := entry ("," entry)*
entry  := key | key "=" value
value  := atom | quoted-string | "[" values? "]" | "{" rules? "}"
values := value ("," value)*
key    := identifier | quoted-string
~~~

Commas separate at their current nesting level. Single and double quoted values support escaped backslash, quote, newline, tab, and carriage return. Numeric atoms retain their spelling. The parser limits tags to 16 KiB and 64 nested levels. The reflection-independent grammar and scope normalization live in internal/taglang for reuse by a future source frontend.

Every object group has two explicit names. The bare name on a serializable struct field constrains the **containing mapping**, even when that carrier field is absent or has a scalar or struct value. The carrier is included only if its YAML name appears in the list. The Keys name constrains the **current field value**, which must be a mapping when present and non-null. For example:

~~~go
type Input struct {
    File string "yaml:\"file,omitempty\" yamlvalidate:\"exactlyOneOf=[file,url,inline],nonempty\""
    URL string "yaml:\"url,omitempty\""
    Inline string "yaml:\"inline,omitempty\""
    Choice map[string]any "yaml:\"choice,omitempty\" yamlvalidate:\"exactlyOneOfKeys=[file,url]\""
}
~~~

| Parent mapping directive | Current value directive |
| --- | --- |
| exactlyOneOf | exactlyOneOfKeys |
| mutuallyExclusive | mutuallyExclusiveKeys |
| anyOfRequired | anyOfRequiredKeys |
| oneOfRequired | oneOfRequiredKeys |
| forbiddenTogether | forbiddenTogetherKeys |
| dependentRequired | dependentRequiredKeys |
| when | whenKeys |
| require | requireKeys |

The parent directives work only on serializable struct fields. Inside items, values, properties, and additional, use Keys directives; bare group names have no parent field binding and are schema errors. For example, items={exactlyOneOfKeys=[a,b]} and values={exactlyOneOfKeys=[a,b]} validate each nested mapping.

Lists name YAML keys after inline flattening. A dot or bracket in a quoted name is literal. Static struct names must exist; dynamic string-keyed maps may name arbitrary keys. A duplicate member inside one declaration is a schema error. Separate group declarations are conjoined. Repeated identical built-in groups in the same effective scope are checked once, including reordered members. Inline struct parent groups bind to the actual flattened mapping; inline map value rules see only captured keys. Key presence includes null, false, zero, and empty strings. Required and notnull separately control container absence and null.

| Area | Rules |
| --- | --- |
| Type and presence | type, types, required, nullable, notnull, nonempty |
| Annotations | deprecated, description, default |
| Scalars | min, max, minLength, maxLength, enum, pattern, url, format |
| Mappings | properties, additional, unknown, values, keys, minProperties, maxProperties |
| Sequences | items, minItems, maxItems, uniqueItems |
| Composition | ref, oneOfSchemas, anyOfSchemas, check, checks |

Strings use Unicode code point lengths. Property counts use effective mappings after YAML merges. Numeric bounds compare exact values. Enum compares YAML scalar text. UniqueItems compares native YAML nodes: aliases and merges resolve, standard numeric scalars normalize, string and numeric tags remain distinct, and custom tags participate in equality. A default produces a warning without inserting a value. No implicit default filling occurs.

Native format accepts exactly hostname, email, idn-hostname, idn-email, ipv4, ipv6, uri, uri-reference, uri-template, uuid, date-time, date, time, and duration. Unknown names are schema errors. Keys rules also support format with the same names, alongside pattern, minLength, maxLength, and custom key checks. These checks use native string predicates and do not compile JSON Schema. The duration format follows the existing ISO 8601 duration predicate; it is not the Go time.Duration syntax such as 5s. Date-time and time use Go time parsing and are not advertised as full JSON Schema format conformance.

Items rules validate each sequence item. Values rules validate each map value, including declared properties; additional object rules apply only to undeclared properties. A string-keyed Go map is open by inference. Additional=forbid/warn/ignore selects an unknown-key policy; unknown=error cannot close an otherwise open map without declared properties.

## Registry

NewRegistry snapshots named native value and key validators, their factories, native FieldSchema references, and encode/decode type bindings. High-level JSON Schema sources and options are not registry fields. Callers can still compose a separate low-level Validator through the Schema option. Low-level CompileJSONSchema and its APIs remain available.

~~~go
registry, err := yamlvalidator.NewRegistry(yamlvalidator.RegistryConfig{
    Schemas: map[string]*yamlvalidator.FieldSchema{
        "positive": {Type: yamlvalidator.TypeInt},
    },
})
if err != nil { return err }
err = (yamlvalidator.UnmarshalOptions{Registry: registry}).Unmarshal(data, &config)
~~~

Factories receive structured arguments. Numeric arguments are json.Number values, retaining their spelling. Factory functions and opaque validator state must be safe for concurrent use when shared. Cold concurrent calls may compile the same type more than once, so factories may run more than once. The successful type plan cache is bounded and isolated per registry. Compilers invoke user factories outside the cache lock. Excessive simultaneous or recursive compilation of the same type fails with a schema error.

## Options and limits

UnmarshalOptions and MarshalOptions include UnknownKeyPolicy, WarningsAsErrors, OnDiagnostic, StopOnFirst, Limits, Registry, Schema, and SkipValidation. MarshalOptions also has Indent. The default unknown key policy is error. OnDiagnostic runs synchronously for final diagnostics. WarningsAsErrors turns warnings into one public ValidationErrors return; validation failure does not decode into the destination.

Default limits are 8 MiB input/output, depth 128, 100 diagnostics, and 1,000,000 node visits. Zero selects the default; negative values are errors. Truncated validation fails closed. These limits do not bound all allocations inside yaml.v3 or the execution time and allocations of custom hooks. ValidationErrors offers Diagnostics and FormatWithSource; GeneratedSource indicates that coordinates refer to Marshal output. SchemaError identifies the offending type, field, tag, byte offset, and reason.

SkipValidation bypasses tag compilation and schema checks while keeping option sanity, byte limits, one-document parsing for Unmarshal, and yaml.v3 decoding. Marshal still rejects ordinary cyclic Go values. A custom YAML or text codec with unknown shape needs a registry type binding or an explicit type=any rule. Compilation never invokes codec hooks.

The runtime supports strings, booleans, integers, floats, structs, slices, arrays, string-keyed maps, pointers, interfaces, time.Time, time.Duration, and byte slices. Arrays require exact item counts. Numeric destination representability is checked before decode. Recursive Go type graphs and non-string map keys are schema errors. Generated source/type frontends, streaming, and default interpolation are outside this stage. The cycle preflight does not execute custom IsZero methods; it can conservatively reject a cyclic field that a custom IsZero would omit during encoding.

See the runnable example in examples/highlevel.
