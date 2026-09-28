package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

var generatedRootMethods = map[string]bool{
	"MarshalYAML":                    true,
	"UnmarshalYAML":                  true,
	"YAMLValidatorEncode":            true,
	"YAMLValidatorDecode":            true,
	"YAMLValidatorDecodeWithContext": true,
	"YAMLValidatorGeneratedType":     true,
	"YAMLValidatorSchemaSpec":        true,
	"YAMLValidatorSchema":            true,
	"YAMLValidatorCheckCycles":       true,
}

func selectRootNames(namesText string, all bool, outputPath string) ([]string, error) {
	selected := map[string]bool{}
	if namesText != "" {
		for _, name := range strings.Split(namesText, ",") {
			if !token.IsIdentifier(name) || selected[name] {
				return nil, fmt.Errorf("invalid or repeated type %q", name)
			}
			selected[name] = true
		}
	}
	if all {
		names, err := discoverStructRoots(outputPath)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			selected[name] = true
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no eligible types selected")
	}
	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func discoverStructRoots(outputPath string) ([]string, error) {
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax,
		Dir:   ".",
		Tests: false,
	}
	loaded, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, err
	}
	if len(loaded) != 1 {
		return nil, fmt.Errorf("expected one package, found %d", len(loaded))
	}
	pkg := loaded[0]
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("load package: %s", pkg.Errors[0])
	}

	candidates := map[string]bool{}
	conflicts := map[string]bool{}
	for i, file := range pkg.Syntax {
		if i >= len(pkg.CompiledGoFiles) {
			break
		}
		filename := filepath.Clean(pkg.CompiledGoFiles[i])
		if filename == filepath.Clean(outputPath) {
			continue
		}
		conditional := sourceFileConditional(file, filename)
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok != token.TYPE || conditional {
					continue
				}
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || ts.Assign.IsValid() || ts.TypeParams != nil {
						continue
					}
					if _, ok := ts.Type.(*ast.StructType); ok {
						candidates[ts.Name.Name] = true
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil || !generatedRootMethods[d.Name.Name] {
					continue
				}
				if name := receiverTypeName(d.Recv.List[0].Type); name != "" {
					conflicts[name] = true
				}
			}
		}
	}
	for name := range conflicts {
		delete(candidates, name)
	}
	names := make([]string, 0, len(candidates))
	for name := range candidates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func receiverTypeName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return receiverTypeName(x.X)
	case *ast.IndexExpr:
		return receiverTypeName(x.X)
	case *ast.IndexListExpr:
		return receiverTypeName(x.X)
	}
	return ""
}

func sourceFileConditional(file *ast.File, filename string) bool {
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			continue
		}
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:build ") || strings.HasPrefix(comment.Text, "// +build ") {
				return true
			}
		}
	}
	goos := os.Getenv("GOOS")
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := os.Getenv("GOARCH")
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	parts := strings.Split(strings.TrimSuffix(filepath.Base(filename), ".go"), "_")
	if len(parts) < 2 {
		return false
	}
	last := parts[len(parts)-1]
	prev := ""
	if len(parts) > 2 {
		prev = parts[len(parts)-2]
	}
	return last == goos || last == goarch || prev == goos || prev == goarch
}
