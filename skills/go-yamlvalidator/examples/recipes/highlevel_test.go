package recipes

import (
	"strings"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
)

type highLevelConfig struct {
	Mode  string   `yaml:"mode" yamlvalidate:"required,enum=[dev,prod]"`
	File  string   `yaml:"file,omitempty" yamlvalidate:"exactlyOneOf=[file,url],nonempty"`
	URL   string   `yaml:"url,omitempty"`
	Hosts []string `yaml:"hosts" yamlvalidate:"required,minItems=1,items={nonempty,format=hostname}"`
}

func TestHighLevelStructTags(t *testing.T) {
	input := []byte("mode: prod\nurl: https://example.org\nhosts: [api.example.org]\n")
	var got highLevelConfig
	if err := v.Unmarshal(input, &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != "prod" || got.URL == "" || len(got.Hosts) != 1 {
		t.Fatalf("unexpected decode: %+v", got)
	}

	for _, bad := range []string{
		"mode: prod\nhosts: [api.example.org]\n",
		"mode: prod\nfile: local\nurl: https://example.org\nhosts: [api.example.org]\n",
		"mode: prod\nurl: https://example.org\nhosts: []\n",
		"mode: prod\nurl: https://example.org\nhosts: [not a host]\n",
		"mode: prod\nurl: https://example.org\nhosts: [api.example.org]\nextra: true\n",
	} {
		var cfg highLevelConfig
		if err := v.Unmarshal([]byte(bad), &cfg); err == nil {
			t.Fatalf("invalid YAML accepted: %q", bad)
		}
	}
}

func TestHighLevelValidationFailureDoesNotMutate(t *testing.T) {
	cfg := highLevelConfig{Mode: "dev", File: "existing", Hosts: []string{"old.example.org"}}
	err := v.Unmarshal([]byte("mode: prod\nfile: a\nurl: b\nhosts: [api.example.org]\n"), &cfg)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if cfg.Mode != "dev" || cfg.File != "existing" || cfg.URL != "" || len(cfg.Hosts) != 1 || cfg.Hosts[0] != "old.example.org" {
		t.Fatalf("destination mutated on validation failure: %+v", cfg)
	}
}

func TestHighLevelMarshalValidatesEmittedYAML(t *testing.T) {
	valid := highLevelConfig{Mode: "prod", URL: "https://example.org", Hosts: []string{"api.example.org"}}
	data, err := v.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "mode: prod") || !strings.Contains(string(data), "url: https://example.org") {
		t.Fatalf("unexpected YAML: %s", data)
	}

	invalid := highLevelConfig{Mode: "prod", Hosts: []string{"api.example.org"}}
	if _, err := v.Marshal(invalid); err == nil {
		t.Fatal("Marshal accepted missing exactlyOneOf member")
	}
}
