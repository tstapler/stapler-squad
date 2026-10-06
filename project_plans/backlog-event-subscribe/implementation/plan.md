# Implementation Plan: backlog-event-subscribe

**Feature**: A new `wait_for_backlog_event` MCP tool that blocks a Claude Code session on the
already-shipped `*pkg/events.EventBus` for one backlog item's next matching event (or a
current-state precheck), replacing `ScheduleWakeup`+`get_backlog_item` polling loops for
verdict/status waits.
**Date**: 2026-08-11
**Status**: Ready for implementation
**ADRs**: [ADR-001: In-process EventBus.Subscribe instead of ConnectRPC client](../decisions/ADR-001-in-process-eventbus-subscribe.md)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `EventBus` | Existing in-process, per-workspace pub/sub hub (`pkg/events/bus.go`) that every backlog mutation publishes through and every live consumer subscribes to. | Not modified by this project. |
| `BacklogItemEventPayload` | Existing struct carried on `*events.Event.BacklogItemPayload` for `EventBacklogItemChanged` events; carries `Kind`, `Item`, `OldStatus`/`NewStatus`, `Verdict`, `UpdatedFields`, `ArchivedAt`, `RemovedReason`. | Not modified. |
| `BacklogChangeKind` | Existing enum of backlog mutation kinds (`status_transition`, `verdict_recorded`, `item_archived`, `item_removed`, `item_updated`, `session_attached`, `triage_progress_updated`). | Not modified. |
| `backlogHandlers` | Existing struct (`server/mcp/tools_backlog.go`) hosting every backlog MCP tool's handler method; already has an `eventBus *events.EventBus` field wired in production. | The new handler is one more method on this receiver — no new field. |
| `waitForBacklogEvent` | New method on `*backlogHandlers`; the Go implementation registered as the `wait_for_backlog_event` MCP tool. | Analogous in role to `getBacklogItem`, `waitForOutput`. |
| `WaitForBacklogEventResult` | New response struct (`server/mcp/tools_backlog.go`), embeds `MCPResult`, returned to the MCP caller for both matched and timeout outcomes. | Mirrors `WaitForOutputResult`'s shape. |
| `eventTypeFilter` | The tool's optional `event_type` parameter, decoded to a Go `string` (`"any"`, `"verdict_recorded"`, `"status_changed"`, `"item_archived"`, `"item_removed"`), default `"any"`. | Plain string, not a newtype — see Pattern Decisions row 6. |
| `backlogEventKindFilterValue` | New helper mapping a `BacklogChangeKind` to its `event_type` filter string, used both to filter live events and to populate `WaitForBacklogEventResult.EventKind`. | Pure function, no receiver. |
| `buildMatchedWaitResult` | New helper building a `WaitForBacklogEventResult` from a live matched `*events.BacklogItemEventPayload`. | Pure function. |
| `currentStateWaitResult` | New helper implementing the "already satisfied" precheck against current storage state (item + latest verdict), scoped to the filters where "already true" has a well-defined answer. | Pure function; returns `nil` when nothing pre-satisfies the wait. |
| `FromCurrentState` | `WaitForBacklogEventResult` field: `true` when the match was resolved by `currentStateWaitResult` (precheck) rather than a live event off the bus. | Lets a caller distinguish "already true when I called" from "just happened." |
| `IsTerminal` | `WaitForBacklogEventResult` field: `true` when no further event will ever arrive for this item (archived, removed, or status already `done`/`archived`). | Signals the caller it's safe to stop waiting on this item entirely. |
| `ErrEventStreamUnavailable` | New MCP error code (`server/mcp/types.go`) returned when `h.eventBus == nil` — the stdio MCP fallback path (main.go's `buildMCPDeps` branch). | Distinct from `WAIT_TIMEOUT` per pitfalls research finding 5. |
| `testAfterWaitSubscribeHook` | New package-level, test-only `func()` var invoked immediately after `Subscribe`, before the precheck read — mirrors `backlog_service_events.go`'s `testAfterSubscribeHook` seam. | Nil in production; lets tests deterministically land a `Publish()` inside the subscribe→precheck race window. |
| `subID` | `Subscribe`'s returned subscriber ID, passed to `Unsubscribe` for cleanup. | Existing `EventBus` API. |
| `waitCtx` | The `context.WithTimeout(ctx, timeoutSecs)`-derived context passed to `Subscribe` and selected on inside the wait loop — distinct from the handler's incoming `ctx`, which is used for the (unbounded) storage precheck read. | Naming matches this file's existing local-variable style (no package prefix needed inside the function). |

---

## Pattern Decisions

**Step 0.5 CREATIVE pass** — two independent decision points, each explored as 2–3 approaches
before committing (research in `build-vs-buy.md` and `features.md`/`architecture.md` already
did much of this comparison; recorded here per the planning template's requirement not to
leave it implicit).

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Event delivery mechanism | Direct in-process pub/sub subscriber (`h.eventBus.Subscribe(ctx)`) | GoF Observer / this repo's existing `EventBus` | Adapt the ConnectRPC `WatchBacklogItems` streaming client in-process | The MCP tool already runs in the same Go process as `BacklogService` (`backlogHandlers.eventBus` is the same instance `BacklogService.SetEventBus` wires). Going through the RPC client would mean the server calling itself over HTTP/2, with proto marshal/unmarshal, plus a second redundant subscription layer under the RPC's own `watchBacklogItems`/`EventBus.Subscribe` call — pure overhead, no benefit, for data already available as a plain Go struct one call away. |
| Bounded wait mechanism | `context.WithTimeout` + `select { <-eventCh, <-ctx.Done() }` | stdlib; same shape as `writeToSession`/`sendControl` in `server/mcp/tools_terminal.go` | Adapt `wait_for_output`'s `time.Ticker`(1s)-driven polling loop | `wait_for_output` polls because there is no push channel for terminal scrollback. Here a real channel (`EventBus.Subscribe`) exists — polling it on a 1s ticker would reintroduce up to 1s of latency per check and repeated storage reads, defeating this project's entire purpose (replacing a polling loop). Only `wait_for_output`'s tool-shape/argument conventions are reused, not its wait mechanism. |
| Result assembly | Transaction Script — a single procedural handler method plus pure helper functions, no Repository/Service-layer indirection | PoEAA | A `BacklogEventWaiter` interface/service type wrapping the wait logic | Single call site, single implementation, no near-term second one — an interface here is a speculative abstraction (`.claude/rules/interface-pollution-checklist.md` smell #1). Matches the existing shape of every other `backlogHandlers` method (`getBacklogItem`, `submitTriageResult`, etc.), none of which are behind an interface. |
| Already-satisfied handling (dimension A) | Precheck limited to filters with a well-defined "already true" answer (`"any"`, `"verdict_recorded"`, `"item_archived"`) — never for `"status_changed"`/`"item_removed"` | Derived from `WatchBacklogItems`'s snapshot-branch precedent, narrowed | Never precheck — always wait for the strictly-next event | Forces every retry-after-restart or immediate-recheck caller to eat a full `timeout_seconds` wait even when the answer (verdict already recorded, item already archived) is already knowable — reintroducing the exact "wasted round trip" problem this project exists to fix (requirements' Problem Statement). |
| Already-satisfied handling (dimension B) | (same choice as above) | — | Unconditional snapshot-first: always return current state as a "match" on the very first call, mirroring `WatchBacklogItems`'s stream-connect snapshot exactly | A one-shot bounded tool call would then conflate "I successfully called the tool" with "something the caller cares about happened" — the tool would return instantly on nearly every call regardless of whether the awaited condition became true, destroying the signal `event_received`/`from_current_state` is supposed to carry. `status_changed`/`item_removed` have no synthesizable "already true" answer from a single state read (no prior state to diff against), so precheck is skipped for those two filters rather than forced. |
| `event_type` filter parameter type | Plain `string`, validated by MCP schema (`mcpgo.Enum(...)`), decoded via a single `args["event_type"].(string)` type assertion | type-driven-design considered and rejected; existing repo convention (`create_backlog_item`'s `category`, `submit_review_verdict`'s `outcome` already use string + `mcpgo.Enum`) | A Go newtype `type BacklogEventFilter string` with named consts | The value crosses the JSON/MCP wire boundary as a string on read and is never compared against a second same-typed primitive parameter on this tool (no primitive-obsession swap risk — `.claude/rules/primitive-obsession-checklist.md` triggers on 2+ same-typed params representing distinct concepts; this tool has exactly one string filter param). A newtype here is unjustified ceremony for a single, schema-validated field. |

---

## Migration Plan

N/A — no schema or data changes. No proto changes (per requirements' Constraints: reuse
`WatchBacklogItems`/`EventBus` as-is; confirmed by research, no gap found requiring a proto
field). No `docs/registry/` entry (confirmed by both `features.md` §6 and `architecture.md` §4:
zero existing MCP tool carries a `+api:`/`+feature:` marker or registry file; the registry
tooling doesn't scan `server/mcp/*.go` — this project follows that established precedent
rather than being the first to add one unprompted).

## Observability Plan

- **Logs**: `WatchBacklogItems` itself logs nothing on connect/disconnect (verified: zero
  `log.*` calls in `server/services/backlog_service_events.go`), and `wait_for_output` — the
  tool this project mirrors most closely — also logs nothing (zero `log.*` calls in
  `server/mcp/tools_terminal.go`). Per requirements' Observability Requirements ("same level of
  detail `WatchBacklogItems`' server-side handler already logs") that baseline level is **zero**
  on the normal match/timeout paths — do not add new logging there beyond what
  `get_backlog_item`/`wait_for_output` already have (none). The one exception: log at
  `log.WarningLog` when the `h.eventBus == nil` guard fires (Task 1.2.1a), matching this file's
  existing convention of logging on genuinely-diagnosable failure paths (`[mcp:request_review]`,
  `[mcp:report_duplicate]`, etc. all log on their rejection paths) — this is the one pitfall
  this project introduces relative to the already-shipped RPC (pitfalls research finding 5) and
  needs to be distinguishable in logs from an ordinary empty timeout.
- **Metrics**: None added — matches every other MCP tool in this codebase (no `tools_*.go` file
  emits metrics today).
- **Alerts**: None — internal dev-tool feature (requirements' Observability Requirements: "No
  new oncall alert").

## Risk Control

- **Feature flag**: None. Additive MCP tool; existing `get_backlog_item`+`ScheduleWakeup`
  polling flows are unaffected and remain a working fallback (per requirements' Risk Control:
  "Direct ship, no feature flag").
- **Rollback procedure**: Standard `git revert` of the tool-registration + handler commit(s); no
  data migration to unwind.
- **Staged rollout**: None — internal dev-tool, single binary, no user-facing surface.

## Unresolved Questions

- [ ] Whether the Claude Code harness itself imposes a client-side MCP tool-call timeout below
      this tool's 60s max — unverifiable from inside this repo (pitfalls research finding 4,
      explicitly named as a gap, not a blocker). Mitigated by choosing the same conservative
      30s default / 60s max `wait_for_output` already ships with, which has no known
      harness-timeout complaints in this codebase's history. Does not block implementation —
      owner: whoever notices a sub-60s harness-side timeout in practice, file a follow-up then.

## Dependency Visualization

```
Phase 1: Backend Tool
  Epic 1.1 (types/helpers) ──┐
                              ├──> Epic 1.2 (handler) ──> Epic 1.3 (guidance text update)
  (no dependency on 1.3)     ┘         │
                                       v
Phase 2: Test Coverage (depends on all of Phase 1)
  Epic 2.1 (validation/guard tests)
  Epic 2.2 (precheck tests)         — all four Epic 2.x stories are
  Epic 2.3 (live-match/filter/timeout tests)   independent of each other,
  Epic 2.4 (concurrency/leak/race-seam tests)  can be implemented in any order
```

---

## Phase 1: Backend Tool

### Epic 1.1: Result type, error code, and event-kind helpers
**Goal**: Add the new response struct, error code, and three small pure helper functions the
handler will call — no behavior yet, just the building blocks, so Epic 1.2's handler body reads
as orchestration rather than inline logic.

#### Story 1.1.1: Add `WaitForBacklogEventResult` and `ErrEventStreamUnavailable`
**As a** session calling `wait_for_backlog_event`, **I want** a structured JSON result with a
distinct error code when the event stream isn't available, **so that** I can tell "this tool
isn't usable right now" apart from "no event fired yet."
**Acceptance Criteria**:
- `WaitForBacklogEventResult` exists in `server/mcp/tools_backlog.go`, embeds `MCPResult`, and
  has JSON fields `event_received`, `from_current_state`, `event_kind`, `item_id`, `status`,
  `old_status`, `new_status`, `verdict_outcome`, `verdict_summary`, `updated_fields`,
  `removed_reason`, `is_terminal` (all `omitempty` except `event_received`/`item_id`).
  - *Given* a `WaitForBacklogEventResult{MCPResult: MCPResult{Success:true}, EventReceived:true, ItemID:"11111111-1111-1111-1111-111111111111", EventKind:"verdict_recorded", VerdictOutcome:"PASS"}`, *When* `json.Marshal`ed, *Then* the output contains `"event_received":true`, `"item_id":"11111111-1111-1111-1111-111111111111"`, `"event_kind":"verdict_recorded"`, `"verdict_outcome":"PASS"` and omits `old_status`/`new_status`/`updated_fields`/`removed_reason`.
- `ErrEventStreamUnavailable = "EVENT_STREAM_UNAVAILABLE"` exists in `server/mcp/types.go`'s
  error-code const block, alongside `ErrPermissionDenied`/`ErrItemNotFound`/`ErrFeatureDisabled`.
  - *Given* `types.go`'s error-code const block, *When* this task is done, *Then* `ErrEventStreamUnavailable` is a distinct string value (`"EVENT_STREAM_UNAVAILABLE"`) not colliding with any existing const in that block.
**Files**: `server/mcp/tools_backlog.go`, `server/mcp/types.go`

##### Task 1.1.1a: Add `ErrEventStreamUnavailable` const (~2 min)
- Add `ErrEventStreamUnavailable = "EVENT_STREAM_UNAVAILABLE"` to the `const (...)` block in
  `server/mcp/types.go` (after `ErrPTYWriteTimeout`).
- Files: `server/mcp/types.go`

##### Task 1.1.1b: Add `WaitForBacklogEventResult` struct (~3 min)
- Add the struct below `latestReviewVerdict`'s doc comment area (or any top-level location in
  `server/mcp/tools_backlog.go` near the other result-shaping helpers), mirroring
  `WaitForOutputResult`'s field-tag style from `tools_terminal.go`:
  ```go
  // WaitForBacklogEventResult is the response for wait_for_backlog_event.
  type WaitForBacklogEventResult struct {
      MCPResult
      EventReceived    bool     `json:"event_received"`
      FromCurrentState bool     `json:"from_current_state,omitempty"`
      EventKind        string   `json:"event_kind,omitempty"`
      ItemID           string   `json:"item_id"`
      Status           string   `json:"status,omitempty"`
      OldStatus        string   `json:"old_status,omitempty"`
      NewStatus        string   `json:"new_status,omitempty"`
      VerdictOutcome   string   `json:"verdict_outcome,omitempty"`
      VerdictSummary   string   `json:"verdict_summary,omitempty"`
      UpdatedFields    []string `json:"updated_fields,omitempty"`
      RemovedReason    string   `json:"removed_reason,omitempty"`
      IsTerminal       bool     `json:"is_terminal,omitempty"`
  }
  ```
