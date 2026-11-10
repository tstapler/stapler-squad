# ADR-002: Live-send composition with bounded, atomic rewrites

## Status
Accepted (2026-11-06)

## Context
`research/ux.md` recommended preview-then-commit; `research/architecture.md` recommended live-send with rewrites bounded to bytes the layer itself sent. Owner decision: live-send, autocorrect and suggestions stay ON. The risks are a rewrite eating text the user typed before the composition, and a rewrite being split by `PASTE_CHUNK_SIZE = 512` chunking (`useTerminalFlowControl.ts:238`) so DELs and text reorder.

## Decision
1. Characters echo as the composition changes (matches hardware feel; one-frame latency SLO).
2. A rewrite is computed only from `sent` (exact string this layer emitted for the current composition region) to the new region text: keep the common prefix, emit one `\x7f` per removed grapheme, then the new tail. Never more DELs than graphemes in `sent`; never any DEL when `sent` is empty. `sent` is scoped to the current delivery epoch and session: after a transport boundary (disconnect, reconnect, a dropped or failed send) the region is deferred and commits only an insert-only tail with no DELs, and after a session change, pool reuse or queue discard it is abandoned with zero bytes (`controller.invalidate(reason)`; ADR-004, Story 1.2.4). Backspace right after a commit (textarea reset to `""`) must still emit one `\x7f`: sentinel character or keydown-8 fallback, chosen from the Task 0.1.2b/0.1.3b spike (pre-mortem P1-3). Hard cap: `MAX_REGION = 64` graphemes. Exceeding the cap, a newline in the region, or any selection change inside the region switches that region to deferred (commit on `compositionend`) and records an anomaly.
3. A rewrite (DELs + text) is emitted as ONE `onData` string and delivered as ONE `sendInput` call that bypasses paste chunking when its UTF-8 length is below a `ATOMIC_EDIT_MAX_BYTES = 1024` ceiling (larger payloads are classified as paste/dictation and use normal chunking). Chunk boundaries never split inside a UTF-8 code point.
4. `insertFromPaste` and multi-line `insertText` are treated as paste; the renderer-side bracketed-paste wrapping decision is out of the layer (it emits text; Phase 3 task checks `DECSET 2004` state availability and records the finding).
5. Autocorrect and suggestions stay ON: the textarea sets `autocapitalize="none"`, `autocomplete="off"` and `enterkeyhint="send"` only. It sets neither `autocorrect="off"` nor `spellcheck="false"`, so the Gboard suggestion strip keeps working. The attribute set lives in `policy.ts` as data so a per-keyboard profile can change it later. Secrets at echo-off prompts are a known limitation: see ADR-008. A rewrite is also the unit the disconnected queue holds (ADR-004). Armed Ctrl/Alt during a live composition flushes the composition first and applies only to the next committed single character. Model B (preview-then-commit) was reconsidered against `design/ux.md` S2 and stays rejected: live atomic rewrite, no preview chrome.

## Alternatives rejected
- Preview-then-commit default: needs a cursor-anchored overlay from each renderer and delays echo. Kept as the fallback region mode, and the compose bar (Story 4.2) covers the long-prompt use case.
- Timing-window dedupe (Codeman 50 ms): non-deterministic, untestable.

## Consequences
Wrong rewrites inside a TUI input box with its own wrap model remain possible; bounded by region and cap, counted via `cm.ime_anomaly`, killable via the flag. Validated by the replay corpus and the owner phone acceptance.

## Amendment 2026-11-08 (triad review): flicker criterion
"One atomic rewrite" is now measurable (plan Story 1.2.5): the rewrite is one `onData` call emitted within one animation frame of the `beforeinput`, and a per-animation-frame sampler across an autocorrect sees at most one intermediate state and never a blank frame or shortened prompt text. A DeferredCommit (over 64 graphemes or a newline) shows a polite "Typing... (N characters waiting)" indicator after 300 ms, because the pane is intentionally silent until the commit. A device-observed flicker the sampler misses is recorded and escalates to a preview/echo follow-up, not a silent change to the decision.
