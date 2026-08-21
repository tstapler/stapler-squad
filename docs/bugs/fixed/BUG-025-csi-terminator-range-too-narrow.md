# BUG-025: CSI Escape-Sequence Terminator Range Too Narrow (Letters Only) [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-07-03
**Fixed**: 2026-07-03
**Impact**: Escape-code stripping used by rate-limit detection, Claude-activity
detection, tmux banner filtering, and the connect-width diagnostic could leave
raw escape bytes in "stripped" text when a CSI sequence terminates on a
non-letter final byte (e.g. `@`, `~`).

## Problem Description

The user reported terminal rendering artifacts since Claude Code switched to
a new (Ink-based) renderer and asked for a full audit confirming the rendering
pipeline handles all escape codes the renderer sends without regressing the
session-state detection features built on top of it.

## Investigation

The live rendering path (`server/services/connectrpc_websocket.go` → raw PTY
passthrough → `TerminalStreamManager`/`EscapeSequenceParser` → xterm.js) was
already repaired for the new renderer in PR #139 (commit `dc71b828a`,
2026-06-29), which fixed four real bugs: missing `{stream:true}` on
`TextDecoder`, an OSC/DCS lookback buffer too short (20→256 bytes), an
unnecessary ED2+ED3 scrollback-clear workaround (removed once xterm.js v6
started handling it natively), and a `RedrawThrottler` over-classifying
Ink's per-line cursor-up redraws as full-screen redraws. All four fixes are
still present and covered by 77 passing tests — no regression there.

Separately, this session had already removed a large amount of genuinely
dead MOSH-style state-sync infrastructure (`server/terminal`, `server/ssp`,
`session/framebuffer`, `session/terminal_state.go`, `StateApplicator.ts`,
`DeltaApplicator.ts`) that was never wired into the live rendering path —
unrelated to this bug, confirmed via exhaustive caller tracing before
deletion.

Auditing `EscapeSequenceParser.ts`'s CSI terminator check
(`hasCSITerminator`) found it explicitly checks for the letter range
(`0x41-0x5A`/`0x61-0x7A`), which is too narrow per ECMA-48 (final bytes are
`0x40-0x7E`) — but it happens to be functionally correct anyway, because its
"unexpected character" fallback branch also terminates the sequence on any
byte outside the recognized parameter/intermediate ranges, which for a
well-formed CSI sequence is exactly the same 0x40-0x7E set. No live bug
there; left as-is with the accidental-correctness confirmed by trace.

Grepping the whole repo for the same narrow-terminator pattern
(`\x1b\[[0-9;]*[a-zA-Z]`) found it copy-pasted into **five** other
implementations, none of which have the accidental-correctness fallback —
these do have a real bug:

- `session/detection/detector.go` (`ansiStripRegex`) — powers Claude
  activity/state detection (spinner verbs, "esc to interrupt", readline
  prompt matching).
- `session/detection/ratelimit/detector.go` (`ansiRegex`) — powers rate-limit
  message detection.
- `session/tmux/banner_filter.go` (`stripANSIRe`) — tmux status-bar banner
  filtering.
- `server/services/connectrpc_websocket.go` (`ansiEscapeRe`) — used only by
  the diagnostic `detectContentWidth` (logs a width-mismatch warning; not
  wired to any rendering decision).
- `web-app/src/components/history/HistoryCardPreview.tsx` (`stripAnsi`) —
  session-history card preview text.

## Root Cause

A regex character class of `[a-zA-Z]` (or `[A-Za-z]`) was used everywhere in
this codebase to match a CSI sequence's terminating final byte. Per ECMA-48,
the CSI final-byte range is `0x40-0x7E`, which includes non-letter bytes like
`@` (Insert Character, CSI `Ps @`) and `~` (used by several real-world xterm
sequences). A CSI sequence ending in one of those bytes would not match any
of these regexes, leaving its raw ESC-prefixed bytes in the "stripped" output
— text that the affected detectors then run word-boundary pattern matching
against (e.g. `retry\s*after\s*(\d+)`, spinner-verb matching, the readline
typing regex). Stray escape bytes adjacent to or inside a matched phrase can
break those patterns, causing missed rate-limit/activity detection.

