package main

import (
	"fmt"
	"os"

	"github.com/Yakwilik/go-yamlvalidator"
	"github.com/Yakwilik/go-yamlvalidator/examples/codegen/model"
)

func main() {
	var config model.Config
	if err := yamlvalidator.Unmarshal([]byte("name: example\nitems: [first]\n"), &config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, err := yamlvalidator.Marshal(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(string(data))
}
