# High-level struct tags and code generation

This reference targets go-yamlvalidator v1.2.0.

Use this path when the application already has Go configuration structs and wants
validation rules next to fields without maintaining a separate FieldSchema.

## High-level entry points

~~~go
func yamlvalidator.Unmarshal(data []byte, out any) error
func yamlvalidator.Marshal(in any) ([]byte, error)

func (o yamlvalidator.UnmarshalOptions) Unmarshal(data []byte, out any) error
func (o yamlvalidator.MarshalOptions) Marshal(in any) ([]byte, error)
~~~

The zero-value Options enable validation. Unmarshal accepts exactly one YAML
document, validates the parsed YAML tree before decoding, and does not mutate the
destination on parse/schema/validation failure. Marshal validates the actual YAML
representation it emits, including omitempty, inline fields and custom codec output.

Unknown struct fields are errors by default in this high-level API.

## Field naming

In v1.2.0, select exact exported Go field names independently for each operation:

~~~go
type Config struct {
    SkipValidateSanPortsForceUPTask bool
    ImageName string `json:"image_name"`
    Host string `yaml:"server_host"`
}

var config Config
options := v.UnmarshalOptions{
    FieldNaming:      v.FieldNamingGo,
    WarningsAsErrors: true,
}
if err := options.Unmarshal(data, &config); err != nil {
    return err
}
encoded, err := (v.MarshalOptions{
    FieldNaming: v.FieldNamingGo,
    Indent:      2,
}).Marshal(config)
~~~

The input keys are <code>SkipValidateSanPortsForceUPTask</code>,
<code>ImageName</code> and <code>server_host</code>. The fragment belongs in a
function returning an error; use <code>encoded</code> and check the final error.
No new struct tags, key rewriting or generation are required.

- <code>FieldNamingYAML</code> is zero and keeps yaml.v3's lowercase defaults.
  Package Marshal/Unmarshal and zero-value Options retain that mode.
- An explicit nonempty <code>yaml</code> name wins in both modes. Tags containing
  only omitempty/flow flags use the selected default name and retain their flags.
  Exclusion, export and inline rules retain their existing semantics.
- Matching is exact and case-sensitive. In Go mode, <code>imagename</code> and
  <code>image_name</code> do not populate <code>ImageName</code>. UnknownKeyPolicy
  handles these keys; ignoring one does not turn it into an alias.
- The mode applies recursively through pointers, slices, arrays, map values and
  inline structs. Map keys are data: <code>product_UP</code> and
  <code>product_up</code> remain distinct. Custom YAML/text codecs control their
  own representation and receive original source nodes.
- Group and conditional rules refer to actual YAML keys for the selected mode.
  Do not rename their arguments or explicit Schema/Registry keys automatically.
- SkipValidation retains field naming and conflict preflight. Invalid enum values
  fail before codecs run; name conflicts fail before destination writes. Shared
  Registry calls in different modes use independent cached plans.

New generated codecs carry the mode through typed schemas and codecs. Older
files use a safe reflection fallback in Go mode; regeneration restores the typed
path and is optional for correctness. Direct yaml.v3 generated hooks receive no
Options and continue to use standard YAML naming. No fixed naming CLI flag is
needed.

See [the executable recipe](../examples/recipes/field_naming_test.go). This API is
new in v1.2.0; inspect the selected version before using it in an older project.

## Basic yamlvalidate tags

~~~go
type Config struct {
    Name  string   `yaml:"name" yamlvalidate:"required,nonempty,minLength=2,maxLength=63"`
    Mode  string   `yaml:"mode" yamlvalidate:"required,enum=[dev,prod]"`
    Port  uint16   `yaml:"port" yamlvalidate:"min=1,max=65535"`
    Hosts []string `yaml:"hosts" yamlvalidate:"required,minItems=1,items={nonempty,format=hostname}"`
}
~~~

Common native directives include:

- presence/null: required, nullable, notnull, nonempty;
- scalar constraints: min, max, minLength, maxLength, enum, pattern, url, format;
- sequences: items, minItems, maxItems, uniqueItems;
- mappings: keys, values, properties, additional, minProperties, maxProperties,
  unknown;
