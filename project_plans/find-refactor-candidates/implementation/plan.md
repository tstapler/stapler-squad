# Find Refactor Candidates — Ranked Inventory and Dispatch Plan

Measured 2026-10-17 at base commit `b829df92e`. This plan is analysis only; it implements no refactor.

## 1. Scoring model (AC 4)

`score = lines × commits`, where `lines = wc -l` of the file today and `commits` = number of commits touching it in the last 6 months. Reproduce:

```bash
git log --since="6 months ago" --name-only --format= -- '*.go' '*.ts' '*.tsx' \
  | grep -v -E '_test\.|\.pb\.|/gen/|session/ent/[a-z]' | sort | uniq -c | sort -rn \
  | awk '$1>=25{print $1, $2}' | while read c f; do [ -f "$f" ] && echo "$c $(wc -l <$f) $((c*$(wc -l <$f))) $f"; done | sort -k3 -rn
```

Excluded: tests, protobuf/generated code, ent-generated code, files under 25 commits. Caveats: the score is a triage filter, not a verdict — it cannot tell a stable well-factored domain from a god file, so each pick below also records the distinct concerns found by reading the file's declarations. Commit count includes feature work and merges of unrelated changes; it is churn, not bug density.

## 2. Ranked list (AC 1, 2)

Plan classification: **existing plan** = a dedicated refactor plan names the file as a work target. Mention counts below are `grep -F` hits inside that plan's directory and are a coarse signal only; items marked `†` were not read to confirm the plan targets the file.

| # | File | Lines | Commits (6 mo) | Score | Class | Plan / note |
|---|---|---:|---:|---:|---|---|
| 1 | `server/services/backlog_service_triage.go` | 3800 | 121 | 459,800 | existing plan | `backlog-service-refactor` † (11 hits) |
| 2 | `session/instance.go` | 2771 | 135 | 374,085 | existing plan | `backend-di-refactor` † (11), `core-domain-decomposition` † |
| 3 | `server/services/connectrpc_websocket.go` | 3857 | 95 | 366,415 | **new** | **RC-01** |
| 4 | `session/tmux/tmux.go` | 3081 | 102 | 314,262 | **new** | **RC-02** |
| 5 | `server/server.go` | 2198 | 141 | 309,918 | existing plan † | `backlog-service-refactor` (6 hits, scope unconfirmed) |
| 6 | `session/backlog_lifecycle.go` | 2441 | 120 | 292,920 | **new (residual)** | **RC-03** — overlaps `backlog-service-refactor` on `spawnReviewGate` only |
| 7 | `server/services/session_service.go` | 1268 | 229 | 290,372 | new, unconfirmed | no split plan; needs a structural pass before dispatch (see §6) |
| 8 | `server/dependencies.go` | 2039 | 134 | 273,226 | existing plan † | `backend-di-refactor` (13 hits) |
| 9 | `server/mcp/tools_backlog.go` | 3235 | 77 | 249,095 | existing plan † | `backlog-service-refactor` (23 hits) |
| 10 | `session/ent_repository_backlog.go` | 3186 | 68 | 216,648 | existing plan † | `backlog-service-refactor` (29 hits) |
| 11 | `config/config.go` | 1989 | 108 | 214,812 | new | |
| 12 | `session/storage.go` | 1781 | 114 | 203,034 | existing plan † | `backend-di-refactor`, `session-path-domain-refactor` (few hits) |
| 13 | `server/services/backlog_service.go` | 1505 | 124 | 186,620 | existing plan | `backlog-service-refactor` P1 (`requirements.md:69`) |
| 14 | `web-app/.../sessions/TerminalOutput.tsx` | 2438 | 75 | 182,850 | existing plan † | `front-end-refactor` (7 hits) |
| 15 | `session/ent_repository.go` | 2271 | 79 | 179,409 | new | |
| 16 | `web-app/.../backlog/BacklogItemDetail.tsx` | 1913 | 83 | 158,779 | new | next-wave (RC-04) |
| 17 | `web-app/.../sessions/Omnibar.tsx` | 2251 | 60 | 135,060 | new | |
| 18 | `session/unfinished/gogit_vcs_reader.go` | 2278 | 49 | 111,622 | new | |
| 19 | `session/instance_tmux.go` | 1680 | 66 | 110,880 | new | |
| 20 | `main.go` | 1909 | 55 | 104,995 | new | |
| 21 | `web-app/src/lib/hooks/useBacklogService.ts` | 1611 | 65 | 104,715 | new † | next-wave (RC-05); 6 hits in `backlog-service-refactor` unconfirmed |
| 22 | `web-app/.../sessions/SessionCard.tsx` | 1312 | 71 | 93,152 | new | |
| 23 | `web-app/.../sessions/SessionDetailView.tsx` | 1981 | 46 | 91,126 | new | |
| 24 | `web-app/src/lib/hooks/useSessionService.ts` | 1442 | 60 | 86,520 | new | next-wave (RC-05) |
| 25 | `web-app/.../sessions/SessionList.tsx` | 1613 | 49 | 79,037 | new | |