- Files: `server/mcp/tools_backlog.go`

#### Story 1.1.2: Add event-kind mapping, matched-result, and precheck helpers
**As a** future maintainer reading `waitForBacklogEvent`, **I want** the filter-matching,
result-building, and precheck logic factored into named pure functions, **so that** the
handler's control flow (validate → guard → subscribe → precheck → select) stays readable and
each piece is independently testable.
**Acceptance Criteria**:
- `backlogEventKindFilterValue(kind events.BacklogChangeKind) string` maps every
  `BacklogChangeKind` const to its `event_type` filter string.
  - *Given* `events.BacklogChangeVerdictRecorded`, *When* `backlogEventKindFilterValue` is called, *Then* it returns `"verdict_recorded"`.
  - *Given* `events.BacklogChangeItemArchived`, *When* called, *Then* it returns `"item_archived"`.
- `buildMatchedWaitResult(itemID string, payload *events.BacklogItemEventPayload) WaitForBacklogEventResult`
  builds a fully-populated result from a live event payload, setting `IsTerminal` for
  archived/removed kinds or a `done`/`archived` item status.
  - *Given* `payload := &events.BacklogItemEventPayload{Kind: events.BacklogChangeVerdictRecorded, Item: &session.BacklogItemData{ID:"11111111-1111-1111-1111-111111111111", Status:"review"}, Verdict: &session.ReviewVerdictData{OverallOutcome: session.ReviewOutcomePass, Summary:"looks good"}}`, *When* `buildMatchedWaitResult("11111111-1111-1111-1111-111111111111", payload)` is called, *Then* the result has `EventReceived:true`, `EventKind:"verdict_recorded"`, `Status:"review"`, `VerdictOutcome:"PASS"`, `VerdictSummary:"looks good"`, `IsTerminal:false`.
  - *Given* `payload := &events.BacklogItemEventPayload{Kind: events.BacklogChangeItemRemoved, RemovedReason:"duplicate of #123"}` (Item nil per removal shape), *When* `buildMatchedWaitResult("11111111-...", payload)` is called, *Then* the result has `EventKind:"item_removed"`, `RemovedReason:"duplicate of #123"`, `IsTerminal:true`.
