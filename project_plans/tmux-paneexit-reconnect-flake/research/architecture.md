# Architecture: Fast-Recheck Integration Point

Research for the `fastRecheckAttempts × (fastRecheckSyncTimeout + fastRecheckInterval) = 700ms`
detection-latency ceiling required by requirements.md goal 2 / AC 5. Based on a full read of
`session/tmux/server_registry.go` (542 lines) and `session/tmux/server_registry_integration_test.go`.

## Recommended concrete values

```go
fastRecheckAttempts    = 2
fastRecheckSyncTimeout = 150 * time.Millisecond
fastRecheckInterval    = 200 * time.Millisecond
// 2 × (150ms + 200ms) = 700ms
```

Chosen because they match the example numbers named directly in AC 5, and because
150ms is comfortably long enough for a local `tmux list-sessions` subprocess (a
few ms in practice) while still being "fast" relative to the old unbounded
backoff wait, and 200ms between attempts avoids hammering the tmux server if
the socket itself is the thing that's unhealthy.

## 1. Integration point: inline in `reconnectLoop`, not a separate goroutine

**Recommendation: replace the two `time.After(backoff)` waits inside `reconnectLoop`
(server_registry.go:326-330 and :387-391) with a call to a new unexported helper**,
e.g. `r.waitBackoffWithFastRecheck(backoff)`, called synchronously from the same
goroutine. Do **not** start a second free-running goroutine in `Start()`.

Reasoning:

- **The gap is specifically the backoff-wait window.** Requirements.md's root
  cause is precise: "no `syncSessions()` call happens at all" while
  `reconnectLoop` sleeps on `time.After(backoff)`. Fast-recheck only needs to
  exist *during that sleep* — there's no detection gap while control-mode is
  connected (live events + the 50ms debounced `%sessions-changed` sync cover
  that), and no gap during an active `startControlMode()` attempt (that's a
  bounded subprocess call, not a sleep). Scoping fast-recheck to the two
  backoff-wait sites means it only runs exactly when it's needed, not
  continuously for the registry's whole lifetime.
- **No new goroutine lifecycle to manage.** `reconnectLoop` already owns a
  single well-defined start (`go r.reconnectLoop()` in `Start()`) and stop
  (`r.ctx`/`r.cancel`, checked via `select { case <-r.ctx.Done(): return }` at
  every wait point). A helper called synchronously from inside that loop
  inherits this for free — no new field on `TmuxServerRegistry`, no second
  `context.WithCancel`, no new place `Stop()` has to remember to tear down.
  Introducing a second goroutine started in `Start()` would need its own
  ticker, its own `r.ctx.Done()` select, and — because `Start()` replaces
  `r.ctx`/`r.cancel` on every call (server_registry.go:84-87) — care that the
  goroutine captures the *current* ctx, not a stale one from a previous
  `Start()` call on a reused registry. That's a whole class of bugs the
  inline approach sidesteps entirely.
- **Avoids a second uncoordinated `syncSessions()` caller.** A free-running
  goroutine calling `syncSessions()` on its own ticker would run concurrently
  with `reconnectLoop`'s own post-connect `syncSessions()` call
  (server_registry.go:342) and with the debounce timer's async call
  (server_registry.go:455-459, already an existing minor overlap). Two
  independent `syncSessions()` diffs racing each other around
  `r.sessions`/`r.mu` is not memory-unsafe (the map swap is lock-protected),
  but it is a second, unnecessary source of interleaving to reason about, and
  it would keep firing during the *connected* state too — pure waste, since
  the whole point of control-mode is to make `syncSessions()` polling
  unnecessary. Calling the helper in place of the plain sleep guarantees
  fast-recheck's `syncSessions()` calls and `reconnectLoop`'s own
  post-connect `syncSessions()` call are strictly sequential: the helper
  returns (backoff/ctx exhausted) *before* `reconnectLoop` moves on to the
  next `startControlMode()` attempt, so there is never a moment where both
  are in flight.

## 2. `syncSessions()` timeout: parameterize it, don't fork the code path

