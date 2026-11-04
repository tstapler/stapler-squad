# Cross-artifact consistency review: frontend-responsiveness

Date: 2026-10-26. Inputs read in full: `requirements.md`, `implementation/plan.md` (1133 lines), `design/ux.md`, `decisions/ADR-001..003`. Severity per `4-validate/cross-artifact-consistency-prompt.md`: BLOCKER = contradiction or coverage gap, CONCERN = scope drift or terminology, NITPICK = UX alignment and cosmetic. Where a finding is a partial gap or a stale-but-superseded statement I downgraded it and say why.

Totals: **2 BLOCKER, 18 CONCERN, 15 NITPICK**.

Numbers re-derived (VERIFIED by command or arithmetic this session):
- `grep -rl createConnectTransport web-app/src | grep -v -E 'gen/|\.test\.|__tests__|__mocks__'` = 37; test files = 66; `useShells.ts` is not in the 37 (grep count 0). Plan's 2 stay + 2 no-change + 33 migrate = 37, plus `useShells` = 34 edits = 12 (batch b) + 22 (batch c). Consistent; no 34 vs 37 conflict.
- 37 `#### Story` headings and 37 estimate rows; 82 `##### Task` headings and the table's per-row task counts sum to 82. Raw tokens sum to 8.230M, CU to 11.977M, x1.5 = 17.97M; per-row CU arithmetic checks (x1.48 / x1.32). The estimate is internally correct.
- Flag names (`request_coalescing`, `unary_over_ws`, `window_keep_alive`), markers (`ws_method_not_allowed`, `ws_unary_remote_disabled`), localStorage keys, 256 KiB / 4x read-limit / 32 in-flight / 64-frame queue, K = 3, 9/9/10/10 method counts match across plan and ADRs.

---

## BLOCKER

### B1. `ux.md` S6 contradicts plan Story 6.1.3 on whether nav-link warm-up starts the Backlog watch subscription
- `design/ux.md` S6: hover/focus "starts the Backlog watch subscription/store hydration once ... (the existing `WatchBacklogItems` stream hydrates the store)".
- `plan.md` Story 6.1.3: "**No Backlog watch subscription is started by warm-up**"; warm-up is hydration-only and issues no stream, because a hover-started subscription would be an owner-less second subscriber (AX2 B1).
- Both give testable behavior (UX AC27 and the 6.1.3 AC assert different things about subscriptions). The plan is the repaired version; `ux.md` was not updated.
- Resolution: edit S6 to "reads the persisted snapshot into `backlogItemsSlice`; starts no stream and issues no RPC", and drop the stream claim from the final bullet.

