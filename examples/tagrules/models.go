// Package tagrules demonstrates native cross-field validation declarations.
package tagrules

type Source struct {
	File   string "yaml:\"file,omitempty\" yamlvalidate:\"nonempty\""
	URL    string "yaml:\"url,omitempty\" yamlvalidate:\"nonempty\""
	Inline string "yaml:\"inline,omitempty\" yamlvalidate:\"nonempty\""
}

type IntrinsicSource struct {
	File   string "yaml:\"file,omitempty\" yamlvalidate:\"nonempty,exactlyOneOf=[file,url,inline]\""
	URL    string "yaml:\"url,omitempty\" yamlvalidate:\"nonempty\""
	Inline string "yaml:\"inline,omitempty\" yamlvalidate:\"nonempty\""
}

type Config struct {
	Source Source "yaml:\"source\" yamlvalidate:\"required,exactlyOneOfKeys=[file,url,inline]\""
}

type SourceList struct {
	Sources []Source "yaml:\"sources\" yamlvalidate:\"required,nonempty,items={exactlyOneOfKeys=[file,url,inline]}\""
}

type SourceMap struct {
	Sources map[string]Source "yaml:\"sources\" yamlvalidate:\"required,nonempty,keys={pattern='^[a-z][a-z0-9_-]*$'},values={exactlyOneOfKeys=[file,url,inline]}\""
}

type Logging struct {
	Debug bool "yaml:\"debug,omitempty\" yamlvalidate:\"mutuallyExclusive=[debug,quiet]\""
	Quiet bool "yaml:\"quiet,omitempty\""
}

type TLS struct {
	Cert string "yaml:\"cert,omitempty\" yamlvalidate:\"nonempty,dependentRequired={cert=[key],key=[cert]}\""
	Key  string "yaml:\"key,omitempty\" yamlvalidate:\"nonempty\""
}

type Connection struct {
	DSN  string "yaml:\"dsn,omitempty\" yamlvalidate:\"nonempty,anyOfRequired=[[dsn],[host,port]]\""
	Host string "yaml:\"host,omitempty\" yamlvalidate:\"nonempty\""
	Port uint16 "yaml:\"port,omitempty\" yamlvalidate:\"min=1\""
}

type ExclusiveConnection struct {
	DSN  string "yaml:\"dsn,omitempty\" yamlvalidate:\"nonempty,oneOfRequired=[[dsn],[host,port]]\""
	Host string "yaml:\"host,omitempty\" yamlvalidate:\"nonempty\""
	Port uint16 "yaml:\"port,omitempty\" yamlvalidate:\"min=1\""
}

type Production struct {
	Mode  string "yaml:\"mode\" yamlvalidate:\"required,enum=[dev,prod],when={field=mode,eq=prod,require=[tls],forbid=[debug]}\""
	TLS   *TLS   "yaml:\"tls,omitempty\" yamlvalidate:\"notnull\""
	Debug bool   "yaml:\"debug,omitempty\""
}

type Diagnostics struct {
	Debug bool "yaml:\"debug,omitempty\" yamlvalidate:\"forbiddenTogether=[[debug,trace,dump]]\""
	Trace bool "yaml:\"trace,omitempty\""
	Dump  bool "yaml:\"dump,omitempty\""
}

type Credentials struct {
	Token    string "yaml:\"token,omitempty\" yamlvalidate:\"nonempty,exactlyOneOf=[token,user]\""
	User     string "yaml:\"user,omitempty\" yamlvalidate:\"nonempty,dependentRequired={user=[password],password=[user]}\""
	Password string "yaml:\"password,omitempty\" yamlvalidate:\"nonempty\""
}

type Job struct {
	Credentials "yaml:\",inline\""
	Name        string "yaml:\"name\" yamlvalidate:\"required,nonempty\""
}

type TwoGroups struct {
	File  string "yaml:\"file,omitempty\" yamlvalidate:\"exactlyOneOf=[file,url]\""
	URL   string "yaml:\"url,omitempty\""
	Token string "yaml:\"token,omitempty\" yamlvalidate:\"exactlyOneOf=[token,user]\""
	User  string "yaml:\"user,omitempty\""
}

type StrictConnection struct {
	DSN  string "yaml:\"dsn,omitempty\" yamlvalidate:\"nonempty,oneOfRequired=[[dsn],[host,port]],forbiddenTogether=[[dsn,host],[dsn,port]]\""
	Host string "yaml:\"host,omitempty\" yamlvalidate:\"nonempty\""
	Port uint16 "yaml:\"port,omitempty\" yamlvalidate:\"min=1\""
}

type UseSiteTLS struct {
	TLS TLS "yaml:\"tls\" yamlvalidate:\"dependentRequiredKeys={cert=[key],key=[cert]}\""
}

type StrictProduction struct {
	Mode  string "yaml:\"mode\" yamlvalidate:\"required,enum=[dev,prod],when={field=mode,eq=prod,require=[tls],forbid=[debug]}\""
	TLS   *TLS   "yaml:\"tls,omitempty\" yamlvalidate:\"notnull,requireKeys=[cert,key]\""
	Debug bool   "yaml:\"debug,omitempty\""
}
