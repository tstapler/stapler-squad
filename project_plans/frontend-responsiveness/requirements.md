# Requirements: frontend-responsiveness

**Date**: 2026-10-25
**Type**: feature addition (performance)
**Complexity**: 3 — system design (transport + render-path + data-fetch changes across web-app and server)

## Problem Statement
Switching windows in the terminal viewer, navigating Sessions <-> Backlog, and loading the Backlog page feel slow for the single operator. Exported evidence (`~/Downloads/stapler-squad-backend.har`, `stapler-squad-Trace-20261025T200313.json.gz`) and an isolated-instance Playwright/CPU profile point to three independent causes.

## Evidence (VERIFIED = command run on the exported files; INFERRED = not proven)
1. **Unary RPCs queue in the browser (VERIFIED symptom, INFERRED cause).** HAR: 234 entries, all `localhost:8543`, HTTP/1.1. 161 entries have status 0 and `blocked` ~30,000 ms (never connected). ~58 `ListProgramsConfig`, ~56 `LogClientEvents`, 19 `ListFiles`, 16 `GetVCSStatus` calls in a 16 s capture. 14 identical `GetFeatureFlags` calls fire at the same instant (t=2.8 s), taking 0.7-4.5 s each. Only streams are WebSocket (20 x status 101); unary uses fetch. Likely cause: the browser's 6-connections-per-origin HTTP/1.1 limit, with slow calls (ListShells 4-6 s, GetVCSStatus 3-4.5 s, GetFeatureFlags 1-2 s server wait) holding slots. What held the slots is not visible in a HAR.
2. **Main-thread layout cost (VERIFIED).** Trace: `getBoundingClientRect` is the top JS self-time (2,614 ms of ~8.8 s non-idle sampled); `UpdateLayoutTree` 3.8 s over 1,646 calls on ~3,200-3,330 elements; 822 animation frames costing 3.4 s; 18 tasks > 100 ms, 49 > 50 ms (max 452 ms). Callers include `useTerminalSizing`, `XtermTerminal`, `PaneTilingContainer`, `SessionList`. DevTools was attached, so absolute times are inflated.
3. **Window switch remounts everything (VERIFIED by reading code + profile).** Each window has its own `paneState`; `SessionDetail` is keyed `${pane.id}-${pane.sessionId}`, so a switch unmounts/remounts the session list and terminals. Isolated-instance measure with empty windows: 0.6-1.8 s per switch with 150-650 ms of long tasks; hard `/backlog` load 4.2 s (3.4 s long tasks); `/` load 18.8 s (15 s long tasks) on a dev-state instance.

## Baseline
Today: unary RPCs fan out per component, each page/hook builds its own transport (34 call-site edits; the file-by-file count and the files that stay are pinned in `implementation/plan.md` Story 2.1.1, not here), flags are fetched by many consumers, windows are torn down on switch.

## Users / Consumers
Single operator using the web UI on localhost:8543 (and remote-access mode). Usage context is **unmeasured**: how often the operator switches windows, and how many sessions and windows are typically open, are not recorded anywhere. The perf fixture therefore assumes 20 sessions across 4 windows (plan Story 1.1.3, `perfFixtureShape`) as a stress shape from "dozens of sessions, 3+ windows" (Non-functional Requirements), not as an observed typical day.

## Success Metrics (to be measured with the same HAR/trace + Playwright method)

**Which baseline gates.** Every baseline figure quoted in this document (0.6-1.8 s switches and 150-650 ms long tasks on empty windows; `getBoundingClientRect` 2,614 ms of ~8.8 s with DevTools attached; 18 long tasks; `/backlog` 3.4 s; the HAR counts) is **directional only**: it came from different instances, datasets and DevTools states and is not comparable to a fixture number. The gated baseline is `tests/e2e/perf/baseline.json`, captured by the Playwright harness on the 20-session fixture before any Epic 2 story merges (plan Story 1.1.3, tag `perf-baseline`). Absolute targets (< 150 ms, < 1 s, no task > 100 ms) are absolute; relative targets (`getBoundingClientRect` -80%) are relative to `baseline.json`, not to 2,614 ms.

