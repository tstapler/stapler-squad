# Research: Build vs. Buy — Terminal Input Batching

Agent 6, SDD Phase 2. Scope: `sendInput()` in `web-app/src/lib/hooks/useTerminalFlowControl.ts`
needs a ~30-60 line accumulate-and-flush-on-timer-or-byte-threshold buffer in front of its
existing chunking path. Evaluated four sourcing options.

## 1. Existing OSS library

**Checked**: `web-app/package.json` dependencies/devDependencies (VERIFIED, read directly) —
no `lodash`, no `lodash.debounce`, no `use-debounce`, no `rxjs`, no `p-debounce`/`p-throttle`
anywhere in the manifest.

**Checked**: existing hand-written debounce precedent already in this codebase —
`web-app/src/lib/hooks/useDebounce.ts` (`useDebounce<T>(value, delay)` and
`useDebouncedCallback`), consumed by `useHistoryFullTextSearch.ts` and
`components/sessions/useVisibilityResync.ts`. This repo already solved "debounce a value/
callback" in ~50 lines of hand-written hook code rather than installing a package — direct
precedent for the same choice here. Note this existing hook is *not* reusable as-is for this
task: it's pure time-based debounce (reset-timer-on-every-call), with no byte-threshold
early-flush and no accumulation/concatenation semantics — AC3 (32-byte early flush) and AC7
(ordered byte accumulation) require different logic, not just a smaller delay.

**Ladder** (stdlib → native → already-installed dep → new dep → hand-write): there is no
stdlib/native browser API for size-triggered debouncing (`setTimeout`/`requestIdleCallback`
give time-only triggers). Nothing already-installed provides byte-threshold early-flush
semantics — `lodash.debounce`'s `maxWait` is time-based, not size-based (see §3). No package
search turned up an established micro-library specifically for "debounce with byte/size-based
early flush" (this is a narrow enough shape that it's normally hand-rolled inline wherever
needed, as herdr-web itself demonstrates — see §4). Landing on hand-write is not a gap in the
search, it's evidence the ladder's early rungs are genuinely empty for this specific
requirement.

- Pros: n/a — no viable candidate found.
- Cons: would add a new dependency (bundle size, supply-chain surface, `make registry-generate`/
  lockfile churn) to solve ~15 lines of accumulation logic that composes with code
  (`PASTE_CHUNK_SIZE` chunking) that already lives inline in this same function.
