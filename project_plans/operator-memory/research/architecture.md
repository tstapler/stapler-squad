# Architecture Research: Operator/Repo Memory Injection

Agent 3 (Architecture), SDD Phase 2 for `project_plans/operator-memory/`.

## Summary of the key finding

The system-prompt-const/prefix-caching model in `session/headless/features.go` is
**not the only layer that matters**. `session/headless/pool.go` + `caller.go`
implement claude-CLI session reuse (`--resume <sessionID>`) keyed by
`FeatureKey` (`"triage"`, `"review"`, etc.), and **`acquireSession` only emits
`--system-prompt` on the first call of a rotation window** (`session/headless/caller.go:192-203`).
On every resumed call (`sessionID != ""`), whatever `systemPrompt` string the
caller passes to `CallBlocking` is silently dropped — the CLI process never
sees it. Rotation happens after `MaxCallsPerSession` (default 25) calls or 3
consecutive errors (`session/headless/pool.go:70-104`), **not** per backlog
item and **not** per repo.

This matters for memory injection because whether "append memory to the
system prompt string, fresh, every call" is actually safe depends entirely on
which code path a given call takes:

| Call site | `CallOptions.WorkDir` | Effect (`caller.go:430-476`) | Fresh system prompt every call? |
|---|---|---|---|
| `TriggerTriage` → `CallBlocking(FeatureKeyTriage, ...)` (`server/services/backlog_service_triage.go:2330-2348`) | `itemRepoPath` (always set) | `CallWithOptions` builds a throwaway `oneShot` Pool with `MaxCallsPerSession: 1` — bypasses session reuse entirely | **Yes, always** |
| `TriggerReReview` empty-diff / codebase-read path → `BuildReviewCallOptions("", codebaseWorkDir)` (`session/backlog_review.go:405-422`) | `codebaseWorkDir` (set) | same oneShot bypass | **Yes, always** |
| `TriggerReReview` normal-diff path → `BuildReviewCallOptions(diff, ...)` with `diff != ""` (`session/backlog_review.go:424`) | `""` (empty — `headless.CallOptions{}`) | Goes through `p.call` → `acquireSession` → real session reuse/`--resume` | **No — only on the first call of each ≤25-call rotation window, and that window is not scoped to a single repo** |

So: triage and the empty-diff/codebase-read review path are trivially safe —
read a fresh snapshot, append it, done, every single call. The plain-diff
review path (`headless.FeatureKeyReview`, `CallOptions{}`) is the one place
where "load once per headless call" and "the running claude-CLI session
actually gets a fresh system prompt" diverge: a memory update (or even just a
different item whose `RepoPath` differs from the one baked into the current
resumed session) will not reach the model until the session next rotates.
Flag this explicitly for Phase 3 planning — it's a real scope decision, not
just an implementation detail:

- **Option A (in-scope, minimal):** accept it as a documented limitation —
  memory on the plain-diff review path is "eventually applied, on session
  rotation" rather than "every call," and can carry a stale/wrong-repo
  snapshot mid-window. This matches the issue's own scope statement
  ("storage + injection layer only") — fixing session-reuse granularity is a
  `Pool`-level change, arguably its own follow-up.
- **Option B (out-of-scope but cheap to flag):** key `Pool.sessions` by
  `(FeatureKey, repoPath)` instead of `FeatureKey` alone, or force rotation
  when a hash of `(systemPrompt)` changes since the last call for that key.
  Either fixes the correctness gap but touches shared `Pool` infrastructure
  well beyond "storage + injection layer," so treat as a candidate follow-up
  item, not something to sneak into this one.

Triage is the primary, most-frequently-invoked path and is completely
unaffected by this caveat — build confidently there. Just don't assume the
same guarantee transfers to the plain-diff review call site without reading
this table again.

## Q1 — Where to append memory without breaking the stable-const property

