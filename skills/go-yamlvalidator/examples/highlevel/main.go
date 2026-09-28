package main

import (
	"fmt"

	v "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
	Mode string `yaml:"mode" yamlvalidate:"required,enum=[dev,prod]"`
	File string `yaml:"file,omitempty" yamlvalidate:"exactlyOneOf=[file,url],nonempty"`
	URL  string `yaml:"url,omitempty"`
}

func main() {
	data := []byte("mode: prod\nurl: https://example.org\n")
	var cfg Config
	if err := v.Unmarshal(data, &cfg); err != nil {
		panic(err)
	}
	fmt.Printf("%s %s\n", cfg.Mode, cfg.URL)
}
