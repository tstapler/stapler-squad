# Adversarial Review: new-renderer

Date: 2026-06-24 (updated — second pass)
Verdict: CONCERNS

---

## Prior Blocker Resolution

### Blocker 1 — TextDecoder constructor options (RESOLVED)
The plan now explicitly states: "Leave the constructor as-is (`new TextDecoder()` — no constructor
options needed)." It correctly identifies that `{ stream: true }` is a call-site concern only.
Line 231 and line 476 are both explicitly named with the exact `{ stream: true }` fix required.
The misleading constructor change from the prior version is gone. **RESOLVED.**

### Blocker 2 — Missing call site line 298 (RESOLVED)
Story 1.1.2 now explicitly names both line 298 (`currentPaneResponse.content`) and line 308
(scrollback chunks) and requires both to move to the dedicated `scrollbackDecoderRef`. The audit
instruction ("Audit ALL call sites of `textDecoderRef.current.decode` before closing this story")
is present. **RESOLVED.**

### Blocker 3 — ED3 removal unverified (RESOLVED WITH NOTE)
Story 1.2.2 now contains an explicit acceptance criterion requiring browser validation before
merge:
> "Verified in browser: xterm.js renders combined `\x1b[2J\x1b[3J` reset without flicker
> regression."
The story also carries a mandatory NOTE block: "Do not merge Story 1.2.2 without browser
validation." Risk Flag 1 repeats this in the Risk Flags section. The gate is now in the plan
rather than left to the implementer's discretion. **RESOLVED — gate is explicit.**

### Blocker 4 — reset() wiring unnamed (RESOLVED)
The plan now specifies exactly where `reset()` must be called:
> "Wire `reset()` by calling `this.reset()` from **inside `resetSequence()` (line 629 of
> `StateApplicator.ts`)**, NOT from a separate or external cleanup location."
The rationale is given (decoder state and sequence-tracking state must reset together), and the
"unmount is too late" warning is explicit. **RESOLVED.**

---

## Verification of Secondary Items

### Dead code stories (1.4.1 / 1.4.2) re-scoped (RESOLVED)
Former Stories 1.4.1 and 1.4.2 are now Phase 3 Stories 3.1.1 and 3.1.2 under "Defensive
Maintenance." The dependency visualization explicitly labels them as off the critical path with
the annotation "dead code today; fix before re-wiring." Risk Flag 2 calls out that they cannot
be the cause of the current regression. The phase 3 section header includes the caveat that these
"cannot cause the current regression." **RESOLVED.**

### RedrawThrottler regex corrected (RESOLVED)
The plan's "After" regex no longer includes `\x1b[H` (cursor-home). The new regex is:
```
/^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/.test(chunk.substring(0, 32))
```
The comment explicitly states: "NOTE: `\x1b[H` (cursor-home) is intentionally excluded — it is
also emitted during non-full-screen interactive prompts... Only erase-screen sequences
(`\x1b[2J` or `\x1b[J`) are reliable indicators of a genuine full-screen redraw." **RESOLVED.**

### Integration tests for combined fixes (NOT ADDRESSED — remaining concern)
The Test Coverage Summary still lists only per-story unit tests. There is no integration test,
smoke test, or validation step that exercises the full pipeline after all Phase 1 fixes land
together. The plan adds an individual browser smoke test for Story 1.2.2 (ED3 removal) but no
end-to-end test that validates the interplay between the TextDecoder fix, the escape parser
lookback fix, and the throttler fix working together. The prior concern about fixes silently
interacting is still open — see Concerns section below.

---

## Remaining Concerns

### Concern 1 — No combined integration validation (carried from prior review)
The plan has per-story unit tests but no test that exercises the whole TypeScript rendering
pipeline after all Phase 1 fixes are applied. Because the stories are declared independent and
parallelizable, they could be merged separately, each passing its own unit test, while a
cross-fix interaction (e.g., the escape parser buffering a partial sequence that the throttler
then discards) causes a silent regression not caught by individual story tests.

**Recommendation**: Add a test in `web-app/src/__tests__/` (or as an E2E playwright test) that
pipes a realistic Claude Code output stream — containing multi-byte UTF-8, an OSC title, and a
full-screen redraw — through `EscapeSequenceParser` → `TerminalStreamManager` → mock xterm.js
write and asserts clean, ordered output. Alternatively, require all Phase 1 stories to land in a
single PR so that the combined state is only shipped once all unit tests pass together.

### Concern 2 — scrollbackDecoderRef fix at line 298 uses wrong field (minor precision issue)
Story 1.1.2 step 3 says: "Replace both the line 298 and line 308 decode calls to use
`scrollbackDecoderRef.current.decode(chunk.data, { stream: true })`." However, line 298 decodes
`currentPaneResponse.content`, not `chunk.data` — `chunk.data` is the scrollback-specific field
on line 308. The fix at line 298 should be
`scrollbackDecoderRef.current.decode(currentPaneResponse.content, { stream: true })`.
This is a minor precision gap that will likely be caught at implementation time, but is worth
noting so the implementer doesn't blindly copy the code snippet from step 3 for both sites.

### Concern 3 — 256-char lookback still heuristic (carried from prior review, now minor)
The prior review flagged 256 as an arbitrary cap that OSC 52 (clipboard) payloads can exceed.
The plan does not address this — 256 remains the proposed value. For the known Claude Code output
patterns (OSC 8 hyperlinks, OSC terminal titles) this is sufficient. For clipboard content,
it is not. Given the plan's scope is fixing the current Claude Code renderer regression rather
than achieving universal correctness, this is acceptable as a known limitation — but the plan
should document it as a known gap so a future maintainer does not assume 256 is the correct
bound.

---

## Minors (carried from prior review, none resolved)

1. **ST terminator false-positive in DCS lookback**: `findPartialEscapeAtEnd` scans backward
   for the last `\x1b`. If the ST terminator (`\x1b\\`) lands in the lookback window, the scan
   finds the `\x1b` of the terminator and treats the trailing `\\` as a short 2-char escape
   (complete), incorrectly marking the containing DCS/PM/APC sequence as done. Test coverage
   should include a DCS sequence whose ST terminator `\x1b\\` lands in the last 20 bytes of
   the chunk. The plan does not name this test case.

2. **`DebugEscapeAnalytics` should be env-var-only**: Story 2.1.2 persists the
   `debugEscapeAnalytics` field to `config.json` via the `json` struct tag. A user who
   accidentally sets this and saves will run analytics at full throughput indefinitely with no UI
   warning. Consider using `json:"-"` (no persistence) and only reading the env var.

3. **throttleMs reduction affects capture-pane path**: The 33 ms throttle is a global change
   to `TerminalStreamManager` used by both the SSP and capture-pane paths. High-frequency
   capture-pane snapshots at 33 ms intervals may increase perceived CPU on slow clients.
   Verify this does not cause noticeable load regression before merging Story 1.3.1.

---

## New Items Not Present in Prior Review

None identified. The plan patches have addressed all four blockers cleanly. No new blockers were
introduced by the patches.

---

## Summary

All 4 prior blockers are resolved. The single remaining concern of substance is the absence of a
combined integration test for the Phase 1 fixes (Concern 1), which is a process risk rather than
a correctness gap in the plan itself. The three minors are low-risk and unlikely to cause merge
failures. The plan is ready for implementation with the Phase 1 stories proceeding in parallel.
