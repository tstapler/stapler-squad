# Architecture Review: new-renderer Implementation Plan

**Reviewer**: Claude Code (architecture verification pass)
**Date**: 2026-06-24
**Branch**: stapler-squad-new-renderer
**Plan file**: `project_plans/new-renderer/implementation/plan.md`

---

## Item 1: StateApplicator.ts TextDecoder fix (Story 1.1.1)

**Verdict: CONFIRMED — bug is real; fix direction is correct; one adjustment needed**

**Evidence**:

File: `web-app/src/lib/terminal/StateApplicator.ts` line 32:
```typescript
private textDecoder: TextDecoder = new TextDecoder();
```

Two call sites without `{ stream: true }`:
- Line 231: `const diffStr = this.textDecoder.decode(diff.diffBytes);` (in `applyDiffImmediate`)
- Line 476: `const lineText = this.textDecoder.decode(line.content);` (in `applyIncrementalState`)

Both are exactly as described in the plan.

**Adjustment needed — decoder instance reuse across RAF frames**:

The plan says to add `{ stream: true }` to all calls. This is necessary but has a subtle
complication: the RAF-batching design in StateApplicator means calls to `applyDiffImmediate`
and `applyIncrementalState` are separated by animation frame boundaries (not just micro-task
boundaries). Between frames, the browser event loop runs — but `TextDecoder` with
`{ stream: true }` holds its internal state until either another `decode()` is called or
`decode()` is called without `{ stream: true }` to flush. Reusing a single instance across
RAF-separated calls is correct IF the sequence of calls is always alternating
decode→decode→... with no intervening non-streaming flush. Tested against the browser spec:
this is safe.

However, the plan's proposed `reset()` method replaces the entire decoder instance. This is
correct for reconnects. The plan should also note that `resetSequence()` (line 629) already
handles reconnect cleanup but does NOT reset the TextDecoder — `reset()` must be called from
the same cleanup path. Current `resetSequence()` does NOT call any decoder reset. The plan
needs to wire `this.textDecoder = new TextDecoder(...)` (or call `reset()`) inside
`resetSequence()`, not just add a standalone `reset()` method.

**File path**: Correct (`web-app/src/lib/terminal/StateApplicator.ts`).

---

## Item 2: EscapeSequenceParser lookback window (Story 1.2.1)

**Verdict: CONFIRMED — 20-char constant exists; 256 is reasonable; DCS/PM/APC gap is real**

**Evidence**:

File: `web-app/src/lib/terminal/EscapeSequenceParser.ts` line 83:
```typescript
const scanLength = Math.min(20, data.length);
```

Exactly matches the plan. The `isCompleteEscapeSequence` method (line 120) handles:
- CSI (`[`) via `hasCSITerminator`
- OSC (`]`) via `hasOSCTerminator`
- Simple 2-char escapes

Missing: DCS (`P`), PM (`^`), APC (`_`), SOS (`X`) — confirmed absent from the
`isCompleteEscapeSequence` switch. All fall through to `return true` at line 158
("Unknown sequence type — assume complete to avoid infinite buffering"), meaning they
are **never buffered** regardless of the lookback window.

**Important clarification on lookback vs. isCompleteEscapeSequence interaction**:

The lookback window and `isCompleteEscapeSequence` are two separate guards that both need
fixing:

1. The 20-char window means an `\x1b` more than 20 bytes from the end is never found — the
   sequence isn't detected as partial at all.
2. Even if the `\x1b` IS found within 20 bytes, `isCompleteEscapeSequence` returns `true`
   for DCS/PM/APC/SOS (treats them as complete), so they are never buffered.

The plan fixes both (window increase + new cases in `isCompleteEscapeSequence`). Both
fixes are required; either alone is insufficient. The plan correctly identifies this.

**OSC 8 hyperlinks**: Claude Code's new renderer emits OSC 8 hyperlinks in the form
`\x1b]8;params;url\x1b\\` where the URL can easily exceed 100 characters. The 20-char
window would miss the `\x1b]` if the chunk boundary lands more than 20 bytes into the
URL. Increasing to 256 is conservative but correct — OSC 8 URLs in practice can approach
256 chars (e.g., deep GitHub URLs) but rarely exceed it. A more defensive value would be
512 or `Math.min(data.length, 512)` but 256 covers all known Claude Code output patterns.

