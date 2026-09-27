package tagrules

import (
	v "github.com/Yakwilik/go-yamlvalidator"
	"testing"
)

func TestDocumentedRules(t *testing.T) {
	cases := []struct {
		name, input string
		out         any
		valid       bool
	}{
		{"intrinsic file", "file: config.yaml", new(IntrinsicSource), true},
		{"absent carrier", "url: https://example.org", new(IntrinsicSource), true},
		{"intrinsic none", "{}", new(IntrinsicSource), false},
		{"intrinsic both", "file: x\nurl: y", new(IntrinsicSource), false},
		{"use site", "source: {file: x}", new(Config), true},
		{"missing container", "{}", new(Config), false},
		{"empty container", "source: {}", new(Config), false},
		{"list", "sources: [{file: x}, {inline: y}]", new(SourceList), true},
		{"list invalid element", "sources: [{file: x}, {}]", new(SourceList), false},
		{"map", "sources: {api: {file: x}, worker: {inline: y}}", new(SourceMap), true},
		{"map invalid value", "sources: {api: {file: x, url: y}}", new(SourceMap), false},
		{"map invalid key", "sources: {Bad: {file: x}}", new(SourceMap), false},
		{"neither switch", "{}", new(Logging), true},
		{"one switch false", "debug: false", new(Logging), true},
		{"both switches false", "debug: false\nquiet: false", new(Logging), false},
		{"cert pair", "cert: a\nkey: b", new(TLS), true},
		{"cert absent carrier", "key: b", new(TLS), false},
		{"cert neither", "{}", new(TLS), true},
		{"any dsn", "dsn: postgres://db/app", new(Connection), true},
		{"any endpoint", "host: db\nport: 5432", new(Connection), true},
		{"any both", "dsn: x\nhost: db\nport: 5432", new(Connection), true},
		{"any incomplete", "host: db", new(Connection), false},
		{"one endpoint", "host: db\nport: 5432", new(ExclusiveConnection), true},
		{"one both", "dsn: x\nhost: db\nport: 5432", new(ExclusiveConnection), false},
		{"one partial other group", "dsn: x\nhost: db", new(ExclusiveConnection), true},
		{"prod", "mode: prod\ntls: {cert: a, key: b}", new(Production), true},
		{"prod missing TLS", "mode: prod", new(Production), false},
		{"prod null TLS", "mode: prod\ntls: null", new(Production), false},
		{"prod forbids false debug", "mode: prod\ntls: {cert: a, key: b}\ndebug: false", new(Production), false},
		{"dev", "mode: dev\ndebug: true", new(Production), true},
		{"two allowed", "debug: true\ntrace: true", new(Diagnostics), true},
		{"three forbidden", "debug: true\ntrace: true\ndump: true", new(Diagnostics), false},
		{"inline token", "name: deploy\ntoken: secret", new(Job), true},
		{"inline user", "name: deploy\nuser: agent\npassword: secret", new(Job), true},
		{"inline missing dependency", "name: deploy\nuser: agent", new(Job), false},
		{"inline conflict", "name: deploy\ntoken: x\nuser: y\npassword: z", new(Job), false},
		{"independent groups", "file: x\ntoken: y", new(TwoGroups), true},
		{"second group missing", "file: x", new(TwoGroups), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Unmarshal([]byte(tc.input+"\n"), tc.out)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}

func TestAdditionalExamples(t *testing.T) {
	cases := []struct {
		name, text string
		dst        any
		valid      bool
	}{
		{"strict dsn", "dsn: x", new(StrictConnection), true},
		{"strict endpoint", "host: db\nport: 5432", new(StrictConnection), true},
		{"strict partial mixing", "dsn: x\nhost: db", new(StrictConnection), false},
		{"outer TLS pair", "tls: {cert: a,key: b}", new(UseSiteTLS), true},
		{"outer TLS dependency", "tls: {key: b}", new(UseSiteTLS), false},
		{"strict prod", "mode: prod\ntls: {cert: a,key: b}", new(StrictProduction), true},
		{"strict empty TLS", "mode: prod\ntls: {}", new(StrictProduction), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Unmarshal([]byte(tc.text), tc.dst)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}
