# Structural hotspot ranking (`/sdd:fix-hotspot`)

Ledger for the `sdd:fix-hotspot` maintenance workflow (complexity × churn hotspot fixes). Written
to disk so the ranking survives a `/clear` or a fresh session — see `code-hotspot-analysis` skill
for the underlying technique (Adam Tornhill / CodeScene's method).

**Do not recompute from scratch on every run.** Resume from the next `pending` row. Only
regenerate the whole table when the repo has changed enough (a big refactor, a new hot area) that
this snapshot looks stale — note the regeneration date below when that happens.

## Methodology (2026-09-12 snapshot)

- **Complexity**: `gocyclo -avg .`, summed per file, excluding `_test.go`, `gen/`, `*.pb.go`,
  `*.connect.go`, and `session/ent/` (generated).
- **Churn**: `git log --oneline -800 -- <file> | wc -l` on `main` as of this branch's fork point —
  commits touching the file within the most recent 800 repo-wide commits.
- **Score**: complexity sum × churn count. A rough ranking signal, not a precise metric — ties or
  near-ties (rows within ~20% of each other) are interchangeable; use judgment (e.g. skip a file
  mid-migration elsewhere) rather than following the score to the letter.
- Prior ad hoc figures quoted in commit messages before this ledger existed (e.g. "66 commits in
  the last 800" for `session_service.go`) may not exactly match the churn counts here — they used
  an unrecorded, possibly narrower windowing method. The ledger's own numbers are the ones to trust
  going forward.

## Ranking

| # | File | Complexity sum | Churn (800) | Score | Status |
|---|------|----------------:|------------:|------:|--------|
| 1 | `server/services/session_service.go` | 945 (was 1025) | 219 | ~207k | **in-progress** — `StreamTerminal` (gocognit 207, gocyclo 74 — highest single function in the repo) extracted to `session_service_stream_terminal.go`, pure file move, no behavior change. Commits [`793f5c93b`](https://github.com/tstapler/stapler-squad/commit/793f5c93b), [`d6333a2e8`](https://github.com/tstapler/stapler-squad/commit/d6333a2e8) (dupl-gate false positive fix). File is still the largest in the repo — more extractions likely worth a future pass. |
| 2 | `session/tmux/tmux.go` | 439 | 149 | ~65k | **done** — `start()` (gocyclo 31, 272 lines) and `RestoreWithWorkDir()` (gocyclo 22, 170 lines), the file's two most complex functions, plus their thin wrappers (`Start`, `StartWithCleanup`, `Restore`) and remote-host equivalent (`EnsureRemoteSession`, `createRemoteSession`, `remoteHasSession`) — one contiguous, self-referencing block — extracted to `tmux_session_start.go`, pure file move. Commit [`de8fad7a1`](https://github.com/tstapler/stapler-squad/commit/de8fad7a1) (gocognit/gocyclo new-code-gate suppressions included in the same commit). |
| 3 | `server/services/connectrpc_websocket.go` | 507 | 107 | ~54k | **done** — `streamShellViaControlMode` (gocyclo 16, the file's highest-complexity function) and its exclusive helpers (`forwardShellControlModeOutput`, `sendShellControlModeOutput`, `runShellControlModeResizeCoalescer`, `applyOneShellResize`, `sendShellPostResizeSnapshot`, `sendShellResizeQuiescence`, `runShellInputReadLoop`, `readOneShellFrame`, `readShellWebSocketMessage`, `handleShellInput`, `dispatchShellResize`, `handleShellScrollbackRequest`, `handleShellMidStreamCurrentPaneRequest`, plus their param structs) extracted to `connectrpc_websocket_shell.go`, pure file move. Helpers shared with the main-terminal/capture-pane paths (`parseInputFrameOrStop`, `dispatchResizeRequest`, `buildScrollbackResponse`, `panePTY`) stayed in the original file since they're used outside this block. Commit [`9e5b43adc`](https://github.com/tstapler/stapler-squad/commit/9e5b43adc). |
| 4 | `server/services/backlog_service_triage.go` | 566 | 94 | ~53k | **partially done** — `TriggerTriage`/`CancelTriage` (+ exclusive helpers: `findPriorTriageResult`, `applyTriageResultToUpdate`, `cleanupProvisionalTriageWorktree`, `retitleTriageWorktreeToFinalBranch`, `sanitizeTriageTitle`, `notifyTriagePersistFailure`, the `testTriageCompleteHook` pair) extracted to `backlog_service_trigger_triage.go`, pure file move. Commit [`601b4550e`](https://github.com/tstapler/stapler-squad/commit/601b4550e) (gocognit new-code-gate suppression included in the same commit this time). `TriggerReReview` (460 lines, a long linear synchronous handler — not the same async-goroutine shape) deliberately left in place; extracting it is a follow-up, not required before moving to row 2/3. |
| 5 | `server/mcp/tools_backlog.go` | 514 | 51 | ~26k | pending |
| 6 | `session/ent_repository.go` | 331 | 70 | ~23.2k | pending |
| 7 | `session/ent_repository_backlog.go` | 429 | 54 | ~23.2k | pending |
| 8 | `session/unfinished/gogit_vcs_reader.go` | 307 | 38 | ~11.7k | pending |

Note: rows 2–3 score higher than row 4 by this snapshot. Row 4 was already selected and diagnosed
in-session before this ledger existed (continuing a prior conversation's verbal hand-off); it's
being finished rather than abandoned mid-diagnosis, but the next `/sdd:fix-hotspot` run after it
should treat rows 2–3 as higher priority, not resume sequentially down this table.

## Regrowth gate

Already in place repo-wide (not per-file) since the row-1 work: `.golangci.yml`'s
`gocyclo`/`gocognit`/`funlen`/`revive`(file-length-limit)/`dupl` settings, enabled in
`.github/workflows/lint.yml` via `--new-from-rev=origin/main` so only new violations in a diff's
own changed code fail CI. See that file's inline comment for the full rationale
(`session/backlog_lifecycle.go`'s 4930-line/91-method growth is the incident that motivated it).
No further gate work needed per hotspot fix unless a fix reveals the thresholds themselves need
tuning.
