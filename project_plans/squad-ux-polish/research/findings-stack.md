# Findings: Stack

**Research date**: 2026-04-17
**Scope**: Libraries and storage mechanisms needed for prompt library/recents, session templates, and review queue UI in stapler-squad.

---

## Summary

All three features (prompt library, session templates, review queue UI) can be implemented without introducing any new Go dependencies. The codebase already has `entgo.io/ent` with SQLite (`mattn/go-sqlite3`) for structured, queryable persistence, a JSON-file config system for user preferences, and a custom `AutocompleteInput` component on the frontend. Each feature maps cleanly to one of the two existing persistence tiers:

- **Prompt library / recents** — extend the existing JSON config tier (`SessionDefaults` in `config.go`); or add an `ent` schema entity backed by the existing SQLite database.
- **Session templates** — the `ProfileDefaults` struct in `config.go` already covers 90% of the shape; a named `Prompt` field is the only missing piece.
- **Review queue UI** — the panel (`ReviewQueuePanel.tsx`) already exists; what is needed is a "create PR" action backed by `github.com/cli/go-gh` or shell-out to `gh`, not a new storage layer.

On the frontend, if the review queue list ever exceeds ~200 entries a virtual list library (`@tanstack/virtual`) is worth adding, but for the expected 10–200 entry range, native CSS scroll + `useMemo` filtering is sufficient and keeps bundle size unchanged.

---

## Options Surveyed

### Backend Storage Options (for prompt library and templates)

**Option A: Extend existing JSON config (`~/.stapler-squad/.../config.json`)**

The `Config` → `SessionDefaults` struct already holds `Profiles map[string]ProfileDefaults`. `ProfileDefaults` has `Name`, `Description`, `Program`, `AutoYes`, `Tags`, `EnvVars`, `CLIFlags`. Adding a `Prompt string` field and a new top-level `PromptHistory []PromptEntry` slice is a two-field addition. Persistence is handled by the existing `ReadConfig`/`SaveConfig` functions. No new library needed.

**Option B: Extend the existing ent/SQLite schema**

The project already uses `entgo.io/ent v0.14.5` with `mattn/go-sqlite3 v1.14.40`. Adding `PromptEntry` and `SessionTemplate` ent schema entities is purely schema + code-gen work (`entgo generate`). This gives indexed queries, deduplication, use-count tracking, and fuzzy search without a new dependency. The SQLite database (`~/.stapler-squad/.../sessions.db`) already exists in the workspace-isolated directory.

**Option C: Introduce a new key-value store (bbolt/badger)**

`bbolt` (BoltDB) is an embedded B-tree key-value store. Good for simple ordered access but adds ~1.5 MB to the binary and a new dependency with its own locking semantics (only one writer at a time). Overkill given ent/SQLite is already present.

**Option D: Pure in-memory + localStorage on the frontend**

Prompt history stored in browser `localStorage` with a ConnectRPC call for persistence fallback. Simple to implement for a browser-only tool, but loses history on browser data clear, does not survive workspace switches, and conflicts with the server-side session model.

### Frontend List Options (for review queue with filtering/sort)

**Option E: Native scroll + CSS (no library)**

For lists of 10–200 items, a plain `<ul>` with `overflow: auto` and `useMemo` filtering is fast enough. Zero bundle impact. Already the pattern used by `SessionList.tsx` and `ReviewQueuePanel.tsx`. No virtual scrolling needed at this scale.

**Option F: `@tanstack/virtual` (v3)**

Headless virtual list library. ~10–15 KB gzipped. Framework-agnostic; works with React 19. Renders only visible rows. Appropriate when the list could grow to 500+ items or when each row is expensive to render (e.g., inline diff previews). Bundle impact: ~10–15 KB against the current 5 MB total JS cap.

**Option G: `virtua` (npm)**

Alternative virtual list. ~8 KB. Less mature community than TanStack Virtual. Fewer examples and integrations with existing patterns.

