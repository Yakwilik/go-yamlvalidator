package yamlvalidator

import (
	"reflect"

	"github.com/Yakwilik/go-yamlvalidator/internal/genspec"
	"github.com/Yakwilik/go-yamlvalidator/internal/yamlcodec"
)

// FieldNaming chooses the default YAML name for an exported Go field.
type FieldNaming = yamlcodec.FieldNaming

const (
	// FieldNamingYAML uses yaml.v3's lowercase default field names.
	FieldNamingYAML = yamlcodec.FieldNamingYAML
	// FieldNamingGo preserves the Go name when a YAML tag has no explicit name.
	FieldNamingGo = yamlcodec.FieldNamingGo
)

type generatedNamingSchemaProvider interface {
	YAMLValidatorSchemaSpecWithNaming(FieldNaming) (genspec.Graph, []reflect.Type)
}

func generatedSchemaForNaming(provider generatedSchemaProvider, value any, naming FieldNaming) (genspec.Graph, []reflect.Type, bool) {
	if naming == FieldNamingGo {
		if named, ok := value.(generatedNamingSchemaProvider); ok {
			graph, types := named.YAMLValidatorSchemaSpecWithNaming(naming)
			return graph, types, true
		}
		// Legacy generated schemas contain only the standard YAML names.
		return genspec.Graph{}, nil, false
	}
	graph, types := provider.YAMLValidatorSchemaSpec()
	return graph, types, true
}