- **SM1** Zero unary requests with `blocked` > 1 s during a window-switch + Sessions<->Backlog session; no status-0 stalls. Resource Timing does not see WebSocket-routed calls, so the metric is `queuedOver1s + wsQueuedOver1s` (plan Story 1.1.2) and moving calls to WS cannot satisfy it vacuously. **Fallback if the stalls prove server-side** (plan Story 1.1.4c finding, Story 5.1.2 server-side row): SM1 is done when (i) Story 2.1.8 has landed; (ii) `queuedOver1s == 0` and status-0 `== 0` hold for the `critical` request class (plan Story 2.1.9) in the Epic 2 exit run (Story 2.1.7), with the `deferred` class capped at 3 concurrent calls; and (iii) the server handler wait p95 for `ListShells` and `GetVCSStatus` is <= 1 s with fewer than 6 handlers in flight, or the before/after p95 is recorded and the remaining suspect is named as a follow-up (for `GetVCSStatus`, open bug `docs/bugs/open/BUG-020-vcs-status-diff-mutex-contention.md`). If (iii) is not met the target-miss rule below applies.
- **SM2** Duplicate-call count: <= 1 `GetFeatureFlags` per page load (baseline 14); `ListProgramsConfig` and `LogClientEvents` batched/deduped (baseline 58 and 56 in 16 s).
- **SM3** Window switch: < 150 ms to interactive, long tasks < 50 ms (directional baseline 0.6-1.8 s, 150-650 ms; gated baseline: `baseline.json`). Which CPU-throttle condition gates this number is `OPEN-DECISION-throttle-gate` (`implementation/plan.md`, Open Decisions; resolved: unthrottled gates, 4x is advisory).
- **SM4** `getBoundingClientRect` self-time reduced by >= 80% relative to `baseline.json` in an equivalent CDP profile pass; no task > 100 ms during switching (directional baseline: 18 tasks).
- **SM5** `/backlog` hard load long-task total < 1 s (directional baseline 3.4 s; gated baseline: `baseline.json` cold median).
- **SM6 (perceived speed)** Input-to-next-paint responsiveness of the two interactions the operator feels: p75 Event Timing duration (the INP definition) of the window-switch click or leader key, and of the Sessions -> Backlog nav click, <= 200 ms on the fixture, with the active tab highlight painted in the first frame after input (`design/ux.md` criterion 1). 200 ms is the web.dev "good" INP threshold (cited from memory, not re-fetched this session; owner-revisable). Measured with a `PerformanceObserver({type: "event", durationThreshold: 16})` in the perf spec (plan Task 1.1.3b), baseline recorded in `baseline.json`, gated run per `OPEN-DECISION-throttle-gate`.
- **SM7 (guard: no terminal regression)** Terminal keystroke-echo latency does not regress: the p95 of an echo-marker round trip (keydown dispatch to the first output frame carrying the marker, timed spec-side from Playwright `page.on("websocket")` frame timestamps so the **same measurement runs against the `perf-baseline` build**) in every flag state is within +10% of the baseline p95, on the same session, fixture and throttle, >= 30 round trips per kept run, >= 5 kept runs. It fails when the p95 exceeds 1.10 x baseline **and** the excess is larger than 2x the MAD (plan Story 1.1.3 noise rule). Reuses the echo-marker driver of plan Task 5.5.1c; the in-page `__ssqPerf.terminalLatency` tap (render-inclusive) remains the B-versus-C measure for Story 5.5.1 criterion 5.

**Target-miss rule.** If a target (for example the 150 ms switch or the < 1 s backlog load) is unreachable after the planned work lands, the report records the achieved number and its cause; the owner then decides accept or extend. A miss never silently re-scopes a target and never reopens a built epic (plan Open Decisions, `RULE-target-miss`).

## Size
Large (3-15M CU) at requirements time (INFERRED band, see ESTIMATION.md). The plan's re-summed estimate is above that band (more than 15M CU with the verification multiplier; see `implementation/plan.md`, Effort estimate); informational only. Full scope is the default and the owner has chosen no downscale.

**Wall-clock blockers**: ~~owner decision on transport direction~~ **resolved 2026-10-26: unary-over-WebSocket is built as a first-class epic (plan Epic 5), the HTTP path stays as permanent fallback and kill switch, and the three-arm measurement decides only the rollout default**; a before/after HAR+trace re-capture by the owner with DevTools closed (cleaner numbers); ~~owner resolution of `OPEN-DECISION-throttle-gate` before the `window_keep_alive` default flip~~ **resolved 2026-10-26: Option A, the unthrottled run gates (plan Open Decisions)**.

## Constraints
- Must not break the live service on :8543; test via isolated instance (CLAUDE.md manual-instance recipe).
- Terminal streaming and keyboard handling must not regress; hidden terminals must not double-handle keys.
- `make ready` gates (lint, dupl, jscpd) apply.

## Non-functional Requirements
- **Performance SLO**: see Success Metrics.
- **Scalability**: dozens of sessions, 3+ windows.
- **Security classification**: internal (localhost and the authenticated remote-access listener); the WebSocket unary path stays behind existing HostGuard on `:8543`, and on the remote listener behind authentication on every request plus a strict same-origin/allow-listed Origin check (plan Task 5.2.1g, ADR-002 item 7); no credential ever travels in the URL.
- **Data residency**: no special requirements.

