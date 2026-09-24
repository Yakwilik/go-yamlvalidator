# Changelog

## Unreleased

- Добавлены first-class functional JSON Schema extensions: custom `format`, custom keywords и named vocabularies без прямой зависимости вызывающего кода от внутреннего JSON Schema engine API.
- Custom keyword diagnostics поддерживают relative YAML paths и `unevaluatedProperties`/`unevaluatedItems` через evaluated-property/item markers.

- Переведён <code>CompileJSONSchema</code>/<code>CompileJSONSchemaWithOptions</code> на полноценный JSON Schema engine вместо частичного преобразования keywords в <code>FieldSchema</code>.
- Поддерживаются draft-04, draft-06, draft-07, 2019-09 и 2020-12, включая references/dynamic references, vocabularies, unevaluated keywords, conditional schemas и остальные constraints движка.
- Добавлены external resources/loaders, управление format/content/vocabulary assertions, <code>ConfigureCompiler</code> и timeout для ECMAScript regexp.
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
- Обновлён EasyP example и покрытие CLI/schema-loader сценариев.

## v0.1.0 — 2025-12-24

- Первый release <code>go-yamlvalidator</code>.
- YAML 1.2 validation core с optional YAML 1.1 boolean compatibility.
- Schema-driven validation: types, required/nullable/default/deprecated, maps/sequences, unknown-key policies и inter-field rules.
- Multi-document YAML, aliases, merge keys и position-aware error formatting.
- Built-in value validators: enum, regex, range, non-empty, length, URL, one-of-type и EasyP-specific validators.
- Built-in key validators: regex, forbidden keys и Unicode-aware length.
- Public pointer helpers, caret renderer и type inference helper.
- Examples для Kubernetes-like manifests и EasyP.
