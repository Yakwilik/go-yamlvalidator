# Настройки ValidationContext и FieldSchema

<code>ValidationContext</code>, передаваемый в <code>ValidateWithOptions</code>:

- <code>StrictKeys</code> — unknown keys как ошибка или предупреждение для native <code>FieldSchema</code> с <code>UnknownKeyInherit</code>.
- <code>StopOnFirst</code> — остановка после первой ошибки.
- <code>StrictTypes</code> — type inference только по YAML-тегам для native schema.
- <code>YAML11Booleans</code> — plain scalars <code>y/n/yes/no/on/off</code> трактуются как YAML 1.1 booleans; quoted scalars остаются strings.
- <code>MaxBytes</code> — максимальный размер YAML в bytes; <code>0</code> означает без лимита.
- <code>MaxDocuments</code> — максимальное число YAML documents; <code>0</code> означает без лимита.
- <code>MaxDepth</code> — максимальная глубина вложенности; root имеет depth 1; <code>0</code> означает без лимита.
- <code>MaxDiagnostics</code> — максимальное число errors + warnings; <code>0</code> означает без лимита. При достижении лимита <code>ValidationResult.Truncated == true</code>.

Основные поля native <code>FieldSchema</code>:

- <code>Type</code>, <code>AllowedTypes</code>, <code>Required</code>, <code>Nullable</code>, <code>Deprecated</code>, <code>Description</code>, <code>Default</code>.
- <code>AllowedKeys</code>, <code>AdditionalProperties</code>, <code>UnknownKeyPolicy</code>, <code>KeyValidators</code>.
- <code>ItemSchema</code>, <code>MinItems</code>, <code>MaxItems</code>.
- <code>Validators</code>.
- Межполевые правила: <code>AnyOf</code>, <code>ExactlyOneOf</code>, <code>MutuallyExclusive</code>, <code>Conditions</code>, <code>OneOfRequired</code>, <code>ForbiddenTogether</code>, <code>DependentRequired</code>.
- Композиция schemas: <code>OneOfSchemas</code>, <code>AnyOfSchemas</code>.

Перед использованием dynamically assembled native schema необходимо выполнять compilation:

~~~go
validator, err := yamlvalidator.CompileFieldSchema(schema)
if err != nil {
    return err
}

result := validator.ValidateWithOptions(data, yamlvalidator.ValidationContext{
    StrictKeys:     true,
    YAML11Booleans: true,
    MaxDepth:       100,
    MaxDiagnostics: 100,
})
~~~

<code>Default</code> не изменяет YAML и не подставляет значение автоматически. Если optional field отсутствует, validator выдаёт warning с предлагаемым default.