- **Verdict: Not recommended.** Squarely in "just write it" territory — matches this repo's
  own `useDebounce.ts` precedent and the size of the change (AC's own framing: "~30-60 line
  addition to one existing hook file, not a new subsystem").

## 2. SaaS / managed API

Not applicable. This is a purely client-side, in-tab buffering optimization over bytes the
user is actively typing into an open WebSocket connection — no vendor or managed service can
sit between a keystroke and the buffer that holds it before the browser tab flushes it
outbound; the entire operation happens before anything leaves the client.

- **Verdict: Not applicable.**

## 3. LLM-generated implementation vs. battle-tested library

Checked whether `lodash.debounce` (the most likely off-the-shelf candidate, even though not
currently installed) could satisfy the byte-threshold requirement on its own:

`lodash.debounce(func, wait, { maxWait })` — `maxWait` bounds the *elapsed time* since the
first call in the current debounce window; it is unrelated to payload size. Lodash's debounce
has no concept of the *content* being passed to it (it debounces invocations of a function,
not bytes of accumulated data) — it cannot implement AC3 ("flush immediately once the
buffered byte count reaches a fixed threshold") without the caller separately: (a) doing its
own byte-counting and concatenation outside lodash, and (b) calling `.flush()` manually when
that count is reached. At that point lodash is only supplying the timer-reset half of the
logic — the harder, more bug-prone half (byte accumulation, ordering, flush-on-unmount,
composing with the existing chunk-splitting path) still has to be hand-written regardless.
Adopting lodash here would add a dependency to save perhaps 5-10 lines of "reset a timer" code
while not touching the AC that actually motivates this item (AC3's early-flush).

Correctness risk of hand-writing ~40 lines: **low**. This is a well-understood, easily
testable pattern — accumulate into a buffer, flush on `setTimeout` OR immediately once
`bytes >= threshold`, clear on flush, flush pending on unmount. AC8 already requires unit
tests for exactly the four states that matter (default-off passthrough, coalescing,
early-flush, flush-on-unmount), which is sufficient coverage for a pure-function buffer with
no external state beyond a `useRef`. The existing `sendChunk`/`PASTE_CHUNK_SIZE` code in the
same file (lines ~139-195) is direct in-repo evidence this team already hand-writes and tests
comparable byte-buffer-with-timer logic correctly.

- **Verdict: Hand-write.** No library — installed or not — fully covers AC3; the byte-counting
  and accumulation logic has to be written by hand either way, so partially adopting lodash
  just for its timer half adds a dependency without removing the risky part of the work.

## 4. Fork or adapt (herdr-web prior art)

`herdr-web` is a real, public, MIT-licensed OSS project: https://github.com/kcosr/herdr-web
(fetched directly via `gh api repos/kcosr/herdr-web/contents/...` — VERIFIED, not just
described secondhand). The relevant file is
`web/src/terminalInputTransport.ts` (48 lines), with matching tests in
`web/src/terminalInputTransport.test.ts`.

Actual source (fetched in full):

```ts
export const DEFAULT_TERMINAL_INPUT_BATCH_DELAY_MS = 0;
export const TERMINAL_INPUT_BATCH_MAX_BYTES = 32;
export const TERMINAL_INPUT_BATCH_DELAY_OPTIONS_MS = [0, 32, 64, 128, 256] as const;

export function shouldSendTerminalInputImmediately(bytes: number, delayMs: number): boolean {
  return delayMs <= 0 || bytes >= TERMINAL_INPUT_BATCH_MAX_BYTES;
}

export function appendTerminalInputBatch(batch, data: string, bytes: number) {
  const next = { parts: [...batch.parts, data], bytes: batch.bytes + bytes };
  return { batch: next, shouldFlush: next.bytes >= TERMINAL_INPUT_BATCH_MAX_BYTES };
}

export function drainTerminalInputBatch(batch) {
  return { data: batch.parts.length === 0 ? null : batch.parts.join(""), batch: emptyTerminalInputBatch() };
}
```

This **confirms requirements.md's citations exactly**: default-off (`0`), 32-byte early-flush
threshold, and the `[0, 32, 64, 128, 256]` option set referenced in AC6 all match the real
source precisely — the requirements doc's prior-art description was accurate, not
approximated.

Notably, herdr-web's own module has **no imports and no dependencies** — it's pure functions
over plain objects (`{ parts: string[]; bytes: number }`), framework-agnostic, and only wired
into React state in the consuming component. This is exactly the shape that belongs
hand-adapted inline into `useTerminalFlowControl.ts` (or a small colocated helper file) rather
than imported as a package — there is no package to import (herdr-web is an application repo,
not a published npm library), and the logic is small and self-contained enough that vendoring
it as a git dependency or submodule would be pure overhead.

- Pros: proven design (already shipped, already tested upstream), exact constants and option
  set to reuse, saves the design step of inventing the accumulate/flush state shape from
  scratch.
- Cons: license note — MIT, permissive, safe to adapt with attribution in a comment if desired;
  not a hard blocker either way. Structural differences from this repo's code must be
  accounted for in Phase 3 planning: herdr-web batches *strings* (`parts: string[]`, joined
  with `.join("")`), while `sendInput` here already works in **bytes**
  (`TextEncoder().encode(input)` → `Uint8Array`, per the existing `PASTE_CHUNK_SIZE` chunker at
  `useTerminalFlowControl.ts:145-146`) — the adapted version should accumulate `Uint8Array`
  chunks (or byte length pre-computed once) rather than strings, to compose cleanly with the
  existing chunk-splitting path per AC4, and to avoid a second `TextEncoder` pass.
- **Verdict: Recommended — adapt inline, not fork/vendor.** Treat herdr-web's
  `terminalInputTransport.ts` as a design reference for Phase 3: port the same three pure
  functions (`shouldSendTerminalInputImmediately`, `appendTerminalInputBatch`,
  `drainTerminalInputBatch`) and the same constants (`32` byte threshold,
  `[0, 32, 64, 128, 256]` ms options), translated from string-batching to
  byte-batching to match this repo's existing `Uint8Array`-based chunking path. No import, no
  submodule, no new dependency — just reference the upstream file directly in the plan/PR
  description as the source of the design, consistent with the repo's citation norms
  (`.claude/CLAUDE.md`'s evidence rules).

## Overall recommendation for Phase 3

**Hand-write**, directly inside `useTerminalFlowControl.ts` (or a small colocated pure-function
helper module beside it, mirroring herdr-web's own file split), adapting herdr-web's
`terminalInputTransport.ts` design (confirmed accurate against the live source) from
string-batching to byte-batching. No new npm dependency is justified — this repo has direct
precedent (`useDebounce.ts`) for hand-writing debounce-shaped utilities rather than installing
one, and no OSS library (installed or not) covers the byte-threshold early-flush requirement
that is central to this feature; `lodash.debounce`'s `maxWait` is time-only and would still
require hand-written byte accounting alongside it, eliminating any adoption benefit.
