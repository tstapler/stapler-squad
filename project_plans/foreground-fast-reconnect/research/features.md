# Research: Foreground Fast Reconnect — Feature Landscape

## 1. How "currently selected/visible" is already tracked

There is **no global `activeSessionId`/`selectedSession` context** — selection lives as local
component state in whichever parent renders `SessionDetail`/`SessionDetailView` (e.g.
`web-app/src/app/page.tsx`, `web-app/src/app/review-queue/page.tsx:365`,
`web-app/src/app/insights/InsightsDashboard.tsx:224`). Each passes a `session={selectedSession}`
prop down through `SessionDetail.tsx:88` → `SessionDetailView.tsx`. `SessionDetailView` is **not**
remounted per session switch (no `key={session.id}`) — the same component instance persists across
switches and just receives a new `session` prop, which is what makes its local keep-alive pool
state durable across switches.

**The exact boolean the requirements ask for already exists**, just not exposed as `foreground`:

- `SessionDetailView.tsx:243-257` — `pooledSessionIds` (`useState<string[]>`) is an LRU pool
  (max 8) of session IDs that have been displayed; a session is added to the pool the first time
  it's selected and stays mounted (hidden) after the user navigates away, so switching back is instant.
- `SessionDetailView.tsx:690-704` — for each pooled session, `TerminalOutput` is rendered with
  `isVisible={poolId === session.id}` — **`poolId === session.id` is precisely "is this the
  session currently selected/displayed"**, i.e. the `isSelected` concept the requirements describe.
  The same pattern repeats for the external-mux pool (`isVisible={poolPath === session.externalMetadata?.muxSocketPath}`,
  line 683) and the shell-PTY pool (`isVisible={activeTabId === shellKey}`, line 781).
- `TerminalOutput.tsx:65` already declares `isVisible?: boolean` on `TerminalOutputProps` and
  consumes it at `TerminalOutput.tsx:950-956`: when a pooled terminal becomes visible, it fits +
  focuses the xterm instance 50ms later. This is the natural place to also flip on `foreground`.

**Implication for AC4 ("passes `foreground={isSelected}`")**: the cleanest wiring is not a *new*
signal invented from scratch — it's renaming/extending the existing `isVisible` boolean (or adding
`foreground={isVisible}` alongside it) as it flows `TerminalOutput` → `useTerminalStream`. No new
prop needs to be threaded from `SessionDetailView` beyond what already exists for the main-terminal
case; `TerminalOutput`'s existing `isVisible` prop is the source of truth.

**Caveat found**: for the *main terminal tab* specifically, `isVisible={poolId === session.id}`
(`SessionDetailView.tsx:702`) is **not** additionally gated on `activeTab === "terminal"` — the
surrounding `<div>` is hidden via `display:none` when a different tab (Diff/Browser/Files/etc.) is
active (`SessionDetailView.tsx:652-653`), but the `isVisible` prop passed into `TerminalOutput`
doesn't know that. So today, if a user is on the "Diff" tab of the selected session, the main
terminal's `isVisible` (and therefore a future `foreground`) would still read `true` even though the
terminal isn't what's on screen. This is a pre-existing inaccuracy, not something this feature
introduces, but it directly affects AC2's "first N foreground attempts" semantics — see Edge Cases below.

## 2. `useVisibilityResync.ts` — a related but distinct mechanism

`web-app/src/components/sessions/useVisibilityResync.ts` handles **browser-tab visibility**
(`document.visibilitychange`/`window focus`), not in-app session selection. On tab-foreground with
an already-connected terminal, it triggers a full resync (`requestFullResync(true)`) with its own
4s stall watchdog (`RESYNC_STALL_TIMEOUT_MS`) and 2s banner-delay (`RESYNC_BANNER_DELAY_MS`); on a
disconnected terminal it falls back to `connect()` (lines 161-167) but does **not** touch backoff
timing or attempt counts — it's a parallel/composed mechanism, not the backoff/reconnect path this
feature modifies. It already has hard-won guards worth reusing as a pattern for the new work:
- **Stale-callback guard** (`sessionId !== sessionIdRef.current`, line 118): a debounced resync
  callback armed for session A must no-op if the user has since switched to session B before the
  debounce fires. The same race applies to a foreground-timeout callback armed while switching.
- **sessionId-keyed cleanup effect** (lines 188-198): tears down pending timers when `sessionId`
  changes, specifically called out as needed because "a watchdog/resync armed for the previous
  session must never fire against the next one's connect()/disconnect()" (comment cites an actual
  adversarial-review blocker from a prior feature).
- **Re-entrancy guard** (`pendingResyncCompletionRef`, line 122): prevents a second resync trigger
  while one is already pending — relevant if `foreground` flips true→false→true rapidly (fast
  session-flapping).

## 3. Pre-flag path in `TerminalOutput.tsx` (~lines 720-874) — what it already handles

Confirmed via `grep`/read, `TerminalOutput.tsx` lines 719-874, gated by
`process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true"`:

