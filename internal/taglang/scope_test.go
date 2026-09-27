package taglang

import (
	"errors"
	"testing"
)

func TestNormalizeScopesAndOffsets(t *testing.T) {
	rules, err := Parse("nonempty,exactlyOneOf=[a,b],exactlyOneOfKeys=[x,y]")
	if err != nil {
		t.Fatal(err)
	}
	value, parent, err := Normalize(rules, FieldScope)
	if err != nil {
		t.Fatal(err)
	}
	if len(value) != 2 || len(parent) != 1 || value[1].Key != "exactlyOneOf" || parent[0].Key != "exactlyOneOf" || parent[0].Offset != len("nonempty,") {
		t.Fatalf("value=%#v parent=%#v", value, parent)
	}
}

func TestNormalizeRejectsUnboundParentDirective(t *testing.T) {
	rules, err := Parse("minLength=1,exactlyOneOf=[a,b]")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Normalize(rules, ValueScope)
	var offset *OffsetError
	if !errors.As(err, &offset) || offset.Offset != len("minLength=1,") {
		t.Fatalf("offset error: %v", err)
	}
}
