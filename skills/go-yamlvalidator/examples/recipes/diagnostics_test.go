package recipes_test

import (
	"strings"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

func TestSourceDiagnostics(t *testing.T) {
	schema := stringFields("name", "port")
	schema.AllowedKeys["port"].Type = v.TypeInt
	result := v.NewValidator(schema).ValidateBytes([]byte("name: api\nport: wrong\n"))
	diagnostics := result.Collector.Errors()
	if len(diagnostics) != 1 {
		t.Fatalf("got %+v", diagnostics)
	}
	d := diagnostics[0]
	if d.Code != "type_mismatch" || d.Path != "port" || d.Line != 2 || d.Column != 7 {
		t.Fatalf("got %+v", d)
	}
	if _, ok := d.Details.(v.TypeMismatchDetails); !ok {
		t.Fatalf("details=%T", d.Details)
	}
	formatted := result.FormatAll(true)
	if !strings.Contains(formatted, "port: wrong") {
		t.Fatalf("missing source: %s", formatted)
	}
	diagnostics[0].Message = "changed"
	if result.Collector.Errors()[0].Message == "changed" {
		t.Fatal("collector exposed mutable element")
	}
	result.SortByPosition()
}

func TestYAMLSemantics(t *testing.T) {
	native := v.NewValidator(&v.FieldSchema{Type: v.TypeAny})
	checkInputs(t, native, v.ValidationContext{}, []inputCase{
		{name: "duplicate", input: "name: first\nname: second\n", code: "duplicate_key"},
		{name: "bad indent", input: "generate:\n  plugins:\n    - name: go\n     out: ./gen\n", code: "yaml_syntax"},
		{name: "cycle", input: "a: &a [*a]"},
		{name: "literal string", input: "\"123\"", valid: true},
	})
	boolean := v.NewValidator(&v.FieldSchema{Type: v.TypeBool})
	checkInputs(t, boolean, v.ValidationContext{}, []inputCase{{name: "default", input: "yes", code: "type_mismatch"}})
	checkInputs(t, boolean, v.ValidationContext{YAML11Booleans: true}, []inputCase{
		{name: "plain", input: "yes", valid: true}, {name: "quoted", input: "\"yes\"", code: "type_mismatch"},
	})
	jsonValidator := compileJSON(t, `true`, v.JSONSchemaCompileOptions{})
	checkInputs(t, jsonValidator, v.ValidationContext{}, []inputCase{
		{name: "numeric key", input: "1: value", code: "json_instance_conversion"},
		{name: "nonfinite", input: ".nan", code: "json_instance_conversion"},
	})
	merged := compileJSON(t, `{"type":"object","properties":{"service":{"properties":{"name":{"const":"explicit"}}}}}`, v.JSONSchemaCompileOptions{})
	checkInputs(t, merged, v.ValidationContext{}, []inputCase{
		{name: "explicit beats merge", input: "base: &base {name: inherited}\nservice:\n  <<: *base\n  name: explicit\n", valid: true},
	})
	stream := v.NewValidator(&v.FieldSchema{Type: v.TypeString})
	result := stream.ValidateBytes([]byte("valid\n---\n123\n"))
	if len(result.Collector.Errors()) != 1 || result.Collector.Errors()[0].Path != "doc[1]" {
		t.Fatalf("got %+v", result.Collector.Errors())
	}
}

func TestStructuredPaths(t *testing.T) {
	path := v.AppendPropertyPath("", "a.b")
	path, err := v.AppendIndexPath(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	path = v.AppendPropertyPath(path, "name")
	tokens, err := v.ParsePathTokens(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 3 || tokens[0].Kind != v.PathTokenProperty || tokens[1].Kind != v.PathTokenIndex {
		t.Fatalf("got %+v", tokens)
	}
	if v.FormatPathTokens(tokens) != path {
		t.Fatal("path round trip failed")
	}
	if _, err := v.AppendIndexPath(path, -1); err == nil {
		t.Fatal("negative index accepted")
	}
}

func TestLowLevelDiagnosticHelpers(t *testing.T) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "42"}
	if got := v.InferNodeType(node, nil); got != v.TypeInt {
		t.Fatalf("type=%s, want integer", got)
	}
	line := "port: wrong"
	rendered, column, width := v.RenderLineWithCaret(line, 7)
	if rendered != line || column != 7 || width != len(line) {
		t.Fatalf("render=%q column=%d width=%d", rendered, column, width)
	}
	diagnostic := v.ValidationError{
		Level: v.LevelError, Code: "type_mismatch", Path: "port",
		Line: 1, Column: 7, Message: "expected an integer",
	}
	formatted := v.FormatErrorWithSource(diagnostic, []string{line})
	if !strings.Contains(formatted, line) || !strings.Contains(formatted, "^") {
		t.Fatalf("missing source/caret: %s", formatted)
	}
	collector := v.NewErrorCollector()
	collector.Add(diagnostic)
	collector.Add(v.ValidationError{Level: v.LevelWarning, Message: "review this field"})
	if !collector.HasErrors() || len(collector.Errors()) != 1 || len(collector.Warnings()) != 1 || len(collector.All()) != 2 {
		t.Fatalf("unexpected collector content: %+v", collector.All())
	}
}
