# Research: Features — goroutine-lifecycle / reconnect / actor-shutdown patterns

Scope per requirements.md: audit `session/` for the "ad hoc boolean-OR classification
of a goroutine-lifecycle decision" smell beyond the 3 documented `session/tymux/stream.go`
incidents, surface edge cases the design must handle, and surface unstated needs.

## 1. The proven case: `session/tymux/stream.go` — exact current shape

`readAttachLoop` (`session/tymux/stream.go:191-232`), on a `stream.Receive()` error:

```go
s.mu.RLock()
exited := s.exited
s.mu.RUnlock()
if s.closing.Load() || exited || ctx.Err() != nil {
    close(done)
    return
}
newStream, first, ok := s.ReconnectLoop(paneID, "error")
```

Three independently-set, independently-owned signals must all be checked correctly at
this one call site for it to classify "stop" vs. "reconnect":

- `s.closing` (`session.go:122`, `atomic.Bool`) — set by `Close()`/`DetachSafely()`
  (`beginClosing()`, `session.go:284-291`) before canceling the stream context. Means
  "the whole session is ending."
- `s.exited` (`session.go:110`, bool under `s.mu`) — set by `deliverExit`
  (`stream.go:289-305`) when an `AttachEvent_Exited` was already observed. Means "the
  pane already told us it's dead cleanly" — orthogonal to `closing`.
- `ctx.Err() != nil` — true when *this specific stream generation's* context was
  canceled, which happens both from `beginClosing()`'s cancel *and* from
  `openStandingStream`'s own tear-down-before-reopen call (`session.go:88`,
  `s.teardownStandingStream()` at the top of every `openStandingStream`), which does
  **not** set `closing` (that flag means "whole session," not "this generation"). This
  third clause is fix #2 from requirements.md, and its own doc comment
  (`stream.go:198-211`) is effectively an inline post-mortem of why the first two
  clauses alone were insufficient.

