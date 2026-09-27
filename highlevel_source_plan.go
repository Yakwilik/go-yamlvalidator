package yamlvalidator

import "reflect"

// SourceTypePlan is the generator's declaration of a selected Go type. The
// native schema compiler lowers these fields with the same rules as reflection
// types. Types without this plan continue through the runtime reflection path.
type SourceTypePlan struct {
	Type   reflect.Type
	Fields []reflect.StructField
}
