package genspec

import (
	"encoding/json"
	"reflect"
)

type NodeType int
type UnknownKeyPolicy int

type ConditionalRule struct {
	ConditionField string
	ConditionValue string
	ThenRequired   []string
	ThenForbidden  []string
}

type ValidatorSpec struct {
	Kind       string
	Name       string
	Text       string
	Number     int
	Minimum    bool
	Properties bool
	Names      []string
	Args       map[string]any
	Factory    bool
}

type Node struct {
	Type                 NodeType
	AllowedTypes         []NodeType
	Required             bool
	Nullable             bool
	Deprecated           string
	Description          string
	Default              any
	DefaultPresent       bool
	SourceIndex          int
	FieldRules           bool
	Ref                  string
	AllowedKeys          map[string]int
	HasAllowedKeys       bool
	AdditionalProperties *int
	ValueSchema          *int
	InlineCapture        *int
	ItemSchema           *int
	ExtraSchemas         []int
	OneOfSchemas         []int
	AnyOfSchemas         []int
	UnknownKeyPolicy     UnknownKeyPolicy
	MinItems             *int
	MaxItems             *int
	Validators           []ValidatorSpec
	KeyValidators        []ValidatorSpec
	AnyOf                [][]string
	ExactlyOneOf         []string
	MutuallyExclusive    []string
	Conditions           []ConditionalRule
	OneOfRequired        [][]string
	ForbiddenTogether    [][]string
	DependentRequired    map[string][]string
	ExactlyGroups        [][]string
	MutuallyGroups       [][]string
	AnyClauses           [][][]string
	OneClauses           [][][]string
	RequiredNames        []string
}

type Graph struct {
	Root  int
	Nodes []Node
}

type RuleValue struct {
	Kind  byte
	Text  string
	List  []RuleValue
	Rules []Rule
}

type Rule struct {
	Key      string
	Value    RuleValue
	HasValue bool
	Offset   int
}

type SourceField struct {
	Key       string
	Node      int
	Inline    bool
	Rules     []Rule
	FieldName string
}

type SourceNode struct {
	Type       NodeType
	Nullable   bool
	GoType     reflect.Type
	NumberKind reflect.Kind
	NumberBits int
	ArrayLen   int
	AliasOf    int
	Item       int
	Value      int
	Fields     []SourceField
	Opaque     bool
	ByteSlice  bool
}

type SourceGraph struct {
	Nodes []SourceNode
	Root  int
}

func Int(value int) *int             { return &value }
func Number(text string) json.Number { return json.Number(text) }
