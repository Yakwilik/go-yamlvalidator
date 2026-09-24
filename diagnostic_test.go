package yamlvalidator_test

import (
	"reflect"
	"testing"

	. "github.com/Yakwilik/go-yamlvalidator"
)

func TestPathTokensRoundTrip(t *testing.T) {
	path := `doc[2].items[0]["a.b"]["0"]`
	tokens, err := ParsePathTokens(path)
	if err != nil {
		t.Fatalf("parse path: %v", err)
	}
	want := []PathToken{
		{Kind: PathTokenDocument, Index: 2},
		{Kind: PathTokenProperty, Name: "items"},
		{Kind: PathTokenIndex, Index: 0},
		{Kind: PathTokenProperty, Name: "a.b"},
		{Kind: PathTokenProperty, Name: "0"},
	}
	if !reflect.DeepEqual(tokens, want) {
		t.Fatalf("tokens mismatch: got %#v want %#v", tokens, want)
	}
	if got := FormatPathTokens(tokens); got != path {
		t.Fatalf("round trip path: got %q want %q", got, path)
	}
}

func TestValidationErrorHasStructuredPathAndDetails(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"items": {
				Type: TypeSequence,
				ItemSchema: &FieldSchema{
					Type: TypeMap,
					AllowedKeys: map[string]*FieldSchema{
						"a.b": {Type: TypeString, Required: true},
					},
					UnknownKeyPolicy: UnknownKeyError,
				},
			},
		},
	}

	result := NewValidator(schema).ValidateBytes([]byte(`items:
  - other: value
`))
	var required *ValidationError
	for _, diagnostic := range result.Collector.Errors() {
		if diagnostic.Code == "required" {
			copy := diagnostic
			required = &copy
			break
		}
	}

	if required == nil {
		t.Fatalf("required diagnostic missing: %v", result.Collector.Errors())
	}
	if required.Path != `items[0]["a.b"]` {
		t.Fatalf("unexpected path %q", required.Path)
	}
	wantTokens := []PathToken{
		{Kind: PathTokenProperty, Name: "items"},
		{Kind: PathTokenIndex, Index: 0},
		{Kind: PathTokenProperty, Name: "a.b"},
	}
	if !reflect.DeepEqual(required.PathTokens, wantTokens) {
		t.Fatalf("unexpected tokens: %#v", required.PathTokens)
	}
	details, ok := required.Details.(RequiredDetails)
	if !ok || details.Field != "a.b" {
		t.Fatalf("unexpected details: %#v", required.Details)
	}
}

func TestUnknownKeyStructuredDetails(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"with_imports": {Type: TypeBool},
		},
		UnknownKeyPolicy: UnknownKeyError,
	}

	result := NewValidator(schema).ValidateBytes([]byte(`with_import: true`))
	if len(result.Collector.Errors()) != 1 {
		t.Fatalf("expected one error, got %v", result.Collector.Errors())
	}
	details, ok := result.Collector.Errors()[0].Details.(UnknownKeyDetails)
	if !ok {
		t.Fatalf("unexpected details type: %T", result.Collector.Errors()[0].Details)
	}
	if details.Key != "with_import" || details.Suggestion != "with_imports" {
		t.Fatalf("unexpected unknown-key details: %+v", details)
	}
}

func TestErrorCollectorReturnsDefensiveCopies(t *testing.T) {
	collector := NewErrorCollector()
	collector.Add(ValidationError{
		Level: LevelError,
		Code:  "type_mismatch",
		Path:  "items[0]",
		Details: TypeMismatchDetails{
			Expected: []string{"string"},
			Actual:   "integer",
		},
	})

	errors := collector.Errors()
	errors[0].Code = "changed"
	errors[0].PathTokens[0].Name = "changed"
	details := errors[0].Details.(TypeMismatchDetails)
	details.Expected[0] = "changed"

	again := collector.Errors()
	if again[0].Code != "type_mismatch" {
		t.Fatalf("collector code was mutated through returned slice: %+v", again[0])
	}
	if again[0].Path != "items[0]" ||
		len(again[0].PathTokens) != 2 ||
		again[0].PathTokens[0].Name != "items" {
		t.Fatalf("collector path tokens were mutated: %+v", again[0])
	}
	gotDetails := again[0].Details.(TypeMismatchDetails)
	if gotDetails.Expected[0] != "string" {
		t.Fatalf("collector details were mutated: %+v", gotDetails)
	}
}

func TestPathTokensRoundTripEmbeddedDocumentToken(t *testing.T) {
	tokens := []PathToken{
		{Kind: PathTokenProperty, Name: "outer"},
		{Kind: PathTokenDocument, Index: 3},
		{Kind: PathTokenIndex, Index: 1},
	}
	path := FormatPathTokens(tokens)
	if path != "outer[doc:3][1]" {
		t.Fatalf("unexpected formatted path %q", path)
	}
	parsed, err := ParsePathTokens(path)
	if err != nil {
		t.Fatalf("parse path: %v", err)
	}
	if !reflect.DeepEqual(parsed, tokens) {
		t.Fatalf("round trip mismatch: got %#v want %#v", parsed, tokens)
	}
}

func TestRootDocPropertyIsNotConfusedWithDocumentPrefix(t *testing.T) {
	schema := &FieldSchema{
		Type: TypeMap,
		AllowedKeys: map[string]*FieldSchema{
			"doc": {
				Type:       TypeSequence,
				ItemSchema: &FieldSchema{Type: TypeInt},
			},
		},
	}
	result := NewValidator(schema).ValidateBytes([]byte("doc: [nope]"))
	if len(result.Collector.Errors()) != 1 {
		t.Fatalf("expected one type error, got %v", result.Collector.Errors())
	}
	diagnostic := result.Collector.Errors()[0]
	if diagnostic.Path != `["doc"][0]` {
		t.Fatalf("ambiguous doc property path: %q", diagnostic.Path)
	}
	want := []PathToken{
		{Kind: PathTokenProperty, Name: "doc"},
		{Kind: PathTokenIndex, Index: 0},
	}
	if !reflect.DeepEqual(diagnostic.PathTokens, want) {
		t.Fatalf("unexpected tokens: got %#v want %#v", diagnostic.PathTokens, want)
	}
}

func TestAppendPathHelpers(t *testing.T) {
	path := AppendPropertyPath("root", "a.b")
	if path != `root["a.b"]` {
		t.Fatalf("unexpected property path: %q", path)
	}
	path = AppendPropertyPath(path, "0")
	if path != `root["a.b"]["0"]` {
		t.Fatalf("unexpected numeric property path: %q", path)
	}
	indexed, err := AppendIndexPath(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if indexed != `root["a.b"]["0"][2]` {
		t.Fatalf("unexpected index path: %q", indexed)
	}
	if _, err := AppendIndexPath("", -1); err == nil {
		t.Fatal("negative path index must fail")
	}
}
