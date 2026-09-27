package yamlvalidator

import (
	"errors"
	"strings"
	"testing"
)

func TestHighLevelRequiredAndCodec(t *testing.T) {
	type config struct {
		Name string `yaml:"name" yamlvalidate:"required,minLength=2"`
		Port int    `yaml:"port" yamlvalidate:"min=1,max=65535"`
	}
	var got config
	if err := Unmarshal([]byte("name: ok\nport: 8080\n"), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "ok" || got.Port != 8080 {
		t.Fatalf("decoded %#v", got)
	}
	got = config{Name: "unchanged"}
	err := Unmarshal([]byte("port: 0\n"), &got)
	var validation *ValidationErrors
	if !errors.As(err, &validation) || len(validation.Diagnostics()) < 2 {
		t.Fatalf("expected required and range errors, got %v", err)
	}
	if got.Name != "unchanged" {
		t.Fatalf("destination changed after validation failure: %#v", got)
	}
	data, err := Marshal(config{Name: "ok", Port: 8080})
	if err != nil || !strings.Contains(string(data), "port: 8080") {
		t.Fatalf("Marshal = %q, %v", data, err)
	}
}
