package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func main() {
	schemaPath := flag.String("schema", "", "path to YAML/JSON schema file describing FieldSchema")
	filePath := flag.String("file", "", "YAML file to validate (default: stdin)")
	strictKeys := flag.Bool("strict-keys", false, "treat unknown keys as errors when policy is inherit")
	stopFirst := flag.Bool("stop-on-first", false, "stop after the first error")
	strictTypes := flag.Bool("strict-types", false, "infer types only from explicit YAML tags")
	yaml11Bools := flag.Bool("yaml11-bools", true, "recognize YAML 1.1 boolean literals (yes/no/on/off)")
	sortOutput := flag.Bool("sort", true, "sort messages by position")
	flag.Parse()

	if *schemaPath == "" {
		fmt.Fprintln(os.Stderr, "schema is required: provide -schema pointing to a YAML or JSON schema file")
		os.Exit(2)
	}

	schema, err := loadSchemaFromFile(*schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load schema: %v\n", err)
		os.Exit(2)
	}

	data, err := readInput(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
		os.Exit(2)
	}

	validator := v.NewValidator(schema)
	result := validator.ValidateWithOptions(data, v.ValidationContext{
		StrictKeys:     *strictKeys,
		StopOnFirst:    *stopFirst,
		StrictTypes:    *strictTypes,
		YAML11Booleans: *yaml11Bools,
	})

	if len(result.Collector.All()) == 0 {
		fmt.Println("valid")
		return
	}

	fmt.Print(result.FormatAll(*sortOutput))
	if result.HasErrors() {
		os.Exit(1)
	}
}

func readInput(path string) ([]byte, error) {
	if path == "" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}
