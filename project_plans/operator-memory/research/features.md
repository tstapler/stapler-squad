# Feature Landscape Research: Persistent Operator Memory

## 1. `session/memory/` is unrelated — naming collision risk (VERIFIED)

`session/memory/reader.go` (`package memory`, doc comment: "Package memory
provides session memory measurement for the hibernation sweeper") implements
`Reader` with `SystemMemoryPct()` and `SessionRSSMB(tmuxSessionName string)` —
RAM measurement via `gopsutil`/`tmux list-panes`, used by the hibernation
sweeper to decide when to suspend idle sessions. Nothing here relates to
knowledge/facts storage. `session/memory/memorytest/fake.go` is a fake of
this RAM-measurement `Reader` interface, not a memory-store fake.

**Naming collision risk**: creating a new knowledge-store package as
`session/memory` is a direct conflict (package already exists at that path).
Recommend a distinct path, e.g. `session/opmemory/`, `session/knowledge/`, or
nest under `config/` since it's fundamentally a config-dir-scoped file store
(`config.GetConfigDirForDir` is the existing precedent for workspace-scoped
storage — see below). Whatever name is chosen, grep for `"package memory"`
under `session/` before implementation to reconfirm no second collision was
introduced by other in-flight work.

## 2. No existing precedent for reading CLAUDE.md and injecting it into a headless prompt

Grepped every `CLAUDE.md` reference in `session/` and `server/services/`
(`session/backlog_context.go:110`, `session/pipeline_mode_seed.go:365`,
`server/services/slash_command_service.go:93,104`,
`server/services/backlog_debug_seed_handler.go:14`,
`server/services/backlog_service.go:160`,
`server/services/backlog_service_triage.go:353`) — every hit is either a
**doc comment pointing a human at this repo's own root `CLAUDE.md`** for
context, or (`slash_command_service.go`) logic that explicitly **skips**
`CLAUDE.md`/`README.md` when scanning a directory for slash-command files.
None of these read `CLAUDE.md`'s file *contents* and inject them into an LLM
prompt. Interactive Claude Code sessions get `CLAUDE.md` for free via
Claude Code's own project-instructions mechanism (outside this repo's
control) — this is exactly why the requirements' non-goals section excludes
interactive sessions ("CLAUDE.md already covers that path").

**Conclusion**: there is no existing "read a file → append to system prompt"
precedent to follow in this codebase for headless calls. This feature is a
genuinely new capability, not an extension of an existing injection
mechanism. The closest *structural* precedents are:

- `server/services/rule_prompt_builder.go`'s `DefaultRulePromptBuilder.BuildSystemPrompt` —
  builds a system prompt by concatenating a stable instruction block +
  dynamically serialized JSON context (existing rules) + seed examples. Same
  "stable prefix + dynamic tail" shape the requirements describe, just for a
  different feature (rule suggestion, not triage/review).
