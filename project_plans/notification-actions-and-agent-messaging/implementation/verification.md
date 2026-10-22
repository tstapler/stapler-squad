# Verification Report: notification-actions-and-agent-messaging

**Date**: 2026-10-19
**Range**: e6e815dd2..5f342b323 (Phase 5 milestones 1-4), plus verification fixes ae674e81f, bebcc18b9, 968cd12d3
**Method**: three independent read-only review lenses (wake concurrency; docs/observability; security/UX/test quality), then fix workers in isolated worktrees, cherry-picked back.
**Out of scope by instruction**: Phase 7, push, PRs, flag flips, `spikeS1HookReplyPassed` / `spikeS6TierBPassed` (owner review pending).

## Verdict

**PASS WITH RESIDUALS.** No Critical findings. One High (cross-session answer leak) and the cheap Mediums are fixed with tests. Remaining Mediums are listed as residual risks. Shipping behind the existing flags is reasonable once the owner decides the open questions below.

## Findings

| # | Severity | Finding | Status |
|---|----------|---------|--------|
| 1 | High | `schedule_wake(guidance_answered / session_state)` was not bound to the caller: a session knowing another's request or session UUID could receive that answer text (`server/wakes/sources_pr.go` checkSource). | FIXED bebcc18b9: asker must equal caller; `session_state` target must be self or same non-empty WorkspaceKey; uniform `ErrForbidden`; negative and positive tests in `server/wakes`. |
| 2 | Medium | Identity role lookup failed open to `roleWork` on any storage error (`server/mcp/caller_identity_gate.go`). | FIXED bebcc18b9: only `ErrNotFound` maps to work; other errors give `roleUnknown` (denied). Side effect: `list_wakes`/`request_help` also denied on transient error. |
| 3 | Medium | UX-AC55: Needs-you chip never rendered (`NotificationPanel` did not pass `needsYou`); unit test passed the prop directly. | FIXED 968cd12d3: panel computes per-session counts, chip is a 44px-coarse button that switches to Notifications and focuses the item; panel-level jest test; e2e row `1.3-failure-and-decision` moved MANUAL_ONLY to COVERED. |
| 4 | Medium | Flag-OFF mirror path ran `sessionUUIDFor` (a `ListInstanceData` scan) on every question hook before `Mirror` checked the flag. | FIXED ae674e81f: `GuidanceHookMirror.Enabled()` short-circuits in `mirrorQuestion`. |
| 5 | Medium | `processNeedsRevive` acted on a stale snapshot; a cancelled wake could still revive its owner. | PARTLY FIXED ae674e81f: row re-read before `TryRevive`. Window while waiting on `reviveMu` inside `TryRevive` remains (see residuals). |
| 6 | Medium | Flag OFF is not byte-equivalent on the hook/notification path: question ordering (`writeDeferDecision` before broadcast), `RedactSecrets` on every question message, `metaToolInputUnrenderable`, 64KB PostToolUse body parse, `ObservePostToolUse`/`observeScheduleWakeup` unconditionally. | OPEN, needs owner decision: either gate these or state in ADR-008 that "flag OFF" means "no durable row", not "identical hook path". |
| 7 | Medium | TOCTOU between tier decision (`RouteFor`) and `ReplyToQuestion` re-read: a record-route answer could become a live keystroke without the Tier B remote gate. Narrow; remote flags still apply. | OPEN. Fix: pass resolved route into `QuestionAnswer` and STALE on mismatch. |
| 8 | Medium | `ExpireOverdue` sweep (via ListWakeTriggers) does not increment `wake_trigger_total{state=missed}` and skips index/digest; CAS prevents double alert or double fire (VERIFIED by reading `transitionWake`). | OPEN: metric undercount for event rows first expired by a UI list. Dispatcher is built after the service so metrics are not reachable at registration. |
| 9 | Medium | `storeAnswer` (record route) does not validate answer against question options, unlike `hook_reply.go:99`. Operator-originated. | OPEN. |
| 10 | Medium | e2e exit assertions only check visible+enabled, never click; 3 rows remain MANUAL_ONLY (canary, multi-select unmeasured, unrenderable options). Per-AC matrix for 88 UX-ACs not produced (25 AC ids tagged). | OPEN. |
| 11 | Low-Med | Doc claim wrong: observability doc says an already-open terminal stream yields no reopen count; a websocket re-subscribe within 60s does count. Reopen tracker also matches by title or id, misses restarts, covers ANSWER_QUESTION only (requirements.md:52 implies approvals). | OPEN (doc fix + known limits). |
| 12 | Low | `reviveWaiting` spawns a goroutine per needs_revive row per Reconcile tick without in-flight dedupe; shutdown releases leader flock before joining workers; `startAndAwaitLive` goroutine untracked; `Start` double-close if reused; `wake_fires_per_session` samples unbounded and zero-heavy; Wait for a Starting owner is bounded only by `expires_at` (no BusyHold-style timeout). | OPEN, none reachable as a correctness failure today. |
| 13 | Low | `escapeWakePayload` leaves C1 controls; proof has no nonce/expiry (documented accepted risk); background row title toggle 24px with no coarse-pointer override; stale note that `inline_action_undo_total` has deny/answer_question values. | OPEN. |

