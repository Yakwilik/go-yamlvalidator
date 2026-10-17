package model

//go:generate go run github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen -all -output=zz_yamlvalidator_generated.go

// Config is a checked-in generated-code example used by the library tests.
type Config struct {
	Name  string         `yaml:"name" yamlvalidate:"required,nonempty"`
	Items []Item         `yaml:"items,omitempty" yamlvalidate:"items={nonempty}"`
	Extra map[string]int `yaml:"extra,omitempty"`
}

type Item string

type Containers struct {
	Items  []string          `yaml:"items"`
	Values map[string]string `yaml:"values"`
}

type Dynamic struct {
	Value any `yaml:"value" yamlvalidate:"type=any"`
}

// Node and Link exercise recursive value and pointer edges in the generated graph.
type Node struct {
	Name     string `yaml:"name" yamlvalidate:"required"`
	Children []Node `yaml:"children,omitempty"`
}

type Link struct {
	Value string `yaml:"value"`
	Next  *Link  `yaml:"next,omitempty"`
}

type Registered struct {
	Value string `yaml:"value" yamlvalidate:"check=registered,ref=registeredSchema"`
}

// FieldNames exercises both operation-scoped naming modes without adding tags
// to fields whose YAML spelling follows the selected mode.
type FieldNames struct {
	ImageName    string `yamlvalidate:"required,nonempty"`
	OptionalName string `yaml:",omitempty"`
	LiteralName  string `yaml:"literal_name"`
	JSONName     string `json:"json_name"`
	Child        *NamingChild
	Children     []NamingChild
	Array        [1]NamingChild
	Workers      map[string]*NamingChild
	NamingInline `yaml:",inline"`
	Extra        map[string]string `yaml:",inline"`
	Hidden       string            `yaml:"-"`
}

type NamingChild struct {
	ImageName string
}

type NamingInline struct {
	InlineName string
}

// GoRuleNames has literal validation references that are valid in Go mode.
type GoRuleNames struct {
	Mode  string `yamlvalidate:"when={field=Mode,eq=prod,require=[Token]}"`
	Token string
}

// GoNamingCollision is valid in YAML mode and conflicts in Go mode.
type GoNamingCollision struct {
	Field    string
	Explicit string `yaml:"Field"`
}

// YAMLCollision is valid in Go mode and conflicts in standard YAML mode.
type YAMLCollision struct {
	ID string
	Id string
}