- `session/backlog_review.go`'s `sanitizeDiff`/`SanitizeDiff` — neutralizes
  ``` ` `` ` sequences in diff content before interpolating it into a prompt,
  specifically to stop diff content from escaping its code fence and being
  read as instructions. This is the only existing "sanitize untrusted content
  before prompt interpolation" pattern in the repo, and is the natural model
  for the injection scanner in AC 2/6 — though it solves fence-escaping, not
  general prompt-injection phrase detection, so it only partially covers what
  "scanned for prompt injection" implies.
- `server/services/ai_interfaces.go`'s `redactedSecret`/`redactedPrompt`
  constants — a secret scanner that substitutes a sentinel string before a
  value reaches AI prompts. Same defense-in-depth spirit (content scan → gate
  before something reaches a prompt), different trigger (secrets, not
  injection phrases) and different target (prompt substitution, not a
  pre-write gate on a file).

`session/headless/features.go` (the file named in the requirements as home
of "stable system prompt" constants) currently contains **no** `Build*`
functions — the actual builders (`BuildHeadlessTriagePrompt`,
`BuildHeadlessRetriagePrompt`, `BuildReviewPrompt`, `BuildHeadlessReviewPrompt`)
live in `session/backlog_triage.go` and `session/backlog_review.go`
respectively. Requirements text should be read as "the stable prompt used by
these builders," not literally sourced from `features.go` — worth flagging
to the plan phase so the touchpoint list references the correct files.

## 3. `config.GetConfigDirForDir` — the workspace-scoping precedent (VERIFIED)

`config/config.go:123` `GetConfigDirForDir(dir string) (string, error)`
resolves a config directory with a documented priority hierarchy:
`STAPLER_SQUAD_TEST_DIR` → `STAPLER_SQUAD_INSTANCE` → test-mode auto-detect →
preferred-workspace file → per-directory workspace isolation
(`STAPLER_SQUAD_WORKSPACE_MODE=true`) → global shared `~/.stapler-squad/`.
This is exactly the mechanism the requirements point to for
`<workspace-config-dir>/memory/REPO.md` — REPO.md should live under whatever
`GetConfigDirForDir(workspaceDir)` resolves to, inheriting all of that
isolation logic (test dirs, named instances, per-workspace mode) for free.
`OPERATOR.md` at fleet scope should NOT go through this per-dir resolution —
requirements explicitly place it at `~/.stapler-squad/memory/OPERATOR.md`,
i.e. the *base* dir (`filepath.Join(homeDir, ".stapler-squad")` from
`GetConfigDir()`'s non-isolated branch), not per-workspace. Note test
isolation still matters for OPERATOR.md too — a test run with
`STAPLER_SQUAD_TEST_DIR` set should not read/write a real operator's fleet
file; needs its own path resolution that still respects test-dir override
without going through the full per-workspace priority chain.

## 4. No `stapler-squad memory` CLI subcommand exists yet

`cmd/commands/` contains one existing subcommand (`get_session.go`, using
`spf13/cobra`, a `connect` RPC client hitting `localhost:8080` — note the
hardcoded port comment says 8080 but the app's actual default is 8543 per
CLAUDE.md; worth flagging as a pre-existing inconsistency, not something to
silently copy). The `memory show` subcommand needs to be added as a new
`cobra.Command` here, following this file's pattern. Since AC 4 is read-only
("displays current contents"), it likely doesn't need an RPC round-trip at
all if it can resolve config dirs and read the files directly (in-process,
no server dependency) — worth deciding in the plan phase whether `memory
show` talks to a running server or reads files directly, since the CLI
binary and the server binary are the same executable (`cmd/main.go` — note:
`cmd/main.go` was NOT found by direct path lookup; the actual entrypoint
should be re-verified during planning, only `cmd/commands/get_session.go`
was confirmed to exist).

## 5. ADR-022 (headless triage) — no direct constraint on prompt structure, but confirms path

`docs/adr/ADR-022-headless-triage-over-autonomous-driver.md` documents why
headless triage calls `pool.CallBlockingWithOptions(ctx, FeatureKeyTriage,
headlessTriageSystemPrompt, prompt, CallOptions{...})` directly rather than
going through `AutonomousDriver`/tmux. It doesn't dictate anything about
prompt *content* structure (no caching-tier discussion, no mention of
CLAUDE.md), but it does confirm the call shape memory injection must fit
into: a synchronous `pool.CallBlocking*` invocation per triage/review, one
system prompt string assembled fresh per call. This matches the
requirements' "frozen snapshot at session/call start" design — there is no
long-lived session object across which memory would need to stay pinned;
each headless call is already a fresh, one-shot prompt assembly, so "load
once per call" is the natural default with no competing existing lifecycle
to reconcile against.

## 6. Edge cases surfaced by the exploration (for requirements/plan review)

- **Concurrent read while a human hand-edits OPERATOR.md**: since this issue
  is read-only (no writer yet, per non-goals), the only concurrency concern
  is a read racing a human's editor save (e.g. vim swap-file replace, or a
  partial write mid-save). A plain `os.ReadFile` is atomic enough for typical
  editors (rename-based saves) but not for editors that truncate-then-write
  in place — worth a byte-cap + read-error-tolerant behavior (treat read
  error as empty-memory, not a hard failure that blocks triage/review).
- **Byte cap**: requirements' non-goals explicitly say "start with a byte
  cap, not a summarization pipeline" — no existing byte-cap-on-file-read
  helper was found in `config/` or `session/` to reuse; this will be new
  code, likely a simple `io.LimitReader` or truncate-after-read.
- **Missing workspace config dir**: `GetConfigDirForDir` already creates the
  test dir via `os.MkdirAll` for the `STAPLER_SQUAD_TEST_DIR` branch, but for
  the normal branches it's unclear from the excerpt read whether the
  workspace-specific subdirectory is guaranteed to exist before a `memory/`
  subfolder read is attempted — the reader must handle "directory doesn't
  exist yet" as equivalent to "file doesn't exist" (empty memory), not error.
- **Missing vs empty vs whitespace-only REPO.md**: AC 5 explicitly groups all
  three as "no system prompt block at all" — implementation must trim
  whitespace before the emptiness check, not just check file length/existence.
- **Multiple workspaces sharing one fleet-level OPERATOR.md concurrently**:
  since this issue ships no writer, concurrent *reads* of one file from
  multiple workspaces' headless calls are safe (no write contention). This
  only becomes a real concern once the companion writer issue lands — worth
  a forward-reference note in the plan so the read-side design doesn't
  preclude adding file locking later (e.g. don't design an in-memory cache of
  OPERATOR.md that would go stale across processes/workspaces without a way
  to invalidate it).

## 7. Unstated operator needs beyond the explicit ACs

- **UI visibility**: no requirement mentions a web-app view of memory
  contents — `memory show` is CLI-only per AC 3. Operators debugging *why* a
  triage/review call behaved a certain way would need to SSH/shell in to run
  the CLI; a "View Operator Memory" panel in the web UI (read-only, mirrors
  `memory show`) is a plausible follow-up gap but explicitly out of this
  issue's stated scope — flagging for requirements review, not assuming it's
  wanted.
- **Snapshot audit trail**: requirements say memory is a "frozen snapshot at
  session/call start," but nothing persists *which* snapshot (content hash,
  byte length, or timestamp) a given historical triage/review run actually
  saw. Without this, debugging "why did triage behave differently last
  Tuesday vs. today" after OPERATOR.md changes is impossible to reconstruct
  after the fact. Worth flagging to plan.md as an open question — even a
  cheap content-hash logged alongside the `ItemSession`/`HeadlessTriageResult`
  record would close this gap without full snapshot archival.
- **Per-item vs per-workspace scoping**: requirements only define fleet
  (OPERATOR.md) and workspace (REPO.md) tiers — no per-backlog-item tier.
  Nothing in the codebase's existing `BacklogItemData`/`ItemSession` model
  suggests a third tier is expected; two tiers appears to be a deliberate,
  sufficient design, not a gap.
