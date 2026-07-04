# Backlog Feature — Gaps & Risks (Synthesis)

Synthesized from `research/implementation-inventory.md`, `research/cross-platform-risks.md`,
`research/test-coverage.md`, and `user-journey.md`. Ranked by how much each blocks trusting
the feature to "just work."

## 0. ~~TriggerTriage's cleanupCtx always expired before it was used~~ — FIXED

**This is the real, universal, 100%-reproducible answer to "I've never seen it work on this
computer."** Found by executing the ranked-order item #1 diagnostic ("verify flag state") for
real on this machine: the flag turned out to already be enabled in the live production
workspace (see #2 below for the full flag-verification writeup), and there were already 3 real
backlog items in the database — all three permanently stuck at `idea` status, one over five
weeks old. Watching a live retry of one of them in production caught the bug directly in the
log:

```
INFO 22:48:28 [TriggerTriage] headless triage started item=c35902a2...
ERROR 22:55:53 [TriggerTriage] persist triage result item=c35902a2...: context deadline exceeded
ERROR 22:55:53 [TriggerTriage] update plan_artifacts_path item=c35902a2...: context deadline exceeded
ERROR 22:55:53 [TriggerTriage] status transition idea→ready item=c35902a2...: context deadline exceeded
INFO 22:55:53 [TriggerTriage] headless triage complete item=c35902a2... suggestions=6 tasks=11
```

The LLM call itself **succeeded** (6 suggestions, 11 tasks — the JSON parser fix from earlier
in this audit worked correctly). But `TriggerTriage`'s `cleanupCtx` — the context guarding the
three DB writes that persist the result, set `plan_artifacts_path`, and transition
`idea→ready` — was created with a fixed 10-second timeout **before** the headless LLM call
(`server/services/backlog_service.go`, `CallBlockingWithOptions`), not after it. Real triage
calls routinely take 7–15 minutes (the prompt instructs 4 parallel research subagents), so by
the time the LLM call returned, `cleanupCtx`'s 10-second budget had been expired for ~7 minutes
and 40 seconds. Every single persistence write failed with `context deadline exceeded` — and
the code still unconditionally logged "headless triage complete" afterward, since that log line
doesn't check whether the writes above it succeeded. This is not a platform-specific,
timing-fluke, or Linux-specific bug: it fires on every successful triage call, on any machine,
every time, because the timeout window never overlapped the work it needed to protect.

This single bug fully explains the original complaint better than every other finding in this
document combined: triage *looks* like it's running (the item session gets created, the log
says "complete"), the LLM does real, valid work, and then the result is silently discarded and
the item is stuck at `idea` forever — exactly matching "partially works" (you can trigger it,
it look like it's doing something) and "never works" (it never actually finishes).

**Fix**: moved `cleanupCtx` creation to immediately after `CallBlockingWithOptions` returns,
so its budget starts counting only once the slow call is already done. Extracted the 10s
literal into a `triageCleanupTimeout` var for testability. Added
`TestTriggerTriage_SlowLLMCallDoesNotExpireCleanupContext`, which simulates a slow LLM call at
test-friendly timescales — verified failing against the pre-fix code ordering and passing
against the fix.

## 1. ~~Execution phase runs on the broken, pre-ADR-022 driver~~ — INVESTIGATED, NOT A BUG

**Update after writing a real regression test** (`server/services/backlog_service_test.go`
`TestBacklogFullLifecycle_TriageApprovalSpawn_CarriesRealPromptContent`, real
CreateBacklogItem→TriggerTriage→ApprovePlan→SpawnSessionFromItem chain, faking only the LLM
call and the tmux/claude subprocess boundary): **this finding does not hold.** The initial
hypothesis (below, kept for the record) assumed `AutonomousDriver`'s `Prompt`/`InitialPrompt`
mismatch from `docs/adr/ADR-022-headless-triage-over-autonomous-driver.md` still broke
execution. It doesn't:

- `CreateDirectorySession` sets `inst.Prompt` to the real item content.
- For a fresh session (`claudeSessionID == ""`, true for every new spawn),
  `buildClaudeCommand` (`session/instance_tmux.go:116-118`) includes `inst.Prompt` directly as
  a CLI argument to `claude` — the real content reaches Claude at launch, no `InitialPrompt`
  needed.
- `session_driver.go:135` ("No initial prompt configured — skip the send step") correctly
  no-ops in this case rather than sending the stale `driverInitialPrompt` fallback — that
  fallback text is dead code today (introduced by `b8d9ed29`, predating ADR-022's writeup by
  a week; it fixed the general case even though ADR-022 still describes the old symptom).
- The regression test confirms empirically: the captured prompt at the session-creation
  boundary contains the real description, AC text, and plan-artifacts pointer — not the
  generic fallback.

The `Prompt`/`InitialPrompt` mismatch was real for the *old* tmux-based `TriggerTriage` design
(pre-ADR-022) specifically, not for `SpawnSessionFromItem`. Do not re-open this without new
evidence (e.g. an actual repro on the second machine).

<details>
<summary>Original (refuted) hypothesis, kept for record</summary>

Triage was migrated off `AutonomousDriver` onto direct headless-pool calls specifically because
`AutonomousDriver` never received the real task goal — a `Prompt`/`InitialPrompt` field
mismatch documented in ADR-022. That fix was assumed to apply to triage only, leaving execution
(`server/services/session_service.go:787,805,1338`) on the same broken path. Investigation
above shows the CLI-arg delivery path makes this moot for fresh spawns.
</details>

## 2. ~~Feature flag is stored per-workspace~~ — VERIFIED ON THIS MACHINE, RULED OUT

`config.GetFeatureFlag("backlog")` defaults to `false`, keyed by `SHA256(cwd)` in
`config.json` (`config/config.go:805-824`). The design itself is still a real footgun (see
below), but **live verification on this machine rules it out as the explanation here**:

- The live systemd unit runs with `WorkingDirectory=$HOME`, so the app's one real production
  workspace hashes to `~/.stapler-squad/workspaces/d685c4b1a423cca3/` — confirmed by matching
  `workspace_meta.json`'s `cwd: "/home/tstapler"` against the systemd unit's `Active: ...
  since` timestamp exactly.
- That workspace's `config.json` has `"backlog": true`. It has for a while.
- `backlog_items` in that workspace's `sessions.db` already has 3 real rows, the oldest from
  2026-05-26 — over five weeks of the user actually trying to use this feature, not a
  never-toggled flag.

So on this machine, the flag was never the problem. What actually explains "never seen it
work" here is gap #0 above — all three of those items are stuck at `idea` because of the
`cleanupCtx` bug, not because they never existed or the flag was off. The per-workspace design
is still worth hardening (no error/banner when off elsewhere, no env var escape hatch, only
reachable via the `UpdateFeatureFlag` RPC/UI toggle) — flag this as the first check on a
*different* machine where the failure mode might genuinely be flag-related, but it is not what
was happening here.

## 3. ~~GitHub/external-source ingestion is a stub with no UI~~ — FIXED (finished, not cut)

**Decided to finish rather than cut** (see triage-order item #3 below for the investigation
that led to this call). `ItemSource` CRUD already worked via RPC; `TriggerSync` and
`GetSyncHistory` were literal `CodeUnimplemented` stubs, and there was no UI anywhere to create
a source or trigger a sync.

**Shipped in PR #138**:
- `TriggerSync`/`GetSyncHistory` implemented for real — wired to the plugin registry and
  encryption key provider so a manual sync decrypts tokens and dispatches to plugins
  identically to the periodic `SyncLoop`.
- A settings UI at `/settings/backlog-sources`: add a GitHub Issues/PRs source, enable/disable,
  delete, trigger a manual sync, and view sync history — closing the "no UI anywhere" half of
  this gap.
- Full e2e coverage (`tests/e2e/backlog-sources-settings.spec.ts`), verified passing against the
  isolated sandboxed test server.

**Real bugs found and fixed along the way** (5-agent parallel code review, converged on
independently by 3 of the 4 review dimensions): `GetBacklogItemByExternalID` matched purely on
external ID with no source scoping — since GitHub issue/PR numbers are only unique *within*
their repo, two sources would silently overwrite each other's items once numbers collided
(e.g. both repo issue #1). Fixed by scoping the lookup to `source_id`, with a regression test
(`TestSyncOne_DoesNotCollideAcrossSourcesWithSameExternalID`) that reproduces the exact
collision. Also fixed: `items_errored`/`error_message` were structurally dead fields (a fully
failed sync left zero history rows), unbounded `ListSourceSyncEvents`, no per-source
concurrency guard (a manual sync racing the periodic tick could double-create an item — closed
with a per-source lock, verified under `-race`), and no transaction wrapping the cursor-advance
+ history-write pair.

## 4. Zero real end-to-end test coverage of the autonomous flow

No test — unit or e2e — drives a real Claude session through triage → execution → review. The
only test that calls a live `claude` binary (`TestTriageHarness_RealClaude`) is
`//go:build harness`-gated out of `make ci`/`make test`, is triage-only, and self-skips in
sandboxed environments (it no-op'd during this very audit). Execution and review are unit-tested
only against fakes (`fakeHeadlessPool`, canned strings). The e2e suite
(`tests/e2e/backlog.spec.ts`) is UI-shell-only: it checks buttons are disabled/enabled and one
manual status transition via a debug button, never a real AI-driven transition. There's an
explicit `test.fixme` at line 426 for a feature ("SuggestNextItem") with no UI button at all.
`docs/registry/features/backend/backlog/*.json` marks all 20 backlog RPCs `tested: false`
(stale relative to the real unit tests that exist, but directionally correct about the
autonomous path). **Bottom line: nobody — human or CI — has actually watched this feature run
a real task start-to-finish before today's audit.**

## 5. ~~Known-fragile triage JSON parser~~ — FIXED

`session/backlog_triage.go`'s `ParseHeadlessTriageResult` located JSON in the model's triage
output via `strings.Index`/`strings.LastIndex` brace-scanning — a naive first-`{`/last-`}` scan.
If the response contained any unrelated brace before the real JSON (e.g. the model illustrating
its output format with "the result looks like `{"example":"schema"}`"), the scan spanned both
blocks and produced an unparseable concatenated blob. This was flagged as a CONCERN in the
e2e-hardening adversarial review and never given a fallback.

**Fix**: replaced the naive scan with `extractTopLevelJSONObjects`, a balanced-brace scanner
that respects string literals (so braces inside quoted JSON values don't affect depth) and
returns every top-level `{...}` span in order. `ParseHeadlessTriageResult` now tries candidates
from the *last* backwards — matching the prompt's instruction to emit the real result last —
and returns the first one that unmarshals cleanly, correctly skipping any earlier decoy object.
Four new regression tests cover: a stray brace in preamble, multiple valid-looking decoy objects
before the real one, a brace inside a string literal, and an unbalanced stray brace. All 12
parser tests (8 existing + 4 new) pass; no other test regressed.

## 6. ~~Linux install path is more fragile than macOS~~ — FIXED

The macOS LaunchAgent plist explicitly appended Homebrew + system fallback paths; the Linux
systemd unit baked in a raw `$PATH` snapshot from install time with no fallback.
`session/headless/caller.go:36` did a bare `exec.LookPath("claude")`. If that snapshot went
stale (nvm/asdf reinstall, PATH change since `make install-service`), the headless pool went
nil (`server/dependencies.go:452-464`, log-warn only) and backlog triage silently no-opped.

**Fix, two layers**:
1. `scripts/install-service.sh`'s Linux path now appends the same class of fallback locations
   the macOS plist already had (`$HOME/.local/bin`, `/usr/local/sbin`, `/usr/local/bin`,
   `/usr/sbin`, `/usr/bin`, `/sbin`, `/bin`) to the systemd unit's `Environment=PATH=`, so a
   freshly (re)installed service is robust the same way macOS already was.
2. `session/headless/caller.go`'s `NewPool` now calls a new `findClaudeBinary` helper that
   falls back to `$HOME/.local/bin/claude` and other well-known install locations if
   `exec.LookPath("claude")` fails — this covers an *already-running* service with a stale
   baked-in PATH, which the install-script fix alone can't retroactively help. 7 new unit
   tests cover: PATH-success passthrough, home-fallback success, non-executable/directory
   rejected as false matches, home-fallback priority over system dirs, and empty-homeDir
   safety. One existing test (`TestNewPool_ReturnsErrClaudeNotFound_WhenBinaryMissing`) had to
   additionally override `HOME` — it previously only broke `PATH`, which no longer proves
   "claude not found anywhere" now that a real fallback exists.

## 7. Historical Linux-only subprocess bug (fixed once, same pattern still lingers elsewhere)

Commit `095e09e3` fixed headless calls failing with `ENOTTY` when run as a systemd service
(no controlling terminal) — Linux-specific (`Noctty` is active in
`executor/managed_process_linux.go`, a no-op on Darwin). Fixed in `session/headless/runner.go`.
The identical `WithNoControllingTerminal()` pattern still exists unaudited in
`session/vnc/manager.go` — not on the backlog critical path today, but worth a note if VNC ever
intersects with headless/backlog execution.

## 8. Backend-complete, UI-orphaned capabilities

`AttachSessionToItem` (retroactively link an existing session to a backlog item) is fully
implemented end-to-end at the RPC layer with no UI caller. (Previously listed alongside #3's
GitHub sync as another instance of this pattern — #3 now has a UI, so `AttachSessionToItem` is
the remaining example.)

## 9. Silent scope-narrowing, no ADR

GitHub-sync UI and inotify-based drift detection were in the original MVP plan and quietly
dropped during implementation with no ADR or adversarial-review flag ever raised. Not a bug,
but it means the plan docs overstate what was actually delivered — don't trust `plan.md`
"done" checkmarks without cross-checking code.

## 10. ~~Architecture-review follow-ups (post-#3 deep pass)~~ — FIXED

After #3 (GitHub sync) and #1 (execution driver) shipped/closed, a dedicated two-agent
architecture review (not a diff review — a deliberate second look at design soundness) was run
against both areas post-merge. Verdict on both: **sound design, no correctness bugs, several
worth-fixing MAJOR items.** All 7 were fixed in one pass (below), verified with `go build`/`go
vet`/`go test -race ./session/... ./server/...` (all green) and `npx tsc --noEmit` + full jest
suite on the frontend (green aside from the pre-existing, unrelated `SessionCard`
`AnalyticsContextProvider` failures already confirmed present on clean `main`).

**Execution-phase prompt driver** (re-examining gap #1's territory):
- `[MAJOR, docs]` ~~`Instance.Prompt` (CLI-arg delivery, fresh spawns only) and
  `Instance.InitialPrompt` (tmux-typed delivery, for resume/attach) have near-identical
  one-line doc comments with no cross-reference, and the proto comment doesn't match the actual
  tmux-typed implementation.~~ **Fixed**: `session/instance.go` (`Instance` and
  `InstanceOptions` structs) now has a linked, mechanism-explaining doc pair for both fields;
  `proto/session/v1/session.proto` fields 7 (`prompt`) and 15 (`initial_prompt`) updated to
  cross-reference each other and describe the real tmux-typed mechanism instead of the stale
  "inject via CLAUDE.md" text.
- `[MAJOR, ordering]` ~~`WriteSlashCommands`/`WriteBacklogContextFile` ran *after*
  `CreateDirectorySession` had already spawned tmux and started `claude`, with no ordering
  guarantee.~~ **Fixed**: `SpawnSessionFromItem` (`backlog_service.go`) now calls a new
  `session.EnsureDirectorySessionPath` helper (extracted from the `SessionTypeDirectory`
  git-init branch in `instance_worktree.go`, so the directory-creation semantics stay identical)
  and writes both files *before* calling `CreateDirectorySession`, eliminating the race
  entirely rather than relying on process-startup latency.
- `[MAJOR, consistency]` ~~`WriteBacklogContextFile` always dropped prior-session history and
  the plan-artifacts line the live CLI prompt includes.~~ **Fixed**: `WriteBacklogContextFile`
  now takes `priorSessions` and threads it through; the plan-artifacts append moved into
  `BuildSessionInitialPrompt` itself (`session/backlog_context.go`) so both the CLI prompt and
  the on-disk fallback render identically by construction. `AttachSessionToItem` updated to load
  and pass prior sessions too, for the same consistency.

**Full backlog sync feature, end-to-end:**
- `[MAJOR]` ~~Feature-flag divergence between disk config and the in-memory
  `FeatureController`: `Enable`/`Disable` failures were only logged, never retried, so disk and
  in-memory state could diverge permanently.~~ **Fixed**: `UpdateFeatureFlag`
  (`feature_flag_service.go`) now rolls back the just-persisted disk flag if the controller
  toggle fails, and returns `CodeInternal` to the caller instead of silently reporting success.
  The startup path (`dependencies.go`) no longer unconditionally logs "backlog feature enabled"
  regardless of whether `Enable` actually succeeded. New regression test
  `TestUpdateFeatureFlag_RollsBackDiskFlagWhenControllerFails`.
- `[MAJOR]` ~~`GitHubPRsPlugin.Fetch` called `fetchCILabel` per PR serially, risking the
  2-minute `TriggerSync` budget on repos with many open PRs.~~ **Fixed**:
  `backlog_plugin_github_prs.go` now fetches CI labels via a bounded `errgroup` (concurrency 5),
  preserving PR order regardless of completion order. New regression test
  `TestGitHubPRsPlugin_Fetch_ConcurrentCIFetchPreservesPerPRLabels`, verified under `-race`.
- `[MAJOR]` ~~Backend plugin architecture was pluggable but `BacklogSourcesSettings.tsx`
  hardcoded an owner/repo form regardless of `pluginId`.~~ **Fixed**: replaced the hardcoded
  fields with a `PLUGIN_SCHEMAS` array (per-plugin field list + token requirement); the form now
  renders fields generically from the schema and resets them on plugin switch. New test
  `"clears schema-driven field values when switching plugin type"`.
- `[MAJOR]` ~~`GetSyncHistory` truncated at 200 with no pagination and no truncation signal.~~
  **Fixed**: `ListSourceSyncEvents` now over-fetches by one row to detect truncation and returns
  a `truncated` bool (new proto field `GetSyncHistoryResponse.truncated`); the settings UI shows
  an inline notice when history is capped. New tests
  `TestListSourceSyncEvents_ReportsTruncatedWhenOverCap`,
  `TestListSourceSyncEvents_NotTruncatedAtOrUnderCap` (guards the exactly-at-cap edge case),
  `TestGetSyncHistory_SetsTruncatedWhenHistoryExceedsCap`, and a frontend test for the notice.
  Full cursor-based pagination was not added — out of scope for this pass; flag again if history
  depth becomes a real problem in practice.

**Confirmed sound, no action needed** (reviewed and explicitly ruled out, don't re-litigate
without new evidence): transaction/crash-recovery story for the sync loop (safe by design —
cursor + per-source external-ID dedupe means a mid-loop crash just re-fetches and skips
already-created items on the next run); `TriggerSync`'s timeout propagation (fully synchronous,
`syncCtx` threaded through `Fetch` and every ent write — a timeout cancels cleanly, no orphaned
background goroutine); per-process lock scope (`sync.Map` in `session/backlog_sync.go:24` only
guards in-process races — a real limitation, but accepted given this app's
single-instance-per-user deployment model).

---

## Suggested triage order if/when this becomes a fix pass

1. ~~Verify #2 (flag state) on the machine where it's never worked~~ — **done, and it led
   straight to the real bug**: the flag was already enabled and 3 real backlog items already
   existed on this machine, all stuck at `idea`. Watching a live retry of one of them in
   production caught gap #0 (`cleanupCtx` expiring before use) directly in the logs — now
   fixed, with a regression test verified against both the buggy and fixed code paths.
2. ~~Add one real e2e/integration test~~ — **done**:
   `TestBacklogFullLifecycle_TriageApprovalSpawn_CarriesRealPromptContent` in
   `server/services/backlog_service_test.go` covers create→triage→approve→spawn with the real
   production code path, faking only the LLM/subprocess boundary.
3. ~~Decide deliberately on #3 (GitHub sync): finish it or explicitly cut it~~ — **done, decided
   to finish**: implemented `TriggerSync`/`GetSyncHistory`, wired the plugin registry + key
   provider, built the `/settings/backlog-sources` UI, and added e2e coverage. Shipped in PR
   #138, see #3 above for the full writeup (including the cross-source data-corruption bug
   found and fixed during code review).
4. ~~Harden #5 (triage JSON parsing)~~ — **done**: replaced the naive brace-scan with
   independent `json.Decoder` attempts per `{` in `session/backlog_triage.go` (a first
   balanced-brace-counter attempt was itself found buggy by code review and superseded — see
   PR #134 history); 10 adversarial regression tests in `session/backlog_triage_test.go`.
5. ~~Linux PATH robustness (#6)~~ — **done**: `scripts/install-service.sh` + a new
   `findClaudeBinary` fallback in `session/headless/caller.go`.
6. ~~TriggerTriage's cleanupCtx expiring before use~~ — **done, see #0** — this is now believed
   to be the actual, complete explanation for "never seen it work" on this machine specifically.
7. If GitHub sync's fate (#3) doesn't fully close things out, the next real test to write is one
   that actually exercises a live tmux session + live `claude` process for the *execution* half
   (like the existing `harness`-tagged test, but extended past triage) — the one class of bug
   this audit's tests still structurally cannot see.
8. ~~Fix the 7 architecture-review follow-ups (#10)~~ — **done**: doc cross-references, spawn
   file-write ordering, prior-session/plan-artifacts consistency, feature-flag rollback on
   controller failure, concurrent CI-label fetch, schema-driven source form, and a
   `GetSyncHistory` truncation indicator. See #10 above for the full per-item writeup.
