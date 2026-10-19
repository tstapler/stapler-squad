# Find Refactor Candidates — Research

## Method

Evidence was gathered from two sources:
1. **Static size**: `wc -l` on all non-test `.go` and `.ts/.tsx` files.
2. **Git churn**: `git log --since="6 months ago"` commit counts per file via `git diff-tree`.

A "double-hotspot" is any file that ranks in both the top-10 by size AND top-10 by churn — these carry the highest review friction and regression risk.

---

## Go Backend: Largest + Most-Churned Files

### Composite ranking (size × churn = impact proxy)

| Rank | File | Lines | 6-mo commits | Impact score |
|---|---|---|---|---|
| 1 | `server/services/backlog_service_triage.go` | 3,800 | 121 | **459,800** |
| 2 | `session/tmux/tmux.go` | 3,081 | 102 | **314,262** |
| 3 | `server/services/connectrpc_websocket.go` | 3,857 | 95 | **366,415** |
| 4 | `session/instance.go` | 2,771 | 135 | **374,085** |
| 5 | `session/backlog_lifecycle.go` | 2,441 | 120 | **292,920** |
| 6 | `session/ent_repository_backlog.go` | 3,186 | 68 | **216,648** |
| 7 | `server/services/session_service.go` | ~1,268 | 229 | **290,372** |
| 8 | `session/backlog_lifecycle_pr.go` | 2,232 | ~60* | **133,920** |
| 9 | `server/dependencies.go` | 2,039 | 134 | **273,226** |
| 10 | `server/server.go` | 2,198 | 141 | **309,918** |

*`backlog_lifecycle_pr.go` churn estimated from sibling file pattern; exact number not directly counted.

### Top double-hotspots (large AND high-churn)

1. **`session/instance.go`** (2,771 lines / 135 commits) — Core session lifecycle, tmux management, git worktrees, approval, serialization, terminal content, checkpoints, tags all in one file.
2. **`server/services/backlog_service_triage.go`** (3,800 lines / 121 commits) — Backlog state machine: spawn, dequeue, active conflict detection, rework cap, notifications.
3. **`server/services/connectrpc_websocket.go`** (3,857 lines / 95 commits) — WebSocket transport + snapshot caching + hub registry + memory pools + alt-screen tracking.
4. **`session/tmux/tmux.go`** (3,081 lines / 102 commits) — All tmux integration: session creation, control mode, input handling, PTY access helpers.
5. **`session/backlog_lifecycle.go`** (2,441 lines / 120 commits) — Backlog lifecycle events: completion detection, retries, escalation, state transitions.

### Files large but lower churn (structural debt, not active friction)

- `session/ent_repository_backlog.go` (3,186 lines / 68 commits) — Fat repository with all backlog CRUD, dependency graph, status events.
- `session/unfinished/gogit_vcs_reader.go` (2,278 lines) — VCS reader; churn not measured but the `unfinished/` placement suggests exploratory code.
- `server/mcp/tools_backlog.go` (3,235 lines) — MCP tool handlers for backlog; high line count but more likely stable API surface.

---

## Frontend: Largest + Most-Imported Files

| Rank | File | Lines | Import count |
|---|---|---|---|
| 1 | `web-app/src/components/sessions/TerminalOutput.tsx` | 2,438 | 46 |
| 2 | `web-app/src/components/sessions/Omnibar.tsx` | 2,251 | 39 |
| 3 | `web-app/src/components/sessions/SessionDetailView.tsx` | 1,981 | 49 |
| 4 | `web-app/src/components/sessions/ReviewQueuePanel.tsx` | 1,921 | 26 |
| 5 | `web-app/src/components/backlog/BacklogItemDetail.tsx` | 1,913 | **72** |
| 6 | `web-app/src/components/sessions/XtermTerminal.tsx` | 1,724 | — |
| 7 | `web-app/src/components/sessions/SessionList.tsx` | 1,613 | — |
| 8 | `web-app/src/lib/hooks/useBacklogService.ts` | 1,611 | — |
| 9 | `web-app/src/lib/hooks/useSessionService.ts` | 1,442 | — |
| 10 | `web-app/src/components/ui/NotificationPanel.css.ts` | 1,334 | — |

**Key observations:**
- `BacklogItemDetail.tsx` has 72 imports — the highest in the codebase — indicating it has become a god component absorbing most business logic.
- `SessionDetailView.tsx` (49 imports) is the session equivalent.
- `useBacklogService.ts` and `useSessionService.ts` are large service hooks mixing API calls, local state, and side effects — prime candidates for split into smaller single-responsibility hooks.
- The `sessions/` component directory contains 7 of the top 10 largest files; it is the dominant frontend hotspot.

---

## Existing Refactor Plans — Status

| Plan | Files | Status | What it covers |
|---|---|---|---|
| `backend-di-refactor` | req + impl/plan + impl/validation | **Active** | Warren DI validators, `session/instance.go` split, interface extraction |
| `backlog-service-refactor` | req + full impl (plan, adversarial, arch, pre-mortem, validation) + 2 ADRs | **Most mature** | `backlog_service.go` decomposition, dependency injection |
| `detection-architecture-refactors` | req + impl (plan, adversarial, validation) | **Active** | StatusDetector decomposition, plugin registry |
| `session-path-domain-refactor` | req + impl/plan + impl/validation + 1 ADR | **Active** | Session path vocabulary (ActiveDir vs ExistingDir vs RepoRoot) |
| `front-end-refactor` | req + 5 ADRs, **no impl** | **Design-only** | CSS standardization, component library, mobile parity |

### Gap: unaddressed high-impact candidates

The following double-hotspots have **no existing dedicated plan**:

| Candidate | Evidence | Why it matters |
|---|---|---|
| `server/services/connectrpc_websocket.go` | 3,857 lines / 95 commits | WebSocket + snapshot + hub registry + memory pools — 4 distinct concerns in one file; any WebSocket bug requires reading the whole thing |
| `session/tmux/tmux.go` | 3,081 lines / 102 commits | All tmux logic in one file; every session lifecycle change touches it; impossible to test sub-behaviors in isolation |
| `session/backlog_lifecycle.go` + `_pr.go` | 4,673 combined / 120 commits (lifecycle) | The PR split already happened structurally but `lifecycle.go` still absorbs completion detection, retries, escalation, notifications together |
| `BacklogItemDetail.tsx` | 1,913 lines / 72 imports | Highest import count in frontend; reviews are full-file reads; adding new detail sections requires touching a single giant file |
| `useBacklogService.ts` / `useSessionService.ts` | 1,611 + 1,442 lines | Service hooks that mix API plumbing, optimistic updates, derived state, and side effects — untestable as units |

---

## References

- Go size data: `wc -l` run over non-test `.go` files in `session/` and `server/`
- Go churn data: `git log --since="6 months ago"` with `git diff-tree --name-only`, counted per file
- Frontend size data: `wc -l` over non-test `.tsx`/`.ts` files in `web-app/src/`
- Existing plans: directory listing of `project_plans/` + `ls` of each subdirectory
- Existing `backend-di-refactor` requirements: `project_plans/backend-di-refactor/requirements.md`
