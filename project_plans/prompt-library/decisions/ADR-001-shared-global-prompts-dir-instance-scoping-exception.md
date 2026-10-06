# ADR-001: Global Prompts Directory Is Shared Across Instances (Deliberate Exception to `GetConfigDir()`'s Instance-Scoping)

**Status**: Accepted
**Date**: 2026-08-06
**Project**: prompt-library

## Context

`config.GetConfigDir()` / `config.GetConfigDirForDir()` (`config/config.go:117-135`) resolve `~/.stapler-squad/` with a priority chain that scopes most state under `~/.stapler-squad/instances/<STAPLER_SQUAD_INSTANCE>/` when `STAPLER_SQUAD_INSTANCE` is set — this is how manual test instances (per `CLAUDE.md`'s "Manual/interactive testing" section, e.g. `STAPLER_SQUAD_INSTANCE=claude-manual-test`) get their own isolated `sessions.json`, `config.json`, etc., without colliding with the live `:8543` deployment.

`requirements.md` §1 is explicit and literal: the global prompt-template directory is `~/.stapler-squad/prompts/` — not `~/.stapler-squad/instances/<name>/prompts/`. Templates are meant to be authored once ("drop a `.md` file") and used by every instance running on the same machine (the live service, a manual test instance, a second workspace-isolation instance) — a curated prompt library is analogous to `~/.claude/commands/` (global, not per-project-instance), not to session/backlog state, which is legitimately per-instance because two instances must never see each other's live sessions.

However, `config/config.go`'s existing `STAPLER_SQUAD_TEST_DIR` override tier (used by e2e tests, `tests/e2e/global-setup.ts`) must still apply — an e2e test run must not read or write the developer's real `~/.stapler-squad/prompts/`, or a test-authored fixture template would leak into the developer's actual picker (or vice versa, a test run could pick up the developer's real templates and produce flaky, environment-dependent assertions).

## Decision

Add `PromptsDirOrDefault() (string, error)` to `config/config.go`, following the exact structural shape of `TriageArtifactDirOrDefault()` / `BacklogAttachmentDirOrDefault()` (`config/config.go:539-557`), but resolving against `GetConfigDir()`'s **test-dir-override tier only**, bypassing the **instance-scoping tier**:

```go
// PromptsDirOrDefault returns the resolved global prompt-template directory
// (~/.stapler-squad/prompts/). Deliberate exception: unlike most state under
// GetConfigDir(), this directory is intentionally SHARED across all
// STAPLER_SQUAD_INSTANCE values on one machine — a prompt template authored
// once should be visible to every local instance, the same way
// ~/.claude/commands/ is global rather than per-project. It still honors
// STAPLER_SQUAD_TEST_DIR so e2e/test runs never touch a developer's real
// templates. Do NOT "fix" this into full instance-scoping — that would
// silently orphan a user's saved templates every time they ran a manual
// test instance with a different STAPLER_SQUAD_INSTANCE value.
func PromptsDirOrDefault() (string, error) {
    if testDir := os.Getenv("STAPLER_SQUAD_TEST_DIR"); testDir != "" {
        return filepath.Join(testDir, "prompts"), nil
    }
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("resolve home directory: %w", err)
    }
    return filepath.Join(home, ".stapler-squad", "prompts"), nil
}
```

The doc comment above is the load-bearing artifact of this ADR — it is written into `config/config.go` verbatim (Task 1.5.1a in `implementation/plan.md`) so a future reader hits the rationale at the point of temptation to "fix" it, not just in this ADR file.

## Alternatives Considered

- **Full instance-scoping via `GetConfigDirForDir()`** (the default pattern every other new state directory in this codebase follows). Rejected: would silently split one user's template library across every `STAPLER_SQUAD_INSTANCE` they've ever used locally — a template saved while running a manual test instance (`STAPLER_SQUAD_INSTANCE=claude-manual-test`) would be invisible from the live `:8543` deployment and vice versa, directly contradicting requirements.md's literal, un-instance-qualified `~/.stapler-squad/prompts/` path and the "drop a `.md` file, it just works" acceptance criterion (AC1).
- **No `STAPLER_SQUAD_TEST_DIR` support at all** (treat the global dir as fully machine-global, ignoring test isolation). Rejected: would make `tests/e2e/prompt-library.spec.ts` non-hermetic — a CI run or a developer's local e2e run would read/write the real `~/.stapler-squad/prompts/`, contaminating either the test (picking up unrelated real templates) or the developer's own library (leaving test fixtures behind). `STAPLER_SQUAD_TEST_DIR` is the existing, already-adopted mechanism (`global-setup.ts`) for exactly this isolation need — reusing it is one `if` branch, not a new mechanism.

## Consequences

- `config/config.go` gains one new exported function with an inline "why" comment; no changes to `GetConfigDir()`/`GetConfigDirForDir()` themselves — this is additive, not a modification of existing instance-scoping behavior for any other state.
- Workspace-local templates (`<workspace>/.stapler-squad/prompts/`) are unaffected by this ADR — they are resolved per-workspace-root via `promptlibrary.WorkspacePromptsDir()` (a separate helper, see `research/stack.md`), not through `GetConfigDir()` at all, so they carry no instance-scoping question in the first place.
- A future contributor adding a *second* shared-not-instance-scoped directory should copy this ADR's pattern (test-dir override only, explicit inline "why") rather than inventing a new bypass mechanism.
