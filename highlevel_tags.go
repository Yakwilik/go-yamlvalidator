package yamlvalidator

import "github.com/Yakwilik/go-yamlvalidator/internal/taglang"

type tagValue struct {
	kind  byte
	text  string
	list  []tagValue
	rules []tagRule
}

const (
	tagAtom   = taglang.Atom
	tagString = taglang.String
	tagList   = taglang.List
	tagObject = taglang.Object
)

type tagRule struct {
	key      string
	value    tagValue
	hasValue bool
	offset   int
}

type tagOffsetError = taglang.OffsetError

func parseTagRules(s string) ([]tagRule, error) {
	parsed, err := taglang.Parse(s)
	if err != nil {
		return nil, err
	}
	return fromSharedRules(parsed), nil
}

func fromSharedRules(rules []taglang.Rule) []tagRule {
	out := make([]tagRule, len(rules))
	for i, rule := range rules {
		out[i] = tagRule{key: rule.Key, value: fromSharedValue(rule.Value), hasValue: rule.HasValue, offset: rule.Offset}
	}
	return out
}

func fromSharedValue(value taglang.Value) tagValue {
	out := tagValue{kind: value.Kind, text: value.Text, rules: fromSharedRules(value.Rules)}
	for _, item := range value.List {
		out.list = append(out.list, fromSharedValue(item))
	}
	return out
}

func toSharedRules(rules []tagRule) []taglang.Rule {
	out := make([]taglang.Rule, len(rules))
	for i, rule := range rules {
		out[i] = taglang.Rule{Key: rule.key, Value: toSharedValue(rule.value), HasValue: rule.hasValue, Offset: rule.offset}
	}
	return out
}

func toSharedValue(value tagValue) taglang.Value {
	out := taglang.Value{Kind: value.kind, Text: value.text, Rules: toSharedRules(value.rules)}
	for _, item := range value.list {
		out.List = append(out.List, toSharedValue(item))
	}
	return out
}
