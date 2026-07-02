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

## 3. GitHub/external-source ingestion is a stub with no UI — backlog can't self-populate

`ItemSource` CRUD works via RPC, but `TriggerSync` and `GetSyncHistory` are literal
`CodeUnimplemented` stubs, and there is **no UI anywhere** to create a source or trigger a
sync. The user-journey trace calls this the single biggest journey-breaking gap: without a
developer manually seeding an `ItemSource` via raw RPC, the backlog only ever contains
manually-typed items. If the expectation is "GitHub issues/PRs flow into the backlog
automatically," that path does not exist for a normal user today.

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
implemented end-to-end at the RPC layer with no UI caller. Combined with #3, roughly a third of
the originally-planned capability surface is reachable only by someone scripting RPCs directly.

## 9. Silent scope-narrowing, no ADR

GitHub-sync UI and inotify-based drift detection were in the original MVP plan and quietly
dropped during implementation with no ADR or adversarial-review flag ever raised. Not a bug,
but it means the plan docs overstate what was actually delivered — don't trust `plan.md`
"done" checkmarks without cross-checking code.

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
3. Decide deliberately on #3 (GitHub sync): finish it or explicitly cut it and remove the
   half-built RPC surface — right now it's neither shipped nor absent. **Remaining open item.**
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
