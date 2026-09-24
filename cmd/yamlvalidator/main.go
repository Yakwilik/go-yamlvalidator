package main

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func main() {
	schemaPath := flag.String("schema", "", "path to schema file")
	schemaFormat := flag.String("schema-format", "field", "schema format: field or jsonschema")
	filePath := flag.String("file", "", "YAML file to validate (default: stdin)")
	strictKeys := flag.Bool("strict-keys", false, "treat unknown keys as errors when FieldSchema policy is inherit")
	stopFirst := flag.Bool("stop-on-first", false, "stop after the first error")
	strictTypes := flag.Bool("strict-types", false, "infer native FieldSchema types only from explicit YAML tags")
	yaml11Bools := flag.Bool("yaml11-bools", true, "recognize plain YAML 1.1 boolean literals (yes/no/on/off)")
	sortOutput := flag.Bool("sort", true, "sort messages by position")
	jsonDraft := flag.String("json-schema-draft", string(v.JSONSchemaDraft2020), "default JSON Schema draft when $schema is absent")
	assertFormat := flag.Bool("assert-format", false, "force JSON Schema format assertions")
	assertContent := flag.Bool("assert-content", false, "enable JSON Schema content assertions")
	assertVocabs := flag.Bool("assert-vocabs", false, "require declared JSON Schema vocabularies to be known")
	regexpTimeout := flag.Duration("regexp-timeout", 0, "maximum duration of one JSON Schema regexp match (0 = unlimited)")
	maxBytes := flag.Int("max-bytes", 0, "maximum YAML input size in bytes (0 = unlimited)")
	maxDocuments := flag.Int("max-documents", 0, "maximum YAML documents in a stream (0 = unlimited)")
	maxDepth := flag.Int("max-depth", 0, "maximum YAML nesting depth (0 = unlimited)")
	maxDiagnostics := flag.Int("max-diagnostics", 0, "maximum diagnostics before validation stops (0 = unlimited)")
	flag.Parse()

	if *schemaPath == "" {
		fmt.Fprintln(os.Stderr, "schema is required: provide -schema")
		os.Exit(2)
	}

	validator, err := loadValidator(*schemaPath, *schemaFormat, jsonSchemaCLIOptions{
		DefaultDraft:  v.JSONSchemaDraft(*jsonDraft),
		AssertFormat:  *assertFormat,
		AssertContent: *assertContent,
		AssertVocabs:  *assertVocabs,
		RegexpTimeout: *regexpTimeout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "load schema: %v\n", err)
		os.Exit(2)
	}

	data, err := readInput(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read input: %v\n", err)
		os.Exit(2)
	}

	result := validator.ValidateWithOptions(data, v.ValidationContext{
		StrictKeys:     *strictKeys,
		StopOnFirst:    *stopFirst,
		StrictTypes:    *strictTypes,
		YAML11Booleans: *yaml11Bools,
		MaxBytes:       *maxBytes,
		MaxDocuments:   *maxDocuments,
		MaxDepth:       *maxDepth,
		MaxDiagnostics: *maxDiagnostics,
	})

	if len(result.Collector.All()) == 0 {
		fmt.Println("valid")
		return
	}

	fmt.Print(result.FormatAll(*sortOutput))
	if result.Truncated {
		fmt.Fprintln(os.Stderr, "validation stopped after reaching the configured diagnostic limit")
	}
	if result.HasErrors() {
		os.Exit(1)
	}
}

type jsonSchemaCLIOptions struct {
	DefaultDraft  v.JSONSchemaDraft
	AssertFormat  bool
	AssertContent bool
	AssertVocabs  bool
	RegexpTimeout time.Duration
}

func loadValidator(path, format string, opts jsonSchemaCLIOptions) (*v.Validator, error) {
	switch strings.ToLower(format) {
	case "field", "fieldschema":
		schema, err := loadSchemaFromFile(path)
		if err != nil {
			return nil, err
		}
		return v.CompileFieldSchema(schema)
	case "jsonschema", "json-schema":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read JSON Schema: %w", err)
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve schema path: %w", err)
		}
		schemaURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}).String()
		resolver, err := v.NewJSONSchemaFileResolver(filepath.Dir(absolute))
		if err != nil {
			return nil, fmt.Errorf("create schema file resolver: %w", err)
		}
		schema, err := v.CompileJSONSchemaWithOptions(data, v.JSONSchemaCompileOptions{
			SchemaURL:     schemaURL,
			DefaultDraft:  opts.DefaultDraft,
			AssertFormat:  opts.AssertFormat,
			AssertContent: opts.AssertContent,
			AssertVocabs:  opts.AssertVocabs,
			RegexpTimeout: opts.RegexpTimeout,
			Resolver:      resolver,
		})
		if err != nil {
			return nil, err
		}
		return v.NewValidator(schema), nil
	default:
		return nil, fmt.Errorf("unknown schema format %q (expected field or jsonschema)", format)
	}
}

func readInput(path string) ([]byte, error) {
	if path == "" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}
