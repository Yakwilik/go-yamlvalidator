# Generated runtime and public API boundaries

## Package ownership

The root <code>yamlvalidator</code> package is the application-facing API. It does not export generation helpers and does not depend on <code>genruntime</code>, directly or transitively.

The generated runtime and the root facade share one internal implementation; they do not register global callbacks or keep duplicate validation engines.

~~~text
application -> yamlvalidator ----------> internal/engine
                                             |
generated -> genruntime ---------------------+
                 |                           |
                 +-----> internal/yamlcodec <-+

generated -> genruntime/spec -> internal/genspec

cmd/yamlvalidator-gen -> internal/schemacompiler -> internal/engine
~~~

<code>internal/engine</code> owns native validation, registry-specific plans, errors and materialization of the generated schema. <code>internal/yamlcodec</code> owns shared node codecs, merge expansion, cycle checks and decode contexts. <code>genruntime</code> provides the generated-code ABI and standard YAML hook adapters.

Generation-time declarations and lowering are internal. The public <code>genruntime/spec</code> package exposes only the normalized data contract used by generated programs, not source graphs or parsed tag rules.

## Generated schema and hooks

A generated type supplies <code>YAMLValidatorSchemaSpec() (spec.Graph, []reflect.Type)</code>. It does not create a native <code>FieldSchema</code> and does not receive a Registry. The engine materializes that data privately on a plan-cache miss; the cache remains separated by type, direction and immutable Registry.

<code>reflect.Type</code> values in this contract identify types for bindings. Generated schema preparation never inspects struct fields or parses validation tags at runtime.

Generated standard hooks delegate to:

~~~go
func (value Config) MarshalYAML() (any, error) {
    return genruntime.MarshalYAML(value)
}

func (value *Config) UnmarshalYAML(node *yaml.Node) error {
    return genruntime.UnmarshalYAML(node, value)
}
~~~

The root package does not export <code>MarshalGenerated</code>, <code>UnmarshalGenerated</code>, <code>LowerGeneratedSchema</code> or <code>BuildGeneratedSchema</code>. The obsolete <code>SourceTypePlan</code> route is also removed.

The root and standard-hook paths share one plan cache and preserve their existing semantic distinction: high-level Options carry explicit configuration and source bytes; standard hooks use defaults and the node positions they receive. No input source is fabricated.

## Regeneration

The generated ABI is introduced in v1.1.0. Regenerate any output produced by development snapshots when updating the generator:

~~~sh
go generate ./examples/codegen/model ./benchmarks/model
~~~

For application packages, the existing <code>-all</code> mode and freshness check remain available. The generator overlays its old output during analysis, so it can replace files that still refer to the previous generated ABI.

## Compatibility boundary

The root facade preserves application imports and the names of public types, functions and constants. Public types are aliases to the shared implementation, not conversion wrappers. Existing custom validators and <code>errors.As</code> continue to work through the root names.

**Canonical reflection package paths change.** For example, <code>reflect.TypeFor[yamlvalidator.FieldSchema]().PkgPath()</code> now identifies <code>github.com/Yakwilik/go-yamlvalidator/internal/engine</code>. Type display names remain <code>yamlvalidator.FieldSchema</code>. Code that serializes or compares canonical package paths must account for this migration. Do not describe this as reflection-identical to v1.0.0.

Validation performed during this refactor:

- Unchanged external tests from v1.0.0 compiled against the facade. 266 test/subtest cases passed when excluding the old test that required all recursive schemas to be rejected; structural recursion was intentionally enabled in an earlier feature commit.
- Comparing v1.0.0's root declarations with the shared engine using apidiff produced no incompatible structural API changes.
- The raw whole-module apidiff reports canonical type relocations through aliases even when the root source signature is unchanged. The release gate canonicalizes only the exact root-to-internal/engine relocation and ignores a report only when the old and new type text then become identical; every other incompatibility still fails the gate.

For v1.1 this canonical reflection-path migration is an accepted compatibility tradeoff. Application-facing root names, imports and call signatures remain available, while code that persists or compares reflect.Type.PkgPath() must account for the new implementation path. Do not describe v1.1 as reflection-identical to v1.0.0.

## Regression gates

Architecture tests enforce the absence of generated implementation exports from root, absence of a transitive root-to-genruntime dependency, no source-time compiler inputs in the public spec package, and no internal import paths in generated client source.

Coverage instruments the root facade, shared engine, codec, generated runtime and schema compiler together across test binaries, then checks each package against the unchanged 80% threshold. Moving statements into internal packages must not make them disappear from the coverage gate.
