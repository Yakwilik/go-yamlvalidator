// Example demonstrates how to use the yamlvalidator package to validate
// Kubernetes-like manifests with comprehensive schema definitions.
package main

import (
	"fmt"
	"os"
	"regexp"

	v "github.com/Yakwilik/go-yamlvalidator"
	keyv "github.com/Yakwilik/go-yamlvalidator/pkg/keyvalidator"
	valv "github.com/Yakwilik/go-yamlvalidator/pkg/valuevalidator"
)

func main() {
	// Define a schema for a Kubernetes-like manifest
	schema := &v.FieldSchema{
		Type: v.TypeMap,
		AllowedKeys: map[string]*v.FieldSchema{
			"apiVersion": {
				Type:     v.TypeString,
				Required: true,
				Validators: []v.ValueValidator{
					valv.EnumValidator{Allowed: []string{"v1", "v1beta1", "apps/v1"}},
				},
			},
			"kind": {
				Type:       v.TypeString,
				Required:   true,
				Validators: []v.ValueValidator{valv.NonEmptyValidator{}},
			},
			"metadata": {
				Type:     v.TypeMap,
				Required: true,
				AllowedKeys: map[string]*v.FieldSchema{
					"name": {
						Type:     v.TypeString,
						Required: true,
						Validators: []v.ValueValidator{
							valv.RegexValidator{
								Pattern: regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`),
								Message: "must be lowercase DNS-compatible name",
							},
						},
					},
					"namespace": {
						Type:    v.TypeString,
						Default: "default",
					},
					"labels": {
						Type: v.TypeMap,
						// Allow arbitrary keys, but validate their format
						AdditionalProperties: &v.FieldSchema{Type: v.TypeString},
						KeyValidators: []v.KeyValidator{
							keyv.RegexKeyValidator{
								Pattern: regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`),
								Message: "invalid label key format",
							},
						},
					},
					"annotations": {
						Type:                 v.TypeMap,
						AdditionalProperties: &v.FieldSchema{Type: v.TypeString},
					},
				},
				// Allow unknown metadata fields with a warning
				UnknownKeyPolicy: v.UnknownKeyWarn,
			},
			"spec": {
				Type:     v.TypeMap,
				Required: true,
				AllowedKeys: map[string]*v.FieldSchema{
					"replicas": {
						Type: v.TypeInt,
						Validators: []v.ValueValidator{
							valv.RangeValidator{Min: v.Ptr[float64](0), Max: v.Ptr[float64](1000)},
						},
					},
					"selector": {
						Type: v.TypeMap,
						AllowedKeys: map[string]*v.FieldSchema{
							"matchLabels": {
								Type:                 v.TypeMap,
								AdditionalProperties: &v.FieldSchema{Type: v.TypeString},
							},
						},
					},
					"template": {
						Type: v.TypeMap,
						AllowedKeys: map[string]*v.FieldSchema{
							"metadata": {
								Type: v.TypeMap,
								AllowedKeys: map[string]*v.FieldSchema{
									"labels": {
										Type:                 v.TypeMap,
										AdditionalProperties: &v.FieldSchema{Type: v.TypeString},
									},
								},
							},
							"spec": {
								Type: v.TypeMap,
								AllowedKeys: map[string]*v.FieldSchema{
									"containers": {
										Type:     v.TypeSequence,
										Required: true,
										MinItems: v.Ptr[int](1),
										ItemSchema: &v.FieldSchema{
											Type: v.TypeMap,
											AllowedKeys: map[string]*v.FieldSchema{
												"name": {
													Type:     v.TypeString,
													Required: true,
													Validators: []v.ValueValidator{
														valv.NonEmptyValidator{},
													},
												},
												"image": {
													Type:     v.TypeString,
													Required: true,
												},
												"ports": {
													Type: v.TypeSequence,
													ItemSchema: &v.FieldSchema{
														Type: v.TypeMap,
														AllowedKeys: map[string]*v.FieldSchema{
															"containerPort": {
																Type:     v.TypeInt,
																Required: true,
																Validators: []v.ValueValidator{
																	valv.RangeValidator{
																		Min: v.Ptr[float64](1),
																		Max: v.Ptr[float64](65535),
																	},
																},
															},
															"protocol": {
																Type: v.TypeString,
																Validators: []v.ValueValidator{
																	valv.EnumValidator{
																		Allowed: []string{"TCP", "UDP", "SCTP"},
																	},
																},
															},
															"name": {Type: v.TypeString},
														},
													},
												},
												"env": {
													Type: v.TypeSequence,
													ItemSchema: &v.FieldSchema{
														Type: v.TypeMap,
														AllowedKeys: map[string]*v.FieldSchema{
															"name":  {Type: v.TypeString, Required: true},
															"value": {Type: v.TypeString},
															"valueFrom": {
																Type: v.TypeMap,
																AllowedKeys: map[string]*v.FieldSchema{
																	"secretKeyRef": {
																		Type: v.TypeMap,
																		AllowedKeys: map[string]*v.FieldSchema{
																			"name": {Type: v.TypeString, Required: true},
																			"key":  {Type: v.TypeString, Required: true},
																		},
																	},
																	"configMapKeyRef": {
																		Type: v.TypeMap,
																		AllowedKeys: map[string]*v.FieldSchema{
																			"name": {Type: v.TypeString, Required: true},
																			"key":  {Type: v.TypeString, Required: true},
																		},
																	},
																},
																// Either value or valueFrom, not both
																ExactlyOneOf: []string{"secretKeyRef", "configMapKeyRef"},
															},
														},
														// Either value or valueFrom for env entry
														MutuallyExclusive: []string{"value", "valueFrom"},
													},
												},
												"resources": {
													Type: v.TypeMap,
													AllowedKeys: map[string]*v.FieldSchema{
														"limits":   {Type: v.TypeMap, AdditionalProperties: &v.FieldSchema{Type: v.TypeString}},
														"requests": {Type: v.TypeMap, AdditionalProperties: &v.FieldSchema{Type: v.TypeString}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
					// Either configFile or (host and port)
					"configFile": {Type: v.TypeString},
					"host":       {Type: v.TypeString},
					"port":       {Type: v.TypeInt},
				},
				// At least one source of configuration
				AnyOf: [][]string{{"template"}, {"configFile"}, {"host", "port"}},
			},
			"oldSpec": {
				Type:       v.TypeMap,
				Deprecated: "use 'spec' instead, will be removed in v2",
			},
		},
	}

	// Test YAML with various errors
	yamlData := []byte(`
apiVersion: v3
kind: Deployment
metadata:
  name: MyApp
  namespace: production
  labels:
    app: nginx
    123-invalid: test
  unknownMeta: ignored
spec:
  replicas: 1500
  template:
    spec:
      containers:
        - name: app
          image: nginx:latest
          ports:
            - containerPort: 80
              protocol: HTTP
        - name: ""
          image: redis
oldSpec:
  legacy: true
`)

	validator := v.NewValidator(schema)

	// Validate with strict keys
	result := validator.ValidateWithOptions(yamlData, v.ValidationContext{
		StrictKeys: true,
	})

	// Print results
	fmt.Println("════════════════════════════════════════════════════════")
	fmt.Println("VALIDATION RESULTS")
	fmt.Println("════════════════════════════════════════════════════════")

	if len(result.Collector.Errors()) > 0 {
		fmt.Println("\n❌ ERRORS:")
		for _, err := range result.Collector.Errors() {
			fmt.Println(v.FormatErrorWithSource(err, result.SourceLines))
		}
	}

	if len(result.Collector.Warnings()) > 0 {
		fmt.Println("\n⚠️  WARNINGS:")
		for _, warn := range result.Collector.Warnings() {
			fmt.Println(v.FormatErrorWithSource(warn, result.SourceLines))
		}
	}

	fmt.Println("════════════════════════════════════════════════════════")
	if result.HasErrors() {
		fmt.Println("❌ Validation FAILED")
		os.Exit(1)
	} else if len(result.Collector.Warnings()) > 0 {
		fmt.Println("⚠️  Validation passed with warnings")
	} else {
		fmt.Println("✅ Validation PASSED")
	}
}
