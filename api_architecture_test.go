package yamlvalidator_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRootHasNoGeneratedImplementationAPI(t *testing.T) {
	forbidden := map[string]bool{"MarshalGenerated": true, "UnmarshalGenerated": true, "LowerGeneratedSchema": true, "BuildGeneratedSchema": true, "SourceTypePlan": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range file.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(name, "/genruntime") {
				t.Fatalf("root imports generated runtime: %s", path)
			}
		}
		check := func(n string) {
			if forbidden[n] || strings.HasPrefix(n, "Generated") {
				t.Errorf("implementation symbol leaked in root: %s", n)
			}
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					check(d.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						check(s.Name.Name)
					case *ast.ValueSpec:
						for _, n := range s.Names {
							check(n.Name)
						}
					}
				}
			}
		}
	}
}

func TestRootDoesNotTransitivelyImportGeneratedRuntime(t *testing.T) {
	cmd := exec.Command("go", "list", "-mod=readonly", "-deps", ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	for _, path := range strings.Fields(string(out)) {
		if strings.Contains(path, "/genruntime") {
			t.Fatalf("root transitively imports %s", path)
		}
	}
}

func TestGeneratedSpecDoesNotExposeCompilerInput(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "genruntime/spec/spec.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, s := range gen.Specs {
			typ, ok := s.(*ast.TypeSpec)
			if !ok {
				continue
			}
			n := typ.Name.Name
			if strings.HasPrefix(n, "Source") || n == "Rule" || n == "RuleValue" {
				t.Fatalf("source-time compiler input exposed in runtime ABI: %s", n)
			}
		}
	}
}
