package yamlvalidator_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Struct tags should look the same in real models and documentation: raw Go
// literals, not interpreted strings with an extra layer of escaped quotes.
func TestStructTagLiteralsUseBackticks(t *testing.T) {
	root := agentRepositoryRoot(t)
	check := func(name string, source []byte, fragment bool) {
		t.Helper()
		prefix := ""
		if fragment && !bytes.HasPrefix(bytes.TrimSpace(source), []byte("package ")) {
			prefix = "package example\n"
		}
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, name, prefix+string(source), parser.AllErrors)
		if err != nil && !fragment {
			t.Errorf("%s: %v", name, err)
			return
		}
		// Documentation may contain statement fragments rather than complete
		// files. The partial AST still includes their type declarations.
		if file == nil {
			return
		}
		ast.Inspect(file, func(node ast.Node) bool {
			field, ok := node.(*ast.Field)
			if ok && field.Tag != nil && !strings.HasPrefix(field.Tag.Value, "`") {
				t.Errorf("%s: use a raw string literal for the struct tag", set.Position(field.Tag.Pos()))
			}
			return true
		})
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".md" {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if ext == ".go" {
			check(name, source, false)
			return nil
		}
		fence, language := "", ""
		start, offset := 0, 0
		for _, line := range bytes.SplitAfter(source, []byte("\n")) {
			trimmed := strings.TrimSpace(string(line))
			if fence == "" && (strings.HasPrefix(trimmed, "~~~") || strings.HasPrefix(trimmed, "```")) {
				n := 0
				for n < len(trimmed) && trimmed[n] == trimmed[0] {
					n++
				}
				fence, language = trimmed[:n], strings.TrimSpace(trimmed[n:])
				start = offset + len(line)
			} else if fence != "" && len(trimmed) >= len(fence) && strings.Trim(trimmed, fence[:1]) == "" {
				if language == "go" || language == "golang" {
					check(name, source[start:offset], true)
				}
				fence, language = "", ""
			}
			offset += len(line)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