Dispatch only the **new** rows. Rows 16, 21, 24 (RC-04/RC-05) are queued behind RC-01–03. Note `core-domain-decomposition` mentions `connectrpc_websocket.go` and `tmux.go` only to say its change does not touch them (`plan.md:127`, ADR-003), so RC-01 and RC-02 are genuinely unaddressed.

## 3. Risk classes (AC 5)

| Candidate | Class | Why |
|---|---|---|
| RC-01 | Same-package file split — **low** | Moves declarations within `server/services`; no import-path or signature change. |
| RC-02 | Same-package file split — **low** | Moves declarations within `session/tmux`; unexported helpers stay visible. |
| RC-03 step A | Same-package file split — **low** | Setter/getter wiring and reconcilers move within package `session`. |
| RC-03 step B (optional) | Cross-package/interface extraction — **moderate** | Pulling the reconcilers behind an interface so they can be unit-tested without a full `BacklogLifecycleListener`; only after step A, and only if the overlap gate (§5) is clear. |

A same-package split improves review surface and merge-conflict rate, not testability. Interface extraction is a separate, later PR.

## 4. Dispatch seeds (AC 3)

### RC-01 — Split `server/services/connectrpc_websocket.go` (3,857 lines, 95 commits)

The file holds one `ConnectRPCWebSocketHandler` plus roughly 150 declarations covering at least six separable concerns, located by reading its declarations: (a) the stream hub — `hubRegistry` (L432), `pumpControlModeOutputIntoHub` (L561), `ensureHubInstanceStarted` (L1922), `hubSubscriberAttachment` (L2002), `sendHubInitialSnapshot` (L2056); (b) snapshot/alt-screen handling — `sessionSnapshot` (L326), `altScreenActiveForSnapshot` (L216), `invalidateSnapshot` (L770), `buildInitialScrollbackResponse` (L1277); (c) control-mode resize coalescing and output forwarding — `controlModeResizeCoalescerParams` (L1498) through `forwardOneControlModeFrame` (L1696); (d) scrollback and capture-pane request handlers — `buildScrollbackResponse` (L2294), `handleCurrentPaneRequest` (L2628), `writeAppScrollbackResponse` (L2985), `handleBatchedCurrentPaneRequestFrame` (L3158), `readOneCapturePaneFrame` (L3431); (e) input reading and tmux send retry — `inputReadLoopParams` (L2716), `sendInputToTmuxWithRetry` (L3702); (f) the shell-pane PTY adapter — `panePTY` (L2322), `shellPanePTY` (L2404). The core file keeps the handler struct, `HandleWebSocket` (L861) and `streamTerminal` (L977). Move each group to a `connectrpc_ws_<concern>.go` file next to the existing `connectrpc_websocket_shell.go`, with no signature or behavior change. First PR also deletes the two tracked leftovers `connectrpc_websocket.go.bak2` and `.before_cursor_fix`. Line numbers will drift; re-locate with `grep -n '^func\|^type'`. Gate per PR: `go build ./... && go test ./server/services -run 'WebSocket|Hub|Scrollback' -timeout=20m`, then `make lint` (custom analyzers). Do not touch the raw-byte scrollback path's behavior (`core-domain-decomposition` ADR-003 treats it as out of scope).

### RC-02 — Split `session/tmux/tmux.go` (3,081 lines, 102 commits)

`tmux.go` holds the `TmuxSession` struct (L42) and 107 functions spanning: socket and server resolution (`Socket` L598, `resolveSocket` L679, `IsServerDown` L457, `CreateKeepaliveSession` L915); session-name handling (`SessionName` L387, `ToStaplerSquadTmuxName` L364); functional options (`TmuxSessionOption` L1048, `WithCommandRunner` L1064); input helpers (`TapDAndEnter` L1344); PTY/winsize handling (`lockedPTMX` L1725, `ClampWinsizeDim` L1939); the exists-cache (`existsCacheState` L350, `invalidateExistsCache` L2260); pane capture and dimensions (`CapturePaneContentRawPriority` L2547, `GetPaneDimensionsPriority` L2787); and a `statusMonitor` (L1311). Sibling files already exist (`tmux_session_start.go`, `tmux_ownership.go`, `pty.go`, `control_mode.go`), so the target layout follows that grain: `tmux_socket.go`, `tmux_options.go`, `tmux_pty.go`, `tmux_capture.go`, `tmux_exists_cache.go`, `tmux_status_monitor.go`, leaving `TmuxSession` and core lifecycle in `tmux.go`. No exported name changes. Gate per PR: `go build ./session/... && go test ./session/tmux/... -timeout=20m` and `make lint`. Platform note: `tmux_unix.go`/`tmux_windows.go` have build tags — confirm with `GOOS=windows go vet ./session/tmux/` that moved code keeps its tags.

### RC-03 — Reduce `session/backlog_lifecycle.go` (2,441 lines, 120 commits) — OVERLAP FLAGGED

