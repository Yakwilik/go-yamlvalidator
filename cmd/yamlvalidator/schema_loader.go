package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	v "github.com/Yakwilik/go-yamlvalidator"
	keyv "github.com/Yakwilik/go-yamlvalidator/pkg/keyvalidator"
	valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
	"gopkg.in/yaml.v3"
)

type schemaNode struct {
	Type              string                 `yaml:"type" json:"type"`
	Required          bool                   `yaml:"required" json:"required"`
	Nullable          bool                   `yaml:"nullable" json:"nullable"`
	Deprecated        string                 `yaml:"deprecated" json:"deprecated"`
	Default           interface{}            `yaml:"default" json:"default"`
	AllowedKeys       map[string]*schemaNode `yaml:"allowedKeys" json:"allowedKeys"`
	AdditionalProps   *schemaNode            `yaml:"additionalProperties" json:"additionalProperties"`
	UnknownKeyPolicy  string                 `yaml:"unknownKeyPolicy" json:"unknownKeyPolicy"`
	KeyValidators     []keyValidatorSpec     `yaml:"keyValidators" json:"keyValidators"`
	ItemSchema        *schemaNode            `yaml:"itemSchema" json:"itemSchema"`
	MinItems          *int                   `yaml:"minItems" json:"minItems"`
	MaxItems          *int                   `yaml:"maxItems" json:"maxItems"`
	Validators        []valueValidatorSpec   `yaml:"validators" json:"validators"`
	AnyOf             [][]string             `yaml:"anyOf" json:"anyOf"`
	ExactlyOneOf      []string               `yaml:"exactlyOneOf" json:"exactlyOneOf"`
	MutuallyExclusive []string               `yaml:"mutuallyExclusive" json:"mutuallyExclusive"`
	Conditions        []conditionalSpec      `yaml:"conditions" json:"conditions"`
	AdditionalRaw     map[string]interface{} `yaml:"-" json:"-"` // catch-all for debugging
}

type valueValidatorSpec struct {
	Name           string   `yaml:"name" json:"name"`
	Allowed        []string `yaml:"allowed" json:"allowed"`               // enum
	Pattern        string   `yaml:"pattern" json:"pattern"`               // regex
	Message        string   `yaml:"message" json:"message"`               // regex
	Min            *float64 `yaml:"min" json:"min"`                       // range (float)
	Max            *float64 `yaml:"max" json:"max"`                       // range (float)
	MinLength      *int     `yaml:"minLength" json:"minLength"`           // length
	MaxLength      *int     `yaml:"maxLength" json:"maxLength"`           // length
	RequireScheme  bool     `yaml:"requireScheme" json:"requireScheme"`   // url
	AllowedSchemes []string `yaml:"allowedSchemes" json:"allowedSchemes"` // url
	Types          []string `yaml:"types" json:"types"`                   // one-of-type
}

type keyValidatorSpec struct {
	Name      string   `yaml:"name" json:"name"`
	Pattern   string   `yaml:"pattern" json:"pattern"`     // regex
	Message   string   `yaml:"message" json:"message"`     // regex
	Forbidden []string `yaml:"forbidden" json:"forbidden"` // forbidden
	MinLength *int     `yaml:"minLength" json:"minLength"` // length
	Min       *int     `yaml:"min" json:"min"`             // alias for length
	MaxLength *int     `yaml:"maxLength" json:"maxLength"` // length
	Max       *int     `yaml:"max" json:"max"`             // alias for length
}

type conditionalSpec struct {
	ConditionField string      `yaml:"conditionField" json:"conditionField"`
	ConditionValue interface{} `yaml:"conditionValue" json:"conditionValue"`
	ThenRequired   []string    `yaml:"thenRequired" json:"thenRequired"`
	ThenForbidden  []string    `yaml:"thenForbidden" json:"thenForbidden"`
}

// loadSchemaFromFile decodes a YAML/JSON schema file into FieldSchema.
func loadSchemaFromFile(path string) (*v.FieldSchema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read schema: %w", err)
	}
	var root schemaNode
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}
	return convertSchemaNode(&root)
}

