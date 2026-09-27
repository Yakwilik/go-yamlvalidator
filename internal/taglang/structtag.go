package taglang

import (
	"fmt"
	"strconv"
)

// ParseStructTag validates the outer Go struct-tag grammar before rule parsing.
func ParseStructTag(tag string) (map[string]string, error) {
	result := map[string]string{}
	for i := 0; i < len(tag); {
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		if i == len(tag) {
			break
		}
		start := i
		for i < len(tag) && tag[i] != ':' && tag[i] != ' ' && tag[i] != '"' && tag[i] < 0x7f {
			i++
		}
		if i == start || i >= len(tag) || tag[i] != ':' || i+1 >= len(tag) || tag[i+1] != '"' {
			return nil, &OffsetError{Offset: start, Reason: "malformed reflect.StructTag"}
		}
		key := tag[start:i]
		i++
		start = i
		i++
		closed := false
		for i < len(tag) {
			if tag[i] == '\\' {
				i += 2
				continue
			}
			if tag[i] == '"' {
				i++
				closed = true
				break
			}
			i++
		}
		if !closed {
			return nil, &OffsetError{Offset: start, Reason: fmt.Sprintf("unterminated reflect.StructTag value for %s", key)}
		}
		body := tag[start+1 : i-1]
		for consumed := 0; consumed < len(body); {
			_, _, rest, err := strconv.UnquoteChar(body[consumed:], '"')
			if err != nil {
				return nil, &OffsetError{Offset: start + 1 + consumed, Reason: fmt.Sprintf("invalid reflect.StructTag value for %s: %v", key, err)}
			}
			consumed = len(body) - len(rest)
		}
		value, err := strconv.Unquote(tag[start:i])
		if err != nil {
			return nil, &OffsetError{Offset: start, Reason: fmt.Sprintf("invalid reflect.StructTag value for %s: %v", key, err)}
		}
		if _, exists := result[key]; exists {
			return nil, &OffsetError{Offset: start, Reason: fmt.Sprintf("duplicate reflect.StructTag key %s", key)}
		}
		result[key] = value
		if i < len(tag) && tag[i] != ' ' {
			return nil, &OffsetError{Offset: i, Reason: "missing space after reflect.StructTag value"}
		}
	}
	return result, nil
}