- **Backoff formula**: `Math.min(1000 * Math.pow(2, connectionAttempts - 1), 10000)` (line 864) —
  a *different*, simpler implementation than the hook-level `BackoffState` (full-jitter, base
  1000ms/cap 30000ms). Two independent backoff mechanisms already coexist in this codebase
  depending on the flag.
- **Reconnect-button delay**: 5000ms after connection loss before showing a manual "Reconnect"
  button (lines 764-770), decoupled from the backoff attempts themselves.
- **Reconnect-banner delay**: 2000ms after disconnection before showing the "reconnecting…" banner
  (lines 796-801), only if the terminal had connected at least once (`hasEverConnectedRef`).
- **Max attempts**: 5 (`connectionAttempts >= 5` clears the loading overlay, line 813; the retry
  effect's own guard is `connectionAttempts < 5`, line 863).
- **No connect-timeout concept at all** — retries are scheduled only from the `error` +
  `connectionAttempts` state changing (which itself only changes reactively to connection-loss
  events), so a hang with no error and no close event would never retry under this path either.

**This confirms AC5's need explicitly**: the requirements' "Current state" section already
establishes the V2 hook path (`useTerminalStream.ts`) is where `foreground` and the new
connect-timeout mechanism belong — its `BackoffState`/`terminalBackoffRef` (line 108) is the
natural integration point, and the acceptance criteria's terminology ("first 2 reconnect attempts",
"reset backoff counter") maps directly onto `terminalBackoffRef.current.attempt`/`.reset()`
(`useTerminalStream.ts:166,334-350,442,465`). The **pre-flag `TerminalOutput.tsx` path (lines
719-874) has no equivalent hook to attach a connect-timeout to** without either (a) duplicating the
timeout logic there too, or (b) leaving pre-flag users (the default, since `NEXT_PUBLIC_RECONNECT_V2`
defaults OFF per `.env.local.example:1`) without the fast-reconnect behavior at all. **The plan
must state explicitly which path gets the feature** — recommend V2-only, since that's where all the
supporting primitives (`BackoffState`, hook-level `connect()`/attempt tracking, `foreground` as a
hook option) already live; note this means the feature is inert until `NEXT_PUBLIC_RECONNECT_V2=true`
is the default, which may itself be an open question for the plan phase.

### Where a connect-timeout must hook in (`useTerminalStream.ts`)

`connect()` (`useTerminalStream.ts:162-361`) opens `clientRef.current.streamTerminal(...)` and
`for await`s messages with **no timeout on the wait for the first message** (`firstMessage` flag,
lines 219-227). If the server accepts the stream but never sends anything (hangs, not a close/error),
the `for await` loop just blocks forever — `isConnectingRef.current` stays `true`, and the retry
logic in the `finally` block (lines 322-352) never runs because the loop never exits. This is the
concrete gap AC1 describes: a connect-timeout must be a `setTimeout` that, if `firstMessage` is
still true when it fires, aborts `abortControllerRef.current` (forcing the `for await` to throw and
fall into the existing `catch`/`finally` retry path) rather than a passive backoff-delay computation.
It must be cleared as soon as `firstMessage` flips to `false` (line 222).

## 4. Edge cases / failure modes the design should handle

1. **Rapid session switching** (explicitly named in the task). If a user clicks through sessions
   A→B→C within a couple hundred ms, `foreground` on A's `TerminalOutput` instance goes
   true→false, B's goes false→true→false, C's goes false→true, all within the pool's lifetime
   (nothing unmounts — pool keeps up to 8). Each `useTerminalStream` instance is independent, so a
   `foreground` toggle must not leak a stale short-timeout connect-timer across the flip — needs
   the same "stale closure" pattern `useVisibilityResync` already uses (ref-mirrored latest value +
   an id/generation check), not a naive `useEffect` cleanup alone, since `foreground` can flip
   multiple times before any single connect attempt resolves.
