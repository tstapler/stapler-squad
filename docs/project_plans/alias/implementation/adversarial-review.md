# Adversarial Review: alias
**Date**: 2026-06-20
**Verdict**: CONCERNS
**Updated**: 2026-06-20 — all 3 blockers resolved in plan.md patches

## Blockers (all resolved)

- [x] **AliasDetector fires `null` for `@foo` (no trailing space)** — RESOLVED: `AliasDetector` returns `InputType.AliasBrowse` for all `^@` input without a trailing space, never `null`. `InputType.AliasBrowse` added to enum. `SessionSearchDetector` cannot claim `^@`. See Task 3.1.3a.

- [x] **`path` is not required for alias creation, but `CreateSession` rejects missing `path` before alias resolution** — RESOLVED: Task 5.1.3d adds `&& req.Msg.AliasName == ""` to the path guard at line ~965. Pathless aliases no longer 400.

- [x] **`CLIFlags` merge on `mergeProfileInto` is replace-not-append** — RESOLVED: Plan documents resolution layers use REPLACE semantics (correct; no change to `mergeProfileInto`). Invocation-time `extraFlags` are APPENDED as an explicit final step in `ResolveAlias`. See Pattern Decisions table and Task 2.1.2c.

## Concerns

- [ ] **`useAliases` hook has no staleness / reload strategy** — the hook fetches on mount, with no polling and no cache invalidation. If the user edits `config.json` while the app is open, they see stale aliases until they reload the page. The WorkflowDetector has the same limitation, but workflows are managed in-app, so edits are synchronous. Aliases are file-edited, making staleness more likely.
  **Recommendation**: Add a stale-while-revalidate pattern (e.g., refetch on omnibar open) or at minimum a "Refresh" button in the empty/error state. Alternatively, document the limitation explicitly in the UX empty state ("changes to config.json require a page reload").

- [ ] **AliasDetector priority 36 is between `NewSessionDetector` (35) and `GitHubShorthandDetector` (40), but `WorkflowDetector` priority is not shown in the plan** — the plan says AliasDetector is dynamically registered like WorkflowDetector, but does not state WorkflowDetector's current priority. If WorkflowDetector is also near 36, a workflow slug that matches an alias name creates an ambiguous collision. The plan's ADR-020 mentions "Workflows win when slug = alias name" but does not state WorkflowDetector's priority, meaning the priority ordering is undocumented.
  **Recommendation**: Read `WorkflowDetector` source and add WorkflowDetector's priority to the plan's dependency table. If they are equal, the ordering depends on registration order, which is fragile.

- [ ] **`expandEnvVars` silently drops unset keys — no observability at session creation time** — the plan and requirements both specify "omit key when unset." A user who sets `"ANTHROPIC_API_KEY": "${ANTHROPIC_API_KEY}"` and forgets to export the variable will see a session start without the key, with no error, warning, or visible indication. The plan acknowledges this ("consider a log-level warning") but marks it non-blocking without assigning a task.
  **Recommendation**: Add a concrete task: emit `log.Warn("[alias] env var ${VAR} referenced but not set in environment", "alias", aliasName, "key", k)` in `expandEnvVars`. Without it, users debug mysterious auth failures with no trace.

- [ ] **`AliasPalette` fuzzy filtering algorithm is unspecified** — Story 4.1.1 says "flat fuzzy-filtered list" but does not specify the algorithm (substring match? fuse.js? prefix? ranked?). The existing `AtCommandDropdown` for workflows almost certainly has a specific implementation. If `AliasPalette` implements a different algorithm, the UX is inconsistent.
  **Recommendation**: Reference the workflow dropdown's filter implementation and use the same algorithm, or explicitly document the chosen algorithm in the story so the implementer doesn't guess.

