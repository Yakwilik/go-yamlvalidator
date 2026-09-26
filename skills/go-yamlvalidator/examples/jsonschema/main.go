package main

import (
	"fmt"
	"os"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func main() {
	schemaData := []byte(`{
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "type": "object",
        "properties": {
            "name": {"type": "string", "minLength": 1},
            "port": {"type": "integer", "minimum": 1, "maximum": 65535}
        },
        "required": ["name"],
        "additionalProperties": false
    }`)
	schema, err := v.CompileJSONSchema(schemaData)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	// Require a document as well as valid instances. An empty stream has none.
	schema.Required = true
	result := v.NewValidator(schema).ValidateBytes([]byte("name: api\nport: 8080\n"))
	if result.HasErrors() {
		fmt.Print(result.FormatAll(true))
		os.Exit(1)
	}
	fmt.Println("valid")
}
