# Build vs. Buy — terminal-resize-fit-loop

Scope: the ResizeObserver → `FitAddon.fit()` → `terminal.onResize` → resize-RPC feedback loop
in `web-app/src/components/sessions/XtermTerminal.tsx`. Three pieces needed:

1. Integer cols/rows gate before calling `fit()`
2. Value-based dedup before sending a resize RPC
3. Oscillation-burst detector (same cols/rows recurring ≥3× within a rolling 2000ms window) →
   triggers WebGL→canvas renderer fallback

---

## 1. Existing OSS library or framework

**Checked**: `web-app/package.json` dependencies (full list read), `@xterm/addon-fit@^0.11.0`
source behavior, and whether any general-purpose "ResizeObserver debounce" / rate-limiter
package is already a transitive/available dependency.

### 1a. `FitAddon` itself

`FitAddon.fit()` is a pure, synchronous "measure container → compute cols/rows → call
`terminal.resize()`" function. It has **no built-in debounce, convergence check, or dedup** —
it will happily call `terminal.resize()` (and thus fire `terminal.onResize`) every time it's
invoked, even with unchanged dimensions in some versions/edge cases. Confirmed by reading how
`XtermTerminal.tsx` already has to hand-roll a 150ms debounce around `resizeObserver` callbacks
(lines ~453-508) precisely because `FitAddon` provides no debounce of its own. Nothing to buy
here — the addon is intentionally minimal and this is expected usage.

- **Verdict**: N/A (not a library gap — `FitAddon` was never meant to own this policy).

### 1b. Newer/alternative xterm.js addon for resize-loop guarding

There is no official `@xterm/addon-*` that solves ResizeObserver feedback loops — the addon
ecosystem (`addon-fit`, `addon-webgl`, `addon-canvas`, `addon-search`, `addon-serialize`,
`addon-web-links`, `addon-unicode11`, `addon-ligatures`, `addon-image`, `addon-clipboard`) is
already fully enumerated in xterm.js's own monorepo and none of them touch resize convergence —
that's explicitly left to the embedding application, since only the app knows its own transport
(RPC in this case) and backend cost of a resize call. A scan of the broader npm ecosystem for
"xterm resize loop" / "terminal resize debounce" turns up no maintained third-party addon either
— this is a known but narrow enough problem that nobody has packaged it.

