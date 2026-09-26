# Agent skill and plugin

This repository also contains a Claude Code plugin and a standalone agent skill
for using <code>go-yamlvalidator</code> in Go. The library is the source of truth;
the plugin contains instructions, references, fixtures and executable recipes.
It does not start a server or add runtime dependencies, automatic hooks or MCP
connections.

## Layout

~~~text
.claude-plugin/plugin.json
skills/go-yamlvalidator/
  SKILL.md
  references/
  examples/native/
  examples/jsonschema/
  examples/recipes/
  assets/
  evals/
  scripts/check-examples.sh
~~~

The manifest is at the repository root and skills are in the conventional
root-level skills directory, as in the supplied example plugins. All files used
by the skill itself are inside its directory so a standalone installation does
not depend on the rest of the repository being copied.

## Use locally

From a cloned repository, pass its root as the plugin directory:

~~~bash
claude --plugin-dir /absolute/path/to/go-yamlvalidator
~~~

In Claude Code, invoke the skill with:

~~~text
/go-yamlvalidator:go-yamlvalidator
~~~

Or ask to integrate YAML validation in a Go project and let the skill description
provide discovery. No settings file needs to be edited for a temporary local
plugin session. To install the standalone skill through the same skills CLI
used by the example repositories:

~~~bash
npx skills add Yakwilik/go-yamlvalidator
~~~

This command installs an agent skill, not the Go library. Installing the library
in an application remains a separate, explicitly authorized go get operation.
Plugin installation support depends on the agent host; this package does not
claim universal support for every chat application.

## Verify the package

From the repository root:

~~~bash
claude plugin validate .
go test -count=1 -run '^TestAgentPlugin' .
go test -count=1 ./skills/go-yamlvalidator/examples/...
go test -race ./skills/go-yamlvalidator/examples/...
bash skills/go-yamlvalidator/scripts/check-examples.sh
~~~

The last command tests against the published v1.0.0 module in a temporary
workspace. It may download modules and does not alter the current module.
Add <code>--local /absolute/path/to/go-yamlvalidator</code> to check local library
changes. Normal repository tests include the packaging checks and recipes.

The [manual evaluation scenarios](../skills/go-yamlvalidator/evals/scenarios.json)
cover agent behavior, version checks, API selection, ambiguous policies and
out-of-scope prompts. They must be evaluated separately; manifest validation and
Go tests are not a substitute for a live agent evaluation.

## Later marketplace registration

The package is prepared for the catalog structure in
[easyp-tech/skills](https://github.com/easyp-tech/skills). It is not registered
there by adding files to this repository. The candidate object in
[agent-plugin-marketplace-entry.json](agent-plugin-marketplace-entry.json)
can later be appended to that catalog's <code>plugins</code> array.

Its <code>source.repo</code> is <code>Yakwilik/go-yamlvalidator</code>, and its
<code>name</code> matches this repository's plugin manifest. No separate skill
repository or root marketplace manifest is required. Check for an existing
entry before appending it and run the catalog's validation before publishing.
Only after that separate catalog change is published would these commands apply:

~~~text
/plugin marketplace add easyp-tech/skills
/plugin install go-yamlvalidator@easyp-skills
~~~

These are future registration instructions, not a statement that the catalog
already contains the plugin.

## Version and maintenance

The initial plugin/skill version is <code>0.1.0</code>; its documented library
version is <code>v1.0.0</code>. Plugin and library versions are independent.
Change the plugin version when its instructions or resources change, and keep
it synchronized with the SKILL frontmatter. No new Go-library tag is required
merely to publish a skill update.

The root license is Apache-2.0. Referenced template repositories informed the
packaging pattern, not the validator API or licensing:

- [protobuf-expert-skill](https://github.com/easyp-tech/protobuf-expert-skill/tree/6dc1ba26bf177145a670d8548fb1420cbd25be10): manifest, decision flow, references and assets.
- [protoc-gen-mcp-skill](https://github.com/easyp-tech/protoc-gen-mcp-skill/tree/4508a757ce3bb19f02ff8c47527f1dd39df362ff): skill with language examples and references.
- [skills marketplace](https://github.com/easyp-tech/skills/tree/57f3673d484dc527ba0d8125ceb65e8fa35893fa): GitHub source entry format.

The [API index](../skills/go-yamlvalidator/references/api-index.md) records the
implementation revision used to verify the reference material. For another
library version, refresh the examples and references before claiming support.
