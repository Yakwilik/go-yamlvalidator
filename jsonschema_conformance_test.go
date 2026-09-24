//go:build conformance

package yamlvalidator_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	. "github.com/Yakwilik/go-yamlvalidator"
)

type conformanceGroup struct {
	Description string `json:"description"`
	Schema      any    `json:"schema"`
	Tests       []struct {
		Description string `json:"description"`
		Data        any    `json:"data"`
		Valid       bool   `json:"valid"`
	} `json:"tests"`
}

func TestOfficialJSONSchemaTestSuite(t *testing.T) {
	base := os.Getenv("JSON_SCHEMA_TEST_SUITE")
	if base == "" {
		t.Skip("set JSON_SCHEMA_TEST_SUITE to the JSON-Schema-Test-Suite checkout")
	}

	resources := loadConformanceResources(t, filepath.Join(base, "remotes"))
	drafts := []struct {
		dir   string
		draft JSONSchemaDraft
	}{
		{"draft4", JSONSchemaDraft4},
		{"draft6", JSONSchemaDraft6},
		{"draft7", JSONSchemaDraft7},
		{"draft2019-09", JSONSchemaDraft2019},
		{"draft2020-12", JSONSchemaDraft2020},
	}

	coreTotal := 0
	optionalTotal := 0
	for _, entry := range drafts {
		entry := entry
		t.Run(entry.dir, func(t *testing.T) {
			core := runConformanceDirectory(t, filepath.Join(base, "tests", entry.dir), entry.draft, resources, false)
			optional := runConformanceDirectory(t, filepath.Join(base, "tests", entry.dir, "optional"), entry.draft, resources, true)
			coreTotal += core
			optionalTotal += optional
		})
	}
	if coreTotal < 4000 {
		t.Fatalf("unexpectedly small core suite: %d tests", coreTotal)
	}
	if optionalTotal < 3000 {
		t.Fatalf("unexpectedly small optional suite: %d tests", optionalTotal)
	}
	t.Logf("official JSON Schema conformance: core=%d optional=%d", coreTotal, optionalTotal)
}

func runConformanceDirectory(t *testing.T, dir string, draft JSONSchemaDraft, resources map[string][]byte, optional bool) int {
	t.Helper()
	files := make([]string, 0)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !optional && entry.IsDir() && path != dir {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".json") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk suite %s: %v", dir, err)
	}
	sort.Strings(files)

	total := 0
	for _, path := range files {
		rel, _ := filepath.Rel(dir, path)
		if optional && filepath.ToSlash(rel) == "zeroTerminatedFloats.json" && draft == JSONSchemaDraft4 {
			// This language-specific optional test expects JSON 1.0 not to satisfy
			// integer. The engine follows mathematical JSON Schema number semantics.
			continue
		}

		var groups []conformanceGroup
		readConformanceJSON(t, path, &groups)
		for groupIndex, group := range groups {
			schemaData, err := json.Marshal(group.Schema)
			if err != nil {
				t.Fatalf("%s group %d marshal schema: %v", rel, groupIndex, err)
			}

			opts := JSONSchemaCompileOptions{
				DefaultDraft: draft,
				Resources:    resources,
			}
			normalized := filepath.ToSlash(rel)
			if strings.Contains(normalized, "format/") || strings.HasSuffix(normalized, "format-assertion.json") {
				opts.AssertFormat = true
			}
			if draft == JSONSchemaDraft7 && strings.HasSuffix(normalized, "content.json") {
				opts.AssertContent = true
			}

			schema, err := CompileJSONSchemaWithOptions(schemaData, opts)
			if err != nil {
				t.Fatalf("%s group %d (%s) compile: %v", rel, groupIndex, group.Description, err)
			}
			validator := NewValidator(schema)

			for _, testCase := range group.Tests {
				total++
				source := []byte(conformanceJSONAsYAML(testCase.Data))
				result := validator.ValidateBytes(source)
				actual := !result.HasErrors()
				if actual != testCase.Valid {
					t.Errorf("%s :: %s / %s: want valid=%v got=%v diagnostics=%v source=%s",
						rel, group.Description, testCase.Description, testCase.Valid, actual,
						result.Collector.Errors(), source)
				}
			}
		}
	}
	return total
}

func loadConformanceResources(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	resources := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		resources["http://localhost:1234/"+rel] = data
		resources["https://localhost:1234/"+rel] = data
		return nil
	})
	if err != nil {
		t.Fatalf("load suite resources: %v", err)
	}
	return resources
}

func readConformanceJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func conformanceJSONAsYAML(value any) string {
	switch value := value.(type) {
	case nil:
		return "null"
	case bool:
		if value {
			return "true"
		}
		return "false"
	case json.Number:
		return value.String()
	case string:
		return strconv.QuoteToASCII(value)
	case []any:
		items := make([]string, len(value))
		for i, item := range value {
			items[i] = conformanceJSONAsYAML(item)
		}
		return "[" + strings.Join(items, ",") + "]"
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		items := make([]string, 0, len(keys))
		for _, key := range keys {
			items = append(items, strconv.QuoteToASCII(key)+":"+conformanceJSONAsYAML(value[key]))
		}
		return "{" + strings.Join(items, ",") + "}"
	default:
		data, _ := json.Marshal(value)
		return string(data)
	}
}
