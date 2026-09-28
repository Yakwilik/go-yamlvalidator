package spec

import (
	"encoding/json"
	"github.com/Yakwilik/go-yamlvalidator/internal/genspec"
)

// These aliases are the data-only contract emitted by yamlvalidator-gen.
// Generation-time parsing/lowering types are internal and are not re-exported.
type NodeType = genspec.NodeType
type UnknownKeyPolicy = genspec.UnknownKeyPolicy
type ConditionalRule = genspec.ConditionalRule
type ValidatorSpec = genspec.ValidatorSpec
type Node = genspec.Node
type Graph = genspec.Graph

func Int(value int) *int             { return genspec.Int(value) }
func Number(text string) json.Number { return genspec.Number(text) }
