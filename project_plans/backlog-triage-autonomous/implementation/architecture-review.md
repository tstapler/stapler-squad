# Architecture Review: backlog-triage-autonomous
**Date**: 2026-06-22
**Verdict**: CONCERNS (0 blockers, 6 concerns, 4 nitpicks)

---

## Constitution Violations

No `ADR-000-architecture-constitution.md` exists in this repository. No constitution constraints apply.

---

## Blockers

None.

---

## Concerns

- [ ] **Story 2.1.1 — goroutine context is the HTTP request context, not a server lifecycle context**
  The plan says the goroutine uses a "30-min timeout context", but does not specify what the parent context is. The existing review gate (`BacklogLifecycleListener`) uses `l.shutdownCtx` — a server-scoped context derived in `newListenerBase` and cancelled by `Shutdown()`. `BacklogService` has no equivalent server-scoped context. If the plan wires the goroutine to `ctx` (the ConnectRPC request context), the 30-minute call will be cancelled as soon as `TriggerTriage` returns its HTTP response, which is immediate. **All headless triage calls will be silently cancelled before completing.** Recommendation: add a `shutdownCtx context.Context` / `shutdownCancel context.CancelFunc` pair to `BacklogService` (mirroring the lifecycle listener), initialise it in `NewBacklogService`, and use it as the parent for all goroutine work.

- [ ] **Story 2.1.1 — triageSem lives on BacklogService but needs a shutdown drain path**
  `BacklogLifecycleListener.reviewSem` is guarded by `<-l.shutdownCtx.Done()` in the semaphore-acquire select, and `Shutdown()` drains in-flight calls cleanly. The plan adds `triageSem` to `BacklogService` but no corresponding `Shutdown()` method or shutdown-context select in the acquire block is mentioned. On process shutdown, in-flight goroutines will be abandoned mid-LLM-call, leaving `ItemSession` rows with `ended_at = NULL` and status stuck at `idea`. Recommendation: add a `Shutdown()` method to `BacklogService` that cancels its shutdown context, and use that context in the `triageSem` acquire select (same pattern as the lifecycle listener).

- [ ] **Story 2.1.1 — concurrent re-trigger TOCTOU window between orphan-check and goroutine start**
  The orphan-awareness guard (steps 3a, 3b) tombstones stale sessions and reads `item.Status` synchronously in the RPC handler. The goroutine that actually runs the headless call is dispatched later. A concurrent `TriggerTriage` call that arrives between the guard completing and the goroutine writing its `ItemSession` will find zero open triage sessions and spawn a second headless call. The review gate avoids this because `TransitionBacklogItemStatus` uses an optimistic precondition (`ExpectedStatus + ExpectedUpdatedAt`) so the second call loses the race atomically. For the triage path, the `ItemSession` should be created **synchronously in the RPC handler** (before returning the response), and the orphan guard should check for open sessions of role=triage **including the just-created synthetic one** — exactly as the review gate creates the `ItemSession` inside the goroutine only after acquiring the semaphore. The plan reverses this order: it creates the session inside the goroutine (step 9 of Story 2.1.1). Recommendation: create the `ItemSession` row synchronously in `TriggerTriage` before launching the goroutine, so subsequent calls see it and hit the `CodeAlreadyExists` guard.

- [ ] **Story 1.1.1 — FeatureKeyTriage must be added to AllowedFeatureKeys to avoid a RunHeadlessCall 400**
  `RunHeadlessCall` validates `headless.AllowedFeatureKeys[featureKey]` and returns `CodeInvalidArgument` for unknown keys. Note that `FeatureKeyAutonomousFix` and `FeatureKeyAutonomousApproval` are **not** in `AllowedFeatureKeys` — those keys are used only by internal callers that bypass the RPC validator. The plan correctly identifies that `FeatureKeyTriage` is an internal key used only by `BacklogService`, so it should follow the same pattern as `FeatureKeyAutonomousFix`: declare the constant but **do not add it to `AllowedFeatureKeys`**. The plan says "Added to AllowedFeatureKeys" in the glossary, which contradicts this pattern. Adding it would also expose a long-running (30-minute) feature key to external callers without a separate timeout guard, since `RunHeadlessCall` caps at `MaxCallTimeout = 1800s`. Recommendation: Do **not** add `FeatureKeyTriage` to `AllowedFeatureKeys`. Clarify this in Story 1.1.1.

