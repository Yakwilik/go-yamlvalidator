# Value Validators

<code>ValueValidator</code> применяется к YAML node после базовой type validation.

Пример:

~~~go
schema := &yamlvalidator.FieldSchema{
    Type: yamlvalidator.TypeString,
    Validators: []yamlvalidator.ValueValidator{
        valuevalidator.RegexValidator{
            Pattern: regexp.MustCompile("^[a-z0-9-]+$"),
            Message: "допустимы только строчные буквы, цифры и дефисы",
        },
    },
}

validator, err := yamlvalidator.CompileFieldSchema(schema)
~~~

Встроенные validators:

- <code>EnumValidator{Allowed: []string{"v1", "v2"}}</code>
- <code>RegexValidator{Pattern: re, Message: "..."}</code>
- <code>RangeValidator{Min: yamlvalidator.Ptr(1.0), Max: yamlvalidator.Ptr(10.0)}</code> — YAML value сравнивается без `float64` round-trip; для exact bounds произвольной величины доступны <code>MinExact</code>/<code>MaxExact</code> и <code>MustExactNumber("...")</code>
- <code>NonEmptyValidator{}</code>
- <code>LengthValidator{Min: yamlvalidator.Ptr(1), Max: yamlvalidator.Ptr(63)}</code>
- <code>URLValidator{RequireScheme: true, AllowedSchemes: []string{"http", "https"}}</code>
- <code>OneOfTypeValidator{Types: []yamlvalidator.NodeType{yamlvalidator.TypeString, yamlvalidator.TypeInt}}</code>

Built-in validators, которые имеют собственные definition invariants, проверяются на этапе <code>CompileFieldSchema</code>. Например, nil regexp, <code>Min > Max</code> и некорректные URL schemes приводят к compile error, а не к panic во время YAML validation.

Кастомный validator:

~~~go
type MyValidator struct{}

func (MyValidator) Validate(node *yaml.Node, path string, ctx *yamlvalidator.ValidationContext) {
    if node.Value != "ok" {
        ctx.AddError(yamlvalidator.ValidationError{
            Level:   yamlvalidator.LevelError,
            Code:    "my_rule",
            Path:    path,
            Line:    node.Line,
            Column:  node.Column,
            Message: "значение должно быть ok",
            Got:     node.Value,
        })
    }
}
~~~

Если кастомный validator имеет собственную configuration validation, он может реализовать <code>DefinitionValidator</code> через метод <code>ValidateDefinition() error</code>.

Compiled validator можно использовать конкурентно, поэтому mutable state внутри custom validator должен быть синхронизирован самим validator. Для cancellable/blocking work используйте <code>ctx.Context()</code>.
