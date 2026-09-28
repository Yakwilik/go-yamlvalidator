package yamlvalidator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestHighLevelHasNoJSONSchemaEngineRoute(t *testing.T) {
	paths, err := filepath.Glob("highlevel*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				if strings.HasPrefix(fn.Name, "CompileJSONSchema") {
					t.Errorf("high-level JSON Schema call in %s", path)
				}
			case *ast.SelectorExpr:
				if strings.HasPrefix(fn.Sel.Name, "CompileJSONSchema") {
					t.Errorf("high-level JSON Schema call in %s", path)
				}
			}
			return true
		})
	}
	typ := reflect.TypeFor[RegistryConfig]()
	for _, name := range []string{"JSONSchemas", "JSONOptions"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Errorf("obsolete RegistryConfig field %s", name)
		}
	}
}