- `currentStateWaitResult(item *session.BacklogItemData, verdict *session.ReviewVerdictSummary, eventTypeFilter string) *WaitForBacklogEventResult`
  returns non-nil only when the current state already satisfies the filter.
  - *Given* `item := &session.BacklogItemData{ID:"11...1", Status:"review"}`, `verdict := &session.ReviewVerdictSummary{OverallOutcome:"PASS", Summary:"lgtm"}`, `eventTypeFilter := "verdict_recorded"`, *When* `currentStateWaitResult(item, verdict, "verdict_recorded")` is called, *Then* it returns non-nil with `FromCurrentState:true`, `EventKind:"verdict_recorded"`, `VerdictOutcome:"PASS"`.
  - *Given* the same `item`/`verdict`, *When* `eventTypeFilter == "status_changed"`, *Then* `currentStateWaitResult` returns `nil` (no precheck for this filter — Pattern Decisions row "Already-satisfied handling").
  - *Given* `item := &session.BacklogItemData{ID:"11...1", Status:"archived"}`, `verdict := nil`, `eventTypeFilter := "any"`, *When* called, *Then* it returns non-nil with `EventKind:"item_archived"`, `IsTerminal:true`.
**Files**: `server/mcp/tools_backlog.go`

##### Task 1.1.2a: Add `backlogEventKindFilterValue` (~3 min)
- Add:
  ```go
  // backlogEventKindFilterValue maps a BacklogChangeKind to the event_type
  // filter string wait_for_backlog_event's callers pass/receive.
  func backlogEventKindFilterValue(kind events.BacklogChangeKind) string {
      switch kind {
      case events.BacklogChangeVerdictRecorded:
          return "verdict_recorded"
      case events.BacklogChangeStatusTransition:
          return "status_changed"
      case events.BacklogChangeItemArchived:
          return "item_archived"
      case events.BacklogChangeItemRemoved:
          return "item_removed"
      case events.BacklogChangeSessionAttached:
          return "session_attached"
      case events.BacklogChangeItemUpdated, events.BacklogChangeTriageProgressUpdated:
          return "item_updated"
      default:
          return string(kind)
      }
  }
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 1.1.2b: Add `buildMatchedWaitResult` (~5 min)
- Add:
  ```go
  // buildMatchedWaitResult builds the tool result for a live event received
  // off the EventBus subscription channel.
  func buildMatchedWaitResult(itemID string, payload *events.BacklogItemEventPayload) WaitForBacklogEventResult {
      res := WaitForBacklogEventResult{
          MCPResult:     MCPResult{Success: true},
          EventReceived: true,
          EventKind:     backlogEventKindFilterValue(payload.Kind),
          ItemID:        itemID,
      }
      if payload.Item != nil {
          res.Status = payload.Item.Status
          status := session.BacklogStatus(payload.Item.Status)
          if status == session.BacklogStatusDone || status == session.BacklogStatusArchived {
              res.IsTerminal = true
          }
      }
      switch payload.Kind {
      case events.BacklogChangeStatusTransition:
          res.OldStatus = payload.OldStatus
          res.NewStatus = payload.NewStatus
      case events.BacklogChangeVerdictRecorded:
          if payload.Verdict != nil {
              res.VerdictOutcome = string(payload.Verdict.OverallOutcome)
              res.VerdictSummary = payload.Verdict.Summary
          }
      case events.BacklogChangeItemUpdated, events.BacklogChangeTriageProgressUpdated:
          res.UpdatedFields = payload.UpdatedFields
      case events.BacklogChangeItemArchived:
          res.IsTerminal = true
      case events.BacklogChangeItemRemoved:
          res.IsTerminal = true
          res.RemovedReason = payload.RemovedReason
      }
      return res
  }
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 1.1.2c: Add `currentStateWaitResult` (~5 min)
- Add:
  ```go
  // currentStateWaitResult implements the "already satisfied" precheck: if the
  // item's current state already satisfies what eventTypeFilter is waiting
  // for, return a result now instead of blocking for a full timeout on state
  // that's already true. Only defined for filters where "already true" has an
  // unambiguous answer from a single state read (no prior state to diff
  // against for status_changed/item_removed — see plan.md Pattern Decisions,
  // "Already-satisfied handling"). Returns nil when nothing pre-satisfies.
  func currentStateWaitResult(item *session.BacklogItemData, verdict *session.ReviewVerdictSummary, eventTypeFilter string) *WaitForBacklogEventResult {
      status := session.BacklogStatus(item.Status)
      terminal := status == session.BacklogStatusDone || status == session.BacklogStatusArchived

      if (eventTypeFilter == "any" || eventTypeFilter == "verdict_recorded") && verdict != nil {
          return &WaitForBacklogEventResult{
              MCPResult:        MCPResult{Success: true},
              EventReceived:    true,
              FromCurrentState: true,
              EventKind:        "verdict_recorded",
              ItemID:           item.ID,
              Status:           item.Status,
              VerdictOutcome:   verdict.OverallOutcome,
              VerdictSummary:   verdict.Summary,
              IsTerminal:       terminal,
          }
      }
      if (eventTypeFilter == "any" || eventTypeFilter == "item_archived") && status == session.BacklogStatusArchived {
          return &WaitForBacklogEventResult{
              MCPResult:        MCPResult{Success: true},
              EventReceived:    true,
              FromCurrentState: true,
              EventKind:        "item_archived",
              ItemID:           item.ID,
              Status:           item.Status,
              IsTerminal:       true,
          }
      }
      return nil
  }
  ```
