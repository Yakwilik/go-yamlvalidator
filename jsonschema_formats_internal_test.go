package yamlvalidator

import "testing"

func TestURIFormatHelpers(t *testing.T) {
	valid := []struct {
		value    string
		absolute bool
	}{
		{"https://example.com/a?b=c#d", true},
		{"urn:example:test", true},
		{"../relative/path?x=1", false},
		{"//user@example.com/path", false},
		{"http://[::1]/", true},
	}
	for _, tc := range valid {
		if err := validateRFC3986Reference(tc.value, tc.absolute); err != nil {
			t.Errorf("valid URI %q rejected: %v", tc.value, err)
		}
	}
}

func TestURIFormatHelpersRejectInvalidReferences(t *testing.T) {
	invalid := []struct {
		value    string
		absolute bool
	}{
		{"", true},
		{"relative/path", true},
		{"1http://example.com", true},
		{"http://example.com/%zz", true},
		{"http://example.com/a b", true},
		{"http://пример.рф", true},
		{"http://a@b@example.com", true},
		{"http://example.com/path[0]", true},
		{"relative[0]", false},
	}
	for _, tc := range invalid {
		if err := validateRFC3986Reference(tc.value, tc.absolute); err == nil {
			t.Errorf("invalid URI %q accepted", tc.value)
		}
	}
}

func TestURIWrapperFormats(t *testing.T) {
	if err := validateJSONSchemaURI(123); err != nil {
		t.Fatalf("non-string URI should be ignored: %v", err)
	}
	if err := validateJSONSchemaURI("https://example.com"); err != nil {
		t.Fatal(err)
	}
	if err := validateJSONSchemaURI("relative"); err == nil {
		t.Fatal("relative URI accepted as absolute")
	}
	if err := validateJSONSchemaURIReference("../x"); err != nil {
		t.Fatal(err)
	}
	if err := validateJSONSchemaURITemplate("https://example.com/{id}"); err != nil {
		t.Fatal(err)
	}
	if err := validateJSONSchemaURITemplate("{"); err == nil {
		t.Fatal("invalid URI template accepted")
	}
}

func TestFormatPrimitiveHelpers(t *testing.T) {
	for _, scheme := range []string{"http", "git+ssh", "a.b-c"} {
		if !validURIScheme(scheme) {
			t.Errorf("valid scheme %q rejected", scheme)
		}
	}
	for _, scheme := range []string{"", "1http", "bad_scheme"} {
		if validURIScheme(scheme) {
			t.Errorf("invalid scheme %q accepted", scheme)
		}
	}
	if !isASCIIAlpha('A') || !isASCIIAlpha('z') || isASCIIAlpha('1') {
		t.Fatal("ASCII alpha helper mismatch")
	}
	for _, ch := range []byte{'0', '9', 'a', 'F'} {
		if !isHexByte(ch) {
			t.Fatalf("hex byte %q rejected", ch)
		}
	}
	if isHexByte('g') {
		t.Fatal("non-hex byte accepted")
	}
}

func TestDurationAndAddressLiteralHelpers(t *testing.T) {
	for _, value := range []string{"P1Y", "P2W", "PT3H", "P1DT2H"} {
		if err := validateJSONSchemaDuration(value); err != nil {
			t.Errorf("valid duration %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"P", "P1Y2D", "PT", "P1DT2Q", "P1T2H"} {
		if err := validateJSONSchemaDuration(value); err == nil {
			t.Errorf("invalid duration %q accepted", value)
		}
	}
	for _, domain := range []string{"[127.0.0.1]", "[IPv6:::1]"} {
		if err := validateJSONSchemaAddressLiteral(domain); err != nil {
			t.Errorf("valid address literal %q rejected: %v", domain, err)
		}
	}
	for _, domain := range []string{"127.0.0.1", "[999.1.1.1]", "[IPv6:not-ip]"} {
		if err := validateJSONSchemaAddressLiteral(domain); err == nil {
			t.Errorf("invalid address literal %q accepted", domain)
		}
	}
}

func TestEmailAndIDNHelpers(t *testing.T) {
	for _, value := range []string{"user@example.com", "\"a b\"@example.com", "a@[127.0.0.1]", "a@[IPv6:::1]"} {
		if err := validateJSONSchemaEmail(value); err != nil {
			t.Errorf("valid email %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"@example.com", "a..b@example.com", "a@[999.0.0.1]", "a@-bad.example"} {
		if err := validateJSONSchemaEmail(value); err == nil {
			t.Errorf("invalid email %q accepted", value)
		}
	}
	if !isJSONSchemaEmailAText('a') || !isJSONSchemaEmailAText('+') || isJSONSchemaEmailAText('(') {
		t.Fatal("email atext helper mismatch")
	}
	if err := validateJSONSchemaIDNALabelContext("l·l"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"a·l", "\u00a1", "\u0375a", "\u05f3a", "a\u30fbb"} {
		if err := validateJSONSchemaIDNALabelContext(value); err == nil {
			t.Errorf("invalid IDNA context %q accepted", value)
		}
	}
}

func TestScalarInferenceHelpers(t *testing.T) {
	for _, value := range []string{"123", "-42", "+7", "0xff", "0o17", "0b101"} {
		if !(&Validator{}).looksLikeInt(value) {
			t.Errorf("integer %q not recognized", value)
		}
	}
	for _, value := range []string{"", "+", "1.5", "abc"} {
		if (&Validator{}).looksLikeInt(value) {
			t.Errorf("non-integer %q recognized", value)
		}
	}
	for _, value := range []string{"1.5", "1e3", ".inf", "-.inf", ".nan"} {
		if !(&Validator{}).looksLikeFloat(value) {
			t.Errorf("float %q not recognized", value)
		}
	}
	for _, value := range []string{"1", "abc"} {
		if (&Validator{}).looksLikeFloat(value) {
			t.Errorf("non-float %q recognized", value)
		}
	}
}

func TestYAML11BooleanHelper(t *testing.T) {
	for _, value := range []string{"y", "YES", "true", "On"} {
		got, ok := yaml11Boolean(value)
		if !ok || !got {
			t.Errorf("true boolean %q not recognized", value)
		}
	}
	for _, value := range []string{"n", "NO", "false", "Off"} {
		got, ok := yaml11Boolean(value)
		if !ok || got {
			t.Errorf("false boolean %q not recognized", value)
		}
	}
	if _, ok := yaml11Boolean("maybe"); ok {
		t.Fatal("non-boolean recognized")
	}
}

func TestSubschemaPositionConstructors(t *testing.T) {
	if JSONSchemaSubschemaProperty("x").property != "x" {
		t.Fatal("property selector lost name")
	}
	if JSONSchemaSubschemaItem(2).index != 2 {
		t.Fatal("item selector lost index")
	}
	if JSONSchemaSubschemaAllProperties().kind == 0 || JSONSchemaSubschemaAllItems().kind == 0 {
		t.Fatal("wildcard selector has zero kind")
	}
}
