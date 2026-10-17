# Field Naming Implementation Plan

> **For agentic workers:** Use the supplied field naming specification and test-driven implementation. Workers own separate codec and generator files; the orchestrator owns public API, engine integration, acceptance tests and documentation.

**Goal:** Implement per-call standard YAML or exact Go field names as documented in `docs/high-level-api.md`.

**Architecture:** Store an immutable naming enum in operation contexts and schema cache keys. Extend the existing mixed codec, preserving scalar and custom codec boundaries. Pass naming through typed generated code and emitted schema metadata; use reflection fallback for legacy generated codecs in Go mode.

**Tech Stack:** Go 1.24, yaml.v3, existing schema engine and generator.

- [x] Add failing public behavior tests for naming, tags, recursion, conflicts, diagnostics and cache isolation.
- [x] Add `FieldNaming` public aliases/options, option validation, mode-aware schema inference and cache keys in `api.go` and `internal/engine/highlevel*.go`.
- [x] Extend `internal/yamlcodec` with `FieldNaming` constants, `FieldName(goName, yamlName, naming)`, `DecodeContext.FieldNaming`, `EncodeMixedWithNaming(value, naming)`, generated naming dispatch and legacy fallback. Preflight field metadata conflicts even when validation is skipped. Preserve custom codecs, special values, nulls and input nodes.
- [x] Extend `cmd/yamlvalidator-gen`, `genruntime`, `internal/genspec`, `internal/engine/highlevel_generated_normalized.go` and `internal/engine/schema_source_lower.go` to carry inferred-vs-explicit field names through typed codecs and schema construction. Generated naming encoder signature: `YAMLValidatorEncodeWithNaming(genruntime.FieldNaming)`. Decoders use context naming. Generated schema selection signature to be coordinated without breaking old files.
- [x] Regenerate checked-in fixtures and verify generator freshness and default hooks.
- [x] Document option behavior, examples and legacy codec fallback in README/high-level docs.
- [x] Review against the specification, run `go test ./...`, `go vet ./...`, generator checks, and `go test -race ./...`; fix all relevant failures.

No corporate dependencies or public error signature changes. Preserve user-owned untracked specification and image. Do not commit unless requested.

Verification completed with `GOFLAGS=-mod=readonly` because the existing local vendor directory is incomplete:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `go mod verify`
- Both checked-in generator `-all -check` invocations
- `./scripts/check-api-compat.sh v1.0.0`
- `./scripts/check-library-coverage.sh /tmp/go-yamlvalidator-field-naming.coverage.out` (82.4% total; every gated package above 80%)
- `git diff --check` and formatting checks

Independent codec and generated-path reviews approved their scopes after regression fixes. Legacy generated files retain standard behavior and use reflection fallback in Go mode; regeneration is optional for correctness. Release target: v1.2.0; committing, tagging and pushing were explicitly authorized.