- Files: `server/mcp/tools_backlog.go`

---

### Epic 1.2: `waitForBacklogEvent` handler
**Goal**: The one genuinely new piece of logic in this project — a bounded, single-shot
subscribe-precheck-select wrapper around `EventBus`, matching every design directive from the
pitfalls/architecture/build-vs-buy research (subscribe-before-read ordering, `ctx`-derived
timeout, mandatory nil guard, no re-fetch after receiving an event).

#### Story 1.2.1: Implement `waitForBacklogEvent` with subscribe-before-read ordering
**As a** session that just called `request_review`, **I want** to call one tool that blocks
until my item's verdict lands (or times out) instead of polling, **so that** I don't waste
wakeup cycles on empty checks.
**Acceptance Criteria**:
- Missing/empty `item_id` returns `errResult(ErrInvalidArgument, ...)`.
  - *Given* `args := map[string]any{}`, *When* `waitForBacklogEvent` is called, *Then* it returns an `MCPResult{Success:false, Error:{Code:"INVALID_ARGUMENT"}}` envelope, with no call to `h.eventBus.Subscribe`.
- Malformed `item_id` (not a 36-char UUID) returns `errResult(ErrInvalidArgument, ...)` with the
  same remediation text `getBacklogItem` uses.
  - *Given* `args := map[string]any{"item_id": "not-a-uuid"}`, *When* called, *Then* it returns `Error.Code == "INVALID_ARGUMENT"` and `Error.Remediation == "Provide a valid UUID (e.g. from list_backlog_items or get_backlog_item)."`.
- `h.eventBus == nil` returns `errResult(ErrEventStreamUnavailable, ...)` immediately — no
  subscribe attempt, no nil-pointer panic — and logs a `WarningLog` line.
  - *Given* `handler := &backlogHandlers{storage: storage, eventBus: nil}` and a valid, existing `item_id`, *When* `waitForBacklogEvent` is called, *Then* it returns `Error.Code == "EVENT_STREAM_UNAVAILABLE"` and `Success == false` (not a `WAIT_TIMEOUT` result).