- [ ] **`create_alias_session` dispatch case forwards `label` as `title` but alias sessions also need `program`, `autoYes`, `tags`, `path` from the resolved alias** — Task 5.1.2a says `dispatch.ts` forwards `aliasName`, `branch`, `label as title`. But the dispatch creates a `createSession()` call; `OmnibarContext.handleCreateSession` doesn't know what `program`, `path`, or `autoYes` the alias resolves to — those are resolved server-side via `alias_name`. The context also sets `effectiveSessionType` based on `data.sessionType`, which will be undefined for alias sessions. This undefined path may produce incorrect session types before the backend resolves the alias.
  **Recommendation**: Verify that `sessionTypeMap` can handle an absent/undefined `sessionType` gracefully, or explicitly set `sessionType: "directory"` in the `create_alias_session` dispatch case as a safe default (matching what the backend will resolve to for most aliases).

- [ ] **E2E test is a `test.skip` stub — this satisfies the registry form but not the intent** — Story 5.1.5 explicitly creates an e2e stub with `test.skip`. The feature-registry rule requires `tested: true` only once a test covers the behavior, but setting `tested: true` in `alias.json` with a skipped test would misrepresent coverage. If `tested: false` is set, `coverage-gaps.json` grows.
  **Recommendation**: Either (a) commit the stub with `tested: false` and explicitly note the coverage gap in the PR description, or (b) implement the e2e test fully as part of Phase 5 (the alias flow is fully synchronous and testable). A skipped stub is worse than an honest gap entry.

- [ ] **`ListAliases` RPC is unauthenticated / any-user — aliases may contain sensitive path or env var information** — The existing `GetSessionDefaults` RPC it mirrors presumably has no authentication beyond the session. `AliasConfig.env_vars` can reference `${ANTHROPIC_API_KEY}` and the `path` field exposes local filesystem layout. If stapler-squad runs in a shared or network-accessible mode, `ListAliases` leaks this data.
  **Recommendation**: Confirm that `ListAliases` is gated behind the same auth middleware as other sensitive RPCs. If auth is enforced app-wide, this is low risk; if the server is ever run in unauthenticated mode, add a note.

## Minors

- `AliasProto` in the plan (Task 2.1.3a) omits `env_vars` and `cli_flags` fields. The frontend `useAliases` hook won't be able to show `description` for the chip if the field is missing, and the palette can't display `cli_flags` hints. `AliasProto` should mirror all displayable `AliasConfig` fields, not just a subset.

- The `AliasName` newtype is described as a "string with validated pattern `^[\w-]+$` at load time" but no task creates a `LoadConfigFromPath` validation step that returns an error for duplicates or invalid names. The plan validates format but does not validate uniqueness. Duplicate alias names are a correctness bug: `FindAlias` returns the first match silently.

- Task 4.1.1a says `AliasPalette` is in grouped view "when `input === "@"`" and flat fuzzy view "when `input` matches `^@[\w-]+$`". This leaves a gap: `input === "@"` (exactly one character) triggers grouped view, but `"@ "` (space after `@`) triggers neither condition. Users who accidentally type `@<space>` will see nothing. The conditions should be `startsWith("@")` with character-count logic, not exact equality.

- `ALIAS-013` from the UX research (config parse errors should surface in the alias palette empty state) has no corresponding task in the plan. If `config.json` has a JSON syntax error, `LoadConfig` returns a zero-value config (or panics), and the alias palette silently shows the empty-state copy rather than "alias config failed to load."

- ALIAS-002 requires the chip to appear as the user types (`@myp` shows chip for `myproj`). The plan implements this via `InputType.Alias` result when `@name` is detected with a trailing space. But ALIAS-002 requires it without the trailing space — the partial name `@myp` must match `myproj` before the space is typed. No task addresses prefix/partial matching for the resolution chip, only exact (post-space) resolution.

- Screen reader / accessibility: the UX research (point 8) requires `role="listbox"` or `role="menu"` with `aria-label` on the alias palette. No task in Plan Phase 4 addresses ARIA roles. The CSS architecture rules also require attention to portal usage for overlays, which the plan doesn't mention for `AliasPalette`.

- The plan targets `@alias-name[:branch][ label][ --flags]` but the grammar regex in Story 3.1.3 uses `(?:\s+((?!--)[^\n]+?))?` for label and `(?:\s+(--\S.*?))?$` for flags. The `--\S` requirement means `-- flag` (flag with space after `--`) would not match. Most CLI tools use `-- ` (double-dash space) as an argument separator. This is a subtle grammar bug.
