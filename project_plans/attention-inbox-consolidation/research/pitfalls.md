# Pitfalls: attention-inbox-consolidation

Scope: what commonly goes wrong for this feature type, and in this repo. Confidence labels: VERIFIED (opened or ran it), INFERRED (reasoned from code read).
Rating scale: Likelihood (L) and Impact (I) are H/M/L.

## Findings that change the requirements' framing

- **`UpdatedAt` is the idle clock inside the determiner.** `session/review_queue_determiner.go:282` fires `ReasonIdle` when `time.Since(inst.UpdatedAt) > 5s`, and `:270` uses it for the 30-minute WaitingForAgent stuck threshold. VERIFIED. This is not the requirements' hypothesis (a display bug only). Any change to what bumps `UpdatedAt` changes detection behavior.
- **Both reads are raw field reads.** `:270` and `:282` read `inst.UpdatedAt`, while `:177` correctly uses `inst.Snapshot().UpdatedAt`. VERIFIED. That is a latent violation of `.claude/rules/instance-lock-free-reads.md`. Fix it while touching the file (see P10).
- **The Claude Stop hook is a no-op today.** `server/services/hook_receivers.go:64-70` (`HandleStop`) only logs session and byte count and drains the body. VERIFIED. So "hook/transcript turn-ended signal" is plumbing to build, not something to reuse. `readBody` (`:105-110`) already bounds payloads at 64 KB and says the body may hold secrets.
- **The reload-resets-timer hypothesis is unconfirmed here.** `touchUpdatedAt` (`session/instance_state.go:36`) exists so the frontend `upsertSession` reducer does not skip an update whose `UpdatedAt` matches the cached copy. VERIFIED. So `UpdatedAt` doubles as a frontend change-detection token. Removing bumps will silently drop UI updates (P6).

## Pitfalls

### P1. Prose-question false positives re-flood the queue
- L: H. I: H.
- Agents end almost every turn with some question-like prose ("Want me to also...?", "Let me know if..."). A regex on `?` at an idle prompt flags most finished turns as "needs me".
- Mitigation:
  - Make "turn ended" the primary signal: wire the Stop hook, which is a stub today, and use hook/transcript data as the gate.
  - Apply the question heuristic only to the last assistant message, never to arbitrary screen tail.
  - Give the new reason its own priority below approvals.
  - Cap per-session emissions (one item per turn-end episode) and measure precision on the fixture corpus before the flag defaults on.
  - Keep `attention_prose_question_detection` default-off through the soak, per memory `feedback_rollout_flags_live_settable_no_env_vars`.
- Evidence:
  - `session/review_queue_determiner.go:166-288` (idle, ack and footer suppression).
  - `hook_receivers.go:64`.
  - Project memory `instinct_detection_status_ready_dead_code`: `StatusIdle` is shared with shell/vim prompts (`command_prompt`, `vim_normal_mode`), so bare idle is not "Claude waiting".

### P2. False negatives for non-Claude programs (aider, agy, pi) and `StatusReady` dead code
- L: H. I: M.
- `detection.StatusReady` is never produced by `MatchLines`. Gating on it makes the feature inert, silently.
- Hooks exist for Claude (and partly agy; `session/detection/binaries/`, `session/mux/hooks.go:30,98`). Memory `instinct_antigravity_hooks_dont_gate_permissions` shows agy hook semantics differ from Claude's.
- Mitigation:
  - Per-program signal table with an explicit "no reliable signal, fall back to idle-for-N" row.
  - Pin allowlists against the real pattern set with a test, as `isSafeSteerStatus` does (`server/services/session_service.go`, test `TestSafeIdleStatusContexts_MatchClaudeIdlePatternDescriptions`).
  - Build the coverage corpus per program from real terminal tails.
