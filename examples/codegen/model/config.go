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
