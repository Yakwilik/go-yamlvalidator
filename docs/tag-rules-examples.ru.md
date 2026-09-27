# Межполевые правила: проверенные примеры

Примеры соответствуют examples/tagrules/models.go и проверяются models_test.go.

Правило без Keys на Go-поле проверяет содержащий mapping; суффикс Keys проверяет mapping-значение поля. Ссылки используют YAML-имена. Проверяется наличие ключей, а не ненулевые Go-значения.

## Общий тип без ограничения выбора

~~~go
type Source struct {
	File   string "yaml:\"file,omitempty\" yamlvalidate:\"nonempty\""
	URL    string "yaml:\"url,omitempty\" yamlvalidate:\"nonempty\""
	Inline string "yaml:\"inline,omitempty\" yamlvalidate:\"nonempty\""
}
~~~

## 1. Один источник: инвариант типа

Ровно один из file, url, inline. Проверка работает и при отсутствии поля File, на котором объявлен тег.

~~~go
type IntrinsicSource struct {
	File   string "yaml:\"file,omitempty\" yamlvalidate:\"nonempty,exactlyOneOf=[file,url,inline]\""
	URL    string "yaml:\"url,omitempty\" yamlvalidate:\"nonempty\""
	Inline string "yaml:\"inline,omitempty\" yamlvalidate:\"nonempty\""
}
~~~

Допустимый YAML:

~~~yaml
url: https://example.org/config.yaml
~~~

## 2. Ограничение в месте использования

Здесь используется Source без внутреннего exactlyOneOf. required относится к наличию source, exactlyOneOfKeys — к его содержимому.

~~~go
type Config struct {
	Source Source "yaml:\"source\" yamlvalidate:\"required,exactlyOneOfKeys=[file,url,inline]\""
}
~~~

Допустимый YAML:

~~~yaml
source:
  file: config.yaml
~~~

## 3. Правило для каждого элемента

Каждый элемент независимо выбирает один способ. Пустой массив запрещён nonempty.

~~~go
type SourceList struct {
	Sources []Source "yaml:\"sources\" yamlvalidate:\"required,nonempty,items={exactlyOneOfKeys=[file,url,inline]}\""
}
~~~

Допустимый YAML:

~~~yaml
sources:
  - file: local.yaml
  - inline: "name: demo"
~~~

## 4. Ключи и значения map

keys проверяет имена источников, values — каждый объект-источник.

~~~go
type SourceMap struct {
	Sources map[string]Source "yaml:\"sources\" yamlvalidate:\"required,nonempty,keys={pattern='^[a-z][a-z0-9_-]*$'},values={exactlyOneOfKeys=[file,url,inline]}\""
}
~~~

Допустимый YAML:

~~~yaml
sources:
  api: {file: api.yaml}
  worker: {inline: "workers: 4"}
~~~

## 5. Не более одного ключа

Оба ключа могут отсутствовать. debug: false вместе с quiet: false всё равно нарушает правило присутствия.

~~~go
type Logging struct {
	Debug bool "yaml:\"debug,omitempty\" yamlvalidate:\"mutuallyExclusive=[debug,quiet]\""
	Quiet bool "yaml:\"quiet,omitempty\""
}
~~~

Допустимый YAML:

~~~yaml
debug: true
~~~

## 6. Взаимная зависимость

Оба поля могут отсутствовать либо присутствовать вместе. Значения должны быть непустыми.

~~~go
type TLS struct {
	Cert string "yaml:\"cert,omitempty\" yamlvalidate:\"nonempty,dependentRequired={cert=[key],key=[cert]}\""
	Key  string "yaml:\"key,omitempty\" yamlvalidate:\"nonempty\""
}
~~~

Допустимый YAML:

~~~yaml
cert: server.crt
key: server.key
~~~

## 7. Хотя бы одна полная группа

Либо dsn, либо host и port. Допускаются одновременно обе полные группы. Один host недостаточен.

