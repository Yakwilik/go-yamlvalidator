package yamlvalidator_test

import (
	"bytes"
	"encoding/json"
	"go/format"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	v "github.com/Yakwilik/go-yamlvalidator"
	"gopkg.in/yaml.v3"
)

const agentSkillPath = "skills/go-yamlvalidator"

type agentPluginManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Author      struct {
		Name string `json:"name"`
	} `json:"author"`
	Homepage   string   `json:"homepage"`
	Repository string   `json:"repository"`
	License    string   `json:"license"`
	Keywords   []string `json:"keywords"`
}

type agentSkillHeader struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	License       string `yaml:"license"`
	Compatibility string `yaml:"compatibility"`
	Metadata      struct {
		Version        string `yaml:"version"`
		LibraryVersion string `yaml:"library-version"`
		SourceRevision string `yaml:"source-revision"`
	} `yaml:"metadata"`
}

func agentRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate agent plugin test")
	}
	return filepath.Dir(file)
}

func readAgentFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(agentRepositoryRoot(t), path))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAgentPluginManifestAndSkill(t *testing.T) {
	var plugin agentPluginManifest
	decoder := json.NewDecoder(bytes.NewReader(readAgentFile(t, ".claude-plugin/plugin.json")))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plugin); err != nil {
		t.Fatal(err)
	}
	if plugin.Name != "go-yamlvalidator" || plugin.Description == "" || plugin.Author.Name == "" {
		t.Fatalf("incomplete plugin metadata: %+v", plugin)
	}
	if plugin.Repository != "https://github.com/Yakwilik/go-yamlvalidator" || plugin.License != "Apache-2.0" {
		t.Fatalf("repository/license mismatch: %+v", plugin)
	}
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(plugin.Version) {
		t.Fatalf("invalid plugin version %q", plugin.Version)
	}
	skill := string(readAgentFile(t, agentSkillPath+"/SKILL.md"))
	parts := strings.SplitN(skill, "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		t.Fatal("missing SKILL frontmatter")
	}
	var header agentSkillHeader
	yd := yaml.NewDecoder(strings.NewReader(parts[1]))
	yd.KnownFields(true)
	if err := yd.Decode(&header); err != nil {
		t.Fatal(err)
	}
	if header.Name != plugin.Name || header.Metadata.Version != plugin.Version || header.License != plugin.License {
		t.Fatalf("skill and plugin metadata differ: %+v", header)
	}
	if len(header.Description) == 0 || len(header.Description) > 1024 || header.Compatibility == "" {
		t.Fatal("skill description/compatibility does not satisfy package contract")
	}
	if header.Metadata.LibraryVersion != "v1.0.0" || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(header.Metadata.SourceRevision) {
		t.Fatal("skill must identify the tested library version and source revision")
	}
	if strings.Count(skill, "\n") > 250 {
		t.Fatal("move detailed material from SKILL.md to references")
	}

	var candidate struct {
		Name   string `json:"name"`
		Source struct {
			Source string `json:"source"`
			Repo   string `json:"repo"`
		} `json:"source"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(readAgentFile(t, "docs/agent-plugin-marketplace-entry.json"), &candidate); err != nil {
		t.Fatal(err)
	}
	if candidate.Name != plugin.Name || candidate.Source.Source != "github" || candidate.Source.Repo != "Yakwilik/go-yamlvalidator" || candidate.Description == "" {
		t.Fatalf("invalid future marketplace entry: %+v", candidate)
	}
	if _, err := os.Stat(filepath.Join(agentRepositoryRoot(t), ".claude-plugin/marketplace.json")); err == nil {
		t.Fatal("the library is a plugin, not a second marketplace")
	}
}

func TestAgentPluginSelfContainedLinks(t *testing.T) {
	root := agentRepositoryRoot(t)
	skillDir := filepath.Join(root, agentSkillPath)
	link := regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	var markdown []string
	err := filepath.WalkDir(skillDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".md") {
			markdown = append(markdown, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range markdown {
		t.Run(filepath.ToSlash(strings.TrimPrefix(path, skillDir+string(filepath.Separator))), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.ContainsRune(data, '`') {
				t.Fatal("skill markdown must use tilde fences and <code> inline markup")
			}
			for _, match := range link.FindAllSubmatch(data, -1) {
				href := string(match[1])
				parsed, err := url.Parse(href)
				if err != nil {
					t.Fatal(err)
				}
				if parsed.IsAbs() || strings.HasPrefix(href, "#") {
					continue
				}
				target := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(parsed.Path)))
				relative, err := filepath.Rel(skillDir, target)
				if err != nil {
					t.Fatal(err)
				}
				if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					t.Errorf("skill dependency escapes install directory: %s", href)
					continue
				}
				if _, err := os.Stat(target); err != nil {
					t.Errorf("broken local link %s: %v", href, err)
				}
			}
		})
	}
}

func TestAgentPluginQuickStartMatchesRunnableExample(t *testing.T) {
	skill := string(readAgentFile(t, agentSkillPath+"/SKILL.md"))
	start := strings.Index(skill, "~~~go\n")
	if start < 0 {
		t.Fatal("missing quick start")
	}
	body := skill[start+len("~~~go\n"):]
	end := strings.Index(body, "\n~~~")
	if end < 0 {
		t.Fatal("unclosed quick start")
	}
	formatted, err := format.Source([]byte(body[:end]))
	if err != nil {
		t.Fatal(err)
	}
	example, err := format.Source(readAgentFile(t, agentSkillPath+"/examples/native/main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(formatted), bytes.TrimSpace(example)) {
		t.Fatal("quick start drifted from runnable example")
	}
}

func TestAgentPluginSchemaFixtures(t *testing.T) {
	schema, err := v.CompileJSONSchema(readAgentFile(t, agentSkillPath+"/assets/schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	validator := v.NewValidator(schema)
	for _, tc := range []struct {
		name  string
		valid bool
	}{{"valid", true}, {"invalid", false}} {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.ValidateBytes(readAgentFile(t, agentSkillPath+"/assets/"+tc.name+".yaml"))
			if result.HasErrors() == tc.valid {
				t.Fatalf("valid=%v errors=%v", tc.valid, result.Collector.Errors())
			}
		})
	}
	var native any
	if err := yaml.Unmarshal(readAgentFile(t, agentSkillPath+"/assets/native-schema.yaml"), &native); err != nil {
		t.Fatal(err)
	}
}

func TestAgentPluginEvaluationManifest(t *testing.T) {
	var suite struct {
		SchemaVersion int    `json:"schemaVersion"`
		Scope         string `json:"scope"`
		Scenarios     []struct {
			ID        string   `json:"id"`
			Prompt    string   `json:"prompt"`
			Reference string   `json:"reference"`
			Expected  []string `json:"expected"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(readAgentFile(t, agentSkillPath+"/evals/scenarios.json"), &suite); err != nil {
		t.Fatal(err)
	}
	if suite.SchemaVersion != 1 || len(suite.Scenarios) < 15 || suite.Scope == "" {
		t.Fatal("incomplete evaluation rubric")
	}
	seen := map[string]bool{}
	for _, scenario := range suite.Scenarios {
		if scenario.ID == "" || seen[scenario.ID] || scenario.Prompt == "" || len(scenario.Expected) == 0 {
			t.Fatalf("invalid scenario %+v", scenario)
		}
		seen[scenario.ID] = true
		path := agentSkillPath + "/references/" + scenario.Reference
		if scenario.Reference == "SKILL.md" {
			path = agentSkillPath + "/SKILL.md"
		}
		readAgentFile(t, path)
	}
}