## Scope
### In Scope
- Request-path: single shared transport, request dedupe/coalescing (feature flags, programs config, client-event logging batching), request-level priority/deferral of non-critical calls (plan Story 2.1.9), and unary-over-WebSocket built as a flagged, reads-only multiplexed channel served on both the local and the remote listener (owner decisions 2026-10-26; HTTP/2 via local TLS and fewer-connections were evaluated and not chosen), measured against baseline and against dedupe alone before it can become the default.
- Accessibility and stall-UX fixes that fix present defects and ship unflagged: window-switch announcement, focus and `tabpanel` wiring, optimistic tab highlight (plan Story 4.1.4); `RoutePending` stall escalation and `aria-busy` (plan Story 6.1.3). Their rollback is revert-by-PR (an exception to Risk Control's flag rollback).
- Zero-size resize guards: a hidden or zero-size terminal must never resize the real tmux window (server guards on the handshake nudge paths and `Instance.ResizePTY`, client handshake guard; plan Stories 3.1.2c/3.1.2d).
- Render-path: remove forced-layout reads (`getBoundingClientRect` loops), batch resize/measure work, stop animation-frame work for hidden surfaces.
- Navigation: keep-alive of recent windows (bounded), faster Sessions<->Backlog return, Backlog page load/render.
- Perf regression guard (a repeatable Playwright profile script / budget).

### Out of Scope
- Backend query speed (ListShells/GetVCSStatus server-side latency) beyond what the frontend can mask — noted as follow-up. One conditional exception: plan Story 2.1.8 (`resolveSessionTitle` reads the whole session list on every shell RPC) is in scope only if the Story 1.1.4 server-phase capture shows the stalls are server-side contention.
- Redesigning the pane/window data model.

## Rabbit Holes
- Unary-over-WebSocket needs a server-side bridge, auth/HostGuard parity, reconnect/backpressure semantics, and a Connect transport adapter; may be larger than fixing the call fan-out.
- Keep-alive touches window-level capture keydown handlers, `isActive`, mobile CSS vars, and hidden xterm sizing (0x0 under `display:none`).
- WebGL (xterm renderer) context limits with many mounted terminals.

## Alternatives Considered
1. Unary-over-WebSocket multiplexed transport (user's expectation).
2. HTTP/2 via TLS on localhost (browser h2 needs TLS) — cert/trust cost.
3. Reduce/dedupe/batch calls so the 6-slot limit is no longer reached (cheapest, also helps option 1/2).
4. Keep-alive windows vs cheaper remounts.

## Feasibility Risks
- 30 s blocked cause is inferred; Phase 2 must confirm it (e.g. re-capture HAR with connection IDs, count concurrent in-flight per origin over time).
- DevTools overhead inflates trace durations.
- **Value risk: Epic 5 may add little over Epic 2.** Unary-over-WebSocket carries the largest security surface in the plan (a new upgrade route, per-listener dispatch, a remote-listener origin and auth policy, a CSWSH test set) and, on remote origins that already negotiate HTTP/2 or on local paths where Epic 2 dedupe leaves little contention, the measured gain over request dedupe plus priority may be small or negative. This is a **measured outcome** of the three-arm comparison (plan Story 5.5.1: arm B Epic 2 only versus arm C Epic 2 + WS, and the remote/h2 arms), not a reason to defer the build. **Owner decision recorded 2026-10-26: build it regardless**; the default flips ON only if the numeric criteria pass, otherwise the epic stays built behind its OFF flag and the result is reported under the target-miss rule.

## Observability Requirements
Client perf marks for window-switch and route-change durations; in-flight/queued request gauge (HTTP and WS; runtime opt-in via `?ssqperf=1` or `localStorage["ssq:perf"]`, absent unless opted in); reuse existing `/actuator` counters where possible.

## Risk Control
Behind a feature flag (default off until verified): keep-alive windows and any new transport; staged flip via existing flag system. Rollback = flip flag (exception: the unflagged accessibility fixes above are reverted by PR).

## Open Questions
- ~~Which callers produce the 58 `ListProgramsConfig` and 56 `LogClientEvents` calls, and why so frequent?~~ Causes identified in code (plan Stories 2.1.5 and 2.1.3): `useAvailablePrograms` has no module cache or in-flight dedupe (one RPC per mounting consumer, `research/stack.md:25`), and each mounted `TerminalOutput` installs its own chained console patch and log buffer (duplicating every line N times). The counts stay unverified until the perf spec runs (Story 1.1.3).
- ~~Does the page run over `localhost` or remote-access (h2 possible there)?~~ The HAR is `localhost:8543` over HTTP/1.1 (Evidence 1); remote access (:8444, TLS) already gets h2. **Owner decision 2026-10-26 (reverses the earlier "WS disabled on remote")**: the WS channel is served on the remote listener in the first release, enabled per origin by the `unary_over_ws_remote` sub-flag after a remote/h2 comparison (plan Risk Control, Stories 5.2.1, 5.4.1, 5.5.1).
- ~~Is the transport choice (WS vs h2 vs fewer calls) the owner's call after research?~~ Resolved: owner chose unary-over-WebSocket on 2026-10-26 (see Wall-clock blockers).

*(unresolved after Phase 2 research)*
- Why 14 identical `GetFeatureFlags` calls fire at once when code reads as 1 (`FeatureFlagsContext.tsx:105`, `useFeatureFlagValue.ts:30`) — needs `console.trace` on a fresh capture.
- What held the browser's other connection slots during the ~30 s stalls (only ~3 unary requests in flight while 160+ were queued, t=12-30 s) — needs net-export or an in-flight gauge. Also open: whether the cause is server-side contention rather than browser queueing (`ListShells` is an in-memory lookup but runs a full session-list read per call; the client's own 30 s abort also yields status-0 / `blocked ~30000` entries); plan Task 1.1.4c captures server handler duration to tell the two apart.