- **Verdict**: Not recommended (doesn't exist in usable form).

### 1c. General-purpose "ResizeObserver debounce" / rate-limiter npm package

Candidates considered: `lodash.debounce`/`lodash.throttle`, `es-toolkit`, `rxjs`
(`bufferTime`/`auditTime`), `p-debounce`, `use-debounce`, `element-resize-detector`,
`rate-limiter-flexible`, `bottleneck`, `limiter`. **None of these are currently a dependency** —
`web-app/package.json` has no debounce/throttle/rate-limit utility of any kind (grepped the full
dependency list). Adopting one purely for this feature means:
- A new dependency (bundle size, supply-chain surface, version-pin maintenance) for a problem
  that's ~10 lines of `Date.now()` array bookkeeping.
- Most of these libraries solve "call this function at most once per N ms" (debounce/throttle),
  which is a *different* problem than "detect that the same value recurred 3 times in a rolling
  window" — a burst/oscillation detector isn't throttling, it's pattern detection over a value
  stream. None of the debounce/throttle libraries above expose that primitive directly; you'd
  still be writing the counting logic on top, just with an unnecessary dependency underneath it.
- The existing 150ms flat debounce in `XtermTerminal.tsx` (ADR-driven, see §4) is already
  hand-rolled with `setTimeout` + refs, matching the codebase's existing convention of not
  reaching for a debounce library for terminal resize logic.

- **Verdict**: Not recommended.

### 1d. Is a library even proportionate given the algorithm size?

No. All three pieces are:
1. Integer gate: `Number.isInteger(cols) && Number.isInteger(rows)` — one line.
2. Value dedup: compare `{cols, rows}` against a `lastSentRef` before firing the RPC — a few
   lines, same shape as dedup patterns already used elsewhere in this codebase for RPC calls.
3. Burst detector: push `{cols, rows, ts}` onto a ref array, prune entries older than 2000ms,
   count entries matching the current `{cols, rows}`, trip if count ≥ 3 — roughly 20-30 lines
   including tests.

This is below the threshold where a dependency's overhead (audit burden, version drift, an API
you don't fully control) is worth it. Writing it inline keeps the logic colocated with the
`XtermTerminal.tsx` resize state it must read (`fitAddonRef`, `isFittingRef`, WebGL addon
handle), which a generic library cannot know about.

- **Verdict**: Build (Recommended).

---

## 2. SaaS / managed API

N/A. This is entirely client-side rendering/state logic (DOM measurement, xterm.js addon
lifecycle, in-memory timestamp bookkeeping) with no external network dependency, no data that
needs centralized storage, and no reason a hosted service could plausibly help. Excluded from
further evaluation.

---

## 3. LLM-generated implementation vs. battle-tested library (burst detector specifically)

The burst/oscillation detector is the one piece with real algorithmic risk: off-by-one on the
rolling-window boundary (`>=` vs `>` at exactly 2000ms), timestamp pruning order (prune-then-push
vs push-then-prune), and correctly resetting the counter after a fallback trips (avoid
re-triggering the fallback repeatedly once already in canvas mode).

**Well-known pattern to model after**: this is a textbook **sliding-window (log-based) rate
limiter** — the same shape as the "sliding window log" algorithm described in rate-limiting
literature (Stripe's/Cloudflare's public write-ups on sliding-window counters use this exact
structure: an array/deque of timestamps, prune everything `<= now - windowMs`, then check
`array.length >= threshold`). This is a well-established, easy-to-verify pattern — not something
that benefits from pulling in a full rate-limiter package (`rate-limiter-flexible`, `bottleneck`,
`limiter`) which are designed for server-side request throttling (with Redis backends, queuing,
retry semantics) — massive overkill for an in-memory client array with a 2000ms window and a
handful of entries.

**Recommendation**: implement from scratch using the sliding-window-log pattern (array of
timestamps + prune + count), not a library — but write it as the *first* thing covered by unit
tests given the off-by-one/boundary risk, and explicitly test:
- exactly 3 recurrences within window → trips
- 3 recurrences spanning >2000ms (oldest one ages out) → does not trip
- entries exactly at the 2000ms boundary (`now - ts === 2000`) → pick and document one
  inclusive/exclusive convention, test it explicitly
- fallback only trips once (doesn't re-trigger every subsequent matching resize after already
  in canvas mode)

**Existing rolling-window pattern in this codebase to reuse as a style reference**:
`web-app/src/lib/hooks/useTerminalMetrics.ts` — grepped for "rolling/window/burst/sample" and
found a related buffering comment ("Flushes immediately if buffer exceeds 4KB (prevents lag on
large bursts)") but it is a *size*-based buffer flush, not a *time*-windowed counter — not
directly reusable code, but confirms the codebase already has precedent for burst-related
buffering logic living inline in a hook rather than as a shared utility. No existing
sliding-window/rate-limit utility function exists in `web-app/src` to extend (grepped
`rolling window|sliding window|burst|rate.?limit` across `web-app/src` — only doc comments and
unrelated UI copy like "sub-status chip" hits, no algorithmic matches).

- **Verdict**: Build from scratch, modeled on the sliding-window-log pattern (Recommended).
  Do not adopt a rate-limiter package (Not recommended — wrong problem shape, needless
  dependency).

---

## 4. Fork or adapt — prior art in this exact codebase

**Found substantial prior art.** `project_plans/terminal-robustness/research/pitfalls.md`
section "2. xterm.js Resize Race Conditions" (lines 64-113) already documented this *exact*
convergence problem during an earlier SDD cycle, including:

```
ResizeObserver fires → setTimeout(debounce) → rAF → rAF → fitAddon.fit()
  → terminal.onResize fires → handleTerminalResize() → resize() RPC
    → server: ioctl SIGWINCH → tmux reflows → capture-pane → snapshot sent
      → client: onOutput callback → terminal.write(snapshot)
```

Five races were catalogued (double-fit-on-mount, adaptive-debounce-too-early,
snapshot-before-fit-stable, ResizeObserver+visualViewport double-fire, snapshot-before-tmux-
quiescence). The concrete fixes from that research were **already implemented**:
- A flat 150ms debounce replaced the old adaptive 10ms/250ms debounce (see
  `XtermTerminal.tsx` ~line 472: *"Flat 150ms debounce (R1.2): ensures tmux has processed the
  previous SIGWINCH before FitAddon measures container and fires terminal.onResize"*).
- The duplicate mount-time `fit()` + `setTimeout` was removed (comment at ~line 397: *"the extra
  setTimeout caused a second terminal.onResize, triggering a duplicate [resize]"*).
- WebGL→canvas fallback scaffolding already exists: `webglAddon.onContextLoss(() => {...
  webglAddon.dispose() ...})` at `XtermTerminal.tsx` lines 269-273 — currently only wired to the
  browser's native WebGL context-loss event, **not** to an oscillation/resize-burst signal. This
  is the natural integration point for piece #3 (burst detector → call the same disposal path).
- Backend-side debounce was called out as a requirement too (R1.5: "coalesce identical cols/rows
  within 50 ms") — worth confirming server-side (`server/`) still honors this; out of scope for
  this client-side research doc but flagged for the implementation plan.

What was **not** previously implemented** and is the actual gap this project closes:
- No integer cols/rows gate before `fit()` (not mentioned in the prior pitfalls doc).
- No *value-based dedup* immediately before the resize RPC send (the 150ms debounce reduces
  frequency but doesn't dedup by value — two different debounced fits at the same final cols/rows
  could still both fire RPCs).
- No rolling-window burst *counter* — the existing fixes reduce race likelihood but don't detect
  or recover from a live oscillation loop once one starts (e.g., a container whose fit-computed
  size itself changes the container size, such as a scrollbar appearing/disappearing at the
  boundary — a classic ResizeObserver feedback loop that a flat debounce alone cannot fully
  prevent, only slow down).

`project_plans/terminal-jank/decisions/ADR-003-cold-start-quiescence.md` is related (tmux-side
quiescence after resize) but addresses server-side capture timing, not client-side fit
convergence — informative context, not directly forkable code.

- **Verdict**: Adapt, not fork wholesale (Recommended). Reuse the existing 150ms debounce and
  `webglAddon.onContextLoss` disposal path as integration points; do not re-architect the
  debounce that was already fixed per ADR/R1.2. Add the three new pieces (gate, dedup, burst
  counter) as incremental additions to the same `resizeObserver` callback block
  (`XtermTerminal.tsx` ~lines 453-508), reusing the `webglAddon` disposal call already present
  at lines 269-273 as the fallback trigger target.

---

## Summary Table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| `FitAddon` built-in guard | — | Doesn't exist; addon is deliberately minimal | N/A |
| Alternative xterm.js addon | — | No such addon exists in official or community ecosystem | Not recommended |
| Generic debounce/rate-limit npm package | Battle-tested edge-case handling | New dependency for ~10-30 lines; wrong primitive (throttle ≠ burst-count); no such dep currently installed | Not recommended |
| Build inline (gate + dedup + burst counter) | Trivial size, colocated with existing resize refs/state, matches codebase convention of no debounce lib | Requires careful unit tests on window-boundary logic | **Recommended** |
| SaaS/managed API | — | Not applicable — client-side only | N/A |
| Model burst detector on sliding-window-log rate-limiter pattern | Well-understood, easy to verify correctness against a known algorithm shape | Still hand-written, not an actual library | **Recommended** (as a design reference, not a dependency) |
| Adapt existing `terminal-robustness` pitfalls research + `webglAddon.onContextLoss` disposal path | Prior art already identifies races 1-5 and fixes 3 of them; fallback disposal code already exists to hook into | Must avoid regressing the already-fixed 150ms debounce | **Recommended** |