**File path**: Correct (`web-app/src/lib/terminal/EscapeSequenceParser.ts`).

---

## Item 3: RedrawThrottler cursor-up regex (Story 1.3.1)

**Verdict: NEEDS ADJUSTMENT — regex in plan has a typo; narrowing logic is sound but has an edge case**

**Evidence**:

File: `web-app/src/lib/terminal/TerminalStreamManager.ts` line 56:
```typescript
const isFullRedraw = /^\x1b\[\d+A/.test(chunk);
```

Exact match. The `RedrawThrottler` is a private inner class (lines 43–89); the regex is at
line 56 of the file (plan says "line 56" in the class context — confirmed).

**Regex typo in the plan**:

The plan's "Before" regex is written in the review text as `\x1b[\d+A` (missing the
backslash before `[`). This is a documentation typo — the actual code has `\x1b\[\d+A`
(correct). The proposed "After" regex in the plan is:
```
/^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J|\x1b\[H)/.test(chunk.substring(0, 32))
```
This is syntactically correct.

**Edge case — cursor-up + CUP (H) without clear**:

The new Claude Code renderer (Ink-based) uses `\x1b[H` (cursor home = `\x1b[1;1H` or
`\x1b[H`) to reposition before redrawing, sometimes WITHOUT a screen clear. The proposed
"After" regex includes `\x1b\[H` as a trigger for full-redraw throttling. However, `\x1b[H`
alone (cursor home) is also emitted during non-full-screen interactive prompts (e.g., moving
to the top of a diff block). Using `\x1b[H` as a full-redraw indicator may over-throttle
non-full-screen redraws that happen to start with cursor-up + cursor-home.

**Recommendation**: Use `\x1b\[2J` or `\x1b\[J` (erase screen) as the corroborating
signal — these are only emitted during genuine full-screen redraws:
```typescript
const isFullRedraw = /^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/.test(chunk.substring(0, 32));
```
Drop `\x1b\[H` from the pattern to avoid false positives.

**throttleMs reduction from 100 to 33**: Correct and safe. The `throttleMs` is a private
field (line 47) with no external getter; changing it only affects throttle frequency.

**File path**: Correct (`web-app/src/lib/terminal/TerminalStreamManager.ts`).

---

## Item 4: sanitizeUTF8Bytes scope verification (Story 1.4.1)

**Verdict: CONFIRMED — sanitizeUTF8Bytes is NOT on the capture-pane display path**

**Evidence**:

`sanitizeUTF8Bytes` is a method of `StateGenerator` (package `terminal`,
`server/terminal/state.go`). The `StateGenerator` struct is defined but **never instantiated
anywhere in the non-test codebase**. There are no imports of `github.com/tstapler/stapler-squad/server/terminal` in any non-test Go file. Backup files (`connectrpc_websocket.go.bak`, `.bak2`, `.before_cursor_fix`) show the `server/terminal` package was previously imported under the old module path `claude-squad/server/terminal`, but it is NOT imported in the current codebase.

**Conclusion**: `server/terminal/state.go` and `server/terminal/delta.go` are effectively
dead code — they exist in the repo and are compiled (the package builds) but are not called
from any production code path. Story 1.4.1 (SO/SI preservation) and Story 1.4.2 (OSC-aware
line splitter) would fix real bugs **in code that does not currently execute**.

**This is a significant finding.** The plan should acknowledge this and either:
1. Mark Stories 1.4.1 and 1.4.2 as "defensive maintenance" (fix bugs in unused code that
   may be re-wired in future), or
2. Remove them from the critical path since they cannot be the cause of the current
   rendering regression (the default `raw-compressed` path through `streamViaTmuxCapturePane`
   does not use `StateGenerator` at all).

The actual default streaming path in `streamViaTmuxCapturePane` sends raw snapshot strings
directly as `[]byte(fullContent)` — no `sanitizeUTF8Bytes` involved.

