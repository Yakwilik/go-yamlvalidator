package recipes_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
)

func TestJSONSchemaContentHandlers(t *testing.T) {
	options := v.JSONSchemaCompileOptions{
		AssertContent:    true,
		ContentEncodings: []v.JSONSchemaContentEncoding{{Name: "hex", Decode: hex.DecodeString}},
		ContentMediaTypes: []v.JSONSchemaContentMediaType{{
			Name: "application/x-example-json",
			Validate: func(data []byte) error {
				if !json.Valid(data) {
					return fmt.Errorf("invalid JSON content")
				}
				return nil
			},
			UnmarshalJSON: func(data []byte) (any, error) {
				decoder := json.NewDecoder(bytes.NewReader(data))
				decoder.UseNumber()
				var value any
				err := decoder.Decode(&value)
				return value, err
			},
		}},
	}
	validator := compileJSON(t, `{"type":"string","contentEncoding":"hex","contentMediaType":"application/x-example-json","contentSchema":{"type":"object","properties":{"count":{"type":"integer","minimum":1}},"required":["count"]}}`, options)
	encode := func(s string) string { return `"` + hex.EncodeToString([]byte(s)) + `"` }
	checkInputs(t, validator, v.ValidationContext{}, []inputCase{
		{name: "valid", input: encode(`{"count":2}`), valid: true},
		{name: "encoding", input: `"not-hex"`, code: "contentEncoding"},
		{name: "media", input: encode("not JSON"), code: "contentMediaType"},
		{name: "schema", input: encode(`{"count":0}`)},
		{name: "missing", input: encode(`{}`)},
	})
}