Focus-area answers:
1. ExpireOverdue vs dispatcher: CAS (`transitionWake`, `ClaimWakeTrigger` refuses expired, `missed` terminal) prevents double report/fire. Only a metric gap (#8).
2. Mirror gate: gated at `stage:render` per call, live flag. Not byte-equivalent otherwise (#6).
3. Starting owner: not held forever; bounded by `expires_at`, which is always set at creation. Earlier alert would need a Wait timeout (#12).
4. Reopen tracker: no double counting (entry deleted on first hit); inflation by reconnects; see #11.
5. `WithQualityMetrics` cardinality: VERIFIED bounded (kind clamped to 4 values, enum-derived labels, role from bounded set). No finding.
6. `wake_fires_per_session`: in-process, doc matches code (#12 for the unbounded slice).
7. Docs: all 20 implemented metric names exist; the two "not implemented" series are indeed unregistered; flag-flip is RPC/panel only, no env var (consistent with owner preference). Wrong claim in #11.
8. Needs-you chip: fixed (#3).
9. ScheduleWakeup baseline (reported, not decided): requirements.md:43 still says matcher coverage is UNVERIFIED, but spike-S7.md:79 shows the matcher fires. Nothing injects `post_tool_logging` by default (only `tools_lifecycle.go:243` caller-supplied, `tools_github.go:273` nil, `backlog_service_triage.go:1145` git-drift only), so `agent_schedule_wakeup_calls_total` and the `schedule_wakeup_receipt` log only fire for sessions that opt into that hook. Baseline row reads NOT CAPTURED; `scripts/wake-baseline.sh` refuses under 7 days or with the flag on. Whether to drop flip criterion (iv) is the owner's call. The `answered_in_terminal` clear shares the same hook dependency. Commit f242e0e57 puts the receipt counter in the polling-reference allow-list.
10. Security: forged header/proof (HMAC, `hmac.Equal`, real-middleware test asserts zero writes), proof files 0600/0700 temp+rename, Tier B remote refusal (unstamped treated as remote, zero pane writes asserted across 5 cases), pane injection (single digit byte; wake text stripped of control bytes), `revive.go` in noarchivedrevival scope (analyzer clean, `IsArchived()` right before `RecoverFromStopped`/`Start(false)`; rule doc already lists it). Sound except #1, #2, #7.

## Test evidence (post-fix, HEAD 968cd12d3)

- `make lint`: custom lint ok, shellcheck ok, golangci-lint "0 issues".
- `go test -race ./server/...`: 19 packages ok, 0 FAIL, 0 `DATA RACE` lines (output in the verification run, not committed).
- `pnpm exec tsc --noEmit`: exit 0.
- `pnpm exec jest --no-coverage`: 653 suites, 8165 tests passed.
- jscpd: 0.13% (limit 0.14%), per chip worker.
- Playwright (agent-wake-triggers, askuserquestion-structured, notification-inline-actions, ux-error-exits), full run at HEAD: 141 passed, 14 flaky (passed on retry), 3 failed (`askuserquestion-structured:193`, `notification-inline-actions:195` and `:249`). That run overlapped the `go test -race` and jest runs on the same machine, and the 3 failures were timeouts or a `?window=` URL assertion. Re-run in isolation on the same HEAD: all 3 passed (4.6m). Treated as load flakes, not regressions; the 14 flaky retries were not individually triaged. These specs are load-sensitive and should be run alone in CI-like conditions before ship.
- Worker-reported: `go test -race ./server/wakes/... ./server/mcp/...` ok (31.7s, 62.5s) at bebcc18b9; Playwright `ux-error-exits` 84/84 at 968cd12d3.

## Residual risks

Items #6-#13 above. Highest value next: decide #6 (ADR-008 wording or gating), pass the resolved route through the answer path (#7), and click-through e2e assertions (#10). Nothing here requires touching the spike gates.
