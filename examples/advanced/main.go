package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"

	v "github.com/Yakwilik/go-yamlvalidator"
	"github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
)

func main() {
	path := flag.String("file", "config.yaml", "path to config YAML")
	flag.Parse()

	data, err := os.ReadFile(*path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *path, err)
		os.Exit(1)
	}

	validator, err := v.CompileFieldSchema(buildSchema())
	if err != nil {
		fmt.Fprintf(os.Stderr, "compile schema: %v\n", err)
		os.Exit(1)
	}

	result := validator.ValidateWithOptions(data, v.ValidationContext{
		StrictKeys: true,
	})
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
	stringMap := &v.FieldSchema{
		Type:                 v.TypeMap,
		AdditionalProperties: &v.FieldSchema{Type: v.TypeString},
	}

	service := &v.FieldSchema{
		Type:     v.TypeMap,
		Required: true,
		AllowedKeys: map[string]*v.FieldSchema{
			"name": {
				Type:     v.TypeString,
				Required: true,
				Validators: []v.ValueValidator{
					valuevalidator.RegexValidator{
						Pattern: regexp.MustCompile(`^[a-z][a-z0-9-]*$`),
					},
				},
			},
			"environment": {
				Type:     v.TypeString,
				Required: true,
				Validators: []v.ValueValidator{
					valuevalidator.EnumValidator{
						Allowed: []string{"development", "staging", "production"},
					},
				},
			},
			"replicas": {
				Type:     v.TypeInt,
				Required: true,
				Validators: []v.ValueValidator{
					valuevalidator.RangeValidator{
						Min: v.Ptr[float64](1),
						Max: v.Ptr[float64](100),
					},
				},
			},
			"endpoint": {
				Type:     v.TypeString,
				Required: true,
				Validators: []v.ValueValidator{
					valuevalidator.URLValidator{
						RequireScheme:  true,
						AllowedSchemes: []string{"http", "https"},
					},
				},
			},
			"labels": stringMap,
		},
		UnknownKeyPolicy: v.UnknownKeyError,
	}

	auth := &v.FieldSchema{
		Type:     v.TypeMap,
		Required: true,
		AllowedKeys: map[string]*v.FieldSchema{
			"token":    {Type: v.TypeString},
			"username": {Type: v.TypeString},
			"password": {Type: v.TypeString},
		},
		OneOfRequired: [][]string{
			{"token"},
			{"username", "password"},
		},
		ForbiddenTogether: [][]string{
			{"token", "username"},
			{"token", "password"},
		},
		DependentRequired: map[string][]string{
			"username": {"password"},
			"password": {"username"},
		},
		UnknownKeyPolicy: v.UnknownKeyError,
	}

	worker := &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"name": {Type: v.TypeString, Required: true},
			"concurrency": {
				Type:     v.TypeInt,
				Required: true,
				Validators: []v.ValueValidator{
					valuevalidator.RangeValidator{
						Min: v.Ptr[float64](1),
						Max: v.Ptr[float64](64),
					},
				},
			},
		},
		UnknownKeyPolicy: v.UnknownKeyError,
	}

	notifications := &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"enabled": {Type: v.TypeBool, Required: true},
			"webhook": {
				Type: v.TypeString,
				Validators: []v.ValueValidator{
					valuevalidator.URLValidator{
						RequireScheme:  true,
						AllowedSchemes: []string{"https"},
					},
				},
			},
		},
		Conditions: []v.ConditionalRule{
			{
				ConditionField: "enabled",
				ConditionValue: "true",
				ThenRequired:   []string{"webhook"},
			},
		},
		UnknownKeyPolicy: v.UnknownKeyError,
	}

	return &v.FieldSchema{
		Type:     v.TypeMap,
		Required: true,
		AllowedKeys: map[string]*v.FieldSchema{
			"service":       service,
			"auth":          auth,
			"workers":       {Type: v.TypeSequence, ItemSchema: worker, MinItems: v.Ptr(1)},
			"notifications": notifications,
		},
		UnknownKeyPolicy: v.UnknownKeyError,
	}
}
