# Find Refactor Candidates — Implementation Plan

## Phase 1: Inventory output (this document)

This plan produces the ranked candidate list required by requirements.md Goals G1–G3. It does not implement any refactor; it prepares each top candidate for dispatch.

---

## Ranked Candidate List

### Scoring model

`impact = lines × 6-month-commits`. Files scoring ≥200,000 are high-priority; 100,000–200,000 are medium; below are backlog.

### Tier 1 — High priority, no existing plan

These candidates have no dedicated project plan and high composite scores. Dispatch them in this order.

#### RC-01: Split `server/services/connectrpc_websocket.go`

- **Score**: ~366,000 (3,857 lines × 95 commits)
- **Current state**: Single 3,857-line file mixing WebSocket framing, origin validation, snapshot invalidation + refresh coordinator, `hubRegistry` (stream hub multiplexing), control-mode stream pumping, memory pool manager for hot-path proto serialization, and alt-screen tracking.
- **Proposed split** (same package, no API changes):
  - `connectrpc_websocket.go` — `HandleWebSocket` dispatch, origin validation, basic request routing (~400 lines)
  - `connectrpc_ws_hub.go` — `hubRegistry`, stream coalescing, multi-subscriber fan-out (~600 lines)
  - `connectrpc_ws_snapshot.go` — snapshot invalidation, refresh coordinator, alt-screen tracking (~700 lines)
  - `connectrpc_ws_pool.go` — memory pool manager, proto serialization helpers (~400 lines)
- **Risk**: Low — same-package file split. Exported types stay at same import paths. Behavior unchanged.
- **Test gate**: `go build ./server/...` + existing `server/services` tests pass unchanged.

#### RC-02: Split `session/tmux/tmux.go`

- **Score**: ~314,000 (3,081 lines × 102 commits)
- **Current state**: All tmux integration in one file: session creation + attachment, control-mode protocol handling, PTY access helpers, input batching, resize handling, alt-screen detection, scrollback capture.
- **Proposed split** (same `session/tmux` package):
  - `tmux.go` — session create/attach/kill, `TmuxSession` struct, core lifecycle (~500 lines)
  - `tmux_control_mode.go` — control-mode protocol: start, pump, parse output panes (~700 lines)
  - `tmux_pty.go` — PTY access, input batching, resize handling (~600 lines)
  - `tmux_scrollback.go` — scrollback capture, alt-screen detection, capture-pane logic (~500 lines)
- **Risk**: Low — same-package file split.
- **Test gate**: `go build ./session/tmux/...` + existing tmux tests pass.

#### RC-03: Decompose `session/backlog_lifecycle.go`

- **Score**: ~293,000 (2,441 lines × 120 commits)
- **Current state**: 2,441-line file absorbing: completion signal detection, retry state machine, escalation triggers, notification dispatch (rework cap, failure, blocked, flaky), and the `BacklogLifecycleCoordinator`. `backlog_lifecycle_pr.go` (2,232 lines) handles the PR creation side but the two files remain tightly coupled.
- **Proposed split**:
  - `backlog_lifecycle.go` — `BacklogLifecycleCoordinator` struct + constructor + `HandleEvent` dispatch (~500 lines)
  - `backlog_lifecycle_completion.go` — completion detection, success conditions, done signals (~600 lines)
  - `backlog_lifecycle_retry.go` — retry state machine, backoff policy, rework cap enforcement (~500 lines)
  - `backlog_lifecycle_notifications.go` — all notification-dispatch functions (~600 lines)
- **Risk**: Low for file split. Moderate if retry/completion coupling requires interface extraction to fully test.
- **Test gate**: Existing `session/...` tests pass + `make build`.

#### RC-04: Decompose `BacklogItemDetail.tsx`

- **Score**: High (72 imports is the codebase maximum; 1,913 lines)
- **Current state**: God component with 72 imports spanning sections for metadata, description editing, GitHub PR linking, acceptance criteria, sub-tasks, status history, cost breakdown, and tag management — all rendered and mutated in a single file.
- **Proposed split**:
  - `BacklogItemDetail.tsx` — layout shell + router between tab sections (~300 lines)
  - `BacklogItemDetailHeader.tsx` — title, status, priority, actions bar (~200 lines)
  - `BacklogItemDetailDescription.tsx` — description editor + markdown preview (~250 lines)
  - `BacklogItemDetailAcceptanceCriteria.tsx` — AC list + inline editing (~300 lines)
  - `BacklogItemDetailLinkedSessions.tsx` — session/PR linking (~200 lines)
  - `BacklogItemDetailCost.tsx` — cost breakdown, token counts (~200 lines)
- **Risk**: Moderate — React component splits can introduce prop-drilling if shared state is not extracted first. The front-end-refactor plan (design-only) should be consulted to ensure this decomposition is consistent with its ADRs.
- **Test gate**: `cd web-app && npx jest --testPathPatterns="BacklogItemDetail" --no-coverage` passes.

