package yamlvalidator

import "testing"

func TestNativeFormats(t *testing.T) {
	cases := []struct{ name, valid, invalid string }{
		{"hostname", "api.example.org", "bad host"},
		{"email", "user@example.org", "not-an-email"},
		{"idn-hostname", "bücher.example", "bad host"},
		{"idn-email", "user@bücher.example", "not-an-email"},
		{"ipv4", "192.0.2.1", "999.0.0.1"},
		{"ipv6", "2001:db8::1", "192.0.2.1"},
		{"uri", "https://example.org/a", "/relative"},
		{"uri-reference", "/relative", "%xx"},
		{"uri-template", "https://example.org/{name}", "https://example.org/{"},
		{"uuid", "123e4567-e89b-12d3-a456-426614174000", "1234"},
		{"date-time", "2026-09-27T09:00:00Z", "2026-99-99"},
		{"date", "2026-09-27", "2026-99-99"},
		{"time", "09:00:00Z", "25:00:00Z"},
		{"duration", "P1DT2H", "P-1D"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			format, err := nativeFormat(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if err := format.validate(tc.valid); err != nil {
				t.Fatalf("valid %q: %v", tc.valid, err)
			}
			if err := format.validate(tc.invalid); err == nil {
				t.Fatalf("invalid %q accepted", tc.invalid)
			}
		})
	}
	if _, err := nativeFormat("not-a-format"); err == nil {
		t.Fatal("unknown format accepted")
	}
}