- Evidence: memories `instinct_detection_status_ready_dead_code`, `instinct_antigravity_hooks_dont_gate_permissions`. Detector history: `7335c2d76` (#678, footer recognition regressed and was patched), `166621092` (#1058, compacting regex needed live-capture verification), `0452a8fc2` (#759, count-parsing edge case). Detection regexes here drift with Claude Code UI updates.

### P3. LLM classification: cost, latency, and a repo precedent that points the other way
- L: M. I: M.
- A per-turn-end LLM call over tens to hundreds of sessions costs tokens and adds seconds. It also needs a failure mode: a classifier outage must not hide approvals.
- Precedent exists: batched classify-once tagger `cdfd4e5cf` (#825) and a cheaper pinned default triage model `d04889c1b` (#926).
- Mitigation:
  - Treat the LLM as an optional tiebreak, not the primary path.
  - Run it only on turn-end when the heuristic is ambiguous, with a single flight per episode and a timeout that fails open to "show as low-priority waiting".
  - Never put it on the poll path.
  - Cache by hash of the last message.
- Evidence: commits above.

### P4. Notification flood and alert fatigue
- L: H. I: H.
- Hidden/system sessions suppress task-complete/idle/stale today. The requirement to surface "finished" more loudly collides with that rule (named in the requirements' rabbit holes).
- The determiner re-adds idle items on a 5 s threshold, so a new reason plus a loose threshold produces a stream of Low items.
- Mitigation:
  - Inbox is the durable surface, with only high-priority reasons (approval, error, prose-question) allowed to toast or push.
  - Finished/idle items are silent inbox rows with a count badge.
  - Keep the hidden-session gate authoritative: routine events for hidden sessions stay suppressed, but may still be logged to the event log.
  - Add a hysteresis or episode ID, as `4e01e8f45` (fork-pressure) did.
  - Add a rate-limit counter and a soak flood check (items per hour per session).
- Evidence:
  - `docs/reference/notification-delivery-gate.md:93-103` (policy table: routine events suppressed for hidden sessions).
  - `0b3ad5387` (#987), `a9b19d863` (#974), `6f59a75a4` (#802).
  - Dedup precedent: `f86a949c8` (dedup question cards by text and labels).

### P5. Dedup collisions with existing dedup and double-counting in the event log
- L: H. I: M.
- Three dedup layers already exist:
  - `NotificationHistoryStore` dedup by `(sessionID, notificationType, questionKey)`: `server/notifications/store.go:646-660`, run once at startup.
  - The hidden-session delivery gate counters.
  - `HookDeduper` (`server/services/hook_deduper.go:34-47`).
- A new event log that records "notification emitted" and "attention item created" for the same transition will count twice. A prose question and an AskUserQuestion for the same turn will produce two cards.
- Mitigation:
  - Event log records state transitions at one chokepoint (the determiner or transition layer), never notification sends.
  - Include a stable episode ID (session, turn-end timestamp) as the dedup key and reuse `questionKey` semantics.
  - Add a test that one turn-end yields exactly one item and one log row across the AskUserQuestion, hook and prose paths.
  - Verify behavior with the delivery gate flag both on and off.
- Evidence: files above; `3350e679b` (legacy hidden-session suppressions counted by site, a double-counting precedent); `a7f2129fe` (names the store dedup characterization test).

### P6. Changing `UpdatedAt` semantics breaks other consumers
- L: H. I: H.
- Consumers found (VERIFIED by grep):
  - Determiner idle clock: `session/review_queue_determiner.go:177,270,282`.
  - Frontend `upsertSession` reducer skips updates whose `UpdatedAt` equals the cached copy: comment at `session/instance_state.go:32-35`, `web-app/src/lib/store/sessionsSlice.ts`.
  - Session list sort option "Sort: Updated": `web-app/src/lib/hooks/useFilteredGroupedSessions.ts:148`, `SessionList.tsx:1143`.
  - `selectActiveSessionsSortedByUpdatedAt`, used by Omnibar and `useSessionRepoPaths`: `sessionsSlice.ts:162`.
  - Persistence: `session/ent_repository.go:271,515,1362`, `session/instance_serialization.go:303`.
  - Session card text: `SessionCard.tsx:1272`.
  - Rename bumps it: `session/instance_terminal.go:116`.
- Backlog `UpdatedAt` is a separate field on `BacklogItem` and is used as a CAS precondition (`session/storage.go:1236`, `ExpectedUpdatedAt`). Do not touch it; keep the two concepts apart in grep and review. Backlog stale-session thresholds (`b2a12eac2`, #219) read session activity, so check them too before redefining.
- Mitigation:
  - Do not change `UpdatedAt`.
  - Add a new snapshot field (for example `LastStateChangeAt`, persisted) for the card and the "unchanged for a while" view.
  - Keep `touchUpdatedAt` as the change-token.
  - Move the determiner's idle clock to `LastMeaningfulOutputTime()`. This fixes a latent bug too: renames, reconnects and status flips currently reset idleness.
  - Characterization-test the sort and reducer first.
  - Confirm the reload cause before designing: check whether hydration or reconnect calls `touchUpdatedAt` (`instance_actor_setters.go:499,847,880,964`; `instance.go:2700`; `instance_workspace.go:368`).

### P7. Ack/snooze semantics resurrect or bury items
- L: M. I: H.
- `suppressedByAck` is `IsAcknowledgedAfterOutput()`: the ack suppresses only until the next output (`review_queue_determiner.go:76-82`, `instance_state.go:258-263`).
- A prose-question item acked then re-emitted on any later spinner output is "resurrection". Conversely, an ack that outlives a new question buries it (false negative).
- "Durable until acknowledged or state changes" (requirements) needs a precise definition of "state changes".
- Mitigation:
  - Ack is keyed to the episode ID, not the session.
  - A new turn-end creates a new episode and un-acks.
  - Output without a turn-end does not.
  - Snooze needs an expiry.
  - Both are table-driven tested.
  - Never use ack to delete event-log rows.
- Evidence: `review_queue_determiner.go:198,282,303`.

### P8. Event-log growth, retention, and secrets in persisted terminal text
- L: M. I: H.
- The notification store already has a retention sweep and an orphan prune interval (`server/notifications/store.go:147,168`). A new per-session log without a cap grows with every status flip. Excerpts of terminal tails can contain tokens, `.env` contents, or credentials.
- The hook body is documented as possibly containing secrets (`hook_receivers.go:102-104`).
- Mitigation:
  - Persist structured fields only (reason enum, timestamp, program, status context description), not raw terminal text.
  - If an excerpt is wanted, cap it (about 200 chars) and run it through the existing redaction code if there is any; otherwise reference the existing scrollback by offset instead of copying it.
  - Per-session and global row caps plus time-based pruning, with a `retention_dropped` counter.
  - Prune on session delete/archive: events `c4a2737e5` (#854) and `b08607e77` show cleanup must be event-driven.
  - ent schema only; never commit generated `session/ent/*` (per CLAUDE.md).

### P9. Removing `/review-queue` breaks deep links, e2e, registry, analytics, shortcuts, onboarding
- L: H. I: M.
- Touchpoints found (VERIFIED by grep):
  - Route tables: `web-app/src/lib/nav/paths.ts:8,74`, `route-meta.ts:46`, `routes.tsx`. A three-file change per memory `instinct_pr_ci_gates_lint_routes_config_surface`.
  - Nav shell: `CockpitShell.tsx:34` hides the floating shortcuts button on that path. Nav components with tests: `BottomNav`, `DrawerNav`, `Header`, `ReviewQueueNavBadge`.
  - Page: `web-app/src/app/review-queue/page.tsx`.
  - Hooks: `useReviewQueueNotifications.ts`, `useReviewQueue.ts`, `reviewQueueVisibility.ts`.
  - Settings: `CallbackSettings.tsx:40` ("On review-queue item created") and `SlackNotificationSettings`.
  - Notification ids: `lib/utils/notifications.ts:17` (`review-queue-tier1-...`).
  - Feature catalog: `lib/features/features/review-queue.ts` and `catalog.test.ts`.
  - Registry: `docs/registry/features/backend/review-queue/watch.json` and siblings.
  - Keyboard shortcuts: `KeyboardShortcutsTab.tsx`, `ShortcutHelpOverlay.tsx`.
  - E2E: 12+ specs in `tests/e2e/` reference it, including `review-queue-severity`, `escalation-reasoning`, `enter-detection`, `idle-session-chip`, `nav-navigation`, `route-titles`, `accessibility`, `visual-regression`, `lighthouse.config.js`. Specs must carry `// @feature` headers (CI-enforced).
  - Onboarding tour modal found: `BacklogTourModal.tsx`. Whether it mentions the review queue was not checked (UNVERIFIED).
- History shows nav churn: `666a7a54c` (#1031, tray became default), `5689b771d` (#999), `c5ce3bc2c` (#1119, nav facade migration).
- Mitigation:
  - Keep the RPCs (`ListReviewQueue`, `WatchReviewQueue`, `Acknowledge`) and callbacks when retiring only the page. Deleting RPCs triggers registry and RPC-surface gates.
  - Replace the page with `<Redirect>` from `@/lib/nav` (not `next/navigation`) behind the `attention_inbox_consolidation` flag. Preserve query params.
  - Grep-driven checklist for the 12+ specs.
  - Run `make registry-generate`.
  - Keep `route-meta.test.ts` green.

### P10. Repo rules the implementation can trip
- L: M. I: M.
- **Snapshot rule:** new code reading `i.UpdatedAt`, `i.Path` or similar must use `Snapshot()`. Existing violations at `review_queue_determiner.go:270,282` should be fixed in the same change, as collateral debt (memory `feedback_fix_collateral_debt`).
- **noarchivedrevival:** the attention surface must read `IsArchived()`/`ArchivedAt` and never call `Start(false)`/`RecoverFromStopped()` to "surface" a crashed or auto-paused session. In-scope files include `session/health.go`, `session/review_queue_poller.go`, `session/session_driver.go`. Cite `c2a30a8b9` (#810) and `e6b1824f6` (#301: archived sessions were once polled into the queue). Any "surface suspended sessions" secondary scope must be read-only.
- **Suspended sessions:** `shouldSkipSession` (`session/review_queue_poller.go:822-835`) skips silently. Adding the required "skip reason" log line must not spam: log once per state change.

### P11. CI gates fail after local checks pass
- L: H. I: M.
- Gates (all from project memory and CLAUDE.md):
  - Go: gocognit applies to modified existing functions. `Determine()` is a long function; adding branches will trip it. Extract per-reason helpers. `dupl` runs new-code-only.
  - Web: `eslint-gate.sh` (zero warnings on changed files, so a merge of main can turn warnings into failures). jscpd at 0.14% absolute (new heavily-mocked test files push it).
  - New RPCs:
    - `TestUIRpcSurface_*` requires classification and runs only under `-tags sinkguard`.
    - Registry `tested`/`testIds` are hand-set.
    - Avoid "gate" in test filenames (BUG-087).
    - `TestConfigSurfaceProof` for each new flag.
  - `server/deliverygate/sink_guard_test.go` `reviewedConsumers`: any new `bus.Subscribe` consumer of `EventNotification` must be registered.
  - E2E: no `waitForTimeout`; `data-testid`/ARIA locators only.
- Mitigation: run `go test -tags sinkguard -short -run UIRpcSurface ./server/services`, `-run TestConfigSurfaceProof`, `tsc`, and the eslint-gate command from memory after merging main.
- Evidence: memories `instinct_new_rpc_ci_gates`, `instinct_pr_ci_gates_lint_routes_config_surface`, `instinct_ci_hermetic_testing_gotchas`, `instinct_flake_root_causes_wakes_and_backoff`.

### P12. Flaky tests from timing in the new code
- L: M. I: M.
- Idle thresholds, turn-end debounce and episode timers invite real sleeps; poller tests have a flake history (`03574716b`, #871; `6b0658cdb`, #934, no raw sleeps). Many unrelated parallel failures usually mean one DATA RACE.
- Mitigation: inject a clock (`timeNow` hook exists at `instance_workspace.go:368`). Use `wait.RequireEventually`. Pin `Math.random` in jest tests that touch backoff. Read the `deterministic-fast-tests` skill.

### P13. Mobile and desktop parity
- L: M. I: M.
- Merging attention items into Up Next (`/unfinished`, PR and backlog tabs) adds a tab or section on a narrow viewport that already has `BottomNav`/`DrawerNav`. The tray had phone deck overlap fixes (`666a7a54c`).
- Mitigation: design both form factors in one pass (memory `feedback_mobile_desktop_ux`). Touch targets of at least 44 px. Test at a phone viewport in e2e. The event-log drawer must be a full-screen sheet on mobile. Axe CI blocks on WCAG AA.

### P14. Process risk: overbuilding the inbox
- L: M. I: M.
- Memory `feedback_review_repair_loops_overbuilding`: repeated new blockers in one subsystem means simplify or delete. The scope here (detector, log, RPCs, page consolidation, timer) is large.
- Mitigation:
  - Ship in independently flagged slices: (1) turn-end signal plus the new reason, (2) event log, (3) timer field, (4) consolidation last.
  - Keep a stop rule for review rounds.
  - Check for parallel work first (memory `feedback_check_for_parallel_work_before_long_pipelines`); `project_plans/review-queue-event-driven/` and `project_plans/backlog-item-activity-log/` are named in the requirements as possible overlap.

## Past regressions in this area (git log)

| Commit | Lesson |
|---|---|
| `e6b1824f6` #301 | Archived sessions were polled into the review queue. Archival check must be first. |
| `5b4d9ea30` #227 | Dead-end notifications for headless review/triage sessions. Exclude non-operator sessions. |
| `fc9198dd7` #325 | Poller race and fragile mutation found at verify of escalation-reasoning. |
| `b2a12eac2` #219 | Stale-session thresholds had to be recalibrated, so thresholds need soak data. |
| `c2a30a8b9` #810 | Auto-revival at 7 sites; invariant now lint-enforced. |
| `7335c2d76` #678, `166621092` #1058, `0452a8fc2` #759 | Detector regex drift and live-capture verification. |
| `0b3ad5387` #987 | Delivery gate enabled by default, legacy hidden checks removed. The gate is now authoritative. |
| `f86a949c8` | Question cards needed dedup by text and labels. Prose questions will hit the same issue. |
