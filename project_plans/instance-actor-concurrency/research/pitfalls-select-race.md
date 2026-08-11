# Pitfall Research: `select` Does Not Prefer a Canceled `ctx.Done()` Over a Ready Mailbox Send

Scoped to adversarial-review.md finding 1 (2026-07-01 pass): closing the gap where `send`/
`sendSync`/`sendCtx`'s bare `select { case i.mailbox <- cmd: ; case <-i.ctx.Done(): ... }` can,
long after `stopActor()` has fully returned, still pick the mailbox-send branch and hang a
`sendSync` caller forever. Research only — no code or plan.md changes made.

## 1. Is this a real, well-known Go pitfall? Yes.

**Go spec, "Select statements"**: *"If one or more of the communications can proceed, a single
one that can proceed is chosen via a uniform pseudo-random selection."* There is no language in
the spec giving any case priority by source order, by channel type, or by how long a case has
been ready. A `case <-ctx.Done()` that has been ready for an hour and a `case ch <- v` that just
became ready this instant are weighted identically.

**Community confirmation, independent of this review**: *100 Go Mistakes and How to Avoid Them*
(Teiva Harsanyi) has a mistake entry on exactly this — developers wrongly assume a `select`'s
first case (often `ctx.Done()`) is checked with priority; the book states Go chooses randomly
among ready cases and source order implies no priority. golang-nuts threads *"Still 'missing'
priority or ordered select in go?"* (https://groups.google.com/g/golang-nuts/c/I9cbvCB86MA) and
*"Priority select in Go"* (https://groups.google.com/g/golang-nuts/c/M2xjN_yWBiQ) confirm there is
no built-in priority-select and document the nested-select-with-`default` workaround; Øyvind Teig's
*"Priority select in Go"* (https://www.teigfam.net/oyvind/home/technology/047-priority-select-in-go/)
independently arrives at the same fix.

**The idiomatic fix, per all of the above**: a non-blocking priority pre-check —
`select { case <-ctx.Done(): return ...; default: }` — executed *before* the real (blocking)
select that also has the ctx.Done() case. This is exactly what adversarial-review.md's finding 1
prescribes.

### Does the pre-check leave its own residual TOCTOU window? No.

Naive reasoning might worry about the gap between the pre-check's `default` branch and entering
the second, blocking select. It doesn't matter, because **a canceled `context.Context` never
un-cancels** — `ctx.Done()`'s channel closes exactly once and stays closed forever. If `ctx` is
already canceled when the pre-check runs, `default` is never taken (the `<-ctx.Done()` case wins
unconditionally over empty `default`), deterministically catching the "long after teardown" case.
If `ctx` cancels *during* the gap, the second select's own `case <-i.ctx.Done()` is now also
ready — the same narrow race `drainMailboxOnStop` (runs once at actor-exit, non-blockingly
draining `i.mailbox`, failing every command with `fail != nil`) already exists to close. The two
mechanisms compose with no overlapping gap: the pre-check owns the long-since-canceled steady
state deterministically; select+`drainMailboxOnStop` owns the genuinely-concurrent instant.

## 2. Does this codebase already use the priority pre-check idiom? Yes — established house style.

Grepped `case <-.*ctx.Done()` immediately followed by `default:` across `session/` and
`server/services/`. This exact non-blocking-cancellation-check-before-further-work shape appears
repeatedly as a loop-top guard (not, so far, paired with a mailbox-style channel send — but
syntactically identical to what Task 3.1a needs):

```go
// session/mux/multiplexer.go:493-497 — top of the per-client read loop, before
// conn.SetReadDeadline/blocking read.
select {
case <-m.ctx.Done():
    return
default:
}
```

Same shape at: `session/tmux/server_registry.go:307-311` (control-mode restart-retry loop, before
`r.startControlMode()`), `session/hibernation_sweeper.go:270-274` (`warmRSSCache`'s per-instance
loop), `session/cdp/manager.go:409-412`/`512-516` (`pollForChrome`/`readLoop`, before HTTP
poll/`conn.ReadMessage()`), `session/external_tmux_streamer.go:278-281` (scanner-read loop), and
`session/mux/multiplexer.go:453-459` (accept-error handler).

By contrast, the three poller run-loops (`session/pr_status_poller.go:178-184`,
`session/review_queue_poller.go:299-301`, `server/services/capacity_monitor.go:82-87`) use a
**single** blocking `select` with `ctx.Done()` as one of several ticker-driven cases — a
different, unrelated idiom (a normal event loop, not a priority-race-against-a-send); not the
pattern to imitate for `send`/`sendSync`/`sendCtx`.

**Conclusion**: the non-blocking `select {…; default:}` pre-check is already this codebase's
established idiom for "don't do more work once canceled," used across `session/mux`,
`session/tmux`, `session/cdp`, and `session` root. Task 3.1a should reuse this exact shape.

## 3. Is the double-select the complete fix, or does the mailbox also need to be closed on stop?

Per `plan.md` (~1244, `runActor()`) and `drainMailboxOnStop` (~1255-1264, quoted in
adversarial-review.md), the actor's exit path is:

```go
case <-i.ctx.Done():
    i.drainMailboxOnStop()   // non-blocking, single pass, drains what's currently buffered
    close(i.done)
    return
```

`drainMailboxOnStop` reads until `i.mailbox` is empty (via `select { case cmd := <-i.mailbox: ...;
default: return }`) and returns — it does **not** call `close(i.mailbox)`. Nothing else in Task
3.1a/3.1b/3.1c's specified body closes the mailbox channel either. So post-`stopActor()`,
`i.mailbox` is **left open, empty, with its full buffer capacity (32, per ADR-027) available**.

**Confirming the consequence**: sending into an open, unread, buffered channel with spare capacity
always succeeds immediately and the value simply sits there — Go gives the sender no signal that
nobody will ever read it. That's exactly why the bare select is racy in steady state: the
mailbox-send case is genuinely, indefinitely ready after stop, not just briefly. This confirms the
non-blocking `ctx.Done()` pre-check does all the real work here — the mailbox's open/closed state
is irrelevant, because the pre-check never lets execution reach the mailbox-send select once `ctx`
is canceled.

**Should `stopActor()` instead/also close `i.mailbox`, so a stray send panics?** No — confirmed,
not assumed. A send on a closed channel *panics*, unrecoverable without the sender's own
`recover()`; a panic in an RPC handler goroutine (e.g. a stale `sendSync` call) is strictly worse
than `ErrInstanceStopped` — it either crashes the process or forces every call site to wrap every
send in `recover()`, exactly the discipline this migration exists to stop requiring. Closing the
channel would also race the *close* itself against any in-flight sender's blocking select the same
way — trading a silent hang for a panic. The non-blocking pre-check fully closes the hang without
touching the channel's open/closed state, so closing `i.mailbox` is pure downside.

**Verdict**: leave `i.mailbox` open (as currently specified); the pre-check select is the complete
additional fix. Do not close the mailbox in `stopActor()`/`drainMailboxOnStop()`.

## 4. Recommended code shape for Task 3.1a

Matches the exact signatures already fixed by Story 3.1's acceptance criteria (`plan.md`
~1221-1243) — no new types or return values introduced, only a pre-check select inserted before
each existing blocking select:

```go
// send — fire-and-forget. Unchanged surface; adds the priority pre-check.
func (i *Instance) send(name string, fn func(s *instanceState)) {
	select {
	case <-i.ctx.Done():
		log.Debug("send: actor stopped, dropping command", "name", name)
		return
	default:
	}
	select {
	case i.mailbox <- command{name: name, fn: fn, fail: nil}:
	case <-i.ctx.Done():
		log.Debug("send: actor stopped mid-send, dropping command", "name", name)
	}
}

// sendSync — package-level generic (Go has no generic methods). Adds the same pre-check.
func sendSync[T any](i *Instance, name string, fn func(s *instanceState) T) (T, error) {
	var zero T
	select {
	case <-i.ctx.Done():
		return zero, ErrInstanceStopped
	default:
	}

	reply := make(chan syncResult[T], 1)
	cmd := command{
		name: name,
		fn:   func(s *instanceState) { reply <- syncResult[T]{val: fn(s)} },
		fail: func(err error) { reply <- syncResult[T]{err: err} },
	}
	select {
	case i.mailbox <- cmd:
		res := <-reply
		return res.val, res.err
	case <-i.ctx.Done():
		return zero, ErrInstanceStopped
	}
}

// sendSyncErr — sugar, unchanged shape (delegates to sendSync[error]).
func sendSyncErr(i *Instance, name string, fn func(s *instanceState) error) error {
	result, err := sendSync(i, name, fn)
	if err != nil {
		return err
	}
	return result
}
```

`sendCtx` is the same shape as `sendSync` above with one addition: the pre-check and both blocking
selects gain a third case, `case <-ctx.Done(): return ctx.Err()`, racing the caller-supplied `ctx`
alongside the actor's own `i.ctx` — otherwise identical (pre-check first, then mailbox-send select,
then reply-wait select). Its receiver shape isn't fully pinned in `plan.md` (~1240 shows only
`sendCtx(ctx context.Context, name string, fn func(s *instanceState)) error`, no explicit `i` param,
unlike `send`'s method form and `sendSync`/`sendSyncErr`'s explicit `i *Instance` first param) —
since it isn't generic it can be a method like `send`; confirm against Task 3.1a's final text.

The pre-check matches the shape already at `multiplexer.go:493-497`, `server_registry.go:307-311`,
`hibernation_sweeper.go:270-274`, `cdp/manager.go:409-412`/`512-516`. Story 2.5.9c/Task 3.1b's
regression test should call `send`/`sendSync` **after** `stopActor()` has returned (not
concurrently), in a loop (e.g. 100 iterations), asserting `ErrInstanceStopped` every time — a
single-call assertion would pass most runs even against the un-fixed code, since the bug is
probabilistic.

## Sources
- [Go spec — Select statements](https://go.dev/ref/spec#Select_statements)
- [100 Go Mistakes and How to Avoid Them — summary](https://www.sglavoie.com/posts/2024/08/24/book-summary-100-go-mistakes-and-how-to-avoid-them/)
- [golang-nuts: "Still 'missing' priority or ordered select in go?"](https://groups.google.com/g/golang-nuts/c/I9cbvCB86MA)
- [golang-nuts: "Priority select in Go"](https://groups.google.com/g/golang-nuts/c/M2xjN_yWBiQ)
- [Øyvind Teig — "Priority select in Go"](https://www.teigfam.net/oyvind/home/technology/047-priority-select-in-go/)
