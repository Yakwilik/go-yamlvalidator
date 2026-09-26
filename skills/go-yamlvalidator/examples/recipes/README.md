# Executable recipes

These test files are complete Go programs in test form. Copy a relevant schema,
callback and test table into an existing module rather than treating every
recipe as a mandatory framework. They use only public library APIs.

| File | Scenarios |
| --- | --- |
| [native_test.go](native_test.go) | Unknown-key policies, presence/null/defaults, containers, every cross-field rule and schema alternatives |
| [builtins_test.go](builtins_test.go) | All native value and key validators, exact bounds and malformed definitions |
| [custom_test.go](custom_test.go) | Application-defined function adapter, custom key/value types, definition validation, cloners |
| [jsonschema_test.go](jsonschema_test.go) | Dialects, scalar/object/array rules, combinations, references, format policies, empty stream |
| [extensions_test.go](extensions_test.go) | Custom formats, Validate/Compile/CompileWithContext, vocabularies, all subschema selectors, evaluated markers |
| [content_test.go](content_test.go) | Custom content encoding, media validation, decoded contentSchema |
| [resolvers_test.go](resolvers_test.go) | Resources, ResolverFunc, chain/cache, rooted files, cancellation and no implicit external loading |
| [diagnostics_test.go](diagnostics_test.go) | Structured diagnostics, source positions, paths, YAML syntax, aliases/merges, duplicate keys, multi-document input |
| [execution_test.go](execution_test.go) | All validation entry points, read errors, cancellation, incomplete results, bounded reading and concurrent reuse |
| [helpers_test.go](helpers_test.go) | Table-driven assertions shared by the recipes |

In the library checkout:

~~~bash
go test -count=1 ./skills/go-yamlvalidator/examples/...
go test -race ./skills/go-yamlvalidator/examples/...
~~~

For a skill installed outside the library repository, explicitly run
[the checker](../../scripts/check-examples.sh). It creates a temporary module,
requires v1.0.0, and tests these files there. The caller's module is not changed:

~~~bash
bash /path/to/go-yamlvalidator-skill/scripts/check-examples.sh
bash /path/to/go-yamlvalidator-skill/scripts/check-examples.sh --local /path/to/go-yamlvalidator
~~~

The second form compares the examples with a local checkout. It does not prove
compatibility with the published version; run the first form for that check.
The optional <code>--race</code> flag additionally enables Go's race detector.
Dependencies may be downloaded if they are not cached. No check is run
implicitly by installing the skill.