**Option H: `react-window` / `react-virtualized`**

Older generation virtual list libraries. React-window is smaller (~6 KB) but less actively maintained. Not recommended for new React 19 projects.

### PR Creation (for "create PR" action in review queue)

**Option I: Shell-out to `gh` CLI**

Simplest path: `exec.Command("gh", "pr", "create", ...)` with the worktree path as the working directory. No new Go library. Requires `gh` to be installed on the user machine. Already the tool Tyler uses manually; this just wraps it. Error handling: check exit code and capture stderr.

**Option J: `github.com/cli/go-gh`**

The Go library that backs the `gh` CLI. Gives access to GitHub's REST and GraphQL APIs with auth token management handled transparently. Adds a single well-scoped dependency (~100 KB compiled). Enables richer error messages and status polling without spawning a subprocess. The go-gh library handles auth via environment `GITHUB_TOKEN` or `gh` keychain.

**Option K: Direct GitHub REST API via `net/http`**

Possible but requires manual OAuth flow, token management, and error parsing. Not recommended when go-gh already abstracts all of that.

---

## Trade-off Matrix

| Option | Go dep weight | Frontend bundle impact | Integration complexity | Query capability | Offline/local-first |
|--------|--------------|----------------------|----------------------|-----------------|---------------------|
| **A: Extend JSON config** | Zero new deps | None | Very low — extend existing structs | In-memory slice scan; sufficient for ≤500 prompts | Full |
| **B: Extend ent/SQLite** | Zero new deps (already present) | None | Low — add schema + `go generate` | SQL WHERE, ORDER BY, LIMIT; full | Full |
| **C: bbolt/badger** | +1 dep, ~1.5 MB | None | Medium — new API paradigm | Key prefix scan only; no SQL | Full |
| **D: localStorage** | None | None | Low frontend / no backend | Client-side filter only | Partial (browser clears) |
| **E: Native scroll** | None | None | None | n/a | n/a |
| **F: @tanstack/virtual** | None | ~12 KB | Low | n/a | n/a |
| **G: virtua** | None | ~8 KB | Low | n/a | n/a |
| **I: gh shell-out** | None | None | Very low | n/a — exec only | Requires `gh` installed |
| **J: go-gh library** | +1 dep, small | None | Low | REST/GraphQL | Requires network + auth |

---

## Risk and Failure Modes

### Prompt library

- **Concurrent write corruption** (JSON config): The config system uses atomic rename (`os.Rename` after writing to `.tmp`). Risk is low but not zero under concurrent saves. The ent/SQLite path has proper transaction semantics and is safer under load.
- **Config file bloat**: A `PromptHistory []PromptEntry` slice in `config.json` will grow unbounded if not capped. Must enforce a max-entries limit (e.g., 500) at save time, matching the pattern in `CommandHistory.SetMaxEntries`.
- **Prompt deduplication**: If the same prompt is submitted multiple times, the history will have duplicates. Need a dedup+frequency strategy (collapse duplicates, track `last_used_at` + `use_count`).

### Session templates

- **Template drift**: A template saved today may reference a branch prefix or tag that no longer makes sense after project reorganization. No mitigation at storage level; document as known limitation.
- **Profile vs template naming collision**: `ProfileDefaults` in `SessionDefaults.Profiles` is already used for session defaults. Adding `Prompt` to it repurposes it as a template. If the semantics diverge (profiles = program/env config; templates = include a full prompt), a new `SessionTemplate` type may be cleaner than extending `ProfileDefaults`.

### Review queue UI

- **Diff preview cost**: If each review queue card renders a diff inline, this is a large DOM cost. Render diffs lazily (on expand/click) or virtualize at 200+ items.
- **go-gh auth failure**: If `GITHUB_TOKEN` is not set and the `gh` keychain is not configured, PR creation silently fails. Must surface the auth error clearly to the user.
- **gh CLI not installed**: Shell-out to `gh` requires it to be on PATH. Degrade gracefully with a "gh CLI not found" error and a link to install instructions.

