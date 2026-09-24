# Changelog

## v0.6.0 — 2026-09-24

- Проведён pre-v1 API cleanup: удалены временные `LoadURL`/`JSONSchemaLoadFunc` и engine-specific `ConfigureCompiler`, поэтому public API больше не зависит от конкретной реализации JSON Schema engine.
- Добавлены first-class `JSONSchemaContentEncoding` и `JSONSchemaContentMediaType`; `JSONSchemaVocabulary.Schema` позволяет валидировать определения custom keywords без low-level compiler hook.
- JSON Schema compilation теперь closed-world по умолчанию: external `$ref` не читает filesystem/network без явно переданного `Resolver`; CLI автоматически использует root-constrained file resolver для директории schema-файла.
- Удалены прикладные `DirectoryValidator`, `PluginSourceValidator`, `ManagedDisableValidator` и `ManagedOverrideValidator`; `examples/advanced` переписан как generic configuration example.
- `InferTypeForPublic` заменён на package-level `InferNodeType` с нейтральным публичным контрактом.
- Случайно экспортированный `JSONSchemaKeywordIssue` и runtime-only `ValidationContext.SourceLines` убраны из public surface; underlying resolver у `JSONSchemaCachingResolver` теперь immutable после constructor.
- Добавлены `AppendPropertyPath` и `AppendIndexPath` для корректных child paths в custom native validators.
- Built-in validator definitions усилены: пустые/duplicate enum и forbidden-key конфигурации отклоняются при compile; `OneOfTypeValidator` унифицирован с native `type_mismatch` diagnostics и `TypeMismatchDetails`.

## v0.5.0 — 2026-09-24

- Добавлен composable JSON Schema resolver API: context-aware `JSONSchemaResolver`, in-memory/chain/cache/rooted-file implementations и `CompileJSONSchemaContext*`; legacy `LoadURL` сохранён как deprecated compatibility path.
- Functional custom JSON Schema keywords теперь могут объявлять и компилировать nested subschemas через `CompileWithContext`, `JSONSchemaSubschemaPath`, `Subschema`/`Reference` и `ValidateSubschema` без прямой зависимости от engine API.
- Добавлены `ValidateReader*` API с I/O errors отдельно от validation diagnostics, context cancellation и bounded reading до `MaxBytes+1`.
- Добавлен benchmark suite для native/JSON Schema compile и small/large validation; source-line indexing оптимизирован до общего backing string, что устраняет по одной string allocation на строку YAML.

## v0.4.0 — 2026-09-24

- Добавлена cooperative cancellation/deadline support через `ValidateContext` и `ValidateContextWithOptions`; custom validators получают `context.Context` через `ValidationContext.Context()`.
- `CompileFieldSchema` теперь создаёт snapshot native schema graph и конфигурации built-in validators; для configurable custom validators добавлены `ValueValidatorCloner` / `KeyValidatorCloner`. Compiled validator безопасен для concurrent validation при concurrency-safe custom validator state.
- `RangeValidator` сравнивает YAML numbers без `float64` round-trip; добавлены `ExactNumber`, `ParseExactNumber`, `MustExactNumber`, `MinExact` и `MaxExact` для точных границ произвольной величины.
- Native type inference сохраняет integer semantics для целых YAML literals за пределами machine-sized integer range.
- `ValidationError` получил `PathTokens` и `Details`; основные native/JSON Schema diagnostics возвращают typed details для required/type/range/item-count/dependency/unknown-key ошибок.
- `ErrorCollector.Errors`, `Warnings` и `All` возвращают defensive copies.
- Добавлены fuzz targets и adversarial tests для native YAML, JSON Schema bridge/compilation, path parsing, exact numbers, depth/diagnostic limits, aliases и больших integers.

## v0.3.1 — 2026-09-24

- Исправлена формулировка minimum Go version: v0.3.0 требует Go 1.24+, без утверждения о её «понижении» относительно предыдущего release.
- Публичная документация, tests и examples очищены от product-specific references; advanced example сделан generic.

## v0.3.0 — 2026-09-24

- Минимальная поддерживаемая версия Go для v0.3.0 — 1.24; она проверяется отдельным CI job наряду с текущим stable Go.
- Добавлены first-class functional JSON Schema extensions: custom `format`, custom keywords и named vocabularies без прямой зависимости вызывающего кода от внутреннего JSON Schema engine API.
- Custom keyword diagnostics поддерживают relative YAML paths и `unevaluatedProperties`/`unevaluatedItems` через evaluated-property/item markers.

