# Agent evaluation rubric

[scenarios.json](scenarios.json) contains manual prompts and expected behaviors.
These are not transcripts or claimed model-evaluation results. The Go packaging
tests validate the fixture structure, not the agent's reasoning.

For each scenario, start a clean agent session with only the installed plugin and
the minimal project/input needed for that prompt. Record the agent version,
plugin version, prompt, files read, generated code, executed checks, and result.
Mark each expected behavior pass/fail and record the concrete output supporting
that score. A snippet that does not compile is a failure even if its explanation
sounds plausible.

Required checks include appropriate skill activation, selecting only relevant
references, preserving the caller's version and policy, correct API names,
valid/invalid tests, and truthful reporting. For negative-scope scenarios, no
unrequested project mutation or dependency installation is allowed.

The executable [recipes](../examples/recipes/README.md) separately verify the
code patterns against the library. Passing them is necessary but does not prove
that an agent will select the right pattern in a live conversation.