**File path**: Correct (`server/terminal/state.go`), but the code is not on the active path.

---

## Item 5: Analytics Stage 2 tap in streamViaTmuxCapturePane (Story 2.1.1)

**Verdict: CONFIRMED — Stage 2 tap is absent from streamViaTmuxCapturePane; function name is correct**

**Evidence**:

The Stage 2 tap in `streamViaControlMode` (`connectrpc_websocket.go` lines 762–769):
```go
if escapeParser == nil {
    escapeParser = instance.GetEscapeParser()
}
if escapeParser != nil && escapeParser.IsEnabled() {
    escapeParser.ParseStage2(buf, instance.GetTotalBytesWritten())
}
```

In `streamViaTmuxCapturePane` (lines 1033–1195), there is no `GetEscapeParser()` call and no
`ParseStage2()` call anywhere in the function body. The plan's description is accurate.

The function signature and variable names match what the plan references:
- `instance.GetEscapeParser()` — confirmed callable (same `*session.Instance` type)
- `escapeParser.ParseStage2([]byte(fullContent), instance.GetTotalBytesWritten())` — matches the
  pattern used in `streamViaControlMode`

**One implementation detail**: The plan says to add the tap "immediately before the
`stream.WriteMessage` call that sends `fullContent`". In `streamViaTmuxCapturePane`, the
output goroutine sends snapshots (full terminal state with `clearAndHome` prefix) rather than
raw diffs. The analytics tap should fire on `fullContent` (after `formatSnapshotForClient`)
because that is what actually goes to the client. The plan's placement is correct.

**Also note**: Stage 1 tap is in `session/response_stream.go` line 279 (verified in stack
research). `streamViaTmuxCapturePane` bypasses `response_stream.go` — there is no Stage 1
tap either. The plan only wires Stage 2; for full pipeline coverage a Stage 1 tap should also
be added at the point where `streamer.GetContent()` returns output, but this is outside the
plan's scope and not a blocker.

**File path**: Correct (`server/services/connectrpc_websocket.go`).

---

## File Path Corrections for plan.md

No path corrections needed — all file paths in the plan are accurate. One structural note:

The plan references `pkg/analytics/escape_code_store.go` and
`session/response_stream.go` for Story 2.1.2. These paths should be verified before
implementation; stack.md confirms they exist but the plan should cross-check the exact
function name `newEscapeParserForSession()` (not verified in this review — add to pre-impl
checklist).

---

## Overall Architecture Health Assessment

**Health: GOOD with two significant adjustments required**

The plan is well-grounded. All five items were verifiable against actual source code, and
four are confirmed correct. The two items requiring adjustment are:

**Adjustment 1 (Story 1.1.1)**: The `resetSequence()` method in `StateApplicator` does not
reset the `TextDecoder` instance. After adding `{ stream: true }` to all decode calls, the
new `reset()` method must also be wired into `resetSequence()` — otherwise a reconnect leaves
a streaming decoder instance with stale internal state, potentially corrupting the first frame
after reconnect.

**Adjustment 2 (Stories 1.4.1 + 1.4.2)**: `StateGenerator` and its `sanitizeUTF8Bytes` /
`splitIntoTerminalLines` functions are currently unused dead code — the `server/terminal`
package is not imported by any production Go file. These stories fix real bugs in real code,
but they CANNOT be the cause of the current rendering regression. They should be re-scoped as
maintenance tasks, and their prioritization in the plan (MEDIUM) should be noted as
"defensive maintenance on unused code." The actual regression root causes are confined to
the TypeScript pipeline (Stories 1.1–1.3) and the ED3 filter (Story 1.2.2).

**Critical path recommendation**: Implement Stories 1.1.1, 1.1.2, 1.2.1, 1.2.2, and 1.3.1
first — these are the only stories on the active default rendering path
(`raw-compressed` mode via `streamViaTmuxCapturePane` → `useTerminalStream.ts` →
`TerminalStreamManager` → `EscapeSequenceParser` → `xterm.js`). Stories 1.4.1 and 1.4.2 can
be addressed in a follow-up once the regression is confirmed fixed.