- Переведён <code>CompileJSONSchema</code>/<code>CompileJSONSchemaWithOptions</code> на полноценный JSON Schema engine вместо частичного преобразования keywords в <code>FieldSchema</code>.
- Поддерживаются draft-04, draft-06, draft-07, 2019-09 и 2020-12, включая references/dynamic references, vocabularies, unevaluated keywords, conditional schemas и остальные constraints движка.
- Добавлены external resources/loaders, управление format/content/vocabulary assertions, <code>ConfigureCompiler</code> и timeout для ECMAScript regexp.
- Patched IDNA/Unicode normalization code из upstream `x/net v0.55.0` и `x/text v0.39.0` встроен во внутренний namespace библиотеки: security fixes для IDN formats сохраняются без повышения minimum Go выше 1.24.
- Добавлен ECMAScript-совместимый regexp engine и усиленные стандартные format validators.
- Добавлен точный YAML → JSON data-model bridge без <code>float64</code>-round-trip для чисел, с сохранением YAML source positions в JSON Schema diagnostics.
- Официальный JSON-Schema-Test-Suite теперь запускается как conformance-test в CI: 4950/4950 обязательных core cases и все применимые optional cases, кроме host-language-specific draft-04 <code>zeroTerminatedFloats</code>.
- Добавлены <code>CompileFieldSchema</code> и <code>ValidateFieldSchema</code> для проверки native schema invariants до валидации YAML.
- Built-in validators проверяют собственные definition errors: nil regexp, некорректные min/max, invalid URL schemes, пустые/дублированные type definitions.
- Добавлены resource limits: <code>MaxBytes</code>, <code>MaxDocuments</code>, <code>MaxDepth</code>, <code>MaxDiagnostics</code> и признак <code>ValidationResult.Truncated</code>.
- CLI теперь напрямую поддерживает стандартную JSON Schema через <code>-schema-format jsonschema</code>, relative file refs, draft/format/content/vocabulary options, regexp timeout и resource limits.
- Добавлены стабильные machine-readable diagnostic codes для native и JSON Schema validation.
- Добавлены подсказки для опечаток unknown keys и однозначное форматирование paths для ключей вроде "a.b" и "0".
- Исправлены YAML merge precedence и duplicate-key detection.
- Добавлен общий AST preflight для recursive/unresolved aliases, invalid merge values и hard MaxDepth enforcement до схемной валидации.
- Исправлен bare <code>TypeAny</code>: произвольные map/sequence больше не превращаются в unknown fields.
- YAML 1.1 compatibility booleans применяются только к plain scalars; quoted values остаются strings.
- Исправлен порядок diagnostics в <code>ErrorCollector.All</code>/<code>SortByPosition</code>.
- Исправлены legacy integer forms и bounded NaN в <code>RangeValidator</code>.
- <code>URLValidator</code> теперь использует стандартный URL parser вместо ручной проверки <code>scheme://</code>.
- CLI loader native schema использует strict known-fields decoding и сохраняет descriptions.
- Минимальная версия Go поднята до 1.25, чтобы использовать security-fixed IDNA dependencies; CI проверяет Go 1.25 + stable.
- Добавлен GitHub Actions CI: tests, vet, race detector, staticcheck, govulncheck и официальный JSON Schema conformance suite.
- README синхронизирован с реальным API и лицензией Apache-2.0.

## v0.2.1 — 2026-03-17

- Исправлено имя Go module/import path.

## v0.2.0 — 2026-02-17

- Добавлен standalone CLI <code>cmd/yamlvalidator</code> для YAML validation через сериализованный <code>FieldSchema</code>.
- Добавлены flags для strict keys, YAML 1.1 booleans, type strictness, stop-on-first и сортировки diagnostics.
- Добавлен schema loader для YAML/JSON native schema и тесты validator names.
- Обновлён расширенный configuration example и покрытие CLI/schema-loader сценариев.

## v0.1.0 — 2025-12-24

- Первый release <code>go-yamlvalidator</code>.
- YAML 1.2 validation core с optional YAML 1.1 boolean compatibility.
- Schema-driven validation: types, required/nullable/default/deprecated, maps/sequences, unknown-key policies и inter-field rules.
- Multi-document YAML, aliases, merge keys и position-aware error formatting.
- Built-in value validators: enum, regex, range, non-empty, length, URL, one-of-type.
- Built-in key validators: regex, forbidden keys и Unicode-aware length.
- Public pointer helpers, caret renderer и type inference helper.
- Examples для Kubernetes-like manifests и расширенной configuration validation.
