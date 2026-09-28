// Package schemacompiler is the generation-time entry to native tag lowering.
// It is not part of the application or generated runtime API.
package schemacompiler

import (
	engine "github.com/Yakwilik/go-yamlvalidator/internal/engine"
	"github.com/Yakwilik/go-yamlvalidator/internal/genspec"
)

// Lower converts source-time declarations to data-only normalized schema nodes.
func Lower(graph genspec.SourceGraph, root int) (genspec.Graph, error) {
	return engine.CompileSourceSchema(graph, root)
}