2. **Multiple terminals mounted simultaneously** (also explicitly named). Because of the pool (up
   to 8 sessions × main terminal, plus shell PTYs, plus mux paths — SessionDetailView.tsx:243-280),
   many `useTerminalStream` instances exist at once, each independently reconnecting per its own
   backoff. Only the foregrounded one should get the fast connect-timeout; backgrounded pooled
   terminals reconnecting after, e.g., a laptop sleep/wake must keep using the slower/normal
   timeout — verify `foreground` is per-instance (it is, since it's a hook option) and that a
   background reconnect storm (8 terminals all reconnecting after network blip) doesn't get
   accidentally sped up.
3. **Tab backgrounded while a terminal is "selected."** `foreground` here means *in-app selection*
   (this session is the one showing in the pane), not *browser-tab visibility*
   (`document.visibilityState`). If the browser tab itself is backgrounded, `foreground` should
   probably stay whatever it was (the session is still "selected" in-app) — but note
   `useVisibilityResync`'s separate tab-visibility listener will *also* fire a resync/connect on
   tab-refocus for the same terminal. The plan needs to state whether tab-refocus should also reset
   the fast-timeout counter (arguably yes — a user tabbing back to the app is exactly the "just
   switched to this and it's disconnected" moment the feature targets) or whether that's scope
   creep beyond AC3's "false→true transition" (which is about the `foreground` *prop*, not tab
   visibility) — recommend treating them as orthogonal per AC3's literal wording, but flag the UX
   overlap for review since both mechanisms fire near-simultaneously when a user alt-tabs back to a
   previously-selected, now-disconnected terminal.
4. **Session switch during an in-flight connect attempt** (explicitly named). If `foreground` flips
   false→true while a connect attempt (fast or normal) is already `isConnectingRef.current === true`
   for that same instance, `connect()`'s own guard (`if (isConnectedRef.current ||
   isConnectingRef.current || !sessionId) return;`, line 163) already no-ops re-entrant calls — but
   the *connect-timeout timer* itself must not restart or double-schedule if `foreground` toggles
   mid-attempt. Also: if the user switches away (foreground true→false) mid-connect, should the
   in-flight attempt be aborted, or allowed to finish and just use normal timeout thereafter? Given
   the pool keeps the terminal mounted regardless of selection, letting the in-flight attempt
   finish (rather than aborting) is almost certainly correct — aborting would throw away scrollback
   handshake progress for no benefit, since the hook keeps running in the background either way.
5. **Only the first 2 attempts are "fast" (AC2)** — need a per-instance counter distinct from
   `terminalBackoffRef.current.attempt` (which drives delay-between-attempts, not per-attempt
   timeout), since the *delay* and the *timeout* are two different axes that both key off "how many
   attempts have happened since foreground went true." A `foregroundAttemptsRef` (reset alongside
   `terminalBackoffRef.current.reset()` on the false→true transition per AC3) is the natural
   counterpart.
6. **The main-terminal `isVisible` caveat from §1** — if `foreground` is wired straight from the
   existing `isVisible` prop, a user parked on the Diff/Files/Browser tab of the *selected* session
   would still count as "foreground" for reconnect-timeout purposes even though the terminal isn't
   on screen. Likely acceptable (the terminal is one click away, arguably still "the user's current
   session"), but worth an explicit call in the plan rather than an accidental inherited behavior.
7. **Unmount during a fast-timeout window.** Pool eviction (LRU over 8 sessions,
   `SessionDetailView.tsx:254`) can unmount a `TerminalOutput`/`useTerminalStream` instance while a
   connect-timeout timer is pending. The existing `autoConnect` cleanup effect
   (`useTerminalStream.ts:420-428`) already clears `reconnectTimerRef` and calls `disconnect()` on
   `sessionId`/`autoConnect` change/unmount — a new connect-timeout timer ref must be added to that
   same cleanup list or it will fire an abort against a torn-down `abortControllerRef`.

## 5. Unstated needs beyond the explicit ACs

- **A connect-timeout constant needs a name and a single source of truth** — today there's zero
  precedent for "max duration of one attempt" anywhere in `backoff.ts` or the hook; the plan should
  add it there (e.g. `FOREGROUND_CONNECT_TIMEOUT_MS = 1200`, `BACKGROUND_CONNECT_TIMEOUT_MS = 3500`)
  rather than inlining magic numbers in the hook, consistent with how `jitteredDelay`/`BackoffState`
  are already centralized in `backoff.ts`.
- **Test-mode timer control**: the existing V2 reconnect test suite
  (`useTerminalStream.test.ts:382-461`) already uses `jest.useFakeTimers()` + `jest.advanceTimersByTime`
  against the existing backoff scheduling — a connect-timeout timer needs the same fake-timer
  compatibility, and tests must advance past the *shorter* fast timeout distinctly from the *longer*
  normal one to assert the correct one is active (AC6).
- **Logging/observability parity**: every existing reconnect trigger logs a structured
  `console.info('[reconnect] stream=terminal trigger=... attempt=... delay=...')` line
  (`useTerminalStream.ts:340,443`) and `useVisibilityResync` does the same with a `[resync]` prefix.
  A connect-timeout abandonment should log similarly (e.g.
  `[reconnect] stream=terminal trigger=connect-timeout foreground=true attempt=... timeoutMs=...`)
  so it's diagnosable in production the same way the rest of the reconnect machinery already is —
  not called out in the ACs but consistent with every other piece of this subsystem.
- **No regression to `isVisible`'s existing fit+focus effect** (AC7 concern): since `foreground`
  likely derives from/aliases the same `isVisible` prop, the plan should confirm whether
  `TerminalOutput.tsx:948-956`'s fit+focus effect and the new foreground-timeout logic share the
  same prop or are kept as two independently-named props (`isVisible` for DOM/pool visibility,
  `foreground` for reconnect-timing semantics) even if their values are usually identical — keeping
  them named separately avoids conflating "should I fit()+focus() the xterm DOM node" with "should I
  use a fast reconnect timeout," which are different concerns despite sharing an input signal today.