#### RC-05: Split `useBacklogService.ts` and `useSessionService.ts`

- **Score**: High (1,611 + 1,442 lines; these hooks are the main state coupling point between UI and backend)
- **Current state**: Monolithic service hooks that mix: API call orchestration, optimistic updates, derived state computations, cache invalidation, and side-effect triggers. Every component that calls either hook gets the entire API surface.
- **Proposed split** (example for `useBacklogService.ts`):
  - `useBacklogItems.ts` — list fetching, filtering, pagination (~300 lines)
  - `useBacklogItemMutations.ts` — create/update/delete/transition mutations (~300 lines)
  - `useBacklogItemDetail.ts` — single item fetch + sub-resource fetching (~300 lines)
  - `useBacklogFilters.ts` — filter state, URL sync, derived filter sets (~200 lines)
- **Risk**: Moderate — component call sites must be updated to the narrower hooks. Requires surveying all callers first.
- **Test gate**: `cd web-app && npx jest --no-coverage` passes for all backlog-related tests.

---

### Tier 2 — Medium priority, partially covered by existing plans

These are high-scoring candidates but have existing plans. Reference here for completeness; dispatch should go to the existing plans.

| Candidate | Existing plan | Score | Gap |
|---|---|---|---|
| `session/instance.go` (2,771 / 135) | `backend-di-refactor` G2 | ~374,000 | Plan has implementation + validation — may be in-flight |
| `server/services/backlog_service_triage.go` (3,800 / 121) | `backlog-service-refactor` | ~460,000 | Most mature plan; check implementation status |
| `server/dependencies.go` (2,039 / 134) | `backend-di-refactor` G1 | ~273,000 | Covered by Warren validation goal |
| `session/ent_repository_backlog.go` (3,186 / 68) | `backlog-service-refactor` | ~216,000 | Fat repository covered in backlog plan |
| CSS + component library | `front-end-refactor` | — | 5 ADRs exist; implementation not started |

---

## Task Breakdown

### Task 1 — Write dispatch-ready project plan for RC-01 (WebSocket split)
- Create `project_plans/websocket-handler-decomposition/requirements.md` with scope from this plan's RC-01 section
- Estimate: 2h
- Category: backend

### Task 2 — Write dispatch-ready project plan for RC-02 (tmux.go split)
- Create `project_plans/tmux-package-decomposition/requirements.md` with scope from RC-02
- Estimate: 2h
- Category: backend

### Task 3 — Write dispatch-ready project plan for RC-03 (backlog_lifecycle split)
- Create `project_plans/backlog-lifecycle-decomposition/requirements.md` with scope from RC-03
- Estimate: 2h
- Category: backend

### Task 4 — Write dispatch-ready project plan for RC-04 (BacklogItemDetail split)
- Create `project_plans/backlog-item-detail-decomposition/requirements.md` with scope from RC-04; cross-reference front-end-refactor ADRs
- Estimate: 2h
- Category: frontend

### Task 5 — Write dispatch-ready project plan for RC-05 (service hook split)
- Create `project_plans/service-hooks-decomposition/requirements.md` with scope from RC-05; identify all consumer call sites
- Estimate: 3h
- Category: frontend

### Task 6 — Implement RC-01: split `connectrpc_websocket.go`
- Same-package file split per RC-01 proposed layout; no API changes
- `go build ./server/...` + existing tests as gate
- Estimate: 4h
- Category: backend

### Task 7 — Implement RC-02: split `session/tmux/tmux.go`
- Same-package file split per RC-02 proposed layout
- `go build ./session/tmux/...` + existing tests as gate
- Estimate: 4h
- Category: backend

### Task 8 — Implement RC-03: split `session/backlog_lifecycle.go`
- Same-package file split per RC-03 proposed layout
- `make build` + `go test ./session/...` as gate
- Estimate: 4h
- Category: backend

### Task 9 — Implement RC-04: decompose `BacklogItemDetail.tsx`
- Component split per RC-04; shared state extraction first
- `cd web-app && npx jest` as gate
- Estimate: 6h
- Category: frontend

### Task 10 — Implement RC-05: split useBacklogService + useSessionService hooks
- Survey consumer call sites; split per RC-05 proposal; update consumers
- `cd web-app && npx jest` as gate
- Estimate: 6h
- Category: frontend

### Task 11 — Verify existing plans (backend-di-refactor, backlog-service-refactor) are not stale
- Check if implementation directories contain merged/complete work; update status in this inventory
- Estimate: 1h
- Category: docs

### Task 12 — Measure post-refactor churn improvement (6 weeks after dispatch)
- Re-run git churn analysis; confirm split files no longer appear in top-10 hotspot list
- Estimate: 1h
- Category: docs
