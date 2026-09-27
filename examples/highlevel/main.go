package main

import (
	"fmt"

	yamlvalidator "github.com/Yakwilik/go-yamlvalidator"
)

type Config struct {
	Mode  string   "yaml:\"mode\" yamlvalidate:\"required,enum=[dev,prod],when={field=mode,eq=prod,require=[tls]}\""
	Port  int      "yaml:\"port\" yamlvalidate:\"required,min=1,max=65535\""
	TLS   bool     "yaml:\"tls,omitempty\""
	Hosts []string "yaml:\"hosts\" yamlvalidate:\"minItems=1,items={minLength=3},uniqueItems\""
}

func main() {
	input := []byte("mode: prod\nport: 8443\ntls: true\nhosts: [api.example.com]\n")
	var config Config
	if err := yamlvalidator.Unmarshal(input, &config); err != nil {
		panic(err)
	}
	output, err := yamlvalidator.Marshal(config)
	if err != nil {
		panic(err)
	}
	fmt.Print(string(output))
}