~~~go
type Connection struct {
	DSN  string "yaml:\"dsn,omitempty\" yamlvalidate:\"nonempty,anyOfRequired=[[dsn],[host,port]]\""
	Host string "yaml:\"host,omitempty\" yamlvalidate:\"nonempty\""
	Port uint16 "yaml:\"port,omitempty\" yamlvalidate:\"min=1\""
}
~~~

Допустимый YAML:

~~~yaml
host: db
port: 5432
~~~

## 8. Ровно один способ без смешивания

oneOfRequired требует ровно одну полную группу. Запреты forbiddenTogether дополнительно исключают даже частичное смешивание dsn с host или port.

~~~go
type StrictConnection struct {
	DSN  string "yaml:\"dsn,omitempty\" yamlvalidate:\"nonempty,oneOfRequired=[[dsn],[host,port]],forbiddenTogether=[[dsn,host],[dsn,port]]\""
	Host string "yaml:\"host,omitempty\" yamlvalidate:\"nonempty\""
	Port uint16 "yaml:\"port,omitempty\" yamlvalidate:\"min=1\""
}
~~~

Допустимый YAML:

~~~yaml
dsn: postgres://db/app
~~~

## 9. Условные требования и запреты

При mode=prod ключ tls обязателен, debug запрещён даже со значением false. Присутствующий tls не может быть null и должен содержать cert и key.

~~~go
type StrictProduction struct {
	Mode  string "yaml:\"mode\" yamlvalidate:\"required,enum=[dev,prod],when={field=mode,eq=prod,require=[tls],forbid=[debug]}\""
	TLS   *TLS   "yaml:\"tls,omitempty\" yamlvalidate:\"notnull,requireKeys=[cert,key]\""
	Debug bool   "yaml:\"debug,omitempty\""
}
~~~

Допустимый YAML:

~~~yaml
mode: prod
tls: {cert: server.crt, key: server.key}
~~~

## 10. Запретить комбинацию целиком

Любая пара разрешена; вся тройка debug, trace, dump одновременно запрещена.

~~~go
type Diagnostics struct {
	Debug bool "yaml:\"debug,omitempty\" yamlvalidate:\"forbiddenTogether=[[debug,trace,dump]]\""
	Trace bool "yaml:\"trace,omitempty\""
	Dump  bool "yaml:\"dump,omitempty\""
}
~~~

Допустимый YAML:

~~~yaml
debug: true
trace: true
~~~

## 11. Две независимые группы

Нужно выбрать один источник и одну авторизацию. Группы объединяются через AND, а не склеиваются в один список.

~~~go
type TwoGroups struct {
	File  string "yaml:\"file,omitempty\" yamlvalidate:\"exactlyOneOf=[file,url]\""
	URL   string "yaml:\"url,omitempty\""
	Token string "yaml:\"token,omitempty\" yamlvalidate:\"exactlyOneOf=[token,user]\""
	User  string "yaml:\"user,omitempty\""
}
~~~

Допустимый YAML:

~~~yaml
file: config.yaml
token: example-token
~~~

## 12. Правила inline-структуры

Credentials встраивается без вложенного YAML-ключа. Правила авторизации проверяют тот же mapping, где находится name.

~~~go
type Credentials struct {
	Token    string "yaml:\"token,omitempty\" yamlvalidate:\"nonempty,exactlyOneOf=[token,user]\""
	User     string "yaml:\"user,omitempty\" yamlvalidate:\"nonempty,dependentRequired={user=[password],password=[user]}\""
	Password string "yaml:\"password,omitempty\" yamlvalidate:\"nonempty\""
}

type Job struct {
	Credentials "yaml:\",inline\""
	Name        string "yaml:\"name\" yamlvalidate:\"required,nonempty\""
}
~~~

Допустимый YAML:

~~~yaml
name: deploy
user: agent
password: example-password
~~~

## Запуск проверок

~~~sh
go test -count=1 ./examples/tagrules
~~~

Для загрузки без генерации используется yamlvalidator.Unmarshal. Стандартный yaml.Unmarshal сам по себе не интерпретирует yamlvalidate.