This is the same class of bug already found and fixed in `server/terminal`
(now-deleted) and `pkg/analytics/escape_code_parser.go` earlier in this
session (PR #156) — evidently reimplemented independently multiple times
across the codebase rather than shared.

## Fix

Widened the CSI final-byte character class from `[a-zA-Z]`/`[A-Za-z]` to
`[@-~]` (the literal ASCII range `0x40`-`0x7E`) in all five locations.

`EscapeSequenceParser.ts` was left unchanged (confirmed functionally correct
via trace, see Investigation above) to avoid unnecessary churn to a
correctly-behaving hot path.

### Follow-up: extracted a shared module (from `/code:review`)

A `/code:review` pass on this change (plus the session's earlier dead-code
removal) converged on the same finding from two independent review agents:
this exact character-class bug had already been copy-pasted into five
separate implementations once, and nothing stopped a sixth. Rather than leave
five independently-maintained widened regexes, the fix was consolidated into
one shared, tested source of truth per language:

- **Go**: new `pkg/ansi` package (`CSIFinalByteClass` constant,
  `CSIRegex`, `StripCSI()` helper with a zero-alloc fast path for
  escape-free input). `session/detection/ratelimit/detector.go`,
  `session/tmux/banner_filter.go`, and `server/services/connectrpc_websocket.go`
  now delegate to `ansi.StripCSI()` directly.
  `session/detection/detector.go`'s `ansiStripRegex` combines the CSI branch
  with two other alternatives (OSC, charset designation) that don't fit the
  simple helper, so it instead builds its regex using the shared
  `ansi.CSIFinalByteClass` constant — sharing the actual bug-prone fragment
  without forcing an unrelated pattern shape onto it.
- **TypeScript**: new `web-app/src/lib/terminal/stripAnsi.ts` exporting
  `stripAnsi()`. `HistoryCardPreview.tsx` now imports it instead of defining
  its own copy.

The same review also found two other MAJOR-severity gaps, fixed in the same
pass:

- `web-app/src/components/history/HistoryCardPreview.tsx`'s CSI fix had no
  regression test (the other four sites did) — added
  `web-app/src/lib/terminal/stripAnsi.test.ts`, testing the shared module
  directly.
- Five orphaned MOSH-protocol message types in the `TerminalData` oneof
  (`TerminalDelta`, `TerminalState`, `TerminalDiff`, `InputWithEcho`,
  `SSPNegotiation`, plus their exclusively-used dependents) were left in
  `proto/session/v1/events.proto` after the earlier dead-code removal deleted
  every producer/consumer but not the proto definitions themselves. Reserved
  field numbers 8, 12, 13, 14, 15 (mirroring the existing `streaming_mode`
  reservation) and deleted the now-unreferenced message types; regenerated Go
  and TypeScript bindings.

### Follow-up: StreamTerminal test coverage found (and fixed) a real race

The review also flagged that `StreamTerminal`'s simplified raw-output path
(introduced by this session's dead-code removal) had zero test coverage —
the only existing tests (`BenchmarkSessionService_StreamTerminal_NotFound`/
`NotStarted`) short-circuit before reaching the PTY-forwarding goroutine.
Adding `server/services/session_service_stream_terminal_test.go` (a real
tmux-backed end-to-end test) immediately caught two genuine, pre-existing
concurrency bugs under `-race`, neither related to the CSI fix. A first pass
at fixing both went out as PR #163; a `/github:pr-ship` code-review gate on
that PR (4 more parallel agents) correctly identified that the first pass
was incomplete on both counts and required a second, deeper fix before
merge:

1. **`StreamTerminal`'s output goroutine raced Connect's own stream-close
   write.** The goroutine reading the PTY and calling `stream.Send()` was
   never joined before the RPC handler returned, so a `stream.Send()` call
   in flight could race with Connect's `Close()` writing the end-of-stream
   frame to the same connection on context cancellation.
   - First pass: a `sync.WaitGroup` bounded to a 2s timeout, plus a
     `streamCtx.Done()` check immediately before each `Send()`. Reviewers
     correctly pointed out this only narrows the window — since the PTY read
     is a blocking syscall with no read deadline, a goroutine still blocked
     past the 2s timeout gets abandoned, and if it later wakes with data it
     still calls `stream.Send()` after the handler has returned.
   - Actual fix: `syscall.Dup()` the PTY fd once per `StreamTerminal` call so
     the output goroutine reads from its own independent `*os.File` (and
     `poll.FD`) rather than the one shared with the instance's other
     internal PTY consumers (response stream, command executor). This makes
     it safe to call `SetReadDeadline` (a shared `*os.File`'s deadline state
     can't be mutated from multiple goroutines without racing/corrupting the
     other readers' blocking calls — that's *why* the first pass avoided a
     deadline at all) without affecting anything else. A 250ms deadline on
     the dup'd fd means the goroutine now reliably notices `streamCtx.Done()`
     within ~250ms regardless of PTY activity, so the `wg.Wait()` bound is
     now a genuine safety net rather than the primary (leaky) mechanism.
2. **`session.Instance.Started()` raced `Instance.start()`.** `Started()`
   read the `started` field without `stateMutex`, while `start()` wrote it
   *outside* the lock it otherwise held for the adjacent status transition —
   a real production bug (this exact path is also used by `StreamTerminal`'s
   own precondition check), not a test artifact.
   - First pass: moved the write inside `start()`'s existing lock scope and
     added the missing `RLock`/`RUnlock` to `Started()`. Reviewers correctly
     pointed out `i.started` was *also* read/written completely unguarded at
     ~30 other sites across the `session` package (tmux lifecycle,
     hibernation, worktree/workspace teardown, serialization restore) — the
     fix only silenced the one repro this PR's own test happened to exercise.
   - Actual fix: converted the field from `bool` to `atomic.Bool` package-wide
     (`session/instance.go` plus every other `instance_*.go` file listed
     below), so every access site is race-free by construction instead of by
     manual lock-discipline audit. The `stateMutex` lock/unlock added around
     the single write/read pair in the first pass was removed as redundant.
3. Added `server/services/stream_terminal_routing_test.go`, pinning a
   routing invariant that was previously only asserted in a code comment:
   `net/http.ServeMux`'s longest-pattern-wins rule means the exact
   `.../StreamTerminal` path is always served by
   `ConnectRPCWebSocketHandler.HandleWebSocket`, never by the general
   ConnectRPC handler — so a non-browser client can never reach
   `SessionService.StreamTerminal` directly through the production HTTP
   routing, only through the WebSocket bridge.
4. Added `TestWaitWithTimeout` (deterministic, no tmux dependency) covering
   both branches of the `waitWithTimeout` helper, and exported
   `ansi.IsCSIFinalByte`/`CSIFinalByteMin`/`CSIFinalByteMax` as a byte-range
   twin to `CSIFinalByteClass` so `pkg/analytics/escape_code_parser.go`'s
   manual byte-scanning CSI check (a 6th independent reimplementation the
   original consolidation missed) now derives from the same source of truth.

## Files Changed

- `pkg/ansi/csi.go`, `pkg/ansi/csi_test.go` — new shared Go module
  (`CSIFinalByteClass`/`CSIRegex`/`StripCSI` for regex-based callers,
  `IsCSIFinalByte`/`CSIFinalByteMin`/`CSIFinalByteMax` for byte-scanning
  callers)
- `pkg/analytics/escape_code_parser.go` — CSI final-byte check now calls
  `ansi.IsCSIFinalByte` instead of an inline literal range
- `web-app/src/lib/terminal/stripAnsi.ts`, `stripAnsi.test.ts` — new shared
  TS module
- `session/detection/detector.go` — `ansiStripRegex` CSI branch, now built
  from `ansi.CSIFinalByteClass`
- `session/detection/ratelimit/detector.go` — `stripANSI` delegates to
  `ansi.StripCSI`
- `session/tmux/banner_filter.go` — `stripANSICodes` delegates to
  `ansi.StripCSI`
- `server/services/connectrpc_websocket.go` — `stripAnsiCodes` delegates to
  `ansi.StripCSI`
- `web-app/src/components/history/HistoryCardPreview.tsx` — imports the
  shared `stripAnsi`
- `proto/session/v1/events.proto`,
  `gen/proto/go/session/v1/events.pb.go`,
  `web-app/src/gen/session/v1/events_pb.ts` — reserved orphaned
  `TerminalData` oneof fields, deleted their message types
- `server/services/session_service.go` — `StreamTerminal`: dup'd PTY fd with
  its own read deadline, `sync.WaitGroup` + bounded wait as a safety net,
  pre-`Send` cancellation check
- `session/instance.go` — `started bool` → `started atomic.Bool`
- `session/instance_state.go`, `instance_terminal.go`, `instance_controller.go`,
  `instance_hibernate.go`, `instance_worktree.go`, `instance_workspace.go`,
  `instance_tmux.go`, `instance_serialization.go`, `instance_checkpoint.go` —
  every direct `.started` access site updated to `.Load()`/`.Store()`
- 15 `session/*_test.go` files — updated `Instance{started: ...}` struct
  literals and direct `.started = ...` assignments to `.started.Store(...)`
  (mechanical; `atomic.Bool` doesn't support struct-literal field init)

## Tests Added

- `session/detection/bug_regression_test.go` —
  `TestBug6_CSINonLetterTerminator_Stripped` (`@` and `~` terminators)
- `session/detection/ratelimit/detector_test.go` — two new cases in
  `TestStripANSI`
- `session/tmux/banner_filter_test.go` — two new cases in
  `TestStripANSICodes`
- `server/services/connectrpc_websocket_test.go` —
  `TestStripAnsiCodesHandlesNonLetterCSITerminators`
- `pkg/ansi/csi_test.go` — `TestStripCSI`,
  `TestStripCSI_ZeroAllocsOnPlainText`
- `web-app/src/lib/terminal/stripAnsi.test.ts` — 5 cases covering the OSC
  and CSI branches, including the `@`/`~` terminators
- `server/services/session_service_stream_terminal_test.go` —
  `TestStreamTerminal_SendsRawOutput` (real tmux-backed end-to-end test;
  caught the two race bugs above)
- `server/services/stream_terminal_routing_test.go` —
  `TestStreamTerminalRouting_WebSocketPathTakesPrecedenceOverGeneralHandler`,
  `TestStreamTerminalRouting_OtherRPCsStillReachGeneralHandler`
- `server/services/session_service_stream_terminal_test.go` —
  `TestWaitWithTimeout` (both branches, deterministic, no tmux dependency)

## Verification

```
go build ./...                                      # clean
go vet ./...                                         # clean
go test ./... -race                                  # all packages ok, race-clean
go test ./server/services/... -run TestStreamTerminal_SendsRawOutput -race -count=15  # 15/15 pass
cd web-app && npx tsc --noEmit && npx jest --no-coverage
    Test Suites: 154 passed, 154 total
    Tests:       2781 passed, 2781 total
make quick-check                                    # ✅ full pass: build, test-coverage, test-race, lint,
                                                      #    lint-css-tokens, registry-diff (0.0% divergence)
```

## Conclusion for the Original Report

The rendering pipeline for the new Claude Code renderer was already properly
fixed (PR #139, June 29) and remains intact. This bug was a latent gap in the
adjacent detection/filtering code discovered during the audit the user
requested, not a regression from that fix or from this session's dead-code
removal. It is now fixed and covered by regression tests everywhere the
pattern was found.
