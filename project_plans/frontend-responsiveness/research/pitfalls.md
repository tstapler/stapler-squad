# Pitfalls research: frontend-responsiveness

Labels: VERIFIED = command run or source opened (cited); INFERRED = reasoning or recalled outside knowledge, not checked here.
Paths are relative to `web-app/src/` unless noted. Trace analysis script: `/tmp/ssqan/callers.py` (and `/tmp/ssqan/chunk.py`), run as `python3 -I /tmp/ssqan/callers.py getBoundingClientRect`.

## 1. Who calls getBoundingClientRect (2,614 ms self time)

**VERIFIED (trace):** 2,578 ms (98.6%) of the 2,614 ms is a single function, `measure` at chunk `2690.*.js:44`, whose parent in the CPU profile is `(root)` (2,534 ms), i.e. it runs from a rAF/timer task, not from a React effect. Only 45 ms comes via `commitHookEffectListMount` (the hook's mount-time `measure()`). Other callers are negligible: `Virtualizer.measureElement` 20 ms, `extractCellMismatchInputs` (XtermTerminal) 3 ms, xterm `_positionFromMouseEvent` ~0 ms.

**VERIFIED (identification):** the same chunk (lines < 120) contains, in order, `usePublishStackTopOffset` (line 21), an anonymous effect (40), `measure` (44), `schedule` (52). That matches `lib/hooks/usePublishStackTopOffset.ts` (`usePublishBottomEdge`: `measure` and `schedule` closures). The line numbers are shifted ~5 from source, so the match rests on the function-name cluster, not the line number. I could not open the built chunk (no `.next/static/chunks/2690*` in the worktree), so the attribution is INFERRED-strong, not byte-verified.

**Mechanism (VERIFIED by reading `lib/hooks/usePublishStackTopOffset.ts:36-78`):** each hook instance installs
- a `MutationObserver` on `document.body` with `{childList: true, subtree: true}`,
- a `ResizeObserver` on `document.body` and on its element,
- a window `resize` listener,

and every one of those schedules `requestAnimationFrame(measure)`, which does `el.getBoundingClientRect()` (plus `extraRef?.getBoundingClientRect()`) and then calls `publish(...)`. Any DOM insertion or removal anywhere on the page (list rows, toasts, xterm DOM nodes, tooltips) therefore schedules one forced-layout read per hook instance per frame. This is the read-after-write layout-thrash pattern: the page writes DOM, then the rAF reads layout.

**Instances (VERIFIED by grep):**
- `usePublishStackTopOffset(tabRowRef)` in `components/sessions/SessionDetailView.tsx:465`: always `enabled`, one per mounted SessionDetailView (including hidden ones).
- `usePublishDeckTopOffset(toolbarRef, isVisible !== false, overflowRowRef)` in `components/sessions/TerminalOutput.tsx:129`: gated by visibility, but the gate only skips setup when hidden; each visible terminal still observes the whole body.
- `usePublishDeckTopOffset(barRef)` in `components/sessions/files/MobileBars.tsx:68`: no `enabled` argument, so always on.

**Keep-alive implication (INFERRED from the above):** the cost scales with the number of mounted SessionDetailViews. Keeping N windows alive multiplies observers by N unless these hooks are gated on `isActive`, or replaced by one shared body observer that measures once per frame and fans the result out (the `toastDock` setters `setStackTopOffset`/`setDeckTopOffset` are already module-level, so a single measurer is natural).

**Why 822 animation frames cost 3.4 s (VERIFIED counts, INFERRED link):** `FireAnimationFrame` events: 822, total 3,380 ms (script `/tmp/ssqan/chunk.py`). `measure` self time is 2.6 s of that, so ~77% of rAF time is this one callback. The feedback loop: `publish` updates the toast dock, which changes DOM or style, which fires the MutationObserver or ResizeObserver, which schedules the next frame. I did not confirm that `publish` mutates the observed DOM; check `lib/utils/toastDock.ts` (set-if-changed equality would break the loop).

### Other layout-read candidates (VERIFIED by reading, not trace-attributed)
- `components/sessions/XtermTerminal.tsx:1246-1370`: a fixed-cadence "decoupled sampler" (setTimeout chain, up to `MAX_SAMPLES` = 20) calls `proposeDimensions()` and, after a fit, `containerRef.current.getBoundingClientRect()` (line 1321) and `extractCellMismatchInputs`. Bounded, so not a never-idle loop. Trace says it is ~3 ms.
- `XtermTerminal.tsx:1408` reads `el.offsetHeight` deliberately to force a reflow before sampling.
- `lib/terminal/TerminalPool.tsx:421-444` (`fitOnShow`): a rAF retry loop (up to `FIT_ON_SHOW_MAX_ATTEMPTS` = 300 frames) reading `anchor.getBoundingClientRect()` each frame until non-zero. A pooled terminal docked into a still-0x0 anchor spins for up to 300 frames (about 5 s at 60 fps); a ResizeObserver also re-arms it. Cheap per frame but it is a rAF loop that can stay alive across window switches.
- `components/pane/PaneTilingContainer.tsx:84`: one read, only on a user action. Not a hotspot.
- xterm 6.x `DomMeasureStrategy.measure` uses `offsetWidth/offsetHeight` (comment: "triggers a synchronous layout"), not `getBoundingClientRect` (`node_modules/@xterm/xterm/src/browser/services/CharSizeService.ts:93-102`). So xterm is not the `getBoundingClientRect` source, but it does force layouts on font or DPR change.
- Not attributable from this trace: `SessionList.tsx`, `ResizeHandle`, `CDPViewer`, `OpenTabsBar` appear nowhere in the `getBoundingClientRect` caller chains (0 ms each).

`UpdateLayoutTree` (3.8 s, 1,646 calls) is the style recalc that follows the DOM mutations those rAF reads force; I did not break it down by trigger (INFERRED).

## 2. Keep-alive hidden xterm terminals

Repo already has relevant machinery (VERIFIED):
- `lib/terminal/TerminalPool.tsx`: pool with LRU eviction, `DEFAULT_TERMINAL_POOL_MAX_SIZE = 8`, pin counts, `docked` entries never evicted; may exceed max when everything is pinned/docked (line 254).
- `lib/terminal/webglBudget.ts`: `MAX_WEBGL_CONTEXTS = 8`, evicts hidden-first then LRU, swapping the victim to another renderer. Its own comment says the ~16 browser limit is "not verified here".
- Keep-alive windows must go through this pool, not bypass it. Pool max (8) and WebGL budget (8) are separate caps; 3 windows x several panes can exceed both.

Pitfalls:
1. **0x0 fit under `display:none` (VERIFIED in code, known xterm behavior).** The existing code guards it: `XtermTerminal.tsx` ResizeObserver sets `wasZero` and refits with `reason: "visibility"` on restore (lines ~1482-1495); `fitOnShow` retries until non-zero. Using `display:none` for hidden windows triggers all these paths on every switch. Prefer `visibility:hidden` + `position:absolute`/`content-visibility` or keep real dimensions, or accept the refit on show. INFERRED: `visibility:hidden` keeps layout, so no 0x0 refit, but still pays style/layout cost for hidden subtrees unless `contain: strict` / `content-visibility: hidden` is used.
2. **A fit of a 0-size container sends a bogus resize to the backend.** Must stay blocked; see `ADR-002` under `project_plans/terminal-resize-fit-loop/decisions/`. A regression here resizes the tmux pane for every client.
3. **WebGL context limit (INFERRED, widely cited ~16 in Chrome, lower on some GPUs).** Context loss blanks the oldest terminals. The budget exists; verify the budget counts hidden-but-alive terminals (the `isVisible()` callback decides victim order) and that evicted terminals do not fall back into a retry loop.
4. **Hidden-tab timers (INFERRED, standard browser behavior).** rAF does not fire for hidden *documents*, but rAF does fire for elements hidden by CSS, so CSS-hidden terminals still run `fitOnShow`/observer rAF work. `setTimeout`/`setInterval` are clamped to 1 s only when the *tab* is backgrounded, not for CSS-hidden content. The 30 s `useFeatureFlagValue` interval and the sampler timers run per mounted component regardless.
5. **Per-instance listeners multiply (VERIFIED, see section 1 and 3).** Each kept-alive window re-registers MutationObservers, window keydown handlers, and `useReviewQueueNavigation`.
6. **Console patching chain (VERIFIED, `lib/hooks/useBrowserLogStream.ts:37-41` comment).** Multiple simultaneous `TerminalOutput` instances each patch `console.*` and chain. `XtermTerminal.tsx` calls `console.log("[XtermTerminal] Sampler confirmed resize ...")` (line ~1315); `log`/`warn`/`error` are streamed unconditionally to `LogClientEvents` (only `debug` is opt-in; `MAX_BUFFER=50`, `FLUSH_INTERVAL_MS=5000`). With N mounted terminals each producing logs and each hook instance flushing its own buffer, `LogClientEvents` calls scale with N. This is a plausible source of the 56 `LogClientEvents` calls in 16 s (INFERRED: not matched to HAR callers). Fix at the source: one process-wide log buffer.
7. **Memory.** Each kept-alive xterm holds its scrollback; the pool max (8) bounds it, but keep-alive needs its own bound (the requirements say "bounded").

## 3. Duplicate global keydown handlers

**VERIFIED (grep):** 40+ `window`/`document` keydown registrations exist (command: `grep -rnE "(window|document)\.addEventListener\(\s*['\"]keydown" web-app/src`). The ones that matter for keep-alive:
- `components/sessions/SessionDetailView.tsx:675` `window.addEventListener("keydown", handleKeyDown)`: handles Escape (exit fullscreen) and Shift+Arrow review-queue navigation (`onNext`/`onPrevious`). It has **no `isActive` gate** (the file does take `isActive`, line 389, and uses it only for `useShortcut(..., isActive)` at line 634). With two alive SessionDetailViews, Shift+ArrowRight fires `onNext` twice.
- `components/pane/PaneTilingContainer.tsx:188`: capture-phase picker handler, only while `pickerPendingSession`; safe, one container.
- `lib/hooks/useReviewQueueNavigation.ts:136`, `lib/hooks/useFilesShortcuts.ts:52`, `lib/hooks/useKeyboard.ts:95`, `components/sessions/SessionList.tsx:790` (only in `selectMode`), `components/sessions/Omnibar.tsx:1369`: each needs an owner check if its component can be mounted twice. I did not audit each one.
- `lib/shortcuts/shortcutRegistry.ts:66-80`: single `document` listener for all `useShortcut` registrations. Good pattern: `useShortcut(..., enabled)` takes an enable flag (usage at `SessionDetailView.tsx:634`), so keep-alive windows should pass `isActive`.

Pitfalls (INFERRED unless cited): duplicate handlers cause double navigation, double `preventDefault`, double dispatch; capture-phase `stopPropagation` in a hidden window's handler can swallow keys for the visible one; `useShortcut` registrations with the same id across windows may collide or last-wins in the registry (check `shortcutRegistry.ts` duplicate-id behavior before keep-alive). Requirement says "hidden terminals must not double-handle keys": add a test mounting two views and asserting one handler call per keypress.

## 4. Unary RPCs over WebSocket

**VERIFIED context:** `lib/api/transport.ts` has three transports: shared unary HTTP `getConnectTransport()` (fetch, auth interceptor, 30 s default timeout), a keepalive variant, and `getWatchTransport()` -> `lib/transport/watch-ws-transport.ts` (WebSocket bridge for server-streaming Watch RPCs). Server side: `server/services/connectrpc_websocket.go` (upgrader with `CheckOrigin: isAllowedOrigin`, line 97), `ws_stream_bridge.go`. `HostGuard` is configured at `server/server.go:1929` (`hostGuardConfig`). I did not verify whether HostGuard wraps the WS routes or only the HTTP mux; the WS handler has its own origin check (INFERRED: parity must be explicitly tested).

Pitfalls:
1. **Head-of-line blocking (INFERRED, protocol reasoning).** WebSocket is one ordered TCP byte stream. Multiplexed unary calls share it with terminal output; a large `ListFiles` response or a terminal burst delays everything behind it, and one slow handler does not block others only if the server dispatches concurrently and writes frames whole. The HTTP/1.1 6-slot queue is replaced by TCP HOL blocking, which can be worse for latency-sensitive keystrokes. A separate socket for unary traffic avoids terminal coupling.
2. **No backpressure on the existing client reader (VERIFIED).** `fromWebSocket` in `watch-ws-transport.ts` pushes every message into an unbounded array `queue` (lines ~33-45). Reusing this path for responses inherits unbounded buffering.
3. **Reconnect semantics.** Unary calls in flight when the socket drops need a defined outcome: fail with `Code.Unavailable` (as `fromWebSocket` does for non-1000 closes) vs transparent retry. Retrying non-idempotent RPCs (create/delete session) is unsafe (INFERRED). Needs per-request IDs, timeouts (`Connect-Timeout-Ms` semantics, which the HTTP path gets from connect-go for free: `transport.ts:14-20`), and cancellation propagation (connect-go ctx cancel on client abort).
4. **Auth parity.** `createAuthInterceptor` (redirect to /login on 401) works on HTTP status codes. A WebSocket carries no per-call HTTP status; auth failure would surface as a close code or an in-band error and the interceptor would not see a 401 (INFERRED). Cookie/Origin checks occur once at upgrade; session expiry mid-connection is not re-checked unless added.
5. **Server bridge cost.** Needs a handler that replays frames through the existing connect-go handler chain (interceptors, HostGuard, metrics, `/actuator` counters, rpcTiming). Anything skipped silently differs from the HTTP path.
6. **Ecosystem fit (INFERRED).** `@connectrpc/connect-web` has no official WebSocket unary transport; a custom `Transport` must implement `unary` + `stream` and Connect envelopes (the repo already hand-rolls envelopes in `watch-ws-transport.ts:encodeEnvelope`).
7. **Does it even fix the stall? (INFERRED, needs verification.)** Chrome treats WebSocket in a separate socket pool from the 6-per-host HTTP/1.1 limit (recalled, not verified here). Requirements already note the 161 status-0 entries with ~30 s `blocked` have an unproven cause. First confirm with a HAR re-capture or `chrome://net-export` which requests held the 6 slots; deduping `GetFeatureFlags` x14 and batching `LogClientEvents`/`ListProgramsConfig` may remove the need entirely (alternative 3 in requirements).
8. **HTTP/2 alternative (INFERRED).** Browsers only do h2 over TLS; localhost needs a trusted cert. Remote-access mode may already be TLS (`--remote-port 8444`; verify) so h2 could help there at zero client cost.

## 5. Request-dedupe caches and stale flags

**VERIFIED state of play:**
- `lib/hooks/useFeatureFlagValue.ts`: module-level cache (30 s TTL) plus in-flight promise sharing (`loadFlags`), 10 s timeout, and a per-hook `setInterval` re-read every 30 s. So hook consumers are already deduped.
- `lib/contexts/FeatureFlagsContext.tsx:102-126`: a *separate* path: `client.getFeatureFlags({})` straight from the provider, with its own staleness guards (a response lands only if no newer fetch landed and no write read back, lines ~97). It does not share `loadFlags`. Two independent fetchers means at least 2 calls per page load, plus any consumer that bypasses both; the 14 simultaneous calls at t=2.8 s are not explained by `useFeatureFlagValue` alone (INFERRED: other call sites exist; `grep` finds only these two files plus `gen/` and `features/flags.ts` metadata, so the fan-out may be per-mount remounts of the provider, or calls from a code path using `createClient` directly: check HAR initiators).
- `lib/hooks/useAvailablePrograms.ts`: **no cache or in-flight sharing**: every mount calls `listProgramsConfig({})` (`// abort-signal-exempt`). Consumers include `ProgramsManager.tsx`. Per-mount calls multiply with keep-alive/remounts: a likely source of the 58 `ListProgramsConfig` calls (INFERRED, not tied to HAR initiators).

Pitfalls:
1. **Stale flags after a write.** The provider's `setFlag` re-reads (line ~167) but `useFeatureFlagValue`'s module cache is not invalidated (only `resetFeatureFlagValueCache()` is exposed, "tests, and after a flag flip a caller knows about"). A flag flipped in settings can read stale for up to 30 s in hook consumers; the kill switch (comment says the 30 s re-read exists for this) bounds worst-case staleness. If the new dedupe layer is the single source, it must invalidate on `setFlag` and on server push/Watch if one exists.
2. **Dedupe must not share an `AbortSignal`.** The existing code deliberately uses `AbortSignal.timeout` rather than per-component abort for exactly this reason (comment in `loadFlags`). A new generic dedupe must do the same, or one unmounting component cancels everyone's request.
3. **Cache key must include request args and identity.** `GetFeatureFlags({})` has no args today; a generic coalescer keyed by method name only will return wrong data for parameterized RPCs (`ListFiles`, `GetVCSStatus`). Key by method + serialized message; never coalesce mutations.
4. **Error caching.** `loadFlags` correctly does not cache failures (`cached` is set only on success; `inFlight` cleared in `finally`). Preserve this; caching a rejection pins a failed state for the TTL. Fallback default-OFF on failure is an explicit safety property.
5. **Batching `LogClientEvents`.** Keep the 50-entry/5 s thresholds in one shared buffer; flush on `pagehide`/`visibilitychange` using the keepalive transport (`getKeepaliveConnectTransport`, 64 KB body limit) or logs are lost on navigation; never log inside the flusher (reentrancy guard `intercepting` exists).
6. **Time source.** Tests inject `now`; keep that seam.

## 6. Repo-specific rules to respect (VERIFIED from `CLAUDE.md` and `.claude/rules/`)

- **`.claude/rules/nav-facade.md`:** import `useRouter`/`usePathname`/`useSearchParams`/`Link`/`Redirect` from `@/lib/nav`, never `next/navigation` or `next/link`; no `window.history.pushState/replaceState` outside `src/lib/nav` (use `useReplaceSearch`, `replaceHash`, `pushTrayEntry`, `pushHistoryMarker`); pass `{ resetScroll: false }`, not `{ scroll: false }`. ESLint enforces it. Any keep-alive/route-persistence work (Sessions <-> Backlog) must go through the facade. A new page needs a `route-meta.ts` row (`route-meta.test.ts` fails otherwise).
- **Jest mocking of `@/lib/nav`:** mock `@/lib/nav` (not `next/navigation`) and spread `jest.requireActual("@/lib/nav/impl.memory")` so `Link` etc. stay defined. Existing examples: `app/insights/session-detail/page.test.tsx:18`, `app/settings/backlog-quota/useUnsavedChangesGuard.test.tsx:7`.
- **`.claude/rules/instance-lock-free-reads.md`:** applies to Go `session/instance*.go`. If any server-side bridge or batching handler reads `*Instance` fields, use `Snapshot()` (or `Workspace().ActiveDir` to compare locations), not raw fields. Pure frontend work is unaffected.
- **`.claude/rules/noarchivedrevival.md`:** any new server code path that calls `Start(false)`/`RecoverFromStopped()` in the listed lifecycle files needs an archival check. A keep-alive feature must not "revive" archived sessions' terminals/streams automatically.
- **`.claude/rules/norawghrequest.md`:** not relevant unless new GitHub HTTP calls are added.
- **Duplication gates (`make ready`):** `dupl` is new-code-only vs `origin/main` (Go); `jscpd` in `web-app/.jscpd.json` is an absolute threshold, currently 0.14%, `minLines/minTokens` = 20/200. Adding a second near-copy of `useBrowserLogStream`-style buffering or of the `usePublishBottomEdge` observer block, or heavily mocked test files (irreducible `jest.mock` blocks), pushes the ratchet. Prefer one shared module.
- **E2E conventions (skill `e2e-test-conventions`, CI-enforced):** first line `// @feature ...`; no `waitForTimeout` (use `expect(...)`/`waitForSelector`); `data-testid`/ARIA locators only; page helpers under `tests/e2e/pages/`. The perf-regression guard script should follow this (use `performance.mark`/`measure` + `PerformanceObserver('longtask')` and poll for state rather than sleeping). Existing `performance.measure("session:click-to-render")` (`SessionDetailView.tsx:547`) and `rpc:<Method>` measures (`lib/telemetry/rpcTiming.ts`) can be reused as budget hooks; `web-app/build/budgets.json` already exists (INFERRED purpose: bundle budgets, not opened).
- **Testing hygiene:** `deterministic-fast-tests` skill (no real sleeps/timeouts); frontend tests run with `cd web-app && bunx jest --no-coverage`; always Bun, never npm/yarn.
- **Manual testing:** never `make install-service` for in-progress work; use the isolated instance recipe in `CLAUDE.md` (ports 62871+, private `STAPLER_SQUAD_TMUX_SOCKET`).
- **New RPC (if a batch endpoint is added):** proto change -> `make proto-gen`, handler in `server/services/`, `make registry-generate`, new RPC registry JSON needs `tested: true` plus `testIds` (memory note `project_registry_tested_ci_gate`).

## 7. Priority list for the planner

1. The single biggest measured cost is one hook (`usePublishBottomEdge`), not xterm or the pane container. Fix first: gate on `isActive`/visibility, dedupe to one body observer, measure only when the value could change, skip `publish` on equal values. Expected effect: most of 2.6 s `getBoundingClientRect` and the associated `UpdateLayoutTree` drops (INFERRED estimate; re-trace to confirm the 80% target).
2. Before keep-alive: gate the unguarded `SessionDetailView.tsx:675` keydown handler on `isActive`; audit the other handlers in section 3; centralize console-log buffering.
3. Confirm the cause of the 30 s `blocked` with a connection-level capture before choosing a transport; the in-repo dedupe gaps (`useAvailablePrograms` uncached, `FeatureFlagsContext` bypassing `loadFlags`, per-instance `LogClientEvents` buffers) are cheap and verified.