Struct fields backing this (`session/tymux/session.go`): `exited`/`exitReason`/`exitFired`
(:110-112), `closing atomic.Bool` (:122), `abortReconnect chan struct{}` (:134),
`reconnecting`/`reconnectAttempt`/`reconnectCause`/`reconnectSince` (:152-155),
`backendRestarted`/`backendRestartedAt` (:166-167), `teardownWait` (:175). Eight fields,
independently mutated from at least three call paths (`readAttachLoop`, `ReconnectLoop`,
`Close`/`beginClosing`, `cacheFromSession`'s per-generation reset at :267-276), collectively
represent "why is this generation ending" with no single type gathering them.

`teardownStandingStream` (`session.go:148-165`) is the other half of incident #2/#3: it
`<-done` waits (now bounded by `maxTeardownWait` = 5s, `session.go:140`) for the reader
goroutine this same classification decision controls. Because this whole call chain runs
inside `session/actor.go`'s serialized actor, a misclassification here doesn't just leak
one goroutine — it wedges the owning `Instance`'s entire command queue (see §3).

## 2. `session/tmux/control_mode.go` — compared structurally

**Does it have an analogous ad hoc boolean-OR at its reader loop's error branch? No** —
but it has the same *class* of smell in a different shape: scattered single-flag checks
repeated at multiple call sites instead of one multi-clause condition at one site.

`readControlModeOutput`'s scanner loop (`control_mode.go:386-422`) checks only one signal
per iteration — `select { case <-doneCh: return; default: }` (:397-400, repeated :416-421
for the scanner-error path) — because this backend **never reconnects** the reader
goroutine transparently. Any exit is terminal; a caller must call `StartControlMode()`
again. So the "stop vs. reconnect" decision that produced tymux's 3 incidents structurally
doesn't exist here — there is nothing to reconnect *to* mid-loop.

What *does* exist, and is the same class of problem: "was this exit intentional
(suppress the `onExit` callback) or unilateral (fire it)" is decided independently at
three call sites, each re-checking `t.intentionalStop.Load()` (`atomic.Bool`,
set at `StopControlMode`, :281):

- `readControlModeOutput`'s scanner-EOF fallback (:491-497)
- `processControlModeLine`'s `%exit` case (:656-662)
- `processControlModeLine`'s `%session-closed` case (:668-674)

Each of these is a single-flag check, not an OR chain — arguably *safer* per-call-site
than tymux's 3-clause condition — but it's the same "onExit should fire exactly once, and
whether it fires here needs a fact that isn't visible at this call site without
independently re-deriving it" shape, spread across three copies instead of one, guarded
separately by `t.onExitOnce.Do(...)` (fire-once) plus a second boolean gate
(`controlModeExited`, mutex-guarded, :440/:635) that dedupes the *drain* logic
independently of the *callback* logic. Four independently-consulted state bits
(`intentionalStop`, `controlModeExited`, `onExitOnce`, and the `doneCh`-identity check
below) for one logical fact ("has this control-mode generation ended, and was it
deliberate").

**A stronger, more principled generation-fencing pattern already exists in the same
file**, worth citing as *prior art the new mechanism should look like*, not a smell:
`processControlModeLine`'s `%exit` handler (:452-481) resolves a genuine
generation-supersession race — a fresh `StartControlMode()` may have already reassigned
`t.controlModeDone` to a new generation's channel by the time an old generation's
unilateral exit gets here — via **channel-identity comparison** (`t.controlModeDone ==
doneCh`) rather than a boolean flag:
```go
if t.controlModeDone == doneCh {
    close(doneCh)
    t.controlModeDone = nil
} else if t.controlModeDone != nil {
    close(doneCh)
}
```
This is structurally closer to what a shared "which generation am I, and has a newer one
superseded me" primitive should look like than tymux's boolean-OR — see §5.

`runCMSender` (:706-780) has its own doneCh + drain-on-exit loop (`for { select {...} }`,
:740-756) but only ever reacts to a closed `doneCh`; no multi-flag classification there.

**Conclusion for control_mode.go**: confirmed as an audit candidate, but a *distinct*
sub-shape of the smell (scattered single-flag re-derivation across N call sites, not one
N-clause OR at one site) plus one place (the `%exit` generation-identity check) that
already solves the generation problem well without a shared abstraction. Any shared
mechanism needs to accommodate "no reconnect, exit is terminal, only the callback-fire
classification is shared" as a distinct usage shape from tymux's "reconnect transparently
unless deliberate" shape.

## 3. `session/actor.go` — does it have the same gap?

**No ad hoc multi-flag stop-vs-continue condition.** `runActor` (`actor.go:121-135`):
```go
for {
    select {
    case <-li.ctx.Done():
        return
    case cmd := <-li.mailbox:
        cmd(li.Instance)
        ...
    }
}
```
One signal (`li.ctx.Done()`), no boolean flags to misclassify. `sendSyncErr`/`send`/
`sendCtx` (:34-101) each independently re-implement the same two-`select`
(enqueue-or-cancelled, then reply-or-cancelled) shape three times — a *duplication* smell,
not a *classification* smell; each occurrence is internally correct and simple.

**The real gap actor.go has is structural, not classificational, and it is exactly the
mechanism of incident #2/#3**: `runActor` is a single goroutine with no timeout or
preemption around `cmd(li.Instance)` (:128). If a command blocks (as
`teardownStandingStream`'s `<-done` wait did, called from inside a `tymuxGRPCSession`
command running on the actor), the entire actor — every future command on that
`Instance`, including its own shutdown — wedges, because nothing about `runActor`'s
contract requires or enforces that a command return promptly. This is a **shared-risk
surface**, not a `session/tymux`-specific bug: any backend's command closure that blocks
unboundedly on I/O reproduces the identical hang shape. The requirements doc's Feasibility
Risks section anticipates this ("the audit may find... `session/actor.go`'s concern might
be about actor shutdown ordering, not stop-vs-reconnect classification") — confirmed:
it's neither shutdown-ordering nor stop-vs-reconnect classification, it's **absence of a
command-execution SLA/preemption contract**. Worth flagging as an adjacent finding for
Phase 3, even though it isn't the same "which state am I in" shape the lifecycle
mechanism targets — the fix belongs at the actor/command-contract layer (e.g. require
commands to respect a bounded-wait convention, or make `runActor` detect and log a
long-running command), not inside the lifecycle-decision type itself.

## 4. Additional candidates found beyond the three named in requirements.md

Searched every `for { select {` / reader-then-classify-error loop under `session/`
(grep for `.Receive()`/`.Read(`/`.Scan()`/`ReadLoop` inside `for {` bodies, ~20 files
matched; each goroutine spawn site in `session/` — ~70 files — was scanned for
`closing`/`closed`/`exited`/`reconnect` density to prioritize which to open). Most
(`orphan_tmux_sweeper.go:78-86`, `hibernation_sweeper.go:219-227`, `response_stream.go:
213-233`, `pi_status_source.go:341-357`, `session/tmux/server_registry.go:423-508`) are
clean: a single `ctx.Done()` (or one other unambiguous signal) governs stop, with no
multi-flag classification. `server_registry.go`'s control-mode reconnect loop is a good
negative example: it reconnects *unconditionally* whenever the reader returns (no "was
this deliberate" distinction needed beyond `ctx.Done()`), which is why it never needed
the ad hoc-flag pattern at all — worth citing in the plan as evidence the smell isn't
universal, and as a model for "when you don't need the distinction, don't build one."

One additional genuine candidate:

**`session/external_streamer.go`'s `readLoop` (:363-459)** — the mux-socket reader for
external terminal monitoring (ssq-mux). Its `mux.DecodeMessage` error-handling
(:405-451) is a **chain of ad hoc, partially string-matching error classifications**
distinguishing "expected timeout, keep looping" from "real disconnect, reconnect":
```go
var netErr net.Error
if errors.As(err, &netErr) && netErr.Timeout() { continue }
if errors.Is(err, os.ErrDeadlineExceeded) { continue }
if errors.Is(err, io.ErrUnexpectedEOF) { continue }
errStr := err.Error()
if strings.Contains(errStr, "i/o timeout") || strings.Contains(errStr, "deadline exceeded") { continue }
```
This is the same underlying problem shape as `session/tymux`'s (a reader goroutine
classifying an I/O error into "ignore and keep reading" vs. "tear down and reconnect")
but with a *worse* implementation smell than a boolean OR: the "fallback for wrapped
errors" comment (:428) admits the typed checks above it are known-incomplete, so it falls
through to matching on `err.Error()` substrings — a classification that silently breaks
if a wrapped library ever changes its error text. Recommend adding this file to the
audit's confirmed-candidate list; it's a strong argument for whatever shared mechanism
gets built to include (or compose with) a canonical, typed "is this a benign
poll-timeout" classifier, since this file, `session/tymux`, and (via `netErr.Timeout()`)
implicitly `session/cdp/manager.go`'s Chrome-poll loop (:346-360, :408-435, clean —
uses only `ctx.Done()` plus a deadline check, no multi-flag OR) all reimplement pieces of
"is this net.Error actually a timeout" independently.

`session/sshremote/approval_relay.go`'s two reader loops (:464-475, :502-...) and
`session/cdp/manager.go`'s three loops were read and are clean single-signal (`ctx.Done()`
only, or one `websocket.IsCloseError` check) — not additional candidates.

## 5. Prior art already in-repo for "superseded generation"

Directly answers the requirements doc's Feasibility Risk about `exited` vs. `closing` vs.
`ctx.Err()` not being fully redundant, and the Alternatives section's pointer to
`session/instance_state.go`'s `TransitionDef` table as prior art: there is a **second**,
independent piece of relevant prior art in `session/tmux/tmux.go` that the requirements
doc doesn't mention — a working generation-fencing mechanism for a *different* lifecycle
(the PTY triple, not the reader goroutine):

- `ptyGen uint64` (`tmux.go:84-88`) — bumped on every PTY install/clear, guarded by
  `ptmxMu`.
- `ptyClosed bool` (`tmux.go:89-93`) — terminal, never reset once true.
- `tryInstallPTYTriple(expectedGen uint64, ...)` (`tmux.go:2212-2228`) — compare-and-swap
  against `expectedGen`: refuses to install if `ptyClosed` or if `ptyGen` has moved on
  since the caller snapshotted it (`ptySnapshot`, :2201-2210), so a late-arriving
  `AttachToExisting`/`RestoreWithWorkDir` install racing a concurrent `Close()` (or a
  second concurrent install) is detected and torn down rather than silently overwriting a
  newer/closed state.

This is structurally the "generation ID + terminal flag + compare-and-swap" shape that a
shared lifecycle mechanism should generalize, and it's a **cleaner** answer to "is this my
generation or a superseded one" than `session/tymux`'s `ctx.Err() != nil` inference
(which only tells you *a* cancellation happened, not *which* logical generation-transition
caused it — exactly the ambiguity that produced incident #2). Recommend the plan phase
treat `tryInstallPTYTriple`'s generation-counter idiom, not just `instance_state.go`'s
`TransitionDef` table, as a second concrete prior-art input to the build-vs-buy decision.

## Edge cases and unstated needs a shared mechanism must handle

Beyond the explicit requirements (exhaustiveness, observability, no behavior change):

1. **Two independent "why did this end" taxonomies must compose, not merge.**
   `session/tymux` needs *both* "deliberate vs. transient" (closing/exited/ctx.Err) *and*
   "which generation" (does this event belong to the stream generation currently live, or
   an older one being torn down). `session/tmux`'s PTY mechanism only needs the second.
   `control_mode.go` needs the second (via doneCh identity) plus a fire-once/callback
   concern layered on top. A single flat enum won't fit all three; the mechanism likely
   needs a generation token *and* a terminal-reason type as separable concerns (matching
   the Rabbit Holes warning against over-generalizing, and the Feasibility Risk that
   different subsystems' "ad hoc flags" may not be the same shape).
2. **A generation boundary that is *not* session-ending must be representable
   without borrowing the whole-session-closing signal.** This is exactly incident #2:
   `openStandingStream`'s internal teardown-before-reopen is a real generation transition
   that must NOT be confused with `Close()`'s `closing` flag, but today the only way to
   detect it is inferring `ctx.Err() != nil` on the specific canceled context — fragile
   because `ctx.Err()` doesn't record *why* it was canceled, only *that* it was. A typed
   "stop reason" carried alongside (or derivable from) the generation transition, not
   inferred after the fact from `ctx.Err()`, would remove that inference step entirely.
3. **Bounded-wait/abandon semantics need a first-class place in the model**, not a bolt-on
   `time.After` race (`teardownStandingStream`'s `maxTeardownWait`, `session.go:148-165`).
   The abandoned-goroutine risk it accepts (a stale generation's queued event still landing
   after abandonment — see `session.go:130-139`'s doc comment) is a correctness property of
   the *mechanism*, not just this one call site; any subsystem that adopts the shared
   building block and also blocks on a goroutine's exit needs the same bounded-wait +
   abandon contract available generically, with the same documented narrow race window
   made explicit rather than each subsystem re-deriving its own timeout constant.
4. **Fire-once callback delivery interacts with the lifecycle decision** (tymux's
   `exitFired`/`deliverExit`, `control_mode.go`'s `onExitOnce`) — both subsystems need "has
   the terminal-exit callback already fired" as a fact that survives whatever lifecycle
   state transition triggers it, from either direction (event arrives before or after
   callback registration). This is adjacent to, not identical with, the stop-vs-continue
   decision itself; the shared mechanism should not force these two concerns into one type
   just because they currently live in neighboring boolean fields.
5. **`session/tmux`'s "always reconnect unless ctx.Done()" shape (`server_registry.go`)
   must remain expressible as a degenerate case** — not every migrated subsystem needs the
   full "deliberate vs. transient vs. superseded" taxonomy; forcing one onto a loop that
   never needed the distinction (Rabbit Holes' "building a fully general FSM" warning)
   would be a regression in simplicity for no benefit.
6. **Actor-command blocking is an orthogonal but coupled risk** (§3): even a perfect
   lifecycle-classification type inside `tymuxGRPCSession` doesn't fully close incident
   #2/#3's blast radius by itself, because the wedging *mechanism* is "a command blocks
   the actor," and the lifecycle type only affects *how quickly* that command's internal
   wait resolves. Phase 3 should decide explicitly whether hardening `runActor`'s
   command-execution contract is in this project's scope or a named follow-on — per
   requirements.md's Users/Consumers section, the actor's "own state/contract is in scope
   for audit," but a full command-timeout enforcement mechanism could itself be a
   scope-creep rabbit hole if not bounded.
7. **String-matching error classification (`external_streamer.go`) is a distinct failure
   mode from boolean-OR proliferation** — a shared mechanism that only replaces boolean
   flags with a typed enum doesn't fix a classifier that's *already* falling through to
   `strings.Contains(err.Error(), ...)`. If `external_streamer.go` is migrated, the
   migration should also introduce (or reuse) a canonical typed timeout/transient-error
   classifier, not just wrap the existing string-matching logic in a nicer-looking type.

## Summary table

| File | Loop | Same shape as `tymux/stream.go`? | Verdict |
|---|---|---|---|
| `session/tymux/stream.go:191-232` | `readAttachLoop` | Yes — the proven case | Migrate (primary target) |
| `session/tmux/control_mode.go:386-498`, `:615-674` | `readControlModeOutput` / `%exit` handling | Same class, different shape (scattered single-flag re-derivation across 3+ call sites, no reconnect) | Migrate, but expect a different usage of the mechanism (no generation-reconnect, fire-once callback classification only) |
| `session/actor.go:121-135` (`runActor`) | actor loop | No — single-signal, correct | Not a migration target for the classification type; flag the separate command-blocking/preemption gap for Phase 3 scoping |
| `session/external_streamer.go:363-459` | `readLoop` | Yes, plus string-matching fallback | New candidate — add to audit's confirmed list |
| `session/tmux/server_registry.go:423-508` | control-mode reconnect loop | No — unconditional reconnect, no distinction needed | Reference example of correctly *not* needing the pattern |
| `session/tmux/tmux.go:2212-2228` (`tryInstallPTYTriple`) | N/A (not a reader loop — PTY install race) | Same underlying "generation fencing" need, already solved well | Prior art for the shared mechanism's generation-token design |
| `session/cdp/manager.go`, `session/sshremote/approval_relay.go`, `session/response_stream.go`, `session/pi_status_source.go`, `session/orphan_tmux_sweeper.go`, `session/hibernation_sweeper.go` | various | No — single-signal (`ctx.Done()` or one typed check) | Not candidates |
