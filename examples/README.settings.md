# Настройки в ValidationContext и схеме

`ValidationContext` (передается в `ValidateWithOptions`):
- `StrictKeys` — unknown keys как ошибка (true) или предупреждение (false, по умолчанию). Работает, когда `UnknownKeyPolicy == UnknownKeyInherit`.
- `StopOnFirst` — останавливать после первой ошибки.
- `StrictTypes` — не пытаться парсить скаляры, брать тип только из YAML‑тега.
- `YAML11Booleans` — трактовать `y/n/yes/no/on/off/true/false` (в т.ч. в кавычках) как bool.

Полезные поля схемы (`FieldSchema`):
- `Type` — ожидаемый тип (`TypeString`, `TypeMap`, и т.д.).
- `Required`, `Nullable`, `Deprecated`, `Default`.
- `AllowedKeys` — известные ключи с под‑схемами.
- `AdditionalProperties` — схема для любых других ключей (включает их в валидацию).
- `UnknownKeyPolicy` — как реагировать на неизвестные ключи (Error/Warn/Ignore/Inherit).
- `KeyValidators` — валидаторы имени ключа.
- `ItemSchema`, `MinItems`, `MaxItems` — для последовательностей.
- `Validators` — value‑валидаторы.
- Межполевые правила: `AnyOf`, `ExactlyOneOf`, `MutuallyExclusive`, `Conditions` (если нужно сложнее — кастомный валидатор).

Пример вызова:
```go
ctx := ValidationContext{
    StrictKeys:     true,
    YAML11Booleans: true,
}
result := validator.ValidateWithOptions(data, ctx)
fmt.Println(result.FormatAll(true))
```
