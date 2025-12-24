package main

import (
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func main() {
	path := flag.String("file", "easyp.yaml", "path to easyp.yaml")
	flag.Parse()

	data, err := os.ReadFile(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *path, err)
		os.Exit(1)
	}

	validator := v.NewValidator(buildSchema())
	ctx := v.ValidationContext{
		StrictKeys:     true,
		YAML11Booleans: true,
	}
	result := validator.ValidateWithOptions(data, ctx)

	if len(result.Collector.All()) == 0 {
		fmt.Println("config is valid")
		return
	}

	fmt.Print(result.FormatAll(true))
	if result.HasErrors() {
		os.Exit(1)
	}
}

func buildSchema() *v.FieldSchema {
	stringSeq := &v.FieldSchema{Type: v.TypeSequence, ItemSchema: &v.FieldSchema{Type: v.TypeString}}
	stringMap := &v.FieldSchema{Type: v.TypeMap, AdditionalProperties: &v.FieldSchema{Type: v.TypeString}}

	lintSchema := &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"use":                    stringSeq,
			"enum_zero_value_suffix": {Type: v.TypeString},
			"service_suffix":         {Type: v.TypeString},
			"ignore":                 stringSeq,
			"except":                 stringSeq,
			"allow_comment_ignores":  {Type: v.TypeBool},
			"ignore_only": {
				Type:                 v.TypeMap,
				AdditionalProperties: stringSeq,
			},
		},
		UnknownKeyPolicy: v.UnknownKeyWarn,
	}

	depsSchema := stringSeq

	inputDirSchema := &v.FieldSchema{
		Type: v.TypeAny, // string or map
		AllowedKeys: map[string]*v.FieldSchema{
			"path": {Type: v.TypeString},
			"root": {Type: v.TypeString},
		},
		UnknownKeyPolicy: v.UnknownKeyWarn,
		Validators:       []v.ValueValidator{DirectoryValidator{}},
	}
	inputGitSchema := &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"url":           {Type: v.TypeString, Required: true},
			"sub_directory": {Type: v.TypeString},
			"out":           {Type: v.TypeString},
			"root":          {Type: v.TypeString},
		},
		UnknownKeyPolicy: v.UnknownKeyWarn,
	}
	inputSchema := &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"directory": inputDirSchema,
			"git_repo":  inputGitSchema,
		},
		AnyOf:             [][]string{{"directory"}, {"git_repo"}},
		MutuallyExclusive: []string{"directory", "git_repo"},
		UnknownKeyPolicy:  v.UnknownKeyWarn,
	}

	pluginSchema := &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"name":         {Type: v.TypeString},
			"remote":       {Type: v.TypeString},
			"path":         {Type: v.TypeString},
			"command":      {Type: v.TypeSequence, ItemSchema: &v.FieldSchema{Type: v.TypeString}},
			"out":          {Type: v.TypeString},
			"opts":         stringMap,
			"with_imports": {Type: v.TypeBool},
		},
		AnyOf:             [][]string{{"name"}, {"remote"}, {"path"}, {"command"}},
		MutuallyExclusive: []string{"name", "remote", "path", "command"},
		UnknownKeyPolicy:  v.UnknownKeyWarn,
		Validators:        []v.ValueValidator{PluginSourceValidator{}},
	}

	managedDisableSchema := &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"module":       {Type: v.TypeString},
			"path":         {Type: v.TypeString},
			"file_option":  {Type: v.TypeString},
			"field_option": {Type: v.TypeString},
			"field":        {Type: v.TypeString},
		},
		AnyOf:             [][]string{{"module"}, {"path"}, {"file_option"}, {"field_option"}, {"field"}},
		MutuallyExclusive: []string{"file_option", "field_option"},
		UnknownKeyPolicy:  v.UnknownKeyWarn,
		Validators:        []v.ValueValidator{ManagedDisableValidator{}},
	}

	managedOverrideSchema := &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"file_option":  {Type: v.TypeString},
			"field_option": {Type: v.TypeString},
			"value":        {Type: v.TypeAny, Required: true},
			"module":       {Type: v.TypeString},
			"path":         {Type: v.TypeString},
			"field":        {Type: v.TypeString},
		},
		AnyOf:             [][]string{{"file_option"}, {"field_option"}},
		MutuallyExclusive: []string{"file_option", "field_option"},
		UnknownKeyPolicy:  v.UnknownKeyWarn,
		Validators:        []v.ValueValidator{ManagedOverrideValidator{}},
	}

	managedSchema := &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"enabled":  {Type: v.TypeBool},
			"disable":  {Type: v.TypeSequence, ItemSchema: managedDisableSchema},
			"override": {Type: v.TypeSequence, ItemSchema: managedOverrideSchema},
		},
		UnknownKeyPolicy: v.UnknownKeyWarn,
	}

	generateSchema := &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"inputs":  {Type: v.TypeSequence, ItemSchema: inputSchema, Required: true, MinItems: v.Ptr[int](1)},
			"plugins": {Type: v.TypeSequence, ItemSchema: pluginSchema, Required: true, MinItems: v.Ptr[int](1)},
			"managed": managedSchema,
		},
		UnknownKeyPolicy: v.UnknownKeyWarn,
	}

	breakingSchema := &v.FieldSchema{Type: v.TypeMap, UnknownKeyPolicy: v.UnknownKeyIgnore}

	return &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"lint":     lintSchema,
			"deps":     depsSchema,
			"generate": generateSchema,
			"breaking": breakingSchema,
		},
		Required:         true,
		UnknownKeyPolicy: v.UnknownKeyWarn,
	}
}

