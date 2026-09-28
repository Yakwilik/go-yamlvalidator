package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAllStructs(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(wd))
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module allfixture\n\ngo 1.24.0\n\nrequire (\ngithub.com/Yakwilik/go-yamlvalidator v0.0.0\ngopkg.in/yaml.v3 v3.0.1\n)\nreplace github.com/Yakwilik/go-yamlvalidator => "+root+"\n")
	write("types.go", `package allfixture

type Child struct {
    Name string `+"`yaml:\"name\" yamlvalidate:\"required,nonempty\"`"+`
}

type Root struct {
    Child Child `+"`yaml:\"child\"`"+`
}

type Empty struct{}

type Scalar string

type Generic[T any] struct {
    Value T `+"`yaml:\"value\"`"+`
}

type Manual struct {
    Value string `+"`yaml:\"value\"`"+`
}

func (Manual) MarshalYAML() (any, error) { return "manual", nil }
`)
	write("conditional.go", "//go:build !never\n\npackage allfixture\n\ntype Conditional struct{}\n")
	runGen := func(args ...string) ([]byte, error) {
		t.Helper()
		cmdArgs := append([]string{"run", "github.com/Yakwilik/go-yamlvalidator/cmd/yamlvalidator-gen"}, args...)
		cmd := exec.Command("go", cmdArgs...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOFLAGS=-mod=mod")
		return cmd.CombinedOutput()
	}
	if out, err := runGen("-all", "-output=zz_generated.go"); err != nil {
		t.Fatalf("generate all: %v\n%s", err, out)
	}
	generated, err := os.ReadFile(filepath.Join(dir, "zz_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(generated)
	for _, name := range []string{"Root", "Child", "Empty"} {
		if !strings.Contains(source, "func (value "+name+") YAMLValidatorGeneratedType()") {
			t.Fatalf("missing generated root %s", name)
		}
	}
	for _, name := range []string{"Manual", "Generic", "Scalar", "Conditional"} {
		if strings.Contains(source, "func (value "+name+") YAMLValidatorGeneratedType()") {
			t.Fatalf("unexpected generated root %s", name)
		}
	}
	if out, err := runGen("-all", "-type=Scalar", "-output=zz_generated.go"); err != nil {
		t.Fatalf("generate all plus explicit scalar: %v\n%s", err, out)
	}
	generated, err = os.ReadFile(filepath.Join(dir, "zz_generated.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "func (value Scalar) YAMLValidatorGeneratedType()") {
		t.Fatal("-type did not extend -all roots")
	}
	if out, err := runGen("-all", "-type=Scalar", "-output=zz_generated.go", "-check"); err != nil {
		t.Fatalf("fresh all check: %v\n%s", err, out)
	}
	original := string(mustRead(t, filepath.Join(dir, "types.go")))
	write("types.go", strings.Replace(original, "type Root struct {\n", "type Root struct {\n    Enabled bool `yaml:\"enabled,omitempty\"`\n", 1))
	if out, err := runGen("-all", "-type=Scalar", "-output=zz_generated.go", "-check"); err == nil || !strings.Contains(string(out), "stale") {
		t.Fatalf("stale all check: %v\n%s", err, out)
	}
	if out, err := runGen("-all", "-type=Scalar", "-output=zz_generated.go"); err != nil {
		t.Fatalf("regenerate all: %v\n%s", err, out)
	}
	cmd := exec.Command("go", "test", "-mod=mod", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated package: %v\n%s", err, out)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
