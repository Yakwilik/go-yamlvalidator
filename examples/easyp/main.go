package main

import (
	"flag"
	"fmt"
	"os"

	v "github.com/Yakwilik/go-yamlvalidator"
	"github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
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
		Validators:       []v.ValueValidator{valuevalidator.DirectoryValidator{}},
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
		Validators:        []v.ValueValidator{valuevalidator.PluginSourceValidator{}},
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
		Validators:        []v.ValueValidator{valuevalidator.ManagedDisableValidator{}},
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
		Validators:        []v.ValueValidator{valuevalidator.ManagedOverrideValidator{}},
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
			"inputs":  {Type: v.TypeSequence, ItemSchema: inputSchema, Required: true, MinItems: v.PtrInt(1)},
			"plugins": {Type: v.TypeSequence, ItemSchema: pluginSchema, Required: true, MinItems: v.PtrInt(1)},
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
