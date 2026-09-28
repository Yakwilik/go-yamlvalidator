package yamlvalidator

import "testing"

func TestQuotedMergeKeyIsLiteral(t *testing.T) {
	schema := &FieldSchema{Type: TypeMap, AllowedKeys: map[string]*FieldSchema{
		"<<": {Type: TypeString, Required: true},
	}}
	result := NewValidator(schema).ValidateBytes([]byte("'<<': literal\n"))
	if result.HasErrors() {
		t.Fatalf("quoted merge key should be ordinary key: %v", result.Collector.Errors())
	}
}

func TestConditionalAliasScalar(t *testing.T) {
	schema := &FieldSchema{Type: TypeMap, AllowedKeys: map[string]*FieldSchema{
		"mode": {Type: TypeString}, "tls": {Type: TypeBool},
	}, Conditions: []ConditionalRule{{ConditionField: "mode", ConditionValue: "prod", ThenRequired: []string{"tls"}}}}
	result := NewValidator(schema).ValidateBytes([]byte("base: &prod prod\nmode: *prod\n"))
	found := false
	for _, d := range result.Collector.Errors() {
		if d.Code == "condition_required" {
			found = true
		}
	}
	if !found {
		t.Fatalf("alias condition did not fire: %v", result.Collector.All())
	}
}
