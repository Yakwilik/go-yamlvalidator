# Key Validators

<code>KeyValidator</code> применяется к имени каждого ключа YAML mapping.

Пример:

~~~go
schema := &yamlvalidator.FieldSchema{
    Type:                 yamlvalidator.TypeMap,
    AdditionalProperties: &yamlvalidator.FieldSchema{Type: yamlvalidator.TypeString},
    KeyValidators: []yamlvalidator.KeyValidator{
        keyvalidator.RegexKeyValidator{
            Pattern: regexp.MustCompile("^[a-z][a-z0-9._-]*$"),
            Message: "недопустимый формат ключа",
        },
    },
}

validator, err := yamlvalidator.CompileFieldSchema(schema)
~~~

Встроенные validators:

- <code>RegexKeyValidator{Pattern: re, Message: "..."}</code>
- <code>ForbiddenKeyValidator{Forbidden: []string{"password", "secret"}}</code>
- <code>LengthKeyValidator{Min: yamlvalidator.Ptr(1), Max: yamlvalidator.Ptr(63)}</code>

Regex и length definition errors проверяются во время <code>CompileFieldSchema</code>.

Кастомный validator:

~~~go
type MyKeyValidator struct{}

func (MyKeyValidator) ValidateKey(
    key string,
    keyNode *yaml.Node,
    path string,
    ctx *yamlvalidator.ValidationContext,
) {
    if strings.HasPrefix(key, "_") {
        ctx.AddError(yamlvalidator.ValidationError{
            Level:   yamlvalidator.LevelWarning,
            Code:    "reserved_key",
            Path:    path,
            Line:    keyNode.Line,
            Column:  keyNode.Column,
            Message: "ключи с префиксом _ зарезервированы",
            Got:     key,
        })
    }
}
~~~