---

## Migration and Adoption Cost

### Option A (JSON config extension)

- Add `Prompt string` to `ProfileDefaults` struct — 1 line.
- Add `PromptHistory []PromptEntry` to `SessionDefaults` struct — 3 lines + new type.
- Update `SaveConfig`/`ReadConfig` — zero changes needed (JSON marshal/unmarshal is additive by default).
- Frontend: add a `ListPromptHistory` / `SavePromptHistory` ConnectRPC endpoint pair — 1 proto change + 1 service method.
- Migration cost: negligible. Existing config files gain new optional fields silently.

### Option B (ent schema extension)

- Add `session/ent/schema/promptentry.go` — ~40 lines.
- Add `session/ent/schema/sessiontemplate.go` — ~40 lines.
- Run `go generate ./session/ent/...` — generates all CRUD code.
- Wire into service layer — ~100 lines new Go.
- Migration cost: ent handles SQLite schema migration automatically with `atlas`. No manual migration SQL needed.

### PR creation (Option I: gh shell-out)

- Add `CreatePR(ctx, worktreePath, title, body string) (prURL string, error)` function in `session/git/` — ~50 lines Go.
- Add proto message + ConnectRPC endpoint `CreatePullRequest` — 1 proto change + 1 service method.
- Frontend: add "Create PR" button on review queue card — ~20 lines TSX.
- Migration cost: zero. No data migration. Feature is additive.

---

## Operational Concerns

- **SQLite file locking**: The existing ent/SQLite setup uses `gofrs/flock v0.12.1` for process-level locking. Extending the schema does not change locking behavior.
- **Config file growth**: `PromptHistory` in the JSON config file could grow to tens of KB. Acceptable for 500 entries of average prompt length ~200 chars = ~100 KB max. If prompts are long (multi-line task descriptions), cap by byte size not just count.
- **Bundle size budget**: The existing `web-app/.size-limit.json` caps total JS at 5 MB (uncompressed). Adding `@tanstack/virtual` (~12 KB) would consume 0.24% of that budget. Not a concern unless many other libraries are added simultaneously.
- **Diff rendering performance**: The review queue card needs to show a diff preview. The codebase already has `DiffViewer.tsx` using shiki for syntax highlighting. Rendering 200 shiki-highlighted diffs simultaneously would be expensive. Use lazy rendering (expand-on-click) at current scale; add `@tanstack/virtual` if the queue grows beyond ~100 cards with visible diffs.

---

## Prior Art and Lessons Learned

- **CommandHistory** (`session/command_history.go`): The codebase already implements a capped ring-buffer history with JSON persistence, dedup via `maxEntries`, and per-session scoping. The `PromptHistory` feature should follow this exact pattern, just scoped globally (not per-session) and stored in the config/state tier rather than per-session log files.
- **ProfileDefaults** (`config/config.go`): The existing `ProfileDefaults` struct is a near-complete session template. It is missing only `Prompt string`. The question is whether to extend `ProfileDefaults` (simpler, fewer types) or create a distinct `SessionTemplate` type (cleaner semantics). Given the codebase preference for additive changes, extending `ProfileDefaults` with an optional `Prompt` field is the path of least resistance.
- **AutocompleteInput** (`web-app/src/components/ui/AutocompleteInput.tsx`): A fully-functional autocomplete dropdown with keyboard navigation is already implemented. The prompt history picker can reuse this component with minimal or no changes. The only addition needed is grouping ("Recent" vs "Saved") which can be achieved by passing a structured suggestion list.
- **ReviewQueuePanel** (`web-app/src/components/sessions/ReviewQueuePanel.tsx`): The review queue panel already exists and handles filtering by priority and attention reason. The "create PR" action is the only UI gap. The panel already consumes a `ReviewItem` protobuf type; a `prUrl` field on that type or a new `CreatePullRequest` RPC is all that is needed.
- **ent + SQLite** (`session/ent/`): The project already committed to ent for structured data. Adding new entity types (PromptEntry, SessionTemplate) follows the established pattern used by ApprovalRule, ClaudeSession, DiffStats, Tag. This is the right home for any data that needs indexed queries or relational integrity.

