package yamlvalidator

import "testing"

func TestHighLevelTagGrammar(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"required,min=1,when={field=mode,eq='a,b',require=[x,y]},when={field=mode,eq=prod,forbid=[debug]}", 4},
		{"pattern='^a\\'b$',properties={name={type=string,required},port={type=int,min=1}}", 2},
		{"enum=[dev,prod],default=[1,{a='x,y'}]", 2},
	}
	for _, tt := range tests {
		rules, err := parseTagRules(tt.input)
		if err != nil || len(rules) != tt.want {
			t.Errorf("parseTagRules(%q) = %d rules, %v; want %d", tt.input, len(rules), err, tt.want)
		}
	}
}

func TestHighLevelTagGrammarRejectsMalformed(t *testing.T) {
	for _, input := range []string{
		"", "min=", "x=[a,]", "x={a=1", "x='bad\\q'", "x=1 garbage", "x=a,,y=b",
		"x=[a b]", "x={a=1,}", "=x", "x=[", "x='unterminated", "x=[a,{b=2}]trailing",
	} {
		if _, err := parseTagRules(input); err == nil {
			t.Errorf("parseTagRules(%q) accepted malformed input", input)
		}
	}
}
