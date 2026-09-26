package main

import (
	"fmt"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func main() {
	schema := &v.FieldSchema{
		Type:             v.TypeMap,
		Required:         true,
		UnknownKeyPolicy: v.UnknownKeyError,
		AllowedKeys: map[string]*v.FieldSchema{
			"name": {Type: v.TypeString, Required: true},
			"port": {Type: v.TypeInt},
		},
	}
	result := v.NewValidator(schema).ValidateBytes([]byte("name: api\nport: 8080\n"))
	if result.HasErrors() {
		fmt.Print(result.FormatAll(true))
		return
	}
	fmt.Println("valid")
}