- [ ] **Story 4.1.2 — integration test polling (up to 2 seconds) is brittle for a 30-minute call**
  The plan says the test "polls storage for status change to ready (up to 2 seconds)". For a real headless pool call this would never complete, so the test must use a fast-path (FakeRunner or a no-op pool). The test description does say "headless.FakeRunner to inject valid JSON" — good. However the integration test also needs to verify that `triageSem` is correctly drained and that the failure path records `ended_at`. Those paths are not mentioned in Story 4.1.2. The fake-runner approach is correct but the coverage scope is narrow. Recommendation: expand Story 4.1.2 to explicitly cover (a) semaphore drain on failure, (b) `ended_at` persistence on parse error, (c) status remaining `idea` on failure, matching the failure cases tested for the review gate in `backlog_lifecycle_test.go`.

- [ ] **Story 2.1.1 — `BuildHeadlessTriagePrompt` takes `BacklogItemData` but the production caller has `*ent.BacklogItem`**
  The plan places `BuildHeadlessTriagePrompt` in `session/headless/features.go` and specifies it takes a `BacklogItemData`. However the existing `buildTriagePrompt` in `backlog_service.go` operates on `*session.BacklogItemData` (a DTO), whereas the review gate's `BuildHeadlessReviewPrompt` in `session/backlog_review.go` takes `*ent.BacklogItem`. The production call site in `BacklogService.TriggerTriage` loads via `s.storage.GetBacklogItem` which returns `*session.BacklogItemData`. Placing the function in `session/headless/features.go` would require importing `session` from the `headless` subpackage — but `headless` must not import `session` to avoid a circular dependency (`session` imports `headless` for `headless.Pool`). The review-gate analogue (`BuildHeadlessReviewPrompt`) is correctly placed in `session/backlog_review.go` to avoid this cycle. Recommendation: move `BuildHeadlessTriagePrompt` and `ParseHeadlessTriageResult` to `session/backlog_triage.go` (a new file, analogous to `backlog_review.go`), keeping `FeatureKeyTriage` and `headlessTriageSystemPrompt` in `session/headless/features.go`.

---

## Nitpicks

- Story 3.1.1 says "Verify submit_triage_result not called in headless mode" — this is a doc note, not a test. The MCP handler still validates `STAPLER_SESSION_UUID` from context; headless calls don't set this, so the gate would trigger ErrPermissionDenied anyway. The "verification" is already implicit. Consider deleting Story 3.1.1 or converting it to a comment in the system prompt.

- `headlessTriageSystemPrompt` has "two responsibilities in one prompt" (write artifact files AND output JSON), acknowledged in ADR-022. Consider flagging this as a TODO for a future streaming-progress enhancement so it does not get forgotten.

- The plan uses `uuid.New().String()` as the `ItemSession.SessionUUID` prefix (`"headless-triage-<uuid>"`). The review gate uses `"headless-review-<uuid>"`. Make the prefix convention explicit in a package-level constant (e.g., `headlessTriageSessionPrefix = "headless-triage-"`) to make it greppable.

- Story 5.1.1 wires `TriageLoadingIndicator` into `BacklogItemCard.tsx` but the plan has no mention of what signal drives it (polling item status, watching ItemSession events, or EventBus). The backend goroutine publishes a notification on completion but nothing signals "in progress". Clarify whether the UI derives loading state from `item.status == "idea" && openTriageSessions.length > 0` or from a dedicated event.
