package model

//go:generate go run github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen -type=Config,Item,Containers,Dynamic -output=zz_yamlvalidator_generated.go

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
