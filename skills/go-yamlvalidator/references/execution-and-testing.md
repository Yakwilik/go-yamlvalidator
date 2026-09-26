# Execution, contexts, snapshots and tests

Executable recipes: [execution_test.go](../examples/recipes/execution_test.go).
These are additional integration options; do not force them into a simple starter.

## Input methods

| Method | Return |
| --- | --- |
| ValidateBytes(data) | *ValidationResult |
| ValidateWithOptions(data, ValidationContext) | *ValidationResult |
| ValidateContext(ctx, data) | *ValidationResult, error |
| ValidateContextWithOptions(ctx, data, options) | *ValidationResult, error |
| ValidateReader(reader) | *ValidationResult, error |
| ValidateReaderWithOptions(reader, options) | *ValidationResult, error |
| ValidateReaderContext(ctx, reader) | *ValidationResult, error |
| ValidateReaderContextWithOptions(ctx, reader, options) | *ValidationResult, error |

Returned error is for context/I/O, not schema mismatch. Handle it before looking
at the result; reader I/O failure may return nil result. After that inspect
Canceled, Truncated, HasErrors and warnings according to the application policy.
Do not treat a truncated warning-only result as proof the entire input is valid.

~~~go
result, err := validator.ValidateReaderContextWithOptions(ctx, reader, v.ValidationContext{
    MaxBytes: 1 << 20,
    MaxDocuments: 1,
    MaxDepth: 100,
    MaxDiagnostics: 100,
})
if err != nil { return err }
if result.Canceled || result.Truncated { return fmt.Errorf("validation incomplete") }
if result.HasErrors() { return fmt.Errorf("invalid configuration") }
~~~

The example uses application-owned ctx and reader in an error-returning function.
Numbers are example budgets, not universal safe limits. MaxDocuments is an upper
bound, not a nonempty requirement.

## ValidationContext fields

StrictKeys controls inherited native unknown-key policy. StopOnFirst stops after
an error, not a warning. StrictTypes uses tag-based inference rather than fallback
parsing for unknown tags; ordinary quoted strings still remain strings either
way. YAML11Booleans affects plain scalar compatibility.

MaxBytes, MaxDocuments, MaxDepth and MaxDiagnostics use zero for unlimited. Set
nonnegative budgets deliberately. Reader methods buffer the source to preserve
context; with MaxBytes=N they read at most N+1 bytes to detect overflow. They are
not streaming result iterators. Unlike these APIs, the current CLI reads the
input fully before validating; do not advertise its max-bytes flag as a bounded
input-reader guarantee.

## Cancellation is cooperative

Native callbacks can inspect ctx.Context(), ctx.ContextErr() and ctx.IsStopped().
The library checks cancellation around its work and resolver calls. A blocking
io.Reader.Read or a synchronous engine/callback operation is not forcibly
interrupted in the middle. RegexpTimeout bounds individual configured matches,
not total validation time; MaxDiagnostics bounds reported diagnostics, not the
entire engine's error-tree allocation. Do not promise a hard execution budget.

## Sharing a validator

NewValidator keeps the source FieldSchema pointer. CompileFieldSchema first
checks definitions and snapshots the graph and built-in configurations. It is
optional for a trusted literal, useful for reusable dynamic schemas. Copies of
opaque custom state still need synchronization. Custom validators can implement
ValueValidatorCloner and KeyValidatorCloner; their clone must isolate mutable
configuration. A callback closure copied by value can still share state.

Compiled JSON Schema instances can be reused. Treat custom callback captures,
resolver maps, chains and configuration as read-only or provide synchronization.
Use local validation contexts per call, not a shared mutable context object.

## Regression checklist

Use table-driven tests with exact source bytes. Cover missing, null, empty and
valid values separately, unknown-key warning/error policy, nested values and
sequences, mutually-exclusive combinations, syntax errors and duplicate keys.
For JSON Schema add a draft case, a relative ref, a failing custom format, and
both valid/invalid keyword configuration. Assert Code, Path and relevant Details
rather than exact engine prose or error ordering. Assert Line/Column only for
known stable source cases; missing nodes and parser failures are different.

~~~bash
go test ./...
go test -race ./...
go vet ./...
~~~

Run only within the task's repository. Do not claim fuzz, performance, official
conformance or live-agent evaluations were run unless they were. The skill's
own recipes are covered by normal go test and by the isolated example checker.
