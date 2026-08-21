# Research Synthesis: Stapler Squad Pain Points

**Date**: 2026-04-16
**Sources**: findings-stack.md, findings-features.md, findings-architecture.md, findings-pitfalls.md

---

## Decision Required

For each of seven pain points, choose the implementation strategy and accept its costs before writing ADRs.

---

## Context

Stapler Squad is a personal daily-driver app for managing concurrent Claude Code sessions. The user experiences friction in seven areas: (1) large session load time due to full scrollback on attach, (2) mobile touch scroll inside xterm.js is broken, (3) mobile layout/keyboard UX, (4) no branch autocomplete in the session creation dialog, (5) no frontend observability to diagnose slowness, (6) no quick rename/retag for sessions, and (7) no bulk session actions. All fixes ship as separate PRs. The codebase already has `CircularBuffer.GetLastN(n)` + `GetRange(fromSeq, limit)`, the `AutocompleteInput` component, OTel on the Go backend, and tmux-native scrollback (xterm `scrollback: 0`).

---

## Options Considered

| Pain Point | Option A | Option B | Key Trade-off |
|---|---|---|---|
| **1. Lazy scrollback** | Separate unary `GetScrollback` RPC + tail-first streaming | Extend bidirectional `StreamTerminal` stream | Simplicity (A) vs. protocol elegance (B) |
| **2. Mobile touch scroll** | App-level touch interception + `terminal.scrollLines()` | CSS `overscroll-behavior: contain` only | Works on iOS 15 (A) vs. CSS-only simplicity (B) |
| **3. Mobile layout/keyboard** | `visualViewport.onresize` + dvh units | Fixed-height layout with JS recalculation | Robustness across iOS versions (A) vs. simplicity (B) |
| **4. Branch autocomplete** | New `ListBranches` unary RPC + existing `AutocompleteInput` | Extend path-completion endpoint | Clean separation (A) vs. reuse (B) |
| **5. Frontend observability** | Lightweight custom JSON endpoint (`/api/telemetry`) | OTel JS SDK (`@opentelemetry/sdk-web`) | Ship fast (A) vs. trace correlation with backend (B) |
| **6. Quick rename/retag** | Inline hidden `<input>` toggle, no modal | Existing tag editor modal, improve discoverability | Best UX (A) vs. lowest risk (B) |
| **7. Bulk actions** | Checkbox multi-select + bulk action bar | Right-click context menu | Standard pattern (A) vs. low real estate cost (B) |

---

## Dominant Trade-off

The fundamental tension across all seven areas: **ship something useful fast vs. build it the right way once**.

- Lazy scrollback done right (cursor-safe prepend, sequence tracking) takes 3–4 weeks. Done fast (just send last 500 lines on attach, no scroll-up load) takes 3 days and fixes 80% of the pain.
- Mobile touch scroll has **no CSS-only solution** — xterm.js issue #5377 (July 2025) is open and unresolved upstream. Application-level interception is required.
- OTel JS SDK is ~60 KB gzipped and requires dynamic import. A custom JSON endpoint gets you actionable data in 2 days.
- Branch autocomplete is a clean, low-risk 1-week feature.

---

## Recommendations

### 1. Lazy Scrollback — Phase the delivery

**Choose**: Two-phase approach
- **Phase 1 (fast, 3 days):** Change `StreamTerminal` to send only the last 500 lines on attach (using existing `CircularBuffer.GetLastN(500)`). Cap at 500 lines. Users with large sessions see immediate improvement. No scroll-up loading.
- **Phase 2 (robust, 3 weeks):** Add `GetScrollback(sessionId, fromSeq, limit)` unary RPC. On scroll-to-top trigger in xterm, fetch 500 older lines and prepend with cursor-state preservation.

**Accept these costs**: Phase 1 discards history older than 500 lines for the browser view (backend still holds all of it). Phase 2 has cursor-corruption risk during prepend — must be carefully tested.

**Reject**: Extending the bidirectional stream — adds message complexity to an already-complex stream and is harder to test.

---

### 2. Mobile Touch Scroll — Application-level interception required

**Choose**: `touchstart`/`touchmove` event listeners on the xterm container + manual `terminal.scrollLines(delta)` calls, combined with CSS `overscroll-behavior: contain` (for iOS 16+ as a belt-and-suspenders).