- Subscribe happens **before** any storage read (subscribe-before-read ordering, closing the
  missed-event race).
  - *Given* `handler` with a real `eventBus` and `testAfterWaitSubscribeHook` set to publish a matching `verdict_recorded` event synchronously, *When* `waitForBacklogEvent` is called for that item with `timeout_seconds=1`, *Then* the result is `EventReceived:true` (not a timeout) — proving the event published inside the hook (after `Subscribe`, before the precheck read) was not missed. (Full test in Story 2.4.1's race-seam task; this AC states the contract the implementation must satisfy.)
- On timeout, returns `Success:true` with `Error:{Code:"WAIT_TIMEOUT", Message: "no new <event_type> event on item <id> within <n> seconds — call ScheduleWakeup for a longer interval before checking again, or call wait_for_backlog_event again only if you intend to keep this session blocked"}` and `EventReceived:false` — never a Go `error` return, never `Success:false`.
  - *Given* a valid item with no verdict and no live events published, `timeout_seconds=1`, *When* `waitForBacklogEvent` is called, *Then* after ~1s it returns `Success:true`, `Error.Code == "WAIT_TIMEOUT"`, `EventReceived:false`.
- `defer h.eventBus.Unsubscribe(subID)` fires on every return path (match, timeout,
  ctx-cancel, item-not-found, invalid-arg-before-subscribe is a non-issue since no subscribe
  happened yet).
  - *Given* the handler completes any of the above cases, *When* the call returns, *Then* `h.eventBus.SubscriberCount()` is back to its pre-call value (verified by the goroutine-leak tests in Story 2.4.1, not re-derived here).
**Files**: `server/mcp/tools_backlog.go`

##### Task 1.2.1a: Add `testAfterWaitSubscribeHook` var and handler skeleton — validation + nil guard (~5 min)
- Add the package-level test seam near the top of `server/mcp/tools_backlog.go` (or adjacent to
  the new handler), mirroring `backlog_service_events.go`'s `testAfterSubscribeHook`:
  ```go
  // testAfterWaitSubscribeHook, when non-nil, is invoked immediately after
  // h.eventBus.Subscribe(ctx) inside waitForBacklogEvent, before the
  // current-state precheck read. Production code never sets this — it exists
  // solely so tests can deterministically land a Publish() call inside the
  // subscribe→precheck race window, mirroring
  // backlog_service_events.go's testAfterSubscribeHook (same rationale: this
  // race depends on non-deterministic goroutine scheduling without a seam).
  var testAfterWaitSubscribeHook func()
  ```
- Add the handler's validation + nil-guard prefix (no subscribe/select body yet — that's Task
  1.2.1b):
  ```go
  func (h *backlogHandlers) waitForBacklogEvent(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
      if r := featureDisabledResult(h.enabledCheck); r != nil {
          return r, nil
      }
      args := req.GetArguments()
      itemID, ok := args["item_id"].(string)
      if !ok || itemID == "" {
          return errResult(ErrInvalidArgument, "item_id is required", ""), nil
      }
      if err := validateUUID(itemID); err != nil {
          return errResult(ErrInvalidArgument, err.Error(), "Provide a valid UUID (e.g. from list_backlog_items or get_backlog_item)."), nil
      }

      eventTypeFilter := "any"
      if v, ok := args["event_type"].(string); ok && v != "" {
          eventTypeFilter = v
      }

      timeoutSecs := 30
      if v, ok := args["timeout_seconds"].(float64); ok && v > 0 {
          timeoutSecs = int(v)
          if timeoutSecs > 60 {
              timeoutSecs = 60
          }
      }

      if h.eventBus == nil {
          log.WarningLog.Printf("[mcp:wait_for_backlog_event] eventBus is nil (stdio fallback path) item=%s", itemID)
          return errResult(ErrEventStreamUnavailable, "backlog event stream is not available on this connection", "This session's MCP call is on the stdio fallback path (daemon unreachable). Fall back to get_backlog_item polling until the daemon is reachable again."), nil
      }

      // Task 1.2.1b continues here: subscribe, precheck, select loop.
  }
  ```
- Files: `server/mcp/tools_backlog.go`

##### Task 1.2.1b: Add subscribe + precheck + select loop body (~5 min)
- Replace the `// Task 1.2.1b continues here` marker with:
  ```go
      waitCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
      defer cancel()

      eventCh, subID := h.eventBus.Subscribe(waitCtx)
      defer h.eventBus.Unsubscribe(subID)

      if testAfterWaitSubscribeHook != nil {
          testAfterWaitSubscribeHook()
      }

      item, err := h.storage.GetBacklogItem(ctx, itemID)
      if err != nil {
          if errors.Is(err, session.ErrNotFound) {
              return errResult(ErrItemNotFound, fmt.Sprintf("backlog item %q not found", itemID), ""), nil
          }
          return errResult(ErrInternalError, fmt.Sprintf("get backlog item: %v", err), ""), nil
      }
      verdict := latestReviewVerdict(ctx, h.storage, itemID)
      if res := currentStateWaitResult(item, verdict, eventTypeFilter); res != nil {
          return okResult(*res), nil
      }

      for {
          select {
          case <-waitCtx.Done():
              return okResult(WaitForBacklogEventResult{
                  MCPResult: MCPResult{Success: true, Error: &MCPError{
                      Code:    "WAIT_TIMEOUT",
                      Message: fmt.Sprintf("no new %s event on item %s within %d seconds — call ScheduleWakeup for a longer interval before checking again, or call wait_for_backlog_event again only if you intend to keep this session blocked", eventTypeFilter, itemID, timeoutSecs),
                  }},
                  EventReceived: false,
                  ItemID:        itemID,
              }), nil
          case evt, ok := <-eventCh:
              if !ok {
                  return okResult(WaitForBacklogEventResult{
                      MCPResult: MCPResult{Success: true, Error: &MCPError{
                          Code:    "WAIT_TIMEOUT",
                          Message: "backlog event stream closed while waiting",
                      }},
                      EventReceived: false,
                      ItemID:        itemID,
                  }), nil
              }
              if evt.Type != events.EventBacklogItemChanged || evt.BacklogItemPayload == nil {
                  continue
              }
              payload := evt.BacklogItemPayload
              if payload.Item == nil || payload.Item.ID != itemID {
                  continue
              }
              kind := backlogEventKindFilterValue(payload.Kind)
              if eventTypeFilter != "any" && kind != eventTypeFilter {
                  continue
              }
              return okResult(buildMatchedWaitResult(itemID, payload)), nil
          }
      }
  }
  ```
- Note the precheck read (`h.storage.GetBacklogItem`, `latestReviewVerdict`) uses the original
  `ctx`, not `waitCtx` — a short `timeout_seconds` (e.g. 1s) must not truncate the one-time
  state read itself, only the subsequent live-event wait.
- Files: `server/mcp/tools_backlog.go`