---

## Open Questions

1. **Prompt in JSON config or ent/SQLite?** For ≤500 prompts without complex queries (filter by tag, usage frequency), the JSON config tier is simpler. For richer querying or if prompts become first-class entities with metadata (tags, categories, per-project scoping), ent/SQLite is cleaner. Decision should be made in the architecture phase.

2. **Should `ProfileDefaults` gain a `Prompt` field, or should a separate `SessionTemplate` type be created?** If templates are meant to be a superset of profiles (they set program + env + prompt), extending `ProfileDefaults` is simplest. If templates and profiles have different lifecycle semantics (templates are one-shot creation helpers; profiles are reusable runtime defaults), a distinct type is cleaner.

3. **Is `go-gh` library justified, or is `gh` shell-out sufficient?** For a local tool where `gh` is likely installed (the README and CLAUDE.md reference it extensively), shell-out is sufficient. go-gh adds value if the PR creation flow needs to poll CI status or read PR comments in the future.

4. **When does the review queue need virtual scrolling?** At 10–200 entries with no inline diff rendering, native scroll is fine. If diffs are rendered inline in each card, even 50 entries could be expensive. Decision depends on the chosen card design.

5. **Global vs per-workspace prompt history?** The codebase has workspace-isolated state by default (`GetConfigDir()` returns a workspace-scoped directory). Prompt history stored in that directory would not be shared across workspaces. Should recent prompts be workspace-local (high relevance, less reuse) or global (more reuse, less relevant)? This is a UX question that the architecture phase should resolve.

---

## Recommendation

**For prompt library / recents**: Extend the JSON config tier first. Add `PromptHistory []PromptEntry` to `SessionDefaults` with a 500-entry cap and use-count deduplication. This requires zero new Go dependencies, follows the established `CommandHistory` pattern, and is reversible. If usage data reveals that prompts need cross-workspace sharing or tag-based querying, migrate to an ent entity in the next iteration.

**For session templates**: Add `Prompt string` to the existing `ProfileDefaults` struct. The rest of the template shape (program, tags, env, cli flags) is already there. This is a one-line Go change with no migration cost.

**For review queue UI**: The panel already exists. Add a `CreatePullRequest` ConnectRPC endpoint backed by a `gh` shell-out in `session/git/`. Do not add a new UI library; use the existing panel and `DiffViewer`. Consider `@tanstack/virtual` only if diff-in-card rendering is adopted and the queue consistently exceeds 100 items.

**Do not add**: bbolt, badger, react-window, react-virtualized, virtua, or a full GitHub REST API client. None of these add value beyond what is already in the codebase.

---

## Pending Web Searches

The following claims should be verified by the parent agent if possible:

1. **`go-gh` module path and current version**: Verify the module is `github.com/cli/go-gh/v2` and check the current stable version tag. [TRAINING_ONLY — verify]
2. **`@tanstack/virtual` v3 actual gzipped size**: Search confirms "10–15 KB" but the exact gzipped figure for the React adapter should be verified against the npm package page. [TRAINING_ONLY — verify]
3. **ent v0.14.5 automatic Atlas migration behavior**: Confirm that `client.Schema.Create(ctx)` in ent v0.14.5 with SQLite will automatically apply new table additions without requiring manual migration SQL. [TRAINING_ONLY — verify]
4. **gh CLI `pr create` JSON output flag**: Confirm that `gh pr create --json url` (or equivalent) outputs the created PR URL as machine-readable JSON for programmatic capture. [TRAINING_ONLY — verify]