**Because**: xterm.js does not support touch scroll natively (confirmed open issue #5377, July 2025). CSS-only (`overscroll-behavior`) was not added to Safari until iOS 16, leaving iOS 15 users broken. The only working approach is app-level touch interception.

**Accept these costs**: Uses xterm.js private `_core.viewport.scrollLines()` API — pin xterm.js version. Momentum scroll requires manual implementation (~100 lines).

**Reject**: CSS-only fix — doesn't work on iOS 15, doesn't fix the underlying issue.

---

### 3. Mobile Layout / Keyboard — `visualViewport` with dvh fallback

**Choose**: `window.visualViewport.addEventListener('resize', ...)` with 300ms debounce for xterm `fit()` on keyboard show/hide. Use `dvh` (dynamic viewport height) CSS units for terminal height instead of `100vh`.

**Because**: iOS Safari does not trigger a `resize` event on the window when the keyboard appears — only `visualViewport.resize` works. However, Safari 15 doesn't always fire `visualViewport.resize` for keyboard events either. `dvh` units are the most robust CSS fallback (supported iOS 15.4+).

**Accept these costs**: `dvh` is not supported below iOS 15.4. Users on older iOS will still see layout jank — acceptable given the audience.

**Reject**: Fixed-height JS calculation — brittle, breaks on device rotation.

---

### 4. Branch Autocomplete — New `ListBranches` RPC

**Choose**: New `ListBranches(repoPath: string, includeRemote: bool)` unary ConnectRPC endpoint. Shell out to `git for-each-ref refs/heads --format='%(refname:short)'` with a 2s timeout and 5-minute in-memory cache. Wire into `SessionWizard.tsx` via `AutocompleteInput` (already exists).

**Because**: Clean separation of concerns. Fast on typical repos (p90 ~75ms for 100 branches verified). go-git's `Branches()` correctly scopes to local branches, so no remote-tracking leakage. The existing `AutocompleteInput` handles keyboard nav and async loading.

**Accept these costs**: New proto RPC to maintain. Shell exec requires context timeout handling.

**Reject**: Extending path-completion endpoint — conflates two different concerns.

---

### 5. Frontend Observability — Custom JSON first, OTel later

**Choose**: Phase 1: lightweight custom telemetry endpoint (`POST /api/telemetry` with JSON body `{action, duration_ms, session_id, timestamp}`). Instrument: session attach latency, first terminal output time, RPC round-trip, page navigation. Store in Go backend stdout / existing structured logging.

Phase 2 (if trace correlation with backend is needed): Migrate to `@opentelemetry/sdk-web` loaded via dynamic import (`await import(...)`) to avoid blocking first paint.

**Because**: The full OTel JS SDK is ~60 KB gzipped (verified). Dynamic import is mandatory. A custom JSON endpoint gets actionable data in 2 days and answers the core question ("which clicks are slow?") without the bundle risk. OTel is the right end state but not the right first step for a single-user tool.

**Accept these costs**: Custom endpoint doesn't correlate with backend OTel traces. When migrating to Phase 2, the endpoint changes.

**Reject**: Full OTel JS SDK in Phase 1 — bundle size risk, setup complexity, React 19 initialization gotchas.

---

### 6. Quick Rename/Retag — Inline hidden input

**Choose**: On session title click → swap to a hidden `<input>` (not contenteditable). Blur/Enter saves; Esc cancels. Optimistic UI update; roll back on error.

**Because**: Contenteditable is fragile (undo stack, paste handling, drag). The hidden input pattern is safe, accessible, and matches the GitHub/Linear UX that users expect. No modal needed.

**Accept these costs**: Requires careful handling of accidental blur-to-save. Show "Saving..." indicator.

**Reject**: Improving discoverability of the existing tag editor modal — the modal still requires too many clicks.

---

### 7. Bulk Actions — Checkbox multi-select

**Choose**: Checkbox appears on session card hover (or long-press on mobile). Selecting any card reveals a floating bulk action bar at the bottom: Pause, Stop, Delete, Add Tag. Standard pattern (GitHub issues, Linear tasks).

**Because**: The pattern is universally understood. `BulkActions.tsx` already exists in the codebase — this is likely already partially wired.

**Accept these costs**: Hover-to-show-checkbox is not discoverable on mobile; long-press alternative required.

---

## Implementation Order (by impact/effort ratio)

| Priority | Feature | Effort | Impact |
|---|---|---|---|
| 1 | Branch autocomplete | 1 week | High — daily friction |
| 2 | Lazy scrollback Phase 1 (tail-only, no scroll-up) | 3 days | High — large sessions load fast |
| 3 | Mobile touch scroll fix | 1 week | High — mobile is unusable today |
| 4 | Mobile layout / keyboard | 3 days | High — complements touch fix |
| 5 | Frontend observability (custom JSON) | 2 days | Medium — enables future diagnosis |
| 6 | Quick rename/retag (inline input) | 3 days | Medium — daily friction |
| 7 | Bulk actions | 1 week | Lower — nice to have |
| 8 | Lazy scrollback Phase 2 (scroll-up loading) | 3 weeks | High — but Phase 1 covers 80% |

---

## Open Questions Before Committing

- [ ] Does `CircularBuffer` persist sequence numbers across server restarts? If not, Phase 2 lazy scrollback needs a "full redraw" flag on reconnect to avoid gaps.
- [ ] Is xterm.js `_core.viewport.scrollLines()` the correct private API for v5+? Or is `terminal.scrollLines()` public? — check `node_modules/@xterm/xterm/src/` before implementing touch scroll.
- [ ] Does `TerminalDiff` / MOSH-style state sync affect how initial scrollback can be sent on attach? Need to confirm with the existing `StreamTerminal` handler logic.
- [ ] What is the current xterm.js version in `package.json`? v5 may have a public `scrollLines` API.

If the xterm private API question cannot be answered from source, a 2-hour spike should confirm the correct touch scroll approach before writing the ADR.

---

## Sources

- [findings-stack.md](./findings-stack.md) — library options, bundle sizes, git API performance
- [findings-features.md](./findings-features.md) — comparable tool patterns (VS Code, GitHub, Linear, ttyd)
- [findings-architecture.md](./findings-architecture.md) — end-to-end integration designs
- [findings-pitfalls.md](./findings-pitfalls.md) — failure modes, mitigation strategies
- [xtermjs/xterm.js#5377](https://github.com/xtermjs/xterm.js/issues/5377) — confirmed: mobile touch is unresolved upstream
- [caniuse.com/css-overscroll-behavior](https://caniuse.com/css-overscroll-behavior) — confirmed: Safari 16+ only
- [signoz.io/blog/reduce-opentelemetry-bundle-size](https://signoz.io/blog/reduce-opentelemetry-bundle-size-for-browser-frontend) — confirmed: ~60 KB gzipped
- [earthly/earthly#3752](https://github.com/earthly/earthly/issues/3752) — confirmed: `--contains HEAD` is the perf cliff, not plain `for-each-ref`