// DirectoryValidator allows directory to be either a string or a map with required path and optional root.
type DirectoryValidator struct{}

func (DirectoryValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" || node.Tag == "!!bool" || node.Tag == "!!int" || node.Tag == "!!float" {
			ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path, Line: node.Line, Column: node.Column, Message: "directory must be a string or mapping", Got: node.Tag})
			return
		}
	case yaml.MappingNode:
		requiredPath := false
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			switch keyNode.Value {
			case "path":
				requiredPath = true
				if valNode.Kind != yaml.ScalarNode {
					ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path + ".path", Line: valNode.Line, Column: valNode.Column, Message: "path must be a string"})
				}
			case "root":
				if valNode.Kind != yaml.ScalarNode {
					ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path + ".root", Line: valNode.Line, Column: valNode.Column, Message: "root must be a string"})
				}
			default:
				ctx.AddError(v.ValidationError{Level: v.LevelWarning, Path: path + "." + keyNode.Value, Line: keyNode.Line, Column: keyNode.Column, Message: "unknown field under directory", Got: keyNode.Value})
			}
		}
		if !requiredPath {
			ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path + ".path", Line: node.Line, Column: node.Column, Message: "directory.path is required"})
		}
	default:
		ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path, Line: node.Line, Column: node.Column, Message: "directory must be string or mapping", Got: fmt.Sprintf("%v", node.Kind)})
	}
}

// PluginSourceValidator ensures exactly one plugin source is set.
type PluginSourceValidator struct{}

func (PluginSourceValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	if node.Kind != yaml.MappingNode {
		return
	}
	var count int
	has := func(key string) bool {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				return true
			}
		}
		return false
	}
	if has("name") {
		count++
	}
	if has("remote") {
		count++
	}
	if has("path") {
		count++
	}
	if has("command") {
		count++
	}
	if count == 0 {
		ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path, Line: node.Line, Column: node.Column, Message: "plugins item must have one of name/remote/path/command"})
	} else if count > 1 {
		ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path, Line: node.Line, Column: node.Column, Message: "plugins item must not set multiple sources (name/remote/path/command)"})
	}
}

type ManagedDisableValidator struct{}

func (ManagedDisableValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	if node.Kind != yaml.MappingNode {
		return
	}
	has := func(k string) bool {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == k {
				return true
			}
		}
		return false
	}
	if !(has("module") || has("path") || has("file_option") || has("field_option") || has("field")) {
		ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path, Line: node.Line, Column: node.Column, Message: "managed.disable entry must set at least one of module/path/file_option/field_option/field"})
	}
	if has("file_option") && has("field_option") {
		ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path, Line: node.Line, Column: node.Column, Message: "managed.disable: file_option and field_option cannot both be set"})
	}
	if has("field") && !has("field_option") {
		ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path, Line: node.Line, Column: node.Column, Message: "managed.disable.field requires field_option"})
	}
}

type ManagedOverrideValidator struct{}

func (ManagedOverrideValidator) Validate(node *yaml.Node, path string, ctx *v.ValidationContext) {
	if node.Kind != yaml.MappingNode {
		return
	}
	has := func(k string) bool {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == k {
				return true
			}
		}
		return false
	}
	if !has("file_option") && !has("field_option") {
		ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path, Line: node.Line, Column: node.Column, Message: "managed.override requires either file_option or field_option"})
	}
	if has("file_option") && has("field_option") {
		ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path, Line: node.Line, Column: node.Column, Message: "managed.override cannot set both file_option and field_option"})
	}
	if has("field") && !has("field_option") {
		ctx.AddError(v.ValidationError{Level: v.LevelError, Path: path, Line: node.Line, Column: node.Column, Message: "managed.override.field can only be used with field_option"})
	}
}