Don't touch the `const` strings themselves (`headlessTriageSystemPrompt`,
`reviewSystemPrompt`, `headlessReviewSystemPrompt`,
`headlessReviewSystemPromptWithCodebaseAccess` in `session/headless/features.go`)
— they stay byte-identical, preserving the documented prefix-caching intent
("Stable prompts enable prefix-caching across repeated calls", `features.go:69`).

Change the exported accessor functions to take the pre-assembled memory
snapshot and concatenate it as a **new trailing block**, e.g.:

```go
// HeadlessTriageSystemPrompt returns the stable system prompt for headless triage
// calls, plus an optional trailing operator/repo memory block. memorySnapshot is
// empty when both OPERATOR.md and REPO.md are empty/missing — in that case the
// return value is byte-identical to the pre-memory prompt (see requirement #5).
func HeadlessTriageSystemPrompt(memorySnapshot string) string {
	if memorySnapshot == "" {
		return headlessTriageSystemPrompt
	}
	return headlessTriageSystemPrompt + "\n\n" + memorySnapshot
}
```

Apply the same shape to `HeadlessReviewSystemPrompt` and
`HeadlessReviewSystemPromptWithCodebaseAccess` (both are in scope — see the
table above; both call sites already resolve a repo path before calling
`CallBlocking`). Leave `ReviewSystemPrompt()` (used only by `BuildReviewPrompt`,
the *interactive*-session prompt builder in `session/review_gate.go` /
`session/pipeline_engine.go`) untouched — interactive-session memory is an
explicit non-goal.

