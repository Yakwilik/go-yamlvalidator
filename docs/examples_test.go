package docs_test

import (
	"context"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestHighLevelExamples executes the actual programs printed in the guide.
// Expected-output blocks become Go example assertions in isolated packages.
func TestHighLevelExamples(t *testing.T) {
	guide, err := os.ReadFile("high-level-api.md")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?s)<!-- highlevel-example: ([a-z0-9-]+) -->\s*~~~go\n(.*?)\n~~~\s*Expected output:\s*~~~text\n(.*?)\n~~~`)
	matches := pattern.FindAllStringSubmatch(string(guide), -1)
	if len(matches) == 0 || len(matches) != strings.Count(string(guide), "<!-- highlevel-example:") {
		t.Fatalf("malformed example markers: %d complete examples", len(matches))
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(wd)
	dir := t.TempDir()
	module := fmt.Sprintf("module example.com/highlevel-guide\n\ngo 1.24.0\n\nrequire github.com/Yakwilik/go-yamlvalidator v1.1.0\n\nreplace github.com/Yakwilik/go-yamlvalidator => %q\n", root)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(module), 0600); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, match := range matches {
		name, program, expected := match[1], match[2], match[3]
		if seen[name] {
			t.Fatalf("duplicate example marker %q", name)
		}
		seen[name] = true
		if strings.Count(program, "func main()") != 1 || !strings.HasSuffix(strings.TrimSpace(program), "}") {
			t.Fatalf("%s must be a standalone program ending with main", name)
		}
		program = strings.Replace(program, "func main()", "func Example()", 1)
		closing := strings.LastIndex(program, "}")
		var output strings.Builder
		output.WriteString("\n// Output:\n")
		for _, line := range strings.Split(expected, "\n") {
			output.WriteString("// " + line + "\n")
		}
		program = program[:closing] + output.String() + program[closing:]
		formatted, err := format.Source([]byte(program))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		path := filepath.Join(dir, strings.ReplaceAll(name, "-", "_"))
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "example_test.go"), formatted, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-mod=mod", "-count=1", "./...")
	cmd.Dir = dir
	// Preserve the caller's proxy/toolchain and isolate the temporary module.
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("guide examples: %v\n%s", err, out)
	}
	t.Logf("%d standalone guide programs compiled and matched their expected output", len(matches))
}