- annotations: deprecated, description, default;
- composition/extensions: check, checks, ref, oneOfSchemas, anyOfSchemas.

Defaults are diagnostic annotations; they do not populate missing Go fields.

## Object-rule scopes

Bare object-group directives on a serializable struct field constrain sibling
keys in the containing mapping. They execute even when the carrier field is
absent from the input.

~~~go
type Source struct {
    File   string `yaml:"file,omitempty" yamlvalidate:"exactlyOneOf=[file,url,inline],nonempty"`
    URL    string `yaml:"url,omitempty"`
    Inline string `yaml:"inline,omitempty"`
}
~~~

Here exactlyOneOf applies to Source itself. The field carrying the tag is not
implicitly added to the list; the list is explicit.

The Keys variants constrain the mapping value of the current field:

~~~go
type Config struct {
    Source map[string]any `yaml:"source" yamlvalidate:"exactlyOneOfKeys=[file,url,inline]"`
}
~~~

The same parent/current-value split exists for:

- exactlyOneOf / exactlyOneOfKeys
- mutuallyExclusive / mutuallyExclusiveKeys
- anyOfRequired / anyOfRequiredKeys
- oneOfRequired / oneOfRequiredKeys
- forbiddenTogether / forbiddenTogetherKeys
- dependentRequired / dependentRequiredKeys
- when / whenKeys
- require / requireKeys

Inside items/values use the explicit Keys form for mapping-value rules, for
example items={exactlyOneOfKeys=[file,url]}.

## Registry and custom codecs

Use UnmarshalOptions.Registry / MarshalOptions.Registry for named native checks,
schema references and type bindings. A custom YAML/text codec whose wire shape
cannot be inferred requires a TypeBinding or an explicit type=any declaration.
Do not silently weaken an unknown custom representation.

## Standard yaml.v3 versus yamlvalidator

Without generated or user-written YAML hooks:

- yamlvalidator.Unmarshal / Marshal interpret yamlvalidate tags;
- yaml.Unmarshal / Marshal from gopkg.in/yaml.v3 do not.

With generated hooks, ordinary yaml.v3 calls use the generated codec and default
validation policy. Use yamlvalidator Options when per-call registry, diagnostics,
limits, field naming, warning policy or SkipValidation matters.

## Code generation

Run from the package containing the models:

~~~sh
go run github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen -all -output=zz_yamlvalidator_generated.go
~~~

Recommended go:generate directive:

~~~go
//go:generate go run github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen -all -output=zz_yamlvalidator_generated.go
~~~

Use -check in CI:

~~~sh
go run github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen -all -output=zz_yamlvalidator_generated.go -check
~~~

-all selects eligible named structs in the current package. Statically known
nested structs, pointers, slices, arrays and maps are followed transitively, so
they do not need to be repeated in -type. Generic structs, aliases,
build-constrained automatic roots and structs with conflicting user-written YAML
or generated methods are skipped by -all. An explicit -type remains strict and
can be combined with -all, for example to add a named scalar root.

Generated source contains typed field/collection mapping and an already
normalized native schema graph. It does not inspect generated struct fields with
reflection or parse yamlvalidate tags at runtime. Recursive Go type graphs are
supported for finite values; actual cyclic Go values are rejected during Marshal.

Keep generated source in version control.

## Performance interpretation

The checked-in Apple M4 Max benchmark fixture measured generated yaml.Node
encoding at about 12.4–13.5x faster than reflective node encoding and generated
node decoding about 15–21% faster. Full validated Unmarshal improved only about
2–6%, because YAML parsing and validation dominate total time. Do not turn the
node-codec result into a claim that the full library is 10x faster.

The measurements below are from v1.1.0, not a new v1.2.0 benchmark run.
See the repository [benchmark suite](https://github.com/Yakwilik/go-yamlvalidator/tree/v1.1.0/benchmarks)
and [recorded M4 Max results](https://github.com/Yakwilik/go-yamlvalidator/blob/v1.1.0/benchmarks/results/2026-09-28-m4-max.md)
for the exact command, methodology and measurements.
