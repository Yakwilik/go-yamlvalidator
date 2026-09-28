# Phase 3 report

Base: `1384251` on `feat/generated-schema`. Verified on 2026-09-28. No push.

## Result

Selected generated roots now emit typed codecs for their reachable static type graph and a semantically lowered Go-native schema blueprint. Generated YAML hooks and yamlvalidator APIs build that schema directly. Runtime recursive type inference uses memoized schema placeholders. Native schema validation permits structural recursion and rejects same-node composition cycles. Generated cycle preflight rejects cyclic Go values.

## Acceptance commands

All commands below exited 0 against the final implementation before the commit.

| Gate | Exact command | Result |
| --- | --- | --- |
| Independent QA | `cd .work-generated-schema/qa && zsh ./run.sh` | PASS; `ok phase3qa 0.007s`; generated-source guards for raw rule metadata and JSON reconstruction passed. |
| Full tests | `go test -mod=readonly -count=1 ./...` | PASS; all packages passed. |
| Race | `go test -mod=readonly -race -count=1 ./...` | PASS; all packages passed. |
| Vet | `go vet -mod=readonly ./...` | PASS; no diagnostics. |
| Go 1.24.0 | `GOTOOLCHAIN=go1.24.0 go test -mod=readonly -count=1 ./...` | PASS; all packages passed. |
| Go 1.24.11 | `GOTOOLCHAIN=go1.24.11 go test -mod=readonly -count=1 ./...` | PASS; all packages passed. |
| Coverage floor | `GOFLAGS=-mod=readonly ./scripts/check-library-coverage.sh .work-generated-schema/coverage.out` | PASS; root 80.2%, valuevalidator 82.2%, keyvalidator 97.9%; aggregate 80.4%. |
| Official JSON Schema conformance | `JSON_SCHEMA_TEST_SUITE=/Users/khasbulatabdullin/Tech/JSON-Schema-Test-Suite go test -mod=readonly -tags=conformance -run '^TestOfficialJSONSchemaTestSuite$' -count=1 -v .` | PASS; core 4,950, optional 3,884 across five drafts. |
| Checked-in example freshness | From `examples/codegen/model`: `go run github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen -type=Config,Item,Containers,Dynamic,Node,Link,Registered -output=zz_yamlvalidator_generated.go -check` | PASS; no stale output. |
| Public API compatibility | `GOFLAGS=-mod=readonly ./scripts/check-api-compat.sh v0.6.0` | PASS; no incompatible API changes. |
| Whitespace | `git diff --check` | PASS; no errors. |

## Remaining limits

Standard yaml.v3 hooks use default validation options; callers needing registry bindings, callbacks, or custom limits use yamlvalidator options. Dynamic interface values and opaque custom codecs can still use reflection fallback. Non-string map keys and streaming remain unsupported as documented. The public library coverage margin is 0.2 percentage points above the required floor.
