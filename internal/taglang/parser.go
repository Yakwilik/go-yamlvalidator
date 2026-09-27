package taglang

import (
	"fmt"
	"strings"
)

// Value retains the spelling of atoms so numeric rules can parse exact bounds.
type Value struct {
	Kind  byte // a atom, q quoted, l list, o object
	Text  string
	List  []Value
	Rules []Rule
}

const (
	Atom   byte = 'a'
	String byte = 'q'
	List   byte = 'l'
	Object byte = 'o'
)

type Rule struct {
	Key      string
	Value    Value
	HasValue bool
	Offset   int
}

type OffsetError struct {
	Offset int
	Reason string
}

func (e *OffsetError) Error() string {
	return fmt.Sprintf("validation tag byte %d: %s", e.Offset, e.Reason)
}

type parser struct {
	s     string
	i     int
	depth int
}

func Parse(s string) ([]Rule, error) {
	if strings.TrimSpace(s) == "" {
		return nil, &OffsetError{Offset: 0, Reason: "empty validation tag"}
	}
	if len(s) > 16<<10 {
		return nil, &OffsetError{Offset: 16 << 10, Reason: "validation tag exceeds 16 KiB"}
	}
	p := &parser{s: s}
	r, err := p.rules(0)
	if err != nil {
		return nil, err
	}
	p.space()
	if p.i != len(p.s) {
		return nil, p.err("trailing input")
	}
	return r, nil
}

func (p *parser) err(reason string) error {
	return &OffsetError{Offset: p.i, Reason: reason}
}
func (p *parser) space() {
	for p.i < len(p.s) && tagSpace(p.s[p.i]) {
		p.i++
	}
}
func tagSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }
func (p *parser) rules(end byte) ([]Rule, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 64 {
		return nil, p.err("tag nesting exceeds 64")
	}
	p.space()
	if end != 0 && p.peek() == end {
		p.i++
		return nil, nil
	}
	if end == 0 && p.i == len(p.s) {
		return nil, nil
	}
	var out []Rule
	for {
		p.space()
		offset := p.i
		var key string
		if p.peek() == '\'' || p.peek() == '"' {
			v, err := p.quoted()
			if err != nil {
				return nil, err
			}
			key = v.Text
		} else {
			start := p.i
			for p.i < len(p.s) && (p.s[p.i] == '_' || p.s[p.i] == '-' || p.s[p.i] >= 'a' && p.s[p.i] <= 'z' || p.s[p.i] >= 'A' && p.s[p.i] <= 'Z' || p.s[p.i] >= '0' && p.s[p.i] <= '9') {
				p.i++
			}
			key = p.s[start:p.i]
			if key == "" {
				return nil, p.err("expected rule key")
			}
		}
		p.space()
		rule := Rule{Key: key, Offset: offset}
		if p.peek() == '=' {
			p.i++
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			rule.Value = v
			rule.HasValue = true
		}
		out = append(out, rule)
		p.space()
		if p.i == len(p.s) {
			if end != 0 {
				return nil, p.err("unterminated object")
			}
			return out, nil
		}
		if end != 0 && p.peek() == end {
			p.i++
			return out, nil
		}
		if p.peek() != ',' {
			return nil, p.err("expected comma or closing brace")
		}
		p.i++
		p.space()
		if p.i == len(p.s) || end != 0 && p.peek() == end {
			return nil, p.err("trailing comma")
		}
	}
}

func (p *parser) peek() byte {
	if p.i >= len(p.s) {
		return 0
	}
	return p.s[p.i]
}
func (p *parser) value() (Value, error) {
	p.space()
	switch p.peek() {
	case '\'', '"':
		return p.quoted()
	case '{':
		p.i++
		r, err := p.rules('}')
		return Value{Kind: 'o', Rules: r}, err
	case '[':
		p.i++
		p.depth++
		defer func() { p.depth-- }()
		if p.depth > 64 {
			return Value{}, p.err("tag nesting exceeds 64")
		}
		p.space()
		var list []Value
		if p.peek() == ']' {
			p.i++
			return Value{Kind: 'l', List: list}, nil
		}
		for {
			v, err := p.value()
			if err != nil {
				return Value{}, err
			}
			list = append(list, v)
			p.space()
			if p.peek() == ']' {
				p.i++
				return Value{Kind: 'l', List: list}, nil
			}
			if p.peek() != ',' {
				return Value{}, p.err("expected comma or closing bracket")
			}
			p.i++
			p.space()
			if p.peek() == ']' || p.peek() == 0 {
				return Value{}, p.err("trailing comma")
			}
		}
	default:
		start := p.i
		for p.i < len(p.s) && !strings.ContainsRune(",=[]{}", rune(p.s[p.i])) && !tagSpace(p.s[p.i]) {
			p.i++
		}
		if start == p.i {
			return Value{}, p.err("expected value")
		}
		return Value{Kind: 'a', Text: p.s[start:p.i]}, nil
	}
}

func (p *parser) quoted() (Value, error) {
	quote := p.peek()
	p.i++
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		p.i++
		if c == quote {
			return Value{Kind: 'q', Text: b.String()}, nil
		}
		if c == '\\' {
			if p.i == len(p.s) {
				return Value{}, p.err("unterminated escape")
			}
			e := p.s[p.i]
			p.i++
			switch e {
			case '\\', '\'', '"':
				b.WriteByte(e)
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				return Value{}, p.err("unknown escape")
			}
			continue
		}
		b.WriteByte(c)
	}
	return Value{}, p.err("unterminated quoted string")
}