### B2. The window-switch pass/fail threshold is specified three incompatible ways (throttled vs unthrottled)
- `ux.md` AC13: median <= 150 ms and no long task >= 50 ms "(4x CPU throttle)".
- `plan.md` Story 1.1.3: "baseline and **all target comparisons** use the same CPU throttle (4x)"; the unthrottled figure "is not comparable".
- `plan.md` Task 4.1.4c and Story 4.1.6 criterion 1: gate on **unthrottled** numbers; 4x is "advisory". `requirements.md` Success Metrics state no throttle.
- This decides the keep-alive default flip (Story 4.1.6) and requirement metric 3, so an implementer cannot know which run gates.
- Resolution: pick one gating condition (the plan's 4.1.4c/4.1.6 wording is the most recent) and make Story 1.1.3's "all target comparisons" sentence and `ux.md` AC13 say "gating: unthrottled; 4x reported/advisory".

---

## CONCERN

### C1. `LogClientEvents` numeric assertion contradicts its own formula (Story 1.1.3)
Formula `ceil(duration_s / 5) + 1` for 16 s = 5; the AC says `LogClientEvents <= 4`. Fix one: use 5, or change the formula to `ceil(duration_s / 5)`.

### C2. Perf fixture cannot exercise LRU eviction (K = 3 with 3 windows) and its pane composition is inconsistent
- Story 1.1.3 fixture: "20 sessions, 3 windows, 200 backlog items". Story 4.1.2 evicts only when a 4th window is visited; Task 4.1.4c's "cold (LRU-evicted)" scenario and UX AC12 need >= 4 windows. `requirements.md` Scalability says "3+ windows".
- Pane counts also differ: Story 1.1.4 and 2.1.6 use an "8-pane window"; Stories 4.1.3 and 4.1.6 use "3 windows x 4 panes".
- Resolution: specify the fixture as >= 4 windows, one with 8 panes, and 3 x 4 for the WebGL-budget case; name it in Task 1.1.3a.

### C3. Requirement "priority/deferral of non-critical calls" has no explicit story (partial coverage only)
`requirements.md` In Scope lists it. Plan covers adjacent mechanisms (lazy FilesTab 2.1.6c, polling gate 4.1.3c, 250 ms rule) but nothing prioritizes or defers slow calls (`ListShells` 4-6 s, `GetVCSStatus` 3-4.5 s) at the request layer. Rated CONCERN rather than BLOCKER because those stories remove most of the load. Resolution: add one AC (e.g. under Story 2.1.6) naming which calls are deferred past first paint, or record in requirements that dedupe + lazy mount satisfies it.

### C4. `requirements.md` is stale against decisions the plan records
- Size: requirements "Large (3-15M CU)"; plan total 17.97M with multiplier, "above the Large band".
- Wall-clock blocker "owner decision on transport direction" and Open Question "Is the transport choice the owner's call" are resolved by the plan's "Owner decision (2026-10-26)".
- Open Questions "which callers produce 58 ListProgramsConfig / 56 LogClientEvents" are answered by Stories 2.1.5 and 2.1.3 but stay open in requirements.
- Resolution: update requirements.md (size, decision, resolved questions) so it stays the source of truth.

### C5. Scope drift: stories with no requirement behind them
- Epic 5 builds a server bridge, client transport, policy and three ADRs; requirements In Scope says "**evaluating** unary-over-WebSocket vs HTTP/2 vs fewer connections" and flags it a Rabbit Hole. The owner decision justifies it but requirements.md does not record it.
- Story 4.1.4 (announce, `tabpanel`, `pendingWindowId`, shipped unflagged) and Story 6.1.3's `RoutePending` stall escalation/`aria-busy` come from `ux.md` gaps, not requirements. They also conflict with requirements Risk Control ("Rollback = flip flag"); the plan documents "revert by PR only".
- Story 5.3.2 (reader-queue instrumentation) is self-described in its own context text as scope drift.
- Resolution: add these as In Scope lines (accessibility fixes, stall UX, WS transport) in requirements.md, or mark them as accepted extensions.

### C6. "Dev-only" gauge vs runtime opt-in vs production presence
- `requirements.md` Observability: gauge "dev only"; plan Glossary `InFlightGauge`: "Dev/test-only ... Off in production builds"; `ux.md` S7: `__ssqPerf` "only in dev/test (`typeof` ... undefined in production)".
- Plan Story 1.1.2/Observability: exists in **any** build when opted in via `?ssqperf=1` or `localStorage["ssq:perf"]` (needed because e2e runs the embedded production binary).
- Resolution: reword Glossary, requirements and ux S7 to "runtime opt-in; absent unless opted in".

### C7. Terminology drift in `ux.md` S4/S6
`ux.md` uses `CachedListSnapshot` and "snapshot warm-up"; plan Glossary says `CachedListSnapshot` was replaced by `BacklogStoreHydration`, warm-up is "hydration-only". The S4 key description also omits the status-filter scoping that Story 6.1.2 adds. Resolution: rename in `ux.md`, add `statusFilterKey`.

### C8. Interceptor placement is stated two ways (double application risk)
ADR-002 item 7 and Story 5.3.1: `UnaryWsTransport.unary` runs through `runUnaryCall` **so that** auth/rpc-timing/30 s deadline apply to WS calls. Story 5.4.1 and Task 2.1.2b: the same interceptors wrap the **policy transport outermost**. Doing both double-applies timing/auth to WS calls; and the HTTP transport still carries its inner interceptors (today's `transport.ts:32-39`). Resolution: state that `runUnaryCall` receives an empty interceptor list (or the HTTP inner ones are removed) and add a test that `rpc:<Method>` is emitted once per call on each path.

### C9. ADR-003 option C trigger vs Story 5.1.2 table
ADR-003 says option C is scheduled "by the Story 5.1.2 gate if arm B shows `queuedOver1s > 0`". Plan table row 2 makes only Task 5.4.1c and the HOL probe mandatory and does not schedule the priority socket; the plan header (line 14) says the gate decides "(c) whether a second priority socket is built". Also header line 14 lists only 5.5.1/5.5.2/flip as gated while Story 5.1.2 adds "Task 5.4.1c ordering/priority". Resolution: add "schedule option C" to row 2 or edit ADR-003.

### C10. Task 2.1.2a depends on a fixture created in Task 5.1.1b
2.1.2a's Jest test asserts `wsEligible` equals the fixture `ws-unary-allowlist.json`, which Task 5.1.1b creates; 5.1.1b is ordered after Story 2.1.1 completes and Story 2.1.2 is Wave B. No stated order between 2.1.2a and 5.1.1b. Resolution: move fixture creation into 2.1.2a (5.1.1b then reuses it) or defer that single assertion to 5.1.1b.

### C11. Story 5.2.1 does not depend on the spike it is meant to validate
Story 5.1.1 (Task 5.1.1a) confirms the dispatch seam and may amend ADR-001/002; Story 5.2.1 "Depends on: 5.1.1b (not on the 5.1.2 gate)" and the diagram draws `5.1.1 spike -> 5.1.1b -> 5.2.1`. Task 5.2.1f/5.2.1b implement the seam the spike proves. Resolution: add Task 5.1.1a as a dependency of 5.2.1f, or state that spike amendments are applied as rework.

### C12. `RootShell.tsx` edit order couples Epic 6.1.3b to Epics 5 and 4
Order `5.4.1c -> 4.1.5a -> 6.1.3b` makes a small a11y fix (`aria-busy`, stall UI) wait on the WS bootstrap and the keep-alive lift, while the diagram claims Epic 6.1 "needs 1.1.5, 2.1.1c" and Phase 4 is "parallel with Phase 3". 5.4.1c's priority is also gated by 5.1.2 (owner-recapture blocker). Resolution: accept and record it in Story 6.1.3's `Depends on`, or give 6.1.3b its own edit seam (separate component) so it is not serialized.

### C13. Story 4.1.4 is "independent of keep-alive" but depends on host-owned pieces
- Focus "into the target window's last-focused pane" needs the per-window focus memory, which Task 4.1.2a (flag-gated, later) creates; with the flag OFF the memory does not exist. UX AC17 demands identical behavior flag ON/OFF.
- Task 4.1.4c measures with `window_keep_alive` ON, so it cannot run before Story 4.1.2.
- Resolution: move `useWindowFocusMemory` into 4.1.4 or state the flag-OFF fallback (active pane from `paneState`), and re-parent Task 4.1.4c under 4.1.6/4.1.2.

### C14. WS "non-local listener" rule is ambiguous and may disable WS in the measuring environments
Risk Control: disabled "when `location.port` is not the local listener"; Story 5.4.1: "non-local listener (`:8444`)". The e2e harness and manual instances use dynamic/62871 ports (`findFreePort`, CLAUDE.md), so a "port != 8543" rule would never open WS in arm C of Story 5.5.1. Resolution: define detection (for example the remote port from `/api/server-info`, or a server-side hint) and test it on a non-8543 port.

### C15. Metric 1 definition narrows from "unary requests" to "HTTP unary"
Requirements: "zero unary requests with blocked > 1 s". Plan metric `queuedOver1s` reads Resource Timing, which excludes WS-routed calls, so moving calls to WS satisfies it vacuously; WS-side waiting (250 ms rule, send queue, HOL) is only covered indirectly by 5.5.1 criteria 2-3. Resolution: add a WS-call queue-time measure (REQUEST send to RESPONSE minus server time, or a client timer) to the Story 1.1.2 gauge.

### C16. `RoutePending` timeout has no value and no owner
`ux.md` S5 and Story 6.1.3: error state "at transport timeout". Route chunk loads are not RPCs, so `DEFAULT_RPC_TIMEOUT_MS` (30 s) does not obviously apply. Resolution: state the timeout (value and source) in Story 6.1.3.

### C17. `window_keep_alive` pre-load behavior is undefined, and Story 2.1.4 contradicts itself
Story 2.1.4 preserves "default-OFF-until-loaded", while Risk Control gives `request_coalescing` a pre-load ON constant. After the Story 4.1.6 flip (`window_keep_alive` default ON) an OFF-until-loaded flag would mount the single container first, then remount into the host when the flag arrives. Risk Control defines pre-load constants for only two of the three flags. Resolution: state the pre-load constant for `window_keep_alive` and how first render avoids a remount (for example persist it like `unary_over_ws`).

### C18. Success-metric baselines not re-based on the fixture (`/backlog` 3.4 s)
Story 3.1.1 correctly re-bases the `getBoundingClientRect` target on `baseline.json` (DevTools inflates the trace). Story 6.1.2 and ux AC28 still say "< 1 s (baseline 3.4 s)", a figure from a different instance/dataset in requirements. Resolution: express the target as "< 1 s and report vs `baseline.json` cold median" like Story 3.1.1.

---

## NITPICK

1. UX ACs with no owning story or test in the plan: AC29 (accessible name + focus ring on Retry/Go to Sessions/badge), AC30 (4.5:1 contrast in default + custom theme), AC31 (Axe sweep "with the Backlog in each of S4's states"; plan only covers flag OFF/ON window states), AC32 (screen-reader switch-and-type flow), AC21 (CLS contribution 0), and AC16's "announce nothing" on rollback. Add one AC each to 4.1.4/6.1.2/6.1.3 or an a11y task.
2. `ux.md` S5 and Gaps text still quote "~300 ms" as the plan's value; Story 6.1.3 now keeps 150 ms. Annotate as resolved.
3. `ux.md` S2 specifies only `display:none` + `inert`; plan allows React `<Activity>` if the 4.1.1 spike passes.
4. `ux.md` S4 "Refreshing" is not mapped to a `BacklogConnectionState` value (plan derives it from the existing states; which ones is unstated).
5. Dependency diagram under-lists prerequisites: Epic 4.1 shows "needs 2.1.0, 3.1.2, 2.1.6c" but Story 4.1.2 also needs 4.1.1, 4.1.4b, 1.1.6 and Story 4.1.3 needs 2.1.6a/d and 4.1.2; Story 3.1.1's `Depends on` omits 2.1.6c that the diagram and edit-order include.
6. Wall-clock blocker list (Effort estimate) omits Task 4.1.6a's owner dogfood sign-off (present in the table row); its item (3) "owner dogfood of `unary_over_ws` before widening" is not attached to any task or row.
7. Story 2.1.7 asserts `queuedOver1s == 0` on "load and window-switch scenarios" only; requirement metric 1 includes the Sessions<->Backlog session (scenario exists in 1.1.3).
8. Story 1.1.3's numeric per-method list has no `ListFiles` (19 calls in the HAR) or `ListShells`; only `FilesTab` laziness (2.1.6c) addresses `ListFiles`, without a perf-spec assertion.
9. `requirements.md` Baseline says "37 files import createConnectTransport" as if each builds its own transport; plan shows 33 callers (+ `useShells`), 2 stay, 2 are a comment/mock.
10. Plan Pattern Decisions: "the 14x/58x/56x duplicates are 161 of 234 stalled requests" reads as 234 stalled; requirements: 161 of 234 entries stalled (the five method counts sum to 163, so the claim is approximate).
11. The number 14 is used for three unrelated things (identical `GetFeatureFlags` calls; "14x" console-chain duplication labelled VERIFIED; "14 mounted `TerminalOutput`" in the Story 2.1.3 AC). Only the first is in requirements evidence.
12. ADR-003 says the socket opens "from the root layout"; plan edits `RootShell.tsx`/`WsUnaryBoot.tsx` and states `layout.tsx` is not edited.
13. ADR-002 item 1 says re-entry is "minus Compress/Logging"; later in the same item and in the plan CORS is excluded too. Item 7 has a dropped sentence start ("...loses no sliding refresh.  the Host/Origin matrix test").
14. Review-disposition table keeps pass-1 figures (34 rows, 16.77M) next to the current 37 rows / 17.97M; labelled as history but easy to misread.
15. Story 5.3.2 and Task 5.1.1a describe themselves as scope drift/throwaway; keep them out of the cost table if they are not required (5.3.2 is still counted at 81k CU).

---

## Coverage check (requirements -> stories)

| Requirement (In Scope) | Story |
|---|---|
| Single shared transport; dedupe; log batching | 2.1.1, 2.1.2, 2.1.3, 2.1.4, 2.1.5, 2.1.6 |
| Priority/deferral of non-critical calls | partial only (C3) |
| Evaluate WS vs HTTP/2 vs fewer connections | Epic 5 (built, not only evaluated; C5), 5.1.2/5.5.1 |
| Render-path layout reads, resize batching, hidden rAF | 3.1.1, 3.1.2 |
| Keep-alive (bounded), Sessions<->Backlog return, Backlog load | 4.1.x, 6.1.x |
| Perf regression guard | 1.1.3, 7.1.1-7.1.3 |
| Observability (marks, gauge, actuator counters) | 1.1.2, 5.2.1e |
| Risk control flags, rollback | 1.1.6, 4.1.6, 5.5.2 (4.1.4/6.1.x unflagged; C5) |

Success metrics 1-5 each have an owning story (2.1.7/5.5.1, 2.1.4/2.1.5/2.1.3, 4.1.4c/4.1.6, 3.1.1, 6.1.2); the caveats are B2, C15, C18.