func convertSchemaNode(sn *schemaNode) (*v.FieldSchema, error) {
	if sn == nil {
		return nil, errors.New("schema node is nil")
	}

	nodeType, err := parseNodeType(sn.Type)
	if err != nil {
		return nil, err
	}
	ukp, err := parseUnknownKeyPolicy(sn.UnknownKeyPolicy)
	if err != nil {
		return nil, err
	}

	fs := &v.FieldSchema{
		Type:             nodeType,
		Required:         sn.Required,
		Nullable:         sn.Nullable,
		Deprecated:       sn.Deprecated,
		Default:          sn.Default,
		UnknownKeyPolicy: ukp,
	}

	if sn.ItemSchema != nil {
		fs.ItemSchema, err = convertSchemaNode(sn.ItemSchema)
		if err != nil {
			return nil, err
		}
	}
	fs.MinItems = sn.MinItems
	fs.MaxItems = sn.MaxItems

	if sn.AllowedKeys != nil {
		fs.AllowedKeys = make(map[string]*v.FieldSchema, len(sn.AllowedKeys))
		for k, child := range sn.AllowedKeys {
			converted, err := convertSchemaNode(child)
			if err != nil {
				return nil, fmt.Errorf("allowedKeys[%s]: %w", k, err)
			}
			fs.AllowedKeys[k] = converted
		}
	}
	if sn.AdditionalProps != nil {
		fs.AdditionalProperties, err = convertSchemaNode(sn.AdditionalProps)
		if err != nil {
			return nil, fmt.Errorf("additionalProperties: %w", err)
		}
	}

	if len(sn.AnyOf) > 0 {
		fs.AnyOf = sn.AnyOf
	}
	if len(sn.ExactlyOneOf) > 0 {
		fs.ExactlyOneOf = sn.ExactlyOneOf
	}
	if len(sn.MutuallyExclusive) > 0 {
		fs.MutuallyExclusive = sn.MutuallyExclusive
	}

	if len(sn.Validators) > 0 {
		vals := make([]v.ValueValidator, 0, len(sn.Validators))
		for _, spec := range sn.Validators {
			val, err := buildValueValidator(spec)
			if err != nil {
				return nil, err
			}
			vals = append(vals, val)
		}
		fs.Validators = vals
	}

	if len(sn.KeyValidators) > 0 {
		vals := make([]v.KeyValidator, 0, len(sn.KeyValidators))
		for _, spec := range sn.KeyValidators {
			val, err := buildKeyValidator(spec)
			if err != nil {
				return nil, err
			}
			vals = append(vals, val)
		}
		fs.KeyValidators = vals
	}

	if len(sn.Conditions) > 0 {
		conds := make([]v.ConditionalRule, 0, len(sn.Conditions))
		for _, c := range sn.Conditions {
			conds = append(conds, v.ConditionalRule{
				ConditionField: c.ConditionField,
				ConditionValue: fmt.Sprint(c.ConditionValue),
				ThenRequired:   c.ThenRequired,
				ThenForbidden:  c.ThenForbidden,
			})
		}
		fs.Conditions = conds
	}

	return fs, nil
}

func parseNodeType(t string) (v.NodeType, error) {
	switch strings.ToLower(t) {
	case "", "any":
		return v.TypeAny, nil
	case "null":
		return v.TypeNull, nil
	case "string":
		return v.TypeString, nil
	case "int", "integer":
		return v.TypeInt, nil
	case "float", "number":
		return v.TypeFloat, nil
	case "bool", "boolean":
		return v.TypeBool, nil
	case "map", "object":
		return v.TypeMap, nil
	case "sequence", "array":
		return v.TypeSequence, nil
	default:
		return v.TypeAny, fmt.Errorf("unknown type: %q", t)
	}
}

func parseUnknownKeyPolicy(p string) (v.UnknownKeyPolicy, error) {
	switch strings.ToLower(p) {
	case "", "inherit":
		return v.UnknownKeyInherit, nil
	case "warn":
		return v.UnknownKeyWarn, nil
	case "error":
		return v.UnknownKeyError, nil
	case "ignore":
		return v.UnknownKeyIgnore, nil
	default:
		return v.UnknownKeyInherit, fmt.Errorf("unknown unknownKeyPolicy: %q", p)
	}
}

func buildValueValidator(spec valueValidatorSpec) (v.ValueValidator, error) {
	switch strings.ToLower(spec.Name) {
	case "enum":
		return valv.EnumValidator{Allowed: spec.Allowed}, nil
	case "regex":
		re, err := regexp.Compile(spec.Pattern)
		if err != nil {
			return nil, fmt.Errorf("regex validator: %w", err)
		}
		return valv.RegexValidator{Pattern: re, Message: spec.Message}, nil
	case "range":
		return valv.RangeValidator{Min: spec.Min, Max: spec.Max}, nil
	case "nonempty":
		return valv.NonEmptyValidator{}, nil
	case "length":
		return valv.LengthValidator{Min: spec.MinLength, Max: spec.MaxLength}, nil
	case "url":
		return valv.URLValidator{RequireScheme: spec.RequireScheme, AllowedSchemes: spec.AllowedSchemes}, nil
	case "oneoftype":
		types := make([]v.NodeType, 0, len(spec.Types))
		for _, t := range spec.Types {
			nt, err := parseNodeType(t)
			if err != nil {
				return nil, err
			}
			types = append(types, nt)
		}
		return valv.OneOfTypeValidator{Types: types}, nil
	default:
		return nil, fmt.Errorf("unknown validator name: %q", spec.Name)
	}
}

func buildKeyValidator(spec keyValidatorSpec) (v.KeyValidator, error) {
	switch strings.ToLower(spec.Name) {
	case "regex":
		re, err := regexp.Compile(spec.Pattern)
		if err != nil {
			return nil, fmt.Errorf("regex key validator: %w", err)
		}
		return keyv.RegexKeyValidator{Pattern: re, Message: spec.Message}, nil
	case "forbidden":
		return keyv.ForbiddenKeyValidator{Forbidden: spec.Forbidden}, nil
	case "length":
		min := spec.MinLength
		if min == nil {
			min = spec.Min
		}
		max := spec.MaxLength
		if max == nil {
			max = spec.Max
		}
		return keyv.LengthKeyValidator{Min: min, Max: max}, nil
	default:
		return nil, fmt.Errorf("unknown key validator name: %q", spec.Name)
	}
}