This satisfies requirement #3 ("memory is a new suffix, not an edit to the
existing text") literally: the stable prefix is unchanged for any given call,
and empty memory reproduces the exact prior string (requirement #5), so no
new test surface is needed for the "no memory" case beyond an equality
assertion against the old constant.

Every existing caller of these functions (zero-arg today) needs updating to
pass the snapshot — a small, mechanical, compile-enforced change (Go won't
let you forget an argument), which is a nice property: nothing can silently
skip memory assembly.

## Q2 — Where "frozen snapshot" loading happens

At the call site, immediately before `CallBlocking`, not at `Pool`
construction and not once per `BacklogService`/server lifetime.

Reasons:

1. **`Pool` construction has no repo context.** `Pool` is a single
   long-lived object wired once via `SetHeadlessPool` at server startup
   (`server/services/backlog_service.go:339-342`) and shared across every
   backlog item regardless of `RepoPath` (multi-repo backlog is a supported,
   already-exercised case — see the `RepoPath` validation in `TriggerTriage`,
   `server/services/backlog_service_triage.go:2153-2179`). `REPO.md` is
   per-workspace/per-repo, so there is no single "the" `REPO.md` to load once
   at `Pool`-construction time.
2. **`OPERATOR.md`, unlike `REPO.md`, genuinely could be loaded once at
   server/`BacklogService` startup** (it's fleet-level, one file, no
   per-repo variance) — but doing so would reintroduce the exact staleness
   problem the Hermes reference design is designed around: an operator
   hand-edits `OPERATOR.md` mid-uptime (this issue's whole write path, since
   there's no writer yet) and every call for the rest of the process's life
   keeps using the stale copy until a restart. Loading it fresh per call is
   one extra, cheap file read (a few KB, at most) against a call that already
   costs a `claude -p` subprocess spin-up (hundreds of ms to seconds) —
   negligible overhead, and it avoids inventing any cache-invalidation
   machinery. Read both files fresh, every call, from the same helper.
3. **The exact call-site seam already exists and is cheap to hook.** Every
   in-scope call site already has a resolved repo path in local scope right
   before its `CallBlocking` call:
   - `TriggerTriage`'s goroutine: `itemRepoPath` (`backlog_service_triage.go:2296`,
     used at the `CallBlocking` call at line 2330-2348).
   - `TriggerReReview`: `codebaseWorkDir` / the `BuildReviewCallOptions`
     call (`backlog_service_triage.go` around the `s.reviewPromptFor` /
     `session.BuildReviewCallOptions` call at line ~2696-2729 per the excerpt
     read during this research).
   A single `memory.LoadSnapshot(repoPath string) (string, error)` (or
   similar) call, dropped in right before each `CallBlocking`, is the whole
   integration — no new struct fields, no `Pool` changes, no startup-time
   wiring.

## Q3 — What "frozen snapshot" / "session boundary" means in this one-shot architecture

There is no long-lived interactive session to hang a "session start" event
off of for triage or the codebase-read review path — each is architecturally
a **fresh subprocess per call** (confirmed above: `WorkDir` set → oneShot
`Pool` with `MaxCallsPerSession: 1`). So for those paths, "frozen snapshot at
session start" collapses to "read the files fresh at the top of this
function, right before this one `CallBlocking` call" — there is nothing to
freeze *against*, since nothing persists between calls anyway. This is
strictly simpler than Hermes's model, not a degraded version of it: Hermes
needs to freeze the snapshot because its session is long-running and multiple
turns share one loaded copy; here every "session" is exactly one turn.

For the plain-diff review path, the real claude-CLI session (`--resume`)
*does* persist across calls (see the summary table) — that is the one place
where a "frozen at session start, valid until rotation" semantic actually
applies literally, except "session start" here means "whenever
`acquireSession` last saw `sessionID == ""`," a moment the memory-loading code
has no visibility into (it runs before `CallBlocking`, which internally
decides whether this call is a rotation or a resume). Practical consequence:
on that one path, memory read at call time may or may not actually reach the
model, depending on Pool-internal state the caller can't see or control
without a `Pool` change (Option B above).

## Q4 — Where OPERATOR.md and REPO.md live relative to `config.GetConfigDirForDir`

- `OPERATOR.md`: `~/.stapler-squad/memory/OPERATOR.md` — i.e.
  `filepath.Join(config.GetConfigDir(), "memory", "OPERATOR.md")`, using the
  **zero-arg** `GetConfigDir()` (`config/config.go:117-119`, which is just
  `GetConfigDirForDir("")`). Requirement #1 states this explicitly ("one
  file, not per-workspace") — do not thread a repo path in for this one.
- `REPO.md`: `<workspace-config-dir>/memory/REPO.md` — i.e.
  `filepath.Join(cfgDir, "memory", "REPO.md")` where
  `cfgDir, _ = config.GetConfigDirForDir(itemRepoPath)`.

**Important nuance confirmed by reading `resolveDefaultConfigDir`
(`config/config.go:176-226`):** passing `itemRepoPath` into
`GetConfigDirForDir` only actually changes the resolved directory when
per-directory workspace isolation is opted into
(`STAPLER_SQUAD_WORKSPACE_MODE=true`, priority 5) — in that mode the dir
argument is SHA-256-hashed into `~/.stapler-squad/workspaces/<hash>/`. In the
**default** configuration (no `STAPLER_SQUAD_WORKSPACE_MODE`, no
`STAPLER_SQUAD_INSTANCE`, no `STAPLER_SQUAD_TEST_DIR`), priority 6 ("global
shared state") wins regardless of what `dir` is passed, so
`GetConfigDirForDir(itemRepoPath)` and `GetConfigDirForDir("")` **resolve to
the exact same directory** — meaning `REPO.md` and `OPERATOR.md` would live
side-by-side under the same `~/.stapler-squad/memory/` in that default mode,
distinguished only by filename, not by directory. This is not a bug to fix
in this issue — it's this repo's existing, intentional multi-instance
convention (`.claude/docs/state-isolation.md`) — but the plan phase should
state this plainly rather than imply `REPO.md` is always isolated per repo:
it's isolated per **config-dir instance** (opt-in workspace mode, named
instance, or test dir), the same granularity every other piece of
per-"workspace" state in this codebase already gets, not per literal
filesystem repo path. Multi-repo backlogs running with default (unisolated)
config will share one `REPO.md` across all repos unless the operator opts
into `STAPLER_SQUAD_WORKSPACE_MODE=true` or per-repo `STAPLER_SQUAD_INSTANCE`
values. Call this out as a known-and-accepted consequence of reusing the
existing convention, not a new gap introduced by this feature.

`STAPLER_SQUAD_TEST_DIR` (priority 1) and `STAPLER_SQUAD_INSTANCE` (priority
2) both still work exactly as documented in
`.claude/docs/state-isolation.md` for isolating `REPO.md`/`OPERATOR.md` in
tests and named-instance deployments — no special-casing needed, this falls
straight out of using `GetConfigDirForDir` as-is.

## Q5 — Data flow

```
Disk                                    In-memory                          Call
────                                    ─────────                          ────
~/.stapler-squad/memory/OPERATOR.md  ─┐
                                       ├─▶ memory.LoadSnapshot(repoPath) ──▶ assembled string
<cfgDir(repoPath)>/memory/REPO.md   ──┘      (fresh read, every call;         (== "" if both
                                               config.GetConfigDir() +         files empty/
                                               config.GetConfigDirForDir       missing/
                                               (repoPath); trim/empty-check    whitespace-only)
                                               each file independently)
                                                        │
                                                        ▼
                                          headless.HeadlessTriageSystemPrompt(snapshot)
                                          headless.HeadlessReviewSystemPrompt(snapshot)
                                          headless.HeadlessReviewSystemPromptWithCodebaseAccess(snapshot)
                                                        │
                                                        │ (stable const + "\n\n" + snapshot,
                                                        │  or just the stable const when
                                                        │  snapshot == "")
                                                        ▼
                                          systemPrompt string  ──────▶  Pool.CallBlocking(ctx, key,
                                                                          systemPrompt, userPrompt, opts)
                                                                                  │
                                                                    ┌─────────────┴─────────────┐
                                                                    │                            │
                                                          opts.WorkDir != ""            opts.WorkDir == ""
                                                          (triage; codebase-read        (plain-diff review)
                                                           review)                              │
                                                                    │                            │
                                                          oneShot Pool,                  acquireSession():
                                                          MaxCallsPerSession=1  ◀── always   sessionID=="" ?
                                                          fresh `claude -p                  --system-prompt
                                                          --system-prompt                    <systemPrompt>
                                                          <systemPrompt>`                   : --resume <id>
                                                          subprocess every call              (systemPrompt
                                                                    │                         SILENTLY DROPPED
                                                                    │                         on resumed calls)
                                                                    ▼                            ▼
                                                              claude -p subprocess (one per call, or
                                                              reused --resume session up to 25 calls/
                                                              3 consecutive errors before rotating)
```

Write path (out of scope for this issue, noted for completeness / to bound
what "scan before write" in requirement #6 applies to): this issue ships no
CLI *write* command — `stapler-squad memory show` is read-only
(requirement #4). The prompt-injection scanner requirement (#6) therefore has
no call site to attach to yet in this issue; state that explicitly rather
than building an unused scanner, per the requirements doc's own note on this
open question. The companion writer item (explicitly out of scope, see
Non-goals) is where the scanner actually gets exercised.

## `stapler-squad memory show` CLI placement

Cobra-based CLI (`main.go`, `rootCmd.AddCommand(...)`), with subcommand
packages living under `cmd/commands/` (e.g. `commands.GetSessionCmd`,
referenced at `main.go:717`, `import "github.com/tstapler/stapler-squad/cmd/commands"`).
Follow that precedent: a `commands.MemoryCmd` (parent) with a `show`
subcommand in `cmd/commands/`, resolving the CWD's `RepoPath`-equivalent via
`config.GetConfigDirForDir(cwd)` for `REPO.md` and `config.GetConfigDir()` for
`OPERATOR.md`, printing both (or an explicit "empty" message per file) —
reusing the same `memory.LoadSnapshot`-adjacent file-read helper the prompt
assembly path uses, so there's exactly one implementation of "how to find and
read these two files," not two that can drift.
