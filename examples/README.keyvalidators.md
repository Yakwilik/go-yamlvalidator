# Key Validators (пример использования)

KeyValidator применяется к имени каждого ключа в `mapping`.

Подключение:
```go
schema := &FieldSchema{
    Type: TypeMap,
    AdditionalProperties: &FieldSchema{Type: TypeString},
    KeyValidators: []KeyValidator{
        RegexKeyValidator{
            Pattern: regexp.MustCompile(`^[a-z][a-z0-9._-]*$`),
            Message: "недопустимый формат ключа",
        },
    },
}
```

Встроенные валидаторы:
- `RegexKeyValidator{Pattern: re, Message: "..."}`
- `ForbiddenKeyValidator{Forbidden: []string{"password","secret"}}`
- `LengthKeyValidator{Min: PtrInt(1), Max: PtrInt(63)}`

Кастомный:
```go
type MyKeyValidator struct{}
func (MyKeyValidator) ValidateKey(key string, keyNode *yaml.Node, path string, ctx *ValidationContext) {
    if strings.HasPrefix(key, "_") {
        ctx.AddError(ValidationError{
            Level:   LevelWarning,
            Path:    path,
            Line:    keyNode.Line,
            Column:  keyNode.Column,
            Message: "ключи с '_' зарезервированы",
            Got:     key,
        })
    }
}
```
