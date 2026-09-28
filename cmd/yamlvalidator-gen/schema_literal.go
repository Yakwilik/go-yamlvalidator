package main

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// renderGeneratedLiteral writes a Go composite literal for the already
// normalized schema. The generated program needs no parser or tag lowering.
func renderGeneratedLiteral(value reflect.Value) string {
	if !value.IsValid() {
		return "nil"
	}
	t := value.Type()
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return "nil"
		}
		return renderGeneratedLiteral(value.Elem())
	case reflect.Pointer:
		if value.IsNil() {
			return "nil"
		}
		if t.Elem().Kind() == reflect.Int {
			return fmt.Sprintf("yamlvalidator.GeneratedInt(%d)", value.Elem().Int())
		}
		panic("unexpected pointer in generated schema")
	case reflect.Struct:
		var b strings.Builder
		b.WriteString(generatedLiteralType(t))
		b.WriteByte('{')
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() || value.Field(i).IsZero() {
				continue
			}
			fmt.Fprintf(&b, "%s:%s,\n", field.Name, renderGeneratedLiteral(value.Field(i)))
		}
		b.WriteByte('}')
		return b.String()
	case reflect.Slice, reflect.Array:
		var b strings.Builder
		b.WriteString(generatedLiteralType(t))
		b.WriteByte('{')
		for i := 0; i < value.Len(); i++ {
			b.WriteString(renderGeneratedLiteral(value.Index(i)))
			b.WriteString(",\n")
		}
		b.WriteByte('}')
		return b.String()
	case reflect.Map:
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		var b strings.Builder
		b.WriteString(generatedLiteralType(t))
		b.WriteByte('{')
		for _, key := range keys {
			fmt.Fprintf(&b, "%s:%s,\n", renderGeneratedLiteral(key), renderGeneratedLiteral(value.MapIndex(key)))
		}
		b.WriteByte('}')
		return b.String()
	case reflect.String:
		text := strconv.Quote(value.String())
		if t.PkgPath() == "encoding/json" && t.Name() == "Number" {
			return "yamlvalidator.GeneratedNumber(" + text + ")"
		}
		if t.Name() != "" && t.PkgPath() != "" {
			return generatedLiteralType(t) + "(" + text + ")"
		}
		return text
	case reflect.Bool:
		if value.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		text := strconv.FormatInt(value.Int(), 10)
		if t.Name() != "" && t.PkgPath() != "" {
			return generatedLiteralType(t) + "(" + text + ")"
		}
		return text
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		text := strconv.FormatUint(value.Uint(), 10)
		if t.Name() != "" && t.PkgPath() != "" {
			return generatedLiteralType(t) + "(" + text + ")"
		}
		return text
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(value.Float(), 'g', -1, t.Bits())
	}
	panic(fmt.Sprintf("unsupported normalized schema literal %s", t))
}

func generatedLiteralType(t reflect.Type) string {
	if t.Name() != "" {
		switch t.PkgPath() {
		case "":
			return t.Name()
		case "github.com/Yakwilik/go-yamlvalidator":
			return "yamlvalidator." + t.Name()
		case "encoding/json":
			return "json." + t.Name()
		}
		panic("unexpected generated schema type " + t.String())
	}
	switch t.Kind() {
	case reflect.Interface:
		return "any"
	case reflect.Slice:
		return "[]" + generatedLiteralType(t.Elem())
	case reflect.Array:
		return fmt.Sprintf("[%d]%s", t.Len(), generatedLiteralType(t.Elem()))
	case reflect.Map:
		return "map[" + generatedLiteralType(t.Key()) + "]" + generatedLiteralType(t.Elem())
	case reflect.Pointer:
		return "*" + generatedLiteralType(t.Elem())
	}
	return t.String()
}