**Recommendation:** change `syncSessions()`'s signature to take a `timeout
time.Duration` parameter, replacing the hardcoded `10*time.Second` at
server_registry.go:211 with a named constant (e.g. `defaultSyncTimeout = 10 *
time.Second`) passed explicitly at the three existing call sites (`Start`'s
bootstrap call at :90, `reconnectLoop`'s post-connect call at :342, and the
debounce timer's call at :456), and `fastRecheckSyncTimeout` (150ms) at the
new fast-recheck call site.

Do **not** give fast-recheck its own separate `list-sessions` invocation.
`syncSessions()` is not just "run list-sessions" — it's `run list-sessions +
diff against r.sessions under r.mu + firePaneExit for every disappeared name,
outside the lock` (server_registry.go:209-246). That diff-and-fire logic is
exactly the mechanism goal 3 says must not be duplicated. A second, shorter
`list-sessions` call site would either (a) re-implement the diff and
subsMu-respecting close discipline a second time — a direct violation of "no
new state" and a second place the subsMu-close-outside-lock rule
(server_registry.go:48-50) has to be gotten right — or (b) call the parsed
result into the same diff function anyway, at which point it's just
`syncSessions()` with a shorter timeout, i.e., the parameterized version.
Parameterizing is strictly less code and reuses an already-correct path.

`syncSessions()` is unexported and only called from within `server_registry.go`,
so widening its signature doesn't touch the exported API surface the
constraints list (`SubscribePaneExit`, `SessionExists`, `ListSessions`,
`IsHealthy`, `Start`, `Stop`, `GetServerRegistry`, `StopServerRegistry`,
`RemoveServerRegistry`).

## 3. Data flow: no new state

Confirmed by reading server_registry.go:209-246: `syncSessions()` already (a)
takes a fresh `list-sessions` snapshot, (b) diffs it against `r.sessions`
under `r.mu.Lock()`, (c) replaces `r.sessions` atomically, and (d) calls
`r.firePaneExit(name)` for every name that disappeared, *after* releasing
`r.mu` — and `firePaneExit` itself already follows the copy-under-lock,
close-outside-lock pattern for `subsMu` (server_registry.go:196-207). Nothing
about "detect a session disappeared and notify subscribers" needs new
per-session or per-subscriber bookkeeping for fast-recheck to work — the
fast-recheck mechanism's only job is to invoke this existing, already-correct
function more often and independently of backoff. The only new "state" is
local to the wait helper itself (a loop counter and two `time.Timer`s), never
promoted to a struct field, so it needs no additional locking.

Sketch (illustrative, not final code):

```go
func (r *TmuxServerRegistry) waitBackoffWithFastRecheck(backoff time.Duration) {
	const (
		fastRecheckAttempts    = 2
		fastRecheckSyncTimeout = 150 * time.Millisecond
		fastRecheckInterval    = 200 * time.Millisecond
	)
	// ponytail: fastRecheckAttempts × (fastRecheckSyncTimeout + fastRecheckInterval)
	// = 700ms is a hard ceiling on pane-exit detection latency while reconnectLoop
	// is sleeping out backoff (which alone climbs 100ms→30s and is tuned for
	// connection-retry pressure, not detection latency). Decoupled on purpose:
	// backoff must stay slow to avoid fork-rate explosion against an unhealthy
	// tmux server, but a caller blocked on SubscribePaneExit must not inherit
	// that slowness. Do not fold this into backoff's own timing.
	deadline := time.NewTimer(backoff)
	defer deadline.Stop()

	for i := 0; i < fastRecheckAttempts; i++ {
		select {
		case <-r.ctx.Done():
			return
		case <-deadline.C:
			return
		default:
		}
		_ = r.syncSessions(fastRecheckSyncTimeout)
		select {
		case <-r.ctx.Done():
			return
		case <-deadline.C:
			return
		case <-time.After(fastRecheckInterval):
		}
	}
	select {
	case <-r.ctx.Done():
	case <-deadline.C:
	}
}
```

Note the sync-then-interval ordering (attempt fires immediately on entry,
*then* waits `fastRecheckInterval` before the next one) rather than
interval-then-sync. This front-loads detection: a pane exit that happens right
as backoff-wait begins is caught by the very first attempt's `syncSessions()`
call, not after paying a full `fastRecheckInterval` first. The worst case —
exit happens just *after* the first attempt's snapshot — is still caught by
attempt 2, within the documented 700ms ceiling.

Both existing backoff-wait sites in `reconnectLoop` (the `startControlMode()`
failure branch, server_registry.go:326-330, and the post-`readLines` branch,
server_registry.go:387-391) should call this same helper — both are
"control-mode is down, syncSessions has gone quiet" windows per the root
cause, so both need the fast path, not just one.

## 4. `ponytail:` comment placement

Existing convention in this codebase (checked via `grep -rn "ponytail:"
--include="*.go"`, e.g. `session/git/worktree.go:69`,
`session/instance_status.go:25`, `session/tmux/tmux.go:1152`) is a single-line
`// ponytail: <succinct why>` immediately above the code it justifies — not a
multi-paragraph block.