The type is `BacklogLifecycleListener`, not a "coordinator". Siblings already exist (`_archive`, `_gates`, `_noop`, `_pr`, `_review`, `_stale`, `_triage`), so the residual file is two blocks: **(1) ~520 lines of dependency wiring** — about 25 `SetX`/`getX` pairs (L283–L780: workflow engine, gate repos, spawners, notifier, archiver, cleaner, PR factories) plus `notify*` helpers; and **(2) ~1,300 lines of stuck-state reconcilers** — `BackfillStuckStates` (L1150), `ReconcileStuck` (L1251, ~300 lines on its own), `reconcileBouncingItems` (L1893), `selfHealStuck` (L2145), `reconcileMultiReasonEscalation` (L2254) and helpers. Proposed: move block 1 to `backlog_lifecycle_wiring.go` and block 2 to `backlog_lifecycle_reconcile.go` (same package). `ReconcileStuck` and `reconcileBouncingItems` are long enough that a later complexity-driven extract-function pass is a separate task. **Overlap:** `backlog-service-refactor` `implementation/plan.md:324` extracts `spawnReviewGate` from this file into `session/review_gate.go` (a `ReviewGateRunner` now exists there, and `spawnReviewGate` itself now lives in `backlog_lifecycle_review.go:380`, so that task looks at least partly landed), and the same plan still lists goals touching this file: removing direct ent references for ItemSession fields (`plan.md:452`) and lowering cognitive complexity (`plan.md:708`). RC-03 must not touch `spawnReviewGate` or ent-reference cleanup the plan still owns. Gate: `go build ./... && go test ./session/... -timeout=20m` and `make lint`.

## 5. Pre-dispatch gate for RC-03 (AC 7)

Before creating or starting RC-03:
1. Read `project_plans/backlog-service-refactor/implementation/plan.md` L324–L360, L452 and L708 and list which of its tasks still have unmerged work touching `session/backlog_lifecycle.go` (`git log --oneline -- session/review_gate.go session/backlog_lifecycle.go`; `grep -n "ent\." session/backlog_lifecycle.go`). As of this analysis `ReviewGateRunner` exists in `session/review_gate.go`; the plan header still reads "Ready for implementation" (`plan.md:5`), so its remaining status is unknown.
2. If any such task is unmerged, schedule RC-03 **after** it, or exclude those regions and rebase onto it; never run both concurrently (both edit this file; `backlog_lifecycle` co-changes with `backlog_service` at ratio 0.71 per `requirements.md:13`).
3. If that plan has since broadened to relocate more of this file, merge RC-03 into it rather than dispatching separately.

The earlier draft of this plan said the file "is not explicitly addressed" by that plan; that was wrong (`plan.md:324,346,452,666,708`).

## 6. Tasks and PR sizing (AC 6)

A file split shows each moved line twice in a diff (delete + add), so one PR moves **≤ ~300 lines**. Each task below is one PR; estimates are diff size.

| Task | Scope | Diff | Gate |
|---|---|---:|---|
| T1 | RC-01 PR0: delete 2 tracked `.bak` files; extract (f) shell-pane PTY adapter | ~350 | `go build ./... && go test ./server/services -timeout=20m` |
| T2 | RC-01: extract (a) hub group | ≤600 | same + `-run Hub` |
| T3 | RC-01: extract (b) snapshot/alt-screen | ≤600 | same |
| T4 | RC-01: extract (c) resize coalescer + output forwarder | ≤600 | same |
| T5 | RC-01: extract (d) scrollback/capture-pane handlers (split in two PRs if > 300 lines) | ≤600 each | same |
| T6 | RC-01: extract (e) input loop + send retry | ≤400 | same |
| T7–T10 | RC-02: socket+options, PTY, capture+exists-cache, status monitor — one PR each | ≤600 | `go build ./session/... && go test ./session/tmux/...`, `GOOS=windows go vet ./session/tmux/` |
| T11 | RC-03 gate check (§5); record result in PR description | docs | — |
| T12 | RC-03: extract wiring block | ≤600 | `go build ./... && go test ./session/...` |
| T13 | RC-03: extract reconcile block (two PRs if > 300 lines) | ≤600 each | same |
| T14 | `session_service.go` structural pass: list its concerns; promote to RC-06 or drop | docs | — |
| T15 | After merges, re-run §1 command; confirm split files fall out of the top 10 | docs | — |

PR line budgets are estimates from declaration positions, not measured; the first PR of each candidate should re-measure and re-slice. Every PR also runs `make lint` and the `gofmt -w` rule from `CLAUDE.md`.

## 7. Limits of this analysis

- Existing-plan classification relies on mention counts except where cited by line; `†` rows need a read before anyone treats them as covered.
- Frontend rows 14–17, 21–25 were ranked from the score only; no structural read was done, so RC-04/RC-05 sketches from earlier drafts were removed rather than carried forward unverified.
- 6-month churn misses files that are large but dormant; any file over 2,000 lines with fewer than 25 commits is not listed.