#### Story 1.2.2: Register `wait_for_backlog_event` in `registerBacklogTools`
**As a** Claude Code session, **I want** `wait_for_backlog_event` discoverable in the MCP tool
list with a schema that structurally caps `timeout_seconds` at 60s, **so that** I can't
accidentally request an unbounded wait and the tool's description tells me when to use it
instead of polling.
**Acceptance Criteria**:
- The tool is registered with `item_id` (required string), `event_type` (optional string enum
  `any|verdict_recorded|status_changed|item_archived|item_removed`, default `any`), and
  `timeout_seconds` (optional number, default 30, min 1, max 60).
  - *Given* the MCP server's registered tool list, *When* a client calls `tools/list`, *Then* `wait_for_backlog_event` appears with a `timeout_seconds` schema whose `maximum` is `60` (structurally preventing a client from requesting a longer wait, per UX research §3's "structural fix, not a wording fix").
- The tool's description states the timeout-is-not-an-error convention and points the caller at
  it instead of `ScheduleWakeup`+`get_backlog_item` polling for verdict waits.
  - *Given* the tool's registered `mcpgo.WithDescription(...)` text, *When* read, *Then* it contains the substring "expected outcome, not an error" and references `get_backlog_item`/`request_review` context (mirrors `wait_for_output`'s description convention, UX research §1).
**Files**: `server/mcp/tools_backlog.go`

##### Task 1.2.2a: Add the `s.AddTool(...)` registration block (~3 min)
- Add inside `registerBacklogTools`, after the `submit_triage_result` block (end of the
  function), before the closing `}`:
  ```go
  s.AddTool(
      mcpgo.NewTool("wait_for_backlog_event",
          mcpgo.WithDescription("Block until a backlog item changes (e.g. a review verdict lands), or until timeout. Returns the event directly — status, verdict outcome/summary, or archival/removal reason — so a follow-up get_backlog_item call is usually unnecessary. If the awaited condition (e.g. a verdict) is already true when this is called, returns immediately with from_current_state=true instead of waiting out the full timeout. On timeout, returns event_received=false with a message naming the next move (a longer ScheduleWakeup interval, or one more bounded wait) — this is an expected outcome, not an error. Use this instead of a ScheduleWakeup + get_backlog_item polling loop when waiting on a specific item's outcome, e.g. after request_review."),
          mcpgo.WithString("item_id",
              mcpgo.Description("UUID of the backlog item"),
              mcpgo.Required(),
          ),
          mcpgo.WithString("event_type",
              mcpgo.Description("Only return when an event of this kind fires (default any). verdict_recorded is the usual choice after request_review — it also returns immediately if a verdict is already recorded. any returns immediately if a verdict already exists or the item is already archived. status_changed/item_removed never return immediately (no 'already true' answer for those)."),
              mcpgo.Enum("any", "verdict_recorded", "status_changed", "item_archived", "item_removed"),
              mcpgo.DefaultString("any"),
          ),
          mcpgo.WithNumber("timeout_seconds",
              mcpgo.Description("How long to wait in seconds (default 30, max 60)"),
              mcpgo.DefaultNumber(30),
              mcpgo.Min(1),
              mcpgo.Max(60),
          ),
      ),
      h.waitForBacklogEvent,
  )
  ```
- Files: `server/mcp/tools_backlog.go`

---

### Epic 1.3: Retire the `get_backlog_item` polling guidance
**Goal**: Close the discoverability gap UX research names as the highest-leverage single-line
change: the one place in the codebase currently *instructing* the polling loop this project
exists to replace.

#### Story 1.3.1: Update work-role step 4 to reference `wait_for_backlog_event`
**As a** work-role session that just called `request_review`, **I want** the guidance text I
read next to tell me about `wait_for_backlog_event`, **so that** I don't default to a
polling loop out of habit even though a better tool now exists.
**Acceptance Criteria**:
- The `case "work":` block's step-4 string (`tools_backlog.go:292`) instructs calling
  `wait_for_backlog_event` instead of "wait a bit, then call get_backlog_item again", while
  preserving the existing PASS/FAIL/PARTIAL and review-attempt-cap guidance verbatim.
  - *Given* a session with role `"work"` calls `get_backlog_item`, *When* the response text is inspected, *Then* step 4 contains the substring `wait_for_backlog_event` and no longer contains the substring `"Wait a bit, then call get_backlog_item again"`.
**Files**: `server/mcp/tools_backlog.go`

##### Task 1.3.1a: Edit the `Fprintf` guidance string (~3 min)
- In `server/mcp/tools_backlog.go` (line 292), change:
  ```go
  fmt.Fprintf(&sb, "4. Do NOT end your session after request_review. Wait a bit, then call get_backlog_item again — once a verdict lands it appears under \"Latest Review Verdict\" above. PASS → ...
  ```
  to:
  ```go
  fmt.Fprintf(&sb, "4. Do NOT end your session after request_review. Call wait_for_backlog_event(item_id, event_type=\"verdict_recorded\") instead of polling — it blocks until the verdict lands (or times out) and returns the outcome directly, or returns immediately if a verdict is already recorded. PASS → ...
  ```
  keeping the remainder of the sentence (the `/backlog/ship` instructions, FAIL/PARTIAL
  handling, and the `%d`-cycle-cap sentence with `session.MaxSameSessionReviewAttempts`)
  byte-for-byte unchanged — only the first two sentences (the "wait a bit" clause) are replaced.
- Files: `server/mcp/tools_backlog.go`

---

## Phase 2: Test Coverage

All tests live in `server/mcp/tools_backlog_test.go`, reusing `newTestBacklogStorage(t)` and
`makeToolReq(...)` (existing helpers in that file / `testhelpers_test.go`) and the
`events.NewEventBus(32)` + `bus.Subscribe`/`bus.Publish` pattern already used by
`TestSubmitTriageResult_PublishesNotificationOnSuccess` (lines 548–585).