Place it directly above the `waitBackoffWithFastRecheck` helper's constant
block (or immediately above the `const (...)` if the constants are hoisted to
package scope instead of function-local — either is fine as long as it sits
next to the three numbers it's naming). Content, following the existing
style's terseness: name the formula and the concrete ceiling
(`fastRecheckAttempts × (fastRecheckSyncTimeout + fastRecheckInterval) =
700ms`), and state *why* it's decoupled from backoff — backoff is tuned for
protecting a possibly-unhealthy tmux server from a reconnect fork-rate
explosion (per the existing comment at server_registry.go:362-366 on
`minStableConnection`), while detection latency is a caller-facing guarantee
that must not inherit backoff's cap of 30s. See the sketch in section 3 above
for suggested wording.

## Constraint check: black-box test package limits how the regression test elevates backoff

Read `session/tmux/server_registry_integration_test.go` in full (`package
tmux_test`, build-tagged `integration`) — it is an **external, black-box test
package**. This matters for how
`TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff` (goal 5) can
"artificially elevate backoff": it has no access to `waitBackoffWithFastRecheck`,
the `backoff` local variable, or any other unexported symbol in
`server_registry.go`. It can only drive behavior through the same 9 exported
functions the constraints already list as frozen (`NewTmuxServerRegistry`,
`Start`, `Stop`, `SubscribePaneExit`, `SessionExists`, `ListSessions`,
`IsHealthy`, plus the package-level `GetServerRegistry`/`StopServerRegistry`/
`RemoveServerRegistry`).

The viable black-box lever: `reconnectLoop` only resets backoff after a
connection survives `minStableConnection` (5s, server_registry.go:366-369);
otherwise every disconnect doubles it. Repeatedly killing the *keepalive*
session the control-mode client is attached to (`tmux -L <socket> kill-session
-t <keepalive-name>`, using the exported `tmux.TmuxPrefix` constant the
existing tests already use at line 84) forces control-mode to exit almost
immediately each time, well under 5s, so 3-4 such cycles reliably pushes
backoff past 700ms (100→200→400→800ms) using only public API plus ordinary
`exec.Command` calls the existing test helpers already make (`newSessionWithRetry`,
the raw `kill-session` calls in `TestTmuxServerRegistry_PaneExitChannel`).
The regression test then creates the real target session, waits for
`SessionExists`, subscribes, kills it, and asserts `exitCh` closes within a
generous-but-bounded window (e.g. ~1-1.5s to absorb scheduling jitter above
the 700ms ceiling) — independent of how large backoff has grown. This is a
test-design detail for `/sdd:3-plan` and `/sdd:4-validate`, not an
architecture decision, but it's worth flagging now because it confirms the
inline (non-goroutine) design from section 1 doesn't block the regression
test: the test only ever needs to observe end-to-end behavior through the
existing public surface, never the internal helper.
