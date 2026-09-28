# High-level YAML API

The high-level API validates one YAML document when encoding or decoding a Go value. Package Marshal and Unmarshal use zero-value options; validation is enabled by default. Standard gopkg.in/yaml.v3 Marshal and Unmarshal do not interpret yamlvalidate tags. They continue to use caller-written YAML hooks when present. Optional typed codecs and standard YAML hooks can be generated with yamlvalidator-gen; see [code generation](code-generation.md).

Start with the [complete runnable examples](#examples), then use the
[tags and scopes reference](#tags-and-scopes), [Registry reference](#registry),
and [Options and limits reference](#options-and-limits).

Unmarshal obtains a cached or newly compiled type plan, parses one document into a yaml.Node, validates that parsed node, then decodes once. Parse and validation failures leave the destination unchanged. A late codec error may partly update it. Marshal compiles the encode type, checks ordinary Go cycles, encodes once, parses and validates the actual output, then returns those bytes. Custom hooks run once. An invalid output returns nil bytes. Omitempty affects the validated output exactly as it affects yaml.v3 encoding.

## Examples

Each Go block in this section is a complete standalone program. Use one example
at a time; repeated type names belong to separate examples. The YAML strings are
the input documents and the text blocks show the expected output. Repository tests
compile and execute these exact Go blocks rather than a separately maintained copy.

| Example | What it demonstrates |
| --- | --- |
| [1. Load and serialize a typed configuration](#quick-start) | This complete program needs only the root package |
| [2. Distinguish required keys, zero values and null](#presence) | <code>required</code> means that the YAML key exists, not that the Go value is nonzero |
| [3. Keep the destination unchanged on validation failure](#unchanged) | A previously populated configuration is not updated when preliminary schema or YAML validation fails |
| [4. Validate lists, map keys, map values and nested lists](#collections) | items and values can be nested; no separate dive marker is needed to discover nested struct fields |
| [5. Give a dynamic map a closed native schema](#dynamic-map) | A map is open by default |
| [6. Declare an invariant on one ordinary field](#parent-group) | Exactly one of file, url and inline must be present in Source |
| [7. Add rules at a use site, including collection elements](#use-site) | The reusable Source type below has no intrinsic group |
| [8. Combine dependencies with environment-dependent requirements](#conditional) | Production requires a non-null TLS object containing both cert and key, and forbids the debug key |
| [9. Distinguish complete groups from strictly exclusive alternatives](#complete-groups) | oneOfRequired counts fully present groups |
| [10. Flatten credentials and validate only captured inline-map entries](#inline) | Rules on an inline struct use the actual flattened YAML keys |
| [11. Choose error, warning or ignore for unknown struct keys](#unknown-keys) | The high-level default is stricter than the native low-level inherited policy: an unknown struct key is an error |
| [12. Collect warnings without silently filling defaults](#warnings-defaults) | default is an annotation, not a configuration-merging mechanism |
| [13. Inspect validation errors and bad tag definitions separately](#diagnostics) | Use errors.As to distinguish YAML failures from invalid tag definitions |
| [14. Reuse named checks and native schemas with a Registry](#named-registry) | A named check selects a ValueValidator; ref selects a native FieldSchema |
| [15. Implement a parameterized native check](#factory) | A factory receives structured tag arguments and returns an ordinary native validator |
| [16. Bind a schema to a custom YAML representation](#custom-codec) | This struct is encoded as one URI string, not as a mapping of its Go fields |
| [17. Accept alternative YAML shapes without JSON Schema](#alternatives) | Native schema alternatives are useful for a dynamic field that may contain simple shorthand or an explicit object |
| [18. Apply an additional native contract through Options](#additional-schema) | Use Schema when a deployment or caller imposes restrictions beyond the tags |
| [19. Understand required together with omitempty during Marshal](#omitempty) | Validation checks what is actually emitted, not the original Go field |
| [20. Handle recursive models and bound validation work](#recursion-limits) | Recursive types are supported for finite data without generation |
| [21. Opt out explicitly without losing the single-document boundary](#skip-validation) | SkipValidation is an explicit escape hatch |

<a id="quick-start"></a>

### 1. Load and serialize a typed configuration

This complete program needs only the root package. In an existing module, run
<code>go get github.com/Yakwilik/go-yamlvalidator@v1.1.0</code> first. No generation
step or separately constructed schema is required.

<!-- highlevel-example: quick-start -->
~~~go
package main

import (
    "fmt"
    yamlvalidator "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
    Host string "yaml:\"host\" yamlvalidate:\"required,nonempty,format=hostname\""
    Port uint16 "yaml:\"port\" yamlvalidate:\"required,min=1\""
}

func main() {
    data := []byte("host: api.example.org\nport: 8443\n")
    var config Config
    if err := yamlvalidator.Unmarshal(data, &config); err != nil {
        panic(err)
    }
    encoded, err := (yamlvalidator.MarshalOptions{Indent: 2}).Marshal(config)
    if err != nil {
        panic(err)
    }
    fmt.Print(string(encoded))
}
~~~

Expected output:

~~~text
host: api.example.org
port: 8443
~~~

To load a file instead, obtain data with <code>os.ReadFile</code>, check its error,
and pass the bytes to the same Unmarshal call. The validation rules do not depend
on how the bytes were obtained. Unlike a raw yaml.v3 call, this also rejects an
unknown key such as <code>hostname</code> instead of silently leaving Host empty.


<a id="presence"></a>

### 2. Distinguish required keys, zero values and null

<code>required</code> means that the YAML key exists, not that the Go value is
nonzero. Add <code>notnull</code> when a nil-capable value must not be null.

<!-- highlevel-example: presence -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
    Enabled bool "yaml:\"enabled\" yamlvalidate:\"required\""
    Retries *int "yaml:\"retries\" yamlvalidate:\"required,notnull,min=0\""
}

func main() {
    var cfg Config
    err := v.Unmarshal([]byte("enabled: false\nretries: 0\n"), &cfg)
    fmt.Println("zero values accepted:", err == nil && !cfg.Enabled && *cfg.Retries == 0)
    err = v.Unmarshal([]byte("enabled: false\nretries: null\n"), &cfg)
    fmt.Println("null rejected:", err != nil)
    err = v.Unmarshal([]byte("retries: 0\n"), &cfg)
    fmt.Println("missing enabled rejected:", err != nil)
}
~~~

Expected output:

~~~text
zero values accepted: true
null rejected: true
missing enabled rejected: true
~~~

For an optional pointer, remove required. Removing notnull also permits explicit
null. Missing and null normally both leave a zero-initialized pointer nil; tags do
not introduce a separate presence-tracking Go type. For a string that must contain
text, use required,nonempty: required alone permits an empty string.


<a id="unchanged"></a>

### 3. Keep the destination unchanged on validation failure

A previously populated configuration is not updated when preliminary schema or
YAML validation fails. The following invalid port prevents even the valid name
from being assigned.

<!-- highlevel-example: unchanged -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
    Name string "yaml:\"name\" yamlvalidate:\"required,nonempty\""
    Port uint16 "yaml:\"port\" yamlvalidate:\"required,min=1\""
}

func main() {
    cfg := Config{Name: "running", Port: 9000}
    err := v.Unmarshal([]byte("name: replacement\nport: 0\n"), &cfg)
    fmt.Println("rejected:", err != nil)
    fmt.Printf("still running: %s:%d\n", cfg.Name, cfg.Port)
}
~~~

Expected output:

~~~text
rejected: true
still running: running:9000
~~~

This is not a transaction around arbitrary codec code. Once actual decoding
starts, a late decoder or custom UnmarshalYAML error can leave partial changes.
Custom hooks can also have their own external side effects.


<a id="collections"></a>

### 4. Validate lists, map keys, map values and nested lists

items and values can be nested; no separate dive marker is needed to discover
nested struct fields. This example also combines uniqueness and native hostname
validation.

<!-- highlevel-example: collections -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
    Hosts []string "yaml:\"hosts\" yamlvalidate:\"required,minItems=1,uniqueItems,items={nonempty,format=hostname}\""
    Limits map[string]int "yaml:\"limits\" yamlvalidate:\"keys={pattern='^[a-z][a-z0-9_-]*$'},values={min=1,max=100}\""
    Matrix [][]int "yaml:\"matrix\" yamlvalidate:\"items={minItems=1,items={min=0,max=255}}\""
}

func main() {
    cases := []struct{ name, input string }{
        {"valid", "hosts: [api.example.org]\nlimits: {worker_1: 5}\nmatrix: [[0, 255], [42]]\n"},
        {"duplicate host", "hosts: [api.example.org, api.example.org]\n"},
        {"invalid key", "hosts: [api.example.org]\nlimits: {Worker: 5}\n"},
        {"invalid value", "hosts: [api.example.org]\nlimits: {worker: 101}\n"},
        {"invalid nested item", "hosts: [api.example.org]\nmatrix: [[256]]\n"},
    }
    for _, tc := range cases {
        var cfg Config
        fmt.Printf("%s: accepted=%t\n", tc.name, v.Unmarshal([]byte(tc.input), &cfg) == nil)
    }
}
~~~

Expected output:

~~~text
valid: accepted=true
duplicate host: accepted=false
invalid key: accepted=false
invalid value: accepted=false
invalid nested item: accepted=false
~~~

Limits and Matrix are optional in this example. To require a nonempty mapping,
add required,nonempty. uniqueItems compares native YAML values: a numeric 1 and a
string '1' are distinct, while equivalent standard numeric spellings normalize.


<a id="dynamic-map"></a>

### 5. Give a dynamic map a closed native schema

A map is open by default. Declare specific properties and additional=forbid to
reject everything outside the contract; unknown=error alone does not close an
otherwise unrestricted Go map.

<!-- highlevel-example: dynamic-map -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
    Settings map[string]any "yaml:\"settings\" yamlvalidate:\"required,notnull,properties={host={type=string,required,format=hostname},port={type=int,required,min=1,max=65535}},additional=forbid\""
}

func main() {
    for _, input := range []string{
        "settings: {host: api.example.org, port: 443}\n",
        "settings: {host: api.example.org, port: 443, debug: true}\n",
        "settings: {host: api.example.org}\n",
        "settings: {host: api.example.org, port: '443'}\n",
    } {
        var cfg Config
        fmt.Println("accepted:", v.Unmarshal([]byte(input), &cfg) == nil)
    }
}
~~~

Expected output:

~~~text
accepted: true
accepted: false
accepted: false
accepted: false
~~~

The last input is rejected because '443' is a string, not an integer. A type rule
is not a request to coerce values. Prefer a named struct for a fixed application
model; properties is useful when the application deliberately retains a dynamic map.


<a id="parent-group"></a>

### 6. Declare an invariant on one ordinary field

Exactly one of file, url and inline must be present in Source. The group belongs
to Source, not to the File value: it runs even when file is absent.

<!-- highlevel-example: parent-group -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Source struct {
    File string "yaml:\"file,omitempty\" yamlvalidate:\"exactlyOneOf=[file,url,inline],nonempty\""
    URL string "yaml:\"url,omitempty\" yamlvalidate:\"nonempty,url={requireScheme=true,schemes=[https]}\""
    Inline string "yaml:\"inline,omitempty\" yamlvalidate:\"nonempty\""
}

func main() {
    for _, input := range []string{
        "url: https://example.org/config.yaml\n",
        "{}\n",
        "file: local.yaml\nurl: https://example.org/config.yaml\n",
        "file: ''\n",
    } {
        var source Source
        fmt.Println("accepted:", v.Unmarshal([]byte(input), &source) == nil)
    }
}
~~~

Expected output:

~~~text
accepted: true
accepted: false
accepted: false
accepted: false
~~~

The first case succeeds without the carrier key file. In the last case exactly
one key exists, but its value fails nonempty. No blank metadata field is needed;
all participants must be explicitly named using their YAML names.


<a id="use-site"></a>

### 7. Add rules at a use site, including collection elements

The reusable Source type below has no intrinsic group. Primary, Optional, each
List element and each Named value acquire their own exactly-one rule where used.

<!-- highlevel-example: use-site -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Source struct {
    File string "yaml:\"file,omitempty\" yamlvalidate:\"nonempty\""
    URL string "yaml:\"url,omitempty\" yamlvalidate:\"nonempty\""
}

type Config struct {
    Primary Source "yaml:\"primary\" yamlvalidate:\"required,exactlyOneOfKeys=[file,url]\""
    Optional *Source "yaml:\"optional,omitempty\" yamlvalidate:\"notnull,exactlyOneOfKeys=[file,url]\""
    List []Source "yaml:\"list\" yamlvalidate:\"items={exactlyOneOfKeys=[file,url]}\""
    Named map[string]Source "yaml:\"named\" yamlvalidate:\"values={exactlyOneOfKeys=[file,url]}\""
}

func main() {
    cases := []struct{ name, input string }{
        {"valid", "primary: {file: main.yaml}\nlist: [{url: remote}]\nnamed: {backup: {file: backup.yaml}}\n"},
        {"empty primary", "primary: {}\n"},
        {"two keys in list item", "primary: {file: main.yaml}\nlist: [{file: a, url: b}]\n"},
        {"empty map value", "primary: {file: main.yaml}\nnamed: {backup: {}}\n"},
        {"explicit null", "primary: {file: main.yaml}\noptional: null\n"},
    }
    for _, tc := range cases {
        var cfg Config
        fmt.Printf("%s: accepted=%t\n", tc.name, v.Unmarshal([]byte(tc.input), &cfg) == nil)
    }
}
~~~

Expected output:

~~~text
valid: accepted=true
empty primary: accepted=false
two keys in list item: accepted=false
empty map value: accepted=false
explicit null: accepted=false
~~~

Omitting optional is valid; its internal group does not make the container
required. notnull rejects an explicitly present null. Bare exactlyOneOf inside
items or values would be a SchemaError: use the Keys form for that mapping value.
Intrinsic and use-site restrictions are combined, not substituted for each other.


<a id="conditional"></a>

### 8. Combine dependencies with environment-dependent requirements

Production requires a non-null TLS object containing both cert and key, and
forbids the debug key. In development TLS is optional, but a supplied certificate
still requires its matching key.

<!-- highlevel-example: conditional -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type TLS struct {
    Cert string "yaml:\"cert,omitempty\" yamlvalidate:\"nonempty,dependentRequired={cert=[key],key=[cert]}\""
    Key string "yaml:\"key,omitempty\" yamlvalidate:\"nonempty\""
}

type Config struct {
    Mode string "yaml:\"mode\" yamlvalidate:\"required,enum=[dev,prod],when={field=mode,eq=prod,require=[tls],forbid=[debug]}\""
    TLS *TLS "yaml:\"tls,omitempty\" yamlvalidate:\"notnull,requireKeys=[cert,key]\""
    Debug bool "yaml:\"debug,omitempty\""
}

func main() {
    for _, input := range []string{
        "mode: dev\n",
        "mode: prod\ntls: {cert: cert.pem, key: key.pem}\n",
        "mode: prod\n",
        "mode: dev\ntls: {cert: cert.pem}\n",
        "mode: prod\ntls: {cert: cert.pem, key: key.pem}\ndebug: false\n",
    } {
        var cfg Config
        fmt.Println("accepted:", v.Unmarshal([]byte(input), &cfg) == nil)
    }
}
~~~

Expected output:

~~~text
accepted: true
accepted: true
accepted: false
accepted: false
accepted: false
~~~

The final case fails even though debug is false: forbid is about key presence.
The same applies to mutuallyExclusive and forbiddenTogether. when compares scalar
text at the declared sibling key; it is not a Go expression evaluator.


<a id="complete-groups"></a>

### 9. Distinguish complete groups from strictly exclusive alternatives

oneOfRequired counts fully present groups. It does not automatically prohibit
partial fields from another group. Add forbiddenTogether when no mixing is allowed.

<!-- highlevel-example: complete-groups -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Connection struct {
    DSN string "yaml:\"dsn,omitempty\" yamlvalidate:\"oneOfRequired=[[dsn],[host,port]]\""
    Host string "yaml:\"host,omitempty\""
    Port uint16 "yaml:\"port,omitempty\" yamlvalidate:\"min=1\""
}

type StrictConnection struct {
    DSN string "yaml:\"dsn,omitempty\" yamlvalidate:\"oneOfRequired=[[dsn],[host,port]],forbiddenTogether=[[dsn,host],[dsn,port]]\""
    Host string "yaml:\"host,omitempty\""
    Port uint16 "yaml:\"port,omitempty\" yamlvalidate:\"min=1\""
}

func main() {
    for _, input := range []string{
        "host: db.example.org\nport: 5432\n",
        "dsn: database\nhost: db.example.org\n",
        "dsn: database\nhost: db.example.org\nport: 5432\n",
    } {
        var a Connection
        var b StrictConnection
        fmt.Printf("one complete group: %t; no mixing: %t\n",
            v.Unmarshal([]byte(input), &a) == nil,
            v.Unmarshal([]byte(input), &b) == nil)
    }
}
~~~

Expected output:

~~~text
one complete group: true; no mixing: true
one complete group: true; no mixing: false
one complete group: false; no mixing: false
~~~

Use anyOfRequired instead when at least one complete group is sufficient and
multiple complete groups are allowed. mutuallyExclusive=[a,b] permits neither key
or one key; exactlyOneOf=[a,b] requires one. forbiddenTogether=[[a,b,c]] rejects the
complete triple, not each pair.


<a id="inline"></a>

### 10. Flatten credentials and validate only captured inline-map entries

Rules on an inline struct use the actual flattened YAML keys. Rules on an
inline map apply only to keys captured by that map, not the declared Job fields.

<!-- highlevel-example: inline -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Credentials struct {
    Token string "yaml:\"token,omitempty\" yamlvalidate:\"nonempty,exactlyOneOf=[token,user]\""
    User string "yaml:\"user,omitempty\" yamlvalidate:\"nonempty,dependentRequired={user=[password],password=[user]}\""
    Password string "yaml:\"password,omitempty\" yamlvalidate:\"nonempty\""
}

type Job struct {
    Name string "yaml:\"name\" yamlvalidate:\"required,nonempty\""
    Credentials "yaml:\",inline\""
    Extra map[string]string "yaml:\",inline\" yamlvalidate:\"keys={pattern='^x_'},values={nonempty}\""
}

func main() {
    for _, input := range []string{
        "name: build\ntoken: secret\nx_region: eu\n",
        "name: build\nuser: alice\npassword: secret\n",
        "name: build\ntoken: secret\nregion: eu\n",
        "name: build\nuser: alice\n",
    } {
        var job Job
        fmt.Println("accepted:", v.Unmarshal([]byte(input), &job) == nil)
    }
}
~~~

Expected output:

~~~text
accepted: true
accepted: true
accepted: false
accepted: false
~~~

There is no credentials: nesting in the YAML. The keys name, token, user and
password do not need the x_ prefix because they are not captured by Extra. Do not
put real credentials in diagnostic logs: source-aware formatting can include them.


<a id="unknown-keys"></a>

### 11. Choose error, warning or ignore for unknown struct keys

The high-level default is stricter than the native low-level inherited policy:
an unknown struct key is an error. Options can make this a warning or ignore it.

<!-- highlevel-example: unknown-keys -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
    Name string "yaml:\"name\" yamlvalidate:\"required,nonempty\""
}

func main() {
    data := []byte("name: api\nextra: true\n")
    var cfg Config
    fmt.Println("default rejects:", v.Unmarshal(data, &cfg) != nil)

    var diagnostics []v.ValidationError
    warn := v.UnmarshalOptions{
        UnknownKeyPolicy: v.UnknownKeyWarn,
        OnDiagnostic: func(d v.ValidationError) { diagnostics = append(diagnostics, d) },
    }
    err := warn.Unmarshal(data, &cfg)
    fmt.Printf("warning mode: accepted=%t, diagnostics=%d\n", err == nil, len(diagnostics))

    ignore := v.UnmarshalOptions{UnknownKeyPolicy: v.UnknownKeyIgnore}
    fmt.Println("ignore mode accepts:", ignore.Unmarshal(data, &cfg) == nil)
}
~~~

Expected output:

~~~text
default rejects: true
warning mode: accepted=true, diagnostics=1
ignore mode accepts: true
~~~

Ignoring an unknown key does not store its value in the struct. Use an inline
map to retain extension fields. An explicit unknown rule on a nested mapping
has precedence over the per-operation inherited policy.


<a id="warnings-defaults"></a>

### 12. Collect warnings without silently filling defaults

default is an annotation, not a configuration-merging mechanism. The missing
mode below produces a warning but remains the empty Go string. A supplied legacy
key produces another warning.

<!-- highlevel-example: warnings-defaults -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
    Mode string "yaml:\"mode\" yamlvalidate:\"enum=[dev,prod],default=dev\""
    Legacy bool "yaml:\"legacy,omitempty\" yamlvalidate:\"deprecated='use features instead'\""
}

func main() {
    var warnings []v.ValidationError
    opts := v.UnmarshalOptions{
        OnDiagnostic: func(d v.ValidationError) {
            if d.Level == v.LevelWarning { warnings = append(warnings, d) }
        },
    }
    var cfg Config
    err := opts.Unmarshal([]byte("legacy: false\n"), &cfg)
    fmt.Printf("accepted=%t, warnings=%d, mode=%q\n", err == nil, len(warnings), cfg.Mode)

    opts.WarningsAsErrors = true
    cfg = Config{Mode: "prod"}
    err = opts.Unmarshal([]byte("legacy: false\n"), &cfg)
    fmt.Printf("strict rejected=%t, previous mode=%q\n", err != nil, cfg.Mode)
}
~~~

Expected output:

~~~text
accepted=true, warnings=2, mode=""
strict rejected=true, previous mode="prod"
~~~

WarningsAsErrors returns an error without decoding, but the diagnostic entries
retain their warning severity. OnDiagnostic is synchronous; use a per-call
collector or protect shared mutable state when reusing Options concurrently.


<a id="diagnostics"></a>

### 13. Inspect validation errors and bad tag definitions separately

Use errors.As rather than parsing Error() strings. Diagnostics identifies YAML
failures; SchemaError identifies an invalid declaration before decoding starts.

<!-- highlevel-example: diagnostics -->
~~~go
package main

import (
    "errors"
    "fmt"
    "strings"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
    Port uint16 "yaml:\"port\" yamlvalidate:\"required,min=1\""
}

type BadDefinition struct {
    Port uint16 "yaml:\"port\" yamlvalidate:\"minimum=1\""
}

func main() {
    var cfg Config
    err := v.Unmarshal([]byte("port: 0\n"), &cfg)
    var validation *v.ValidationErrors
    if !errors.As(err, &validation) { panic(err) }
    d := validation.Diagnostics()[0]
    fmt.Printf("path=%s, line=%d, has code=%t\n", d.Path, d.Line, d.Code != "")
    fmt.Println("source excerpt available:", strings.Contains(validation.FormatWithSource(), "port: 0"))
    fmt.Println("source is Marshal output:", validation.GeneratedSource())

    var bad BadDefinition
    err = v.Unmarshal([]byte("port: 443\n"), &bad)
    var definition *v.SchemaError
    fmt.Println("bad tag is SchemaError:", errors.As(err, &definition))
}
~~~

Expected output:

~~~text
path=port, line=1, has code=true
source excerpt available: true
source is Marshal output: false
bad tag is SchemaError: true
~~~

The directive is min, not minimum. SchemaError includes Type, Field, Tag, Offset
and Reason. For other errors preserve the wrapped cause instead of assuming every
failure is a ValidationErrors. FormatWithSource intentionally includes source
lines: do not log it automatically for configuration that may contain secrets.


<a id="named-registry"></a>

### 14. Reuse named checks and native schemas with a Registry

A named check selects a ValueValidator; ref selects a native FieldSchema.
Both are resolved from an explicitly supplied Registry, never a global registry.

<!-- highlevel-example: registry -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
    vv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
)

type Source struct {
    File string "yaml:\"file,omitempty\""
    URL string "yaml:\"url,omitempty\""
}

type Config struct {
    Mode string "yaml:\"mode\" yamlvalidate:\"required,check=environment\""
    Source Source "yaml:\"source\" yamlvalidate:\"required,ref=source\""
}

func main() {
    registry, err := v.NewRegistry(v.RegistryConfig{
        ValueValidators: map[string]v.ValueValidator{
            "environment": vv.EnumValidator{Allowed: []string{"dev", "prod"}},
        },
        Schemas: map[string]*v.FieldSchema{
            "source": {
                Type: v.TypeMap,
                AllowedKeys: map[string]*v.FieldSchema{
                    "file": {Type: v.TypeString},
                    "url": {Type: v.TypeString},
                },
                ExactlyOneOf: []string{"file", "url"},
            },
        },
    })
    if err != nil { panic(err) }
    opts := v.UnmarshalOptions{Registry: registry}
    var cfg Config
    fmt.Println("valid:", opts.Unmarshal([]byte("mode: prod\nsource: {file: local.yaml}\n"), &cfg) == nil)
    fmt.Println("bad mode rejected:", opts.Unmarshal([]byte("mode: test\nsource: {file: local.yaml}\n"), &cfg) != nil)
    fmt.Println("empty source rejected:", opts.Unmarshal([]byte("mode: prod\nsource: {}\n"), &cfg) != nil)
    _, err = (v.MarshalOptions{Registry: registry}).Marshal(cfg)
    fmt.Println("same registry on Marshal:", err == nil)
}
~~~

Expected output:

~~~text
valid: true
bad mode rejected: true
empty source rejected: true
same registry on Marshal: true
~~~

The two failed Unmarshal calls leave the previous valid cfg unchanged. Pass the
same Registry to Marshal when its tags refer to the same names. Calling the plain
package function with an unresolved check or ref is a SchemaError, not a reason to
silently skip that rule.


<a id="factory"></a>

### 15. Implement a parameterized native check

A factory receives structured tag arguments and returns an ordinary native
validator. Validate arguments when compiling the schema, not on each YAML value.

<!-- highlevel-example: factory -->
~~~go
package main

import (
    "fmt"
    "strings"
    v "github.com/Yakwilik/go-yamlvalidator"
    "gopkg.in/yaml.v3"
)

type Prefix struct { Value string }

func (p Prefix) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
    if !strings.HasPrefix(node.Value, p.Value) {
        ctx.AddError(v.ValidationError{
            Level: v.LevelError, Code: "prefix", Path: path,
            Line: node.Line, Column: node.Column,
            Message: "value must start with " + p.Value,
        })
    }
}

type Config struct {
    Name string "yaml:\"name\" yamlvalidate:\"required,check={name=prefix,args={value='svc-'}}\""
}

func main() {
    registry, err := v.NewRegistry(v.RegistryConfig{
        ValueFactories: map[string]v.ValueValidatorFactory{
            "prefix": func(args map[string]any) (v.ValueValidator, error) {
                prefix, ok := args["value"].(string)
                if len(args) != 1 || !ok || prefix == "" {
                    return nil, fmt.Errorf("prefix requires one nonempty string argument: value")
                }
                return Prefix{Value: prefix}, nil
            },
        },
    })
    if err != nil { panic(err) }
    opts := v.UnmarshalOptions{Registry: registry}
    var cfg Config
    fmt.Println("matching prefix:", opts.Unmarshal([]byte("name: svc-api\n"), &cfg) == nil)
    fmt.Println("wrong prefix rejected:", opts.Unmarshal([]byte("name: api\n"), &cfg) != nil)
}
~~~

Expected output:

~~~text
matching prefix: true
wrong prefix rejected: true
~~~

Numeric args are json.Number, not float64; lists and objects are []any and
map[string]any. KeyFactories provides the analogous extension for mapping keys.
Factories and returned validators must be safe for the sharing pattern of their
Registry. Cold concurrent compilation can call a factory more than once.


<a id="custom-codec"></a>

### 16. Bind a schema to a custom YAML representation

This struct is encoded as one URI string, not as a mapping of its Go fields.
A TypeBinding tells validation about that representation without invoking the
custom decoder during schema compilation.

<!-- highlevel-example: custom-codec -->
~~~go
package main

import (
    "fmt"
    "net/url"
    "reflect"
    v "github.com/Yakwilik/go-yamlvalidator"
    "gopkg.in/yaml.v3"
)

type Endpoint struct { URL *url.URL }

func (e *Endpoint) UnmarshalYAML(node *yaml.Node) error {
    parsed, err := url.Parse(node.Value)
    if err != nil { return err }
    if parsed.Scheme != "https" || parsed.Host == "" {
        return fmt.Errorf("endpoint must be an absolute HTTPS URL")
    }
    e.URL = parsed
    return nil
}

func (e Endpoint) MarshalYAML() (any, error) {
    if e.URL == nil { return nil, fmt.Errorf("endpoint URL is nil") }
    return e.URL.String(), nil
}

type Config struct {
    Endpoint Endpoint "yaml:\"endpoint\" yamlvalidate:\"required,url={requireScheme=true,schemes=[https]}\""
}

func main() {
    wire := &v.FieldSchema{Type: v.TypeString}
    registry, err := v.NewRegistry(v.RegistryConfig{
        TypeBindings: map[reflect.Type]v.TypeBinding{
            reflect.TypeFor[Endpoint](): {Encode: wire, Decode: wire},
        },
    })
    if err != nil { panic(err) }
    var cfg Config
    if err := (v.UnmarshalOptions{Registry: registry}).Unmarshal(
        []byte("endpoint: https://api.example.org/v1\n"), &cfg); err != nil { panic(err) }
    fmt.Println("host:", cfg.Endpoint.URL.Host)
    data, err := (v.MarshalOptions{Registry: registry}).Marshal(cfg)
    if err != nil { panic(err) }
    fmt.Print(string(data))
}
~~~

Expected output:

~~~text
host: api.example.org
endpoint: https://api.example.org/v1
~~~

Encode and Decode bindings may differ when the representations differ. A
custom codec with unknown shape otherwise needs a binding or an explicit type=any
rule. type=any deliberately stops inference; it is not an equally strict substitute
for a known wire schema. Plain yaml.v3 hooks cannot receive this per-call Registry.


<a id="alternatives"></a>

### 17. Accept alternative YAML shapes without JSON Schema

Native schema alternatives are useful for a dynamic field that may contain
simple shorthand or an explicit object. No JSON Schema tag or metadata field is
involved.

<!-- highlevel-example: alternatives -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
    Target any "yaml:\"target\" yamlvalidate:\"required,notnull,oneOfSchemas=[{type=string,nonempty},{type=map,properties={host={type=string,required,nonempty},port={type=int,required,min=1,max=65535}},additional=forbid}]\""
}

func main() {
    for _, input := range []string{
        "target: primary\n",
        "target: {host: api.example.org, port: 443}\n",
        "target: {host: api.example.org}\n",
        "target: []\n",
    } {
        var cfg Config
        fmt.Println("accepted:", v.Unmarshal([]byte(input), &cfg) == nil)
    }
}
~~~

Expected output:

~~~text
accepted: true
accepted: true
accepted: false
accepted: false
~~~

oneOfSchemas requires exactly one matching alternative. anyOfSchemas requires
at least one; alternatives may also refer to named Registry schemas. This is
validation, not a union-type decoder: Target remains an ordinary interface value,
not an automatically selected application struct.


<a id="additional-schema"></a>

### 18. Apply an additional native contract through Options

Use Schema when a deployment or caller imposes restrictions beyond the tags.
The additional schema is checked together with the inferred schema; it does not
replace it or populate missing properties.

<!-- highlevel-example: additional-schema -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
    vv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
)

type Config struct {
    Name string "yaml:\"name\" yamlvalidate:\"required,nonempty\""
    Mode string "yaml:\"mode\" yamlvalidate:\"required,enum=[dev,prod]\""
}

func main() {
    production, err := v.CompileFieldSchema(&v.FieldSchema{
        Type: v.TypeMap,
        AllowedKeys: map[string]*v.FieldSchema{
            "mode": {Type: v.TypeString, Validators: []v.ValueValidator{
                vv.EnumValidator{Allowed: []string{"prod"}},
            }},
        },
        AdditionalProperties: &v.FieldSchema{Type: v.TypeAny},
    })
    if err != nil { panic(err) }
    var cfg Config
    opts := v.UnmarshalOptions{Schema: production}
    fmt.Println("production accepted:", opts.Unmarshal([]byte("name: api\nmode: prod\n"), &cfg) == nil)
    fmt.Println("development rejected:", opts.Unmarshal([]byte("name: api\nmode: dev\n"), &cfg) != nil)
    fmt.Println("required name still checked:", opts.Unmarshal([]byte("mode: prod\n"), &cfg) != nil)
}
~~~

Expected output:

~~~text
production accepted: true
development rejected: true
required name still checked: true
~~~

AdditionalProperties=TypeAny keeps this extra contract from closing unrelated
fields such as name; the original typed schema still validates them. An additional
closed schema would apply its own unknown-key policy rather than merge its field
list with the inferred one. Compile a reusable schema outside the request loop.


<a id="omitempty"></a>

### 19. Understand required together with omitempty during Marshal

Validation checks what is actually emitted, not the original Go field. A zero
integer tagged required,omitempty disappears and then fails required. A non-nil
pointer can intentionally emit a zero instead.

<!-- highlevel-example: omitempty -->
~~~go
package main

import (
    "errors"
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type ValueConfig struct {
    Attempts int "yaml:\"attempts,omitempty\" yamlvalidate:\"required,min=0\""
}

type PointerConfig struct {
    Attempts *int "yaml:\"attempts,omitempty\" yamlvalidate:\"required,notnull,min=0\""
}

func main() {
    data, err := v.Marshal(ValueConfig{})
    var validation *v.ValidationErrors
    if !errors.As(err, &validation) { panic(err) }
    fmt.Println("invalid bytes withheld:", data == nil)
    fmt.Println("diagnostics describe generated YAML:", validation.GeneratedSource())

    zero := 0
    data, err = (v.MarshalOptions{Indent: 2}).Marshal(PointerConfig{Attempts: &zero})
    if err != nil { panic(err) }
    fmt.Print(string(data))
}
~~~

Expected output:

~~~text
invalid bytes withheld: true
diagnostics describe generated YAML: true
attempts: 0
~~~

Without omitempty a zero integer is emitted normally and required succeeds,
subject to any numeric bounds. Marshal returns nil bytes on a validation error;
do not try to use a partially valid result.


<a id="recursion-limits"></a>

### 20. Handle recursive models and bound validation work

Recursive types are supported for finite data without generation. Actual
cyclic Go values are a different case and are rejected when encoding.

<!-- highlevel-example: recursion-limits -->
~~~go
package main

import (
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Node struct {
    Name string "yaml:\"name\" yamlvalidate:\"required,nonempty\""
    Children []*Node "yaml:\"children,omitempty\""
}

func main() {
    data := []byte("name: root\nchildren:\n  - name: child\n    children:\n      - name: leaf\n")
    var tree Node
    if err := v.Unmarshal(data, &tree); err != nil { panic(err) }
    fmt.Println("leaf:", tree.Children[0].Children[0].Name)

    bounded := v.UnmarshalOptions{Limits: v.Limits{MaxDepth: 2}}
    fmt.Println("depth limit rejects:", bounded.Unmarshal(data, &tree) != nil)
    bounded.Limits = v.Limits{MaxBytes: 16}
    fmt.Println("byte limit rejects:", bounded.Unmarshal(data, &tree) != nil)

    cycle := &Node{Name: "cycle"}
    cycle.Children = []*Node{cycle}
    _, err := v.Marshal(cycle)
    fmt.Println("value cycle rejected:", err != nil)
}
~~~

Expected output:

~~~text
leaf: leaf
depth limit rejects: true
byte limit rejects: true
value cycle rejected: true
~~~

Zero limit fields select defaults; they do not mean unlimited. MaxDiagnostics
and MaxNodeVisits can also be set. Truncated validation returns an error rather
than decoding an unchecked tail; inspect ValidationErrors.Truncated(). These
limits are not a sandbox around arbitrary custom validators or codec hooks.


<a id="skip-validation"></a>

### 21. Opt out explicitly without losing the single-document boundary

SkipValidation is an explicit escape hatch. It bypasses tag compilation as well
as validation, so even a bad declaration below does not prevent decoding. Do not
use it automatically after a validation error.

<!-- highlevel-example: skip-validation -->
~~~go
package main

import (
    "errors"
    "fmt"
    v "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
    Port int "yaml:\"port\" yamlvalidate:\"unknown_directive\""
}

func main() {
    var cfg Config
    var definition *v.SchemaError
    err := v.Unmarshal([]byte("port: 0\n"), &cfg)
    fmt.Println("normal path rejects bad tag:", errors.As(err, &definition))

    opts := v.UnmarshalOptions{SkipValidation: true}
    err = opts.Unmarshal([]byte("port: 0\nextra: true\n"), &cfg)
    fmt.Printf("explicit skip: accepted=%t, port=%d\n", err == nil, cfg.Port)
    err = opts.Unmarshal([]byte("port: 1\n---\nport: 2\n"), &cfg)
    fmt.Println("multiple documents still rejected:", err != nil)
}
~~~

Expected output:

~~~text
normal path rejects bad tag: true
explicit skip: accepted=true, port=0
multiple documents still rejected: true
~~~

Options sanity, byte limits and one-document parsing still apply. Marshal also
continues to reject ordinary cyclic values. An unknown key ignored by the
underlying struct decoder does not become a retained extension field.


### Optional code generation

All examples above use the runtime-capable root API. Generation is optional:

~~~go
//go:generate go run github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen -all
~~~

Run <code>go generate ./...</code> after adding that directive to a real model
package. To check the generated file from that package without rewriting it:

~~~sh
go run github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen -all -check
~~~

After generation, ordinary <code>yaml.Unmarshal</code> and <code>yaml.Marshal</code>
use the generated YAML hooks with default validation settings. They do not accept
a per-call Registry or other yamlvalidator Options: examples requiring those
settings should continue to call the root Options API. Without generated or
user-written YAML hooks, ordinary yaml.v3 does not interpret yamlvalidate at all.

Selecting a root follows its statically known nested type graph. The -all flag
also selects eligible independent structs in the package. See the
[code-generation guide](code-generation.md) for supported roots, conflicts,
standard-hook boundaries, and the runnable generated example. In particular,
yaml.v3 may not invoke a custom UnmarshalYAML hook for null; the high-level byte
API enforces its own complete-document policy.

For more focused parent/current-value group examples with YAML blocks, see
[cross-field rule examples](tag-rules-examples.ru.md).

## Tags and scopes

The yaml tag controls field names, inline flattening, and codec flags. The yamlvalidate tag declares validation. A blank _ field with yamlvalidate is a SchemaError, as is yamljsonschema. Unexported and YAML-excluded fields cannot carry validation declarations. Unknown directives, including jsonschema, fail compilation.

~~~text
rules  := entry ("," entry)*
entry  := key | key "=" value
value  := atom | quoted-string | "[" values? "]" | "{" rules? "}"
values := value ("," value)*
key    := identifier | quoted-string
~~~

Commas separate at their current nesting level. Single and double quoted values support escaped backslash, quote, newline, tab, and carriage return. Numeric atoms retain their spelling. The parser limits tags to 16 KiB and 64 nested levels. The reflection-independent grammar and scope normalization live in internal/taglang and are shared by the runtime and source-generation frontends.

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

See the [named check and schema example](#named-registry), the
[parameterized factory](#factory), and the [custom codec binding](#custom-codec)
for complete Registry setup and matching encode/decode Options.

Factories receive structured arguments. Numeric arguments are json.Number values, retaining their spelling. Factory functions and opaque validator state must be safe for concurrent use when shared. Cold concurrent calls may compile the same type more than once, so factories may run more than once. The successful type plan cache is bounded and isolated per registry. Compilers invoke user factories outside the cache lock. Excessive simultaneous or recursive compilation of the same type fails with a schema error.

## Options and limits

UnmarshalOptions and MarshalOptions include UnknownKeyPolicy, WarningsAsErrors, OnDiagnostic, StopOnFirst, Limits, Registry, Schema, and SkipValidation. MarshalOptions also has Indent. The default unknown key policy is error. OnDiagnostic runs synchronously for final diagnostics. WarningsAsErrors turns warnings into one public ValidationErrors return; validation failure does not decode into the destination.

Default limits are 8 MiB input/output, depth 128, 100 diagnostics, and 1,000,000 node visits. Zero selects the default; negative values are errors. Truncated validation fails closed. These limits do not bound all allocations inside yaml.v3 or the execution time and allocations of custom hooks. ValidationErrors offers Diagnostics and FormatWithSource; GeneratedSource indicates that coordinates refer to Marshal output. SchemaError identifies the offending type, field, tag, byte offset, and reason.

SkipValidation bypasses tag compilation and schema checks while keeping option sanity, byte limits, one-document parsing for Unmarshal, and yaml.v3 decoding. Marshal still rejects ordinary cyclic Go values. A custom YAML or text codec with unknown shape needs a registry type binding or an explicit type=any rule. Compilation never invokes codec hooks.

The runtime supports strings, booleans, integers, floats, structs, slices, arrays, string-keyed maps, pointers, interfaces, time.Time, time.Duration, and byte slices. Arrays require exact item counts. Numeric destination representability is checked before decode. Recursive Go type graphs are supported when each schema recursion descends into a YAML child value; non-string map keys remain schema errors. Streaming and default interpolation are not implemented. The optional source frontend and typed generated codecs are documented separately. Reflection-path cycle preflight does not execute custom IsZero methods; it can conservatively reject a cyclic field that a custom IsZero would omit during encoding.

See the runnable example in examples/highlevel.
