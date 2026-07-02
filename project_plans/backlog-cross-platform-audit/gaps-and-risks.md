# Backlog Feature — Gaps & Risks (Synthesis)

Synthesized from `research/implementation-inventory.md`, `research/cross-platform-risks.md`,
`research/test-coverage.md`, and `user-journey.md`. Ranked by how much each blocks trusting
the feature to "just work."

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

## 2. Feature flag is stored per-workspace, with no visible indicator when off

`config.GetFeatureFlag("backlog")` defaults to `false`, keyed by `SHA256(cwd)` in
`config.json` (`config/config.go:805-824`). This checkout already has it enabled — but any
other workspace directory (a different clone, a different worktree, a different machine
entirely) starts with it unset. When off: the frontend redirects away from `/backlog` and hides
the nav item; the backend interceptor returns `CodeNotFound` for backlog RPCs. **No error, no
banner — it just looks like the feature doesn't exist.** There is no env var escape hatch, only
the `UpdateFeatureFlag` RPC (reachable via a UI toggle, if you know to look for it). This fully
explains "never seen it work" if the flag was never toggled in whatever workspace was used on
the other machine — verify this first, it's the cheapest thing to rule out.

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

## 6. Linux install path is more fragile than macOS for finding `claude`/`tmux`/`git`

The macOS LaunchAgent plist explicitly appends Homebrew + system fallback paths; the Linux
systemd unit bakes in a raw `$PATH` snapshot from install time with no fallback.
`session/headless/caller.go:36` does a bare `exec.LookPath("claude")`. If that snapshot goes
stale (nvm/asdf reinstall, PATH change since `make install-service`), the headless pool goes
nil (`server/dependencies.go:452-464`, log-warn only) and backlog triage silently no-ops.
**Actionable check on this machine**: confirm `journalctl --user -u stapler-squad` has no
"headless pool" warnings, and that `claude`/`tmux`/`git` resolve the same way inside the
systemd unit's environment as in an interactive shell.

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

1. Verify #2 (flag state) on the machine where it's never worked — 5 minutes, rules out the
   cheapest explanation. (#1's original hypothesis is now ruled out — see above.)
2. ~~Add one real e2e/integration test~~ — **done**:
   `TestBacklogFullLifecycle_TriageApprovalSpawn_CarriesRealPromptContent` in
   `server/services/backlog_service_test.go` now covers create→triage→approve→spawn with the
   real production code path, faking only the LLM/subprocess boundary. It passes today,
   which rules out #1 but does not by itself explain the "never worked here" report — the
   remaining candidates are #2 (flag) and unverified real-tmux/real-Claude behavior this test
   deliberately doesn't cover.
3. Decide deliberately on #3 (GitHub sync): finish it or explicitly cut it and remove the
   half-built RPC surface — right now it's neither shipped nor absent.
4. ~~Harden #5 (triage JSON parsing)~~ — **done**: replaced the naive brace-scan with a
   balanced-brace, string-literal-aware scanner in `session/backlog_triage.go`; 4 new
   adversarial regression tests in `session/backlog_triage_test.go`.
5. Linux PATH robustness (#6) — cheap, worth doing opportunistically.
6. If #1-#5 don't explain it, the next real test to write is one that actually exercises a
   live tmux session + live `claude` process (like the existing `harness`-tagged test, but
   extended past triage into execution) — that's the one class of bug this audit's tests
   structurally cannot see.
