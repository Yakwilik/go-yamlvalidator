package model

//go:generate go run github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen -all -output=zz_yamlvalidator_generated.go

type Config struct {
	Name        string            `yaml:"name" yamlvalidate:"required,nonempty,minLength=3"`
	Environment string            `yaml:"environment" yamlvalidate:"required,enum=[dev,stage,prod]"`
	Server      Server            `yaml:"server" yamlvalidate:"required"`
	Services    []Service         `yaml:"services" yamlvalidate:"required,minItems=1"`
	Labels      map[string]string `yaml:"labels,omitempty" yamlvalidate:"keys={pattern='^[a-z][a-z0-9_-]*$'},values={nonempty}"`
	Features    map[string]bool   `yaml:"features,omitempty"`
}

type Server struct {
	Host string `yaml:"host" yamlvalidate:"required,nonempty,format=hostname"`
	Port int    `yaml:"port" yamlvalidate:"required,min=1,max=65535"`
	TLS  *TLS   `yaml:"tls,omitempty"`
}

type TLS struct {
	Cert string `yaml:"cert" yamlvalidate:"required,nonempty"`
	Key  string `yaml:"key" yamlvalidate:"required,nonempty"`
}

type Service struct {
	Name      string     `yaml:"name" yamlvalidate:"required,nonempty,pattern='^[a-z][a-z0-9-]*$'"`
	Replicas  int        `yaml:"replicas" yamlvalidate:"min=1,max=1000"`
	Endpoints []Endpoint `yaml:"endpoints" yamlvalidate:"required,minItems=1"`
}

type Endpoint struct {
	Path      string `yaml:"path" yamlvalidate:"required,nonempty,pattern='^/'"`
	TimeoutMS int    `yaml:"timeout_ms" yamlvalidate:"min=1,max=60000"`
}
