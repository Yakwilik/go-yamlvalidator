package yamlvalidator

import (
	"fmt"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// JSONSchemaContentEncoding registers a custom JSON Schema contentEncoding.
type JSONSchemaContentEncoding struct {
	Name   string
	Decode func(string) ([]byte, error)
}

// JSONSchemaContentMediaType registers a custom JSON Schema contentMediaType.
// UnmarshalJSON is optional and should be set only when the media type contains
// a JSON-compatible data model that contentSchema may validate.
type JSONSchemaContentMediaType struct {
	Name          string
	Validate      func([]byte) error
	UnmarshalJSON func([]byte) (any, error)
}

func registerJSONSchemaContentExtensions(
	compiler *jsonschema.Compiler,
	encodings []JSONSchemaContentEncoding,
	mediaTypes []JSONSchemaContentMediaType,
) error {
	seenEncodings := make(map[string]bool, len(encodings))
	for i, encoding := range encodings {
		if encoding.Name == "" {
			return fmt.Errorf("JSON Schema content encoding[%d]: name must not be empty", i)
		}
		if encoding.Decode == nil {
			return fmt.Errorf(
				"JSON Schema content encoding %q: decode function is nil",
				encoding.Name,
			)
		}
		if seenEncodings[encoding.Name] {
			return fmt.Errorf(
				"JSON Schema content encoding %q registered more than once",
				encoding.Name,
			)
		}
		seenEncodings[encoding.Name] = true
		compiler.RegisterContentEncoding(&jsonschema.Decoder{
			Name:   encoding.Name,
			Decode: encoding.Decode,
		})
	}

	seenMediaTypes := make(map[string]bool, len(mediaTypes))
	for i, mediaType := range mediaTypes {
		if mediaType.Name == "" {
			return fmt.Errorf("JSON Schema content media type[%d]: name must not be empty", i)
		}
		if mediaType.Validate == nil {
			return fmt.Errorf(
				"JSON Schema content media type %q: validate function is nil",
				mediaType.Name,
			)
		}
		if seenMediaTypes[mediaType.Name] {
			return fmt.Errorf(
				"JSON Schema content media type %q registered more than once",
				mediaType.Name,
			)
		}
		seenMediaTypes[mediaType.Name] = true
		compiler.RegisterContentMediaType(&jsonschema.MediaType{
			Name:          mediaType.Name,
			Validate:      mediaType.Validate,
			UnmarshalJSON: mediaType.UnmarshalJSON,
		})
	}
	return nil
}
