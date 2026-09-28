package yamlvalidator

import "testing"

func TestGroupIdentityDoesNotCollideOnYAMLKeyContents(t *testing.T) {
	s := &FieldSchema{exactlyGroups: [][]string{{"a\x00b", "c"}, {"a", "b\x00c"}}}
	deduplicateHighLevelGroups(s)
	if len(s.exactlyGroups) != 2 {
		t.Fatal("distinct groups conflated by separator in YAML keys")
	}
}

func TestOneOfAlternativeMultiplicityIsNotRewritten(t *testing.T) {
	s := &FieldSchema{oneClauses: [][][]string{{{"a"}, {"a"}}, {{"a"}}}}
	deduplicateHighLevelGroups(s)
	if len(s.oneClauses) != 2 || len(s.oneClauses[0]) != 2 {
		t.Fatal("deduplication changed exactly-one alternative match count")
	}
}
