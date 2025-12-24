# Value Validators (пример использования)

ValueValidator применяется к значению узла после базовой проверки типа.

Базовый паттерн:
```go
schema := &FieldSchema{
    Type: TypeString,
    Validators: []ValueValidator{
        RegexValidator{
            Pattern: regexp.MustCompile(`^[a-z0-9-]+$`),
            Message: "допустимы только строчные буквы/цифры/дефисы",
        },
    },
}
```

Встроенные валидаторы:
- `EnumValidator{Allowed: []string{"v1","v2"}}`
- `RegexValidator{Pattern: re, Message: "..."}`
- `RangeValidator{Min: PtrFloat(1), Max: PtrFloat(10)}` — для чисел.
- `NonEmptyValidator{}` — строка/массив/карта не пусты.
- `LengthValidator{Min: PtrInt(1), Max: PtrInt(63)}`
- `URLValidator{RequireScheme: true, AllowedSchemes: []string{"http","https"}}`
- `OneOfTypeValidator{Types: []NodeType{TypeString, TypeInt}}`

Кастомный:
```go
type MyValidator struct{}
func (MyValidator) Validate(node *yaml.Node, path string, ctx *ValidationContext) {
    if node.Value != "ok" {
        ctx.AddError(ValidationError{
            Level:   LevelError,
            Path:    path,
            Line:    node.Line,
            Column:  node.Column,
            Message: "должно быть \"ok\"",
            Got:     node.Value,
        })
    }
}
```