### Epic 2.1: Validation & guard tests
**Goal**: Cover the fast-fail paths — invalid input, unknown item, and the mandatory
nil-`eventBus` guard (pitfalls research finding 5's "mandatory" test directive).

#### Story 2.1.1: Invalid `item_id` / not-found / nil-`eventBus` tests
**Acceptance Criteria**:
- `TestWaitForBacklogEvent_ReturnsErrorWhenEventBusNil` asserts `Error.Code ==
  "EVENT_STREAM_UNAVAILABLE"` and `Success == false` (not a `WAIT_TIMEOUT`/timeout-shaped
  result) for a `backlogHandlers{eventBus: nil}` handler.
  - *Given* `handler := &backlogHandlers{storage: newTestBacklogStorage(t), eventBus: nil}` and a real backlog item created via `storage.CreateBacklogItem`, *When* `handler.waitForBacklogEvent(ctx, makeToolReq(map[string]any{"item_id": item.ID}))` is called, *Then* the decoded result has `Success:false`, `Error.Code:"EVENT_STREAM_UNAVAILABLE"`.
- `TestWaitForBacklogEvent_ReturnsErrorForInvalidItemID` and
  `TestWaitForBacklogEvent_ReturnsItemNotFoundForUnknownItem` mirror
  `TestGetBacklogItem_ReturnsNotFoundError`'s existing pattern (line 413) for the new tool.
  - *Given* `args := map[string]any{"item_id": "not-a-uuid"}`, *When* called (with a real, non-nil `eventBus`), *Then* `Error.Code == "INVALID_ARGUMENT"`.
  - *Given* `args := map[string]any{"item_id": "00000000-0000-0000-0000-000000000099"}` (well-formed UUID, no such item), *When* called, *Then* `Error.Code == "ITEM_NOT_FOUND"`.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 2.1.1a: Add `TestWaitForBacklogEvent_ReturnsErrorWhenEventBusNil` (~4 min)
- Add the test per the AC above, following `TestReportProgress_RejectsWhenNoSessionUUID`'s
  structure (storage setup, `makeToolReq`, decode `result.Content[0].(mcpgo.TextContent).Text`
  into `WaitForBacklogEventResult` via `json.Unmarshal`, assert fields).
- Files: `server/mcp/tools_backlog_test.go`

##### Task 2.1.1b: Add invalid-`item_id` and not-found tests (~4 min)
- Add `TestWaitForBacklogEvent_ReturnsErrorForInvalidItemID` and
  `TestWaitForBacklogEvent_ReturnsItemNotFoundForUnknownItem`, each constructing a handler with
  a real `events.NewEventBus(32)` (so the nil-guard doesn't short-circuit before the
  input-validation assertion being tested).
- Files: `server/mcp/tools_backlog_test.go`

---

### Epic 2.2: Already-satisfied / precheck tests
**Goal**: Prove `currentStateWaitResult`'s two supported cases return immediately without
waiting out the timeout, and that the unsupported filter (`status_changed`) correctly does not
short-circuit.

#### Story 2.2.1: Verdict-already-recorded and item-already-archived prechecks
**Acceptance Criteria**:
- A verdict recorded before the call, `event_type` omitted (defaults to `any`), returns
  `FromCurrentState:true` well under `timeout_seconds` (assert via a short test timeout, e.g.
  a wall-clock budget assertion, not `timeout_seconds` itself, to avoid a flaky wall-clock
  race — see Task 2.2.1a).
  - *Given* an item at status `"review"` with an `ItemSession` carrying a `ReviewVerdictData{OverallOutcome: session.ReviewOutcomePass, Summary:"lgtm"}` already recorded, *When* `waitForBacklogEvent(item_id, timeout_seconds=30)` is called, *Then* it returns within a few hundred ms with `EventReceived:true`, `FromCurrentState:true`, `EventKind:"verdict_recorded"`, `VerdictOutcome:"PASS"`.
- An item already at status `"archived"`, `event_type="any"`, returns `FromCurrentState:true`,
  `EventKind:"item_archived"`, `IsTerminal:true` immediately.
  - *Given* an item created and then transitioned to `session.BacklogStatusArchived` via `storage.UpdateBacklogItemStatus` (or the repository's equivalent existing test helper), *When* `waitForBacklogEvent(item_id, event_type="any", timeout_seconds=30)` is called, *Then* it returns immediately with `EventKind:"item_archived"`, `IsTerminal:true`.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 2.2.1a: Add `TestWaitForBacklogEvent_ReturnsImmediatelyWhenVerdictAlreadyRecorded` (~5 min)
- Set up an item + `ItemSession` + recorded verdict (reuse whatever existing test helper in
  `tools_backlog_test.go` creates a `ReviewVerdictData` for `submit_review_verdict` tests — grep
  for `RecordReviewVerdict`/`CreateItemSession` call sites in that file first rather than
  hand-rolling a new one). Assert `time.Since(start) < 5*time.Second` (generous bound, not a
  tight race, per `.claude/rules/fix-flaky-tests-dont-defer.md`'s "prefer a synchronization
  primitive... keep timeouts generous" guidance) to prove it didn't wait out `timeout_seconds=30`.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 2.2.1b: Add `TestWaitForBacklogEvent_ReturnsImmediatelyWhenItemAlreadyArchived` (~5 min)
- Set up an item, transition it to archived via existing storage methods, call with
  `event_type="any"`, assert `IsTerminal:true` and the same "returned fast" bound as 2.2.1a.
- Files: `server/mcp/tools_backlog_test.go`

---

### Epic 2.3: Live-event matching tests
**Goal**: Cover the actual "wait, then a live event arrives" path — the core replacement for
the polling loop — plus filtering and the plain timeout case.

#### Story 2.3.1: Matched live event, filter rejection, and timeout
**Acceptance Criteria**:
- A `verdict_recorded` event published on the shared `eventBus` for the item mid-wait is
  received and returned as `EventReceived:true`, `FromCurrentState:false`.
  - *Given* a handler waiting via `waitForBacklogEvent(item_id, timeout_seconds=5)` in a goroutine, and, shortly after the call starts, a test goroutine calls `handler.eventBus.Publish(&events.Event{Type: events.EventBacklogItemChanged, BacklogItemPayload: &events.BacklogItemEventPayload{Kind: events.BacklogChangeVerdictRecorded, Item: &session.BacklogItemData{ID: itemID, Status:"review"}, Verdict: &session.ReviewVerdictData{OverallOutcome: session.ReviewOutcomePass, Summary:"ship it"}}})`, *When* the handler's goroutine returns its result, *Then* `EventReceived:true`, `FromCurrentState:false`, `VerdictOutcome:"PASS"`, `VerdictSummary:"ship it"`.
- A `status_changed` event for a **different** `item_id` is ignored (loop continues); a
  `status_changed` event for the right item with `event_type="verdict_recorded"` requested is
  also ignored; the intended `verdict_recorded` event for the right item is the one that
  matches.
  - *Given* the handler is called with `event_type="verdict_recorded"`, and the test publishes (in order) a `status_transition` event for a different item, then a `status_transition` event for the right item, then a `verdict_recorded` event for the right item, *When* the handler returns, *Then* `EventKind == "verdict_recorded"` — proving both the item_id filter and the event_type filter correctly skip non-matching events rather than returning on the first item-matching-but-wrong-kind event.
- No matching event published within `timeout_seconds` returns the `WAIT_TIMEOUT` shape from
  Story 1.2.1's AC.
  - *Given* `timeout_seconds=1` and no `Publish` call for that item, *When* `waitForBacklogEvent` is called, *Then* it returns after ~1s with `Success:true`, `Error.Code:"WAIT_TIMEOUT"`, `EventReceived:false`.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 2.3.1a: Add `TestWaitForBacklogEvent_ReturnsMatchedEventOnLiveVerdict` (~5 min)
- Run `waitForBacklogEvent` in a goroutine (result channel), publish the matching event from the
  test's main goroutine after a short `time.Sleep` (or, better, after asserting
  `bus.SubscriberCount() == 1` via a short poll loop to avoid a sleep-based race — prefer the
  poll-on-subscriber-count approach per the fix-flaky-tests rule), assert the result on the
  result channel with a generous `time.After(3*time.Second)` failure bound.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 2.3.1b: Add `TestWaitForBacklogEvent_FiltersByEventType` (~5 min)
- Same goroutine/publish shape as 2.3.1a, but publish the three events described in the AC in
  sequence before asserting the final matched result's `EventKind`.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 2.3.1c: Add `TestWaitForBacklogEvent_TimesOutWithNoEvent` (~4 min)
- Straightforward: `timeout_seconds=1`, no publish, assert `WAIT_TIMEOUT` shape and that
  wall-clock elapsed is `>= 1*time.Second` (proves it actually waited, not an instant
  false-positive timeout).
- Files: `server/mcp/tools_backlog_test.go`

---

### Epic 2.4: Concurrency & leak tests
**Goal**: Directly satisfy requirements.md's Non-functional Requirement ("multiple
sessions/tool calls... must not interfere... or leak stream subscriptions/goroutines") and
pitfalls research findings 1 and 6 (goroutine-leak test, deterministic race-window seam).

#### Story 2.4.1: Concurrent waiters, goroutine leak, and the subscribe-before-read race seam
**Acceptance Criteria**:
- Two concurrent `waitForBacklogEvent` calls on the **same** `item_id` both receive the same
  published event independently (no double-fire, no missed delivery for either caller).
  - *Given* two goroutines each call `waitForBacklogEvent(item_id, timeout_seconds=5)` concurrently, and once both have subscribed (`bus.SubscriberCount() == 2`), the test publishes one matching `verdict_recorded` event, *When* both goroutines return, *Then* both results have `EventReceived:true` with identical `VerdictOutcome`/`VerdictSummary` — proving `EventBus`'s per-subscriber fan-out (already-existing behavior) is exercised correctly by two `wait_for_backlog_event` calls specifically.
- `goleak.VerifyNone` reports no leaked goroutines after (a) a timeout-exit call and (b) a
  matched-event-exit call.
  - *Given* `baseline := goleak.IgnoreCurrent()` taken before either call, *When* a timeout-path call (`timeout_seconds=1`, no matching event) and a matched-path call (as in Task 2.3.1a) both complete, *Then* `goleak.VerifyNone(t, baseline)` reports no leaks — proving `defer h.eventBus.Unsubscribe(subID)` and the bus's own `ctx.Done()`-triggered cleanup goroutine (`pkg/events/bus.go:59-63`) both terminate cleanly on every exit path.
- A test using `testAfterWaitSubscribeHook` proves the subscribe-before-read ordering actually
  closes the missed-event race: an event published *inside* the hook (i.e., after `Subscribe`
  returns but before the precheck read) is not missed.
  - *Given* `testAfterWaitSubscribeHook = func() { bus.Publish(matchingVerdictEvent) }` set for the duration of the test (restored via `t.Cleanup`), *When* `waitForBacklogEvent(item_id, timeout_seconds=2)` is called for an item with no precheck-satisfying state, *Then* the result is `EventReceived:true` (not a timeout) — if subscribe-before-read were reversed, this event would land in the gap and be missed, and the test would see a `WAIT_TIMEOUT` instead.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 2.4.1a: Add `TestWaitForBacklogEvent_ConcurrentWaitersBothReceiveEvent` (~5 min)
- Two goroutines + a `sync.WaitGroup` or two result channels; poll `bus.SubscriberCount() == 2`
  before publishing (avoids a sleep-based race per the fix-flaky-tests rule); assert both
  results.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 2.4.1b: Add `TestWaitForBacklogEvent_NoGoroutineLeak` (~5 min)
- Import `"go.uber.org/goleak"` (already imported elsewhere in the package's tests, e.g.
  `analytics_store_test.go`; confirm whether `tools_backlog_test.go` needs its own import or
  whether adding it here is the first use in this file — either is fine, it's already a project
  dependency). Cover both the timeout-exit and matched-event-exit paths in one test (two
  sub-calls) before the single `goleak.VerifyNone` assertion, per pitfalls research finding 7's
  explicit "not just context-cancellation" directive.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 2.4.1c: Add `TestWaitForBacklogEvent_SubscribeBeforeReadClosesRace` (~5 min)
- Set `testAfterWaitSubscribeHook`, restore it to `nil` via `t.Cleanup` (this is a shared
  package-level var — tests using it must not run `t.Parallel()` relative to each other; note
  this in a short test comment rather than silently relying on Go's default serial test
  execution, matching `backlog_service_events_test.go`'s equivalent test's own precedent for
  `testAfterSubscribeHook`).
- Files: `server/mcp/tools_backlog_test.go`

---

## Verification Checklist (for `sdd:4-validate` / implementer self-check)

- [ ] `go build ./...` passes with the new handler, helpers, result type, and error code.
- [ ] `make lint` passes (no new `go vet`/staticcheck findings on the new code).
- [ ] `go test ./server/mcp/... -run TestWaitForBacklogEvent` passes, including the goleak test.
- [ ] `go test ./server/mcp/...` (full package) passes — confirms the guidance-text edit
      (Epic 1.3) didn't break `TestGetBacklogItem_WorkRoleGuidance_InstructsShipOnPassAndAfterAttemptCap`
      or any other existing string-matching test against that block.
- [ ] Manual smoke test (optional but recommended): run a second instance per this repo's
      `CLAUDE.md` "Manual/interactive testing" section, call `wait_for_backlog_event` via an
      MCP client against a real backlog item, confirm the precheck and live-event paths both
      behave as designed.
