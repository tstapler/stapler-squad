# Pitfalls Research: terminal-redraw-corruption

**Date**: 2026-08-06
**Scope**: Phase 2 research — known pitfalls/risks for "stale characters surviving a redraw" bug class.

## 1. Classic root causes of "stale glyphs under a shorter redrawn line"

| # | Cause | Applies here? |
|---|---|---|
| a | App emits EL (`\x1b[K`) but the terminal's parser fails to recognize/apply it | UNLIKELY as primary cause. xterm.js ships a proper FSM-based escape parser (see §2) that correctly implements CSI EL (0K/1K/2K); no evidence in this repo's code that anything strips or rewrites `\x1b[K` before it reaches xterm.js (see §3/§5 — the one regex that used to touch escape *content* was removed). |
| b | EL sequence is split across two separately-processed chunks and the second half is lost | LOW risk for xterm.js's own parser (it is documented as stream-aware — state persists across `.write()` calls, see §2, Issue #1395/#943 discussion). This repo *also* adds its own chunk-boundary buffering in `EscapeSequenceParser.processChunk()` (`web-app/src/lib/terminal/EscapeSequenceParser.ts:34`) ahead of xterm — belt-and-suspenders, but see §5 for a hypothesis about it. |
| c | A buffering/coalescing layer merges frames and forwards only the LAST frame's cursor position/erase, silently dropping earlier frames' erases | **HIGH-CONFIDENCE HYPOTHESIS, code-grounded** — see §4. `RedrawThrottler` in `web-app/src/lib/terminal/TerminalStreamManager.ts:42-92` does exactly this by design. |
| d | Terminal deduplicates "diff-like" redundant output and drops a row it thinks is a duplicate | No evidence of a dedup/diffing layer in this stack — ruled out. |
| e | Resize/reflow misplaces stale characters under an assumed wrong width | Not implicated by the screenshot (no resize event in the described flow); not pursued further. |
| f | Multiple independent writers racing on the same buffer (e.g. `docs/tasks/terminal-jank.md` Story 2's quiescence-triggered `tmux capture-pane` snapshot vs. a live streaming path) | **Plausible secondary contributor**, not primary. See §4b. |

## 2. xterm.js issues on EL correctness / chunked writes / ghosting

WebSearch findings (queries: "xterm.js github issue erase in line stale/ghost characters", "xterm.js issue chunked write splits escape sequence corruption"):

- [xterm.js #943 — "clear() is not correctly clearing the buffers"](https://github.com/xtermjs/xterm.js/issues/943): describes a node-pty race between `data` and `exit` events where `exit` can fire before all data is flushed, so the *final* write from the PTY can contain an unfinished ANSI sequence — the *next* write's first line then gets prefixed with the unfinished tail of the previous write. Directly analogous risk shape to this bug, but the trigger (process exit racing pty flush) doesn't match our scenario (Claude Code is actively running, not exiting).
- [xterm.js #1395 — "proposal: switch to DEC/ANSI compliant escape sequence parser"](https://github.com/xtermjs/xterm.js/issues/1395): xterm.js's parser is FSM-based and maintains state across `.write()` calls — i.e. xterm.js is documented to handle escape sequences split across separate writes correctly at its own parser layer. This means classic cause (b) is a low-probability culprit for a *modern* xterm.js version, and reduces (but does not eliminate — see §5) the value of a duplicate chunk-boundary parser sitting in front of it.
- [xterm.js #145 — "broken escape sequence parser states"](https://github.com/xtermjs/xterm.js/issues/145): older, general class of parser-state bugs (control chars not resetting/breaking parser state mid-sequence) — background context, not directly on point.
- [JetBrains/jediterm #118 — "Erase in Line CSI sequence does not erase line content beyond the terminal width"](https://github.com/JetBrains/jediterm/issues/118): a *different* terminal emulator's EL bug (obscured off-screen-width content not cleared by `\x1b[K`/`\x1b[2K`). Notable because it establishes EL-scope bugs are a real, recurring category across terminal emulator implementations — but it's evidence about jediterm's behavior, not xterm.js's; no confirmed xterm.js-specific report of this exact defect was found.
- No exact xterm.js issue titled specifically "ghost/stale characters surviving EL" was found. Confidence: WebSearch coverage of GitHub issues is not exhaustive; absence of a hit is not proof xterm.js has never had this bug, just that it wasn't surfaced by these two queries.

## 3. Stack-specific risks (Go tmux relay + custom `EscapeSequenceParser.ts` + xterm.js)

Custom regex/string preprocessing of an ANSI byte stream *before* a real terminal emulator sees it is a known risk category in general, for a structural reason: a terminal emulator's escape-sequence grammar is context-sensitive and stream-oriented (CSI/OSC/DCS/PM/APC/SOS each have different terminators, some terminators are literal bytes that can also appear as ordinary payload bytes inside a different sequence type, e.g. a `J` inside an OSC window-title payload is not a CSI terminator). A regex or word-boundary scan that doesn't track "which sequence type am I currently inside" can:
- Consume bytes it shouldn't (over-greedy match swallows real content or a following sequence).
- Fail to consume bytes it should (under-greedy match treats a mid-payload byte as a terminator, splitting a sequence into "processed" + "leftover" halves that get treated independently).
- Silently reorder or drop sequences when it coalesces multiple frames (see §4).

This repo already has a **confirmed historical instance of exactly this category**: commit `0ac0ca1dad9f102d7072857c87714fb2d1905e05` (PR #163) found `stripANSIBytes`/`sanitizeUTF8Bytes` in `web-app/src/lib/terminal/stripAnsi.ts` treated any ASCII letter as a CSI terminator, which is wrong for OSC/DCS/PM/APC/SOS (their terminators are BEL or ST, not "any letter"), causing their payload text to leak as literal characters.

**Distinguishing this from the current bug** (per the requirement to confirm these are separate classes, not assume a regression): `stripAnsi.ts`'s `stripANSIBytes`/`sanitizeUTF8Bytes` are used only by `web-app/src/components/history/HistoryCardPreview.tsx` (a static backlog/history card preview render) — grep confirms no import of `stripAnsi` in the live terminal streaming path (`TerminalStreamManager.ts`, `EscapeSequenceParser.ts`, `useTerminalStream.ts`). PR #163's fix could not have caused or fixed anything in the live xterm.js redraw path; it's a genuinely different code path serving a different feature. **Confirmed distinct, VERIFIED by grep**, not just asserted.

The live path's own custom preprocessing is `EscapeSequenceParser.processChunk()` (chunk-boundary buffering only, no longer any content-mutating regex — see §5) and `RedrawThrottler.process()` (frame coalescing — see §4). The relay itself: Go backend streams tmux control-mode / capture-pane bytes over ConnectRPC/WebSocket to the browser in whatever chunks the transport happens to produce; the browser-side `write(output)` in `TerminalStreamManager.ts:228` is called once per received message, and nothing in the reviewed code guarantees that message boundaries align with tmux's own write boundaries or line boundaries — i.e., a single logical redraw (cursor-up + erase + new text) can legitimately arrive split across two `write()` calls, or two independent tmux writes can land in the same `write()` call. Any coalescing/classification logic keyed on "does this chunk start with X" (as `RedrawThrottler` does) is therefore inherently exposed to chunk-boundary risk regardless of the escape parser's own buffering, because it can only see one `write()` call's data at a time when it decides "is this chunk a full redraw."

## 4. The concrete, code-grounded hypothesis (this bug's most likely mechanism)

### 4a. `RedrawThrottler` frame-coalescing drop (primary hypothesis, HIGH confidence)

`web-app/src/lib/terminal/TerminalStreamManager.ts:42-92`:

```ts
class RedrawThrottler {
  process(chunk: string): string | null {
    const isFullRedraw = /^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/.test(chunk.substring(0, 32));
    if (!isFullRedraw) {
      this.flushPending();
      return chunk;
    }
    this.pendingRedraw = chunk;   // <-- overwrites any previously-pending, not-yet-flushed redraw
    if (!this.throttleTimer) {
      this.throttleTimer = setTimeout(() => this.flushPending(), this.throttleMs); // 33ms
    }
    return null; // Don't output yet
  }
}
```

When two chunks that both match the "full redraw" pattern (cursor-up N + `\x1b[2K` or `\x1b[J`) arrive within the same 33ms window, the **first chunk is discarded outright** — `this.pendingRedraw = chunk` unconditionally replaces it, and `flushPending()` only ever emits the most recent one. This is safe *only if* every "full redraw" frame from the app re-erases and re-draws the exact same set of rows every time (in which case dropping an intermediate frame just skips a visually-identical repaint). It is **not** safe if the app's redraws vary in how many rows they touch from frame to frame — e.g. Claude Code's Ink-based renderer redrawing a multi-line status block (cursor up 5, erase, write 5 lines) on one frame, then redrawing only its last, now-shorter status line (cursor up 1, erase, write 1 line) on the very next frame. If the second frame is classified as "full redraw" and coalesces with/replaces the first before the first is flushed, the first frame's erase-and-rewrite of the *other 4 rows* never reaches xterm.js at all in that cycle — those rows keep whatever content was already on screen from an even earlier frame, while only the bottom row gets touched. That reproduces exactly the screenshot symptom: a longer previous line's leading fragment surviving under a new shorter line on the row below/adjacent, because the coalescing point cut out the frame that would have erased it.

This mechanism is item (c) from §1's enumerated causes, made concrete in this codebase. **Confidence: code-verified mechanism that is capable of producing this exact symptom class; NOT yet confirmed as the actual cause of the specific screenshot** — that requires either a live repro with the throttler instrumented/disabled, or a packet capture showing two full-redraw-classified frames landing within 33ms of each other with differing row spans. Hand this to the architecture/stack researchers to confirm or rule out with a targeted repro (e.g. temporarily setting `throttleMs = 0` or logging `pendingRedraw` discards) before the planner commits to a fix here.

### 4b. Secondary/contributing factor: dual-writer race (capture-pane snapshot vs. live stream)

`docs/tasks/terminal-jank.md` Story 2 documents a quiescence-gated `tmux capture-pane` snapshot path (cold-start / reconnect) that exists alongside the live control-mode streaming path, with a per-session dirty-flag cache (`server/services/connectrpc_websocket.go`). The plan's own "Bug 4: tmux capture-pane Alternate Screen Race" entry already flags a race window between the SIGWINCH-forced redraw and the actual `capture-pane` exec, mitigated but "not eliminate[d]" by the quiescence detector. This is a plausible **secondary** contributor if the screenshot session had just reconnected or cold-started (client refresh, session switch, pool eviction) around the time the corruption appeared — worth checking session/client logs for a recent connect event, but it does not explain steady-state corruption in an already-connected, continuously-streaming session, which is what the screenshot's tool-call cadence suggests. Treat as a secondary hypothesis to rule in/out via reconnect-timing correlation, not the primary suspect.

## 5. Is Story 1's existing custom preprocessing (`EscapeSequenceParser.ts`) itself eating/mangling EL sequences?

**Read the file** (`web-app/src/lib/terminal/EscapeSequenceParser.ts`). Two things are relevant:

1. `docs/tasks/terminal-jank.md` Story 1 (Task 1.1) describes adding a regex that stripped `\x1b[3J` when paired with `\x1b[2J`. **That regex no longer exists in the current code.** It was added, then explicitly removed 5 weeks ago in commit `dc71b828a` (PR #139, "fix(terminal): repair escape code pipeline for new Claude Code renderer"), whose commit message states: *"ED2+ED3 stripping: parser stripped `\x1b[3J` when paired with `\x1b[2J`... xterm.js v6 handles this correctly without intervention."* The current `processChunk()` (`EscapeSequenceParser.ts:39`) literally reads `const filtered = fullData; // No sequence stripping - xterm.js v6 handles ED2+ED3 correctly` — i.e. **no content-mutating regex runs on the byte stream today.** The only remaining logic is `findPartialEscapeAtEnd`/`isCompleteEscapeSequence`/`hasCSITerminator`/`hasOSCTerminator`, which only ever *hold back* bytes at the tail of a chunk (to prepend to the next chunk) — they never rewrite, drop, or mutate bytes that are classified as complete.
2. Manually walking `hasCSITerminator` against `\x1b[K` / `\x1b[2K` / `\x1b[0K` (the EL family): `K` is in the `0x41-0x5A` (uppercase A-Z) terminator range checked at `EscapeSequenceParser.ts:178`, and the loop starts scanning parameters at index 2, so `\x1b[K`, `\x1b[0K`, `\x1b[1K`, `\x1b[2K` are all correctly recognized as *complete* CSI sequences the moment all their bytes are present in the buffer — there is no code path where a complete EL sequence gets treated as partial-and-buffered, and no code path where a complete EL sequence gets its bytes altered.

**Conclusion for this bug**: the ED3-filter regex (the one known instance of this file mutating escape *content*) is gone from the current code and cannot be the mechanism — HIGH confidence, verified directly by reading `EscapeSequenceParser.ts:39` and confirming via `git log -p` that the regex was removed in `dc71b828a`, not merely never-yet-added. The remaining buffering logic in this file is a **hypothesis worth a brief, explicit second look but LOW confidence as this bug's cause**: it only affects the tail of a chunk when that tail contains an as-yet-*incomplete* escape sequence, and the analysis above finds no case where a complete `\x1b[K`-family sequence would be misclassified. The stronger, code-verified candidate is `RedrawThrottler` (§4a), which is a distinct file/mechanism from Story 1's parser. State this clearly to the architecture/stack researchers: **do not spend further effort re-auditing `EscapeSequenceParser.ts`'s buffering logic as the primary suspect — its one content-mutating regex is already gone, and its remaining logic is verified not to touch complete EL sequences. Focus confirmation effort on `RedrawThrottler` (§4a) instead.**

## 6. Pitfalls list for the Phase 3 planner (design against these explicitly)

1. **Do not add another custom regex to "patch" this.** This repo has now added-then-removed one content-mutating regex in this exact file (the ED3 filter) because "xterm.js v6 handles it correctly" — trust the real emulator's parser for anything it already implements correctly (CSI EL family is basic, well-supported functionality); reach for custom preprocessing only for problems xterm.js genuinely cannot solve (e.g. chunk-boundary buffering for *this transport's* framing, which xterm's parser doesn't need but doesn't hurt either).
2. **Any remaining/needed preprocessing must be sequence-aware, not word-boundary regex.** If a fix requires classifying "is this a full-screen redraw," the classifier needs to reason over the actual erase/cursor state (or at minimum a state machine that tracks how many rows a sequence actually erases and moves through), not a fixed-alternation regex like `/^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/` that silently fails to match e.g. bare `\x1b[K` or `\x1b[1K` variants, or note that this regex misses `\x1b[2K` variants preceded by a different terminator ordering (e.g. `ansi-escapes`-style erase-then-cursor-up rather than cursor-up-then-erase — the comment in `RedrawThrottler` explicitly assumes one specific byte order).
3. **Any chunk-buffering must never split mid-escape-sequence** — this one is already correctly designed-for in `EscapeSequenceParser.ts` (verified in §5); preserve that invariant in any rewrite.
4. **If a coalescing/throttling layer must exist for flicker reduction, it must not silently discard a pending frame whose erase footprint (rows/columns touched) differs from the frame replacing it.** The fix should either (a) always flush every full-redraw frame that has a *different* footprint from the currently-pending one before replacing it, (b) merge frames by applying each one's erase set rather than only the latest one's, or (c) abandon coalescing by content-sniffing entirely and instead rely on `requestAnimationFrame`/rAF-based batching at the xterm.js `write()` level (xterm.js already internally batches writes efficiently; a bespoke drop-on-replace coalescer duplicates work xterm.js's own renderer scheduling already does more safely).
5. **Any snapshot/live-stream dual path (cold-start `capture-pane` vs. live control-mode stream) must have a single arbitration point, not two independent writers racing to the same terminal buffer.** `docs/tasks/terminal-jank.md`'s own "Bug 4" entry already names this as an open, only-partially-mitigated risk — the eventual fix here should not reintroduce or deepen that race while chasing the throttler fix.
6. **Add a regression test that asserts row-erase coverage is monotonic/complete across coalesced frames**, not just that "some output eventually reaches xterm.js" — the existing `EscapeSequenceParser.test.ts` / `TerminalStreamManager.test.ts` suites test partial-sequence buffering and throttling *timing*, but (based on file names reviewed) do not appear to assert that a *dropped* pending redraw's erased-row set is a subset of the replacing frame's erased-row set. Phase 5 implementation should add exactly that test against `RedrawThrottler`.

## Sources

- [xterm.js #943 — clear() is not correctly clearing the buffers](https://github.com/xtermjs/xterm.js/issues/943)
- [xterm.js #1395 — proposal: switch to DEC/ANSI compliant escape sequence parser](https://github.com/xtermjs/xterm.js/issues/1395)
- [xterm.js #145 — broken escape sequence parser states](https://github.com/xtermjs/xterm.js/issues/145)
- [JetBrains/jediterm #118 — Erase in Line CSI sequence does not erase line content beyond the terminal width](https://github.com/JetBrains/jediterm/issues/118)
- Repo commit `0ac0ca1dad9f102d7072857c87714fb2d1905e05` (PR #163) — CSI/OSC/DCS terminator fix in `stripAnsi.ts`
- Repo commit `dc71b828a6d6a61883cd12b7c666e836eda97c90` (PR #139) — removal of ED2+ED3 stripping regex from `EscapeSequenceParser.ts`
- `web-app/src/lib/terminal/EscapeSequenceParser.ts`, `web-app/src/lib/terminal/TerminalStreamManager.ts`, `web-app/src/lib/terminal/stripAnsi.ts`, `web-app/src/components/history/HistoryCardPreview.tsx`
- `docs/tasks/terminal-jank.md` (Story 1, Story 2, "Known Issues" Bug 4)
