# Findings: Architecture

## Summary

Stapler Squad's codebase has significant prior infrastructure for all four must-have features: the circular buffer already has `GetLastN(n)` and `GetRange(fromSeq, limit)` with sequence numbers; proto definitions include `ScrollbackRequest`/`ScrollbackResponse` messages (not yet wired); the `AutocompleteInput` component exists; and the frontend terminal wrapper (`XtermTerminal.tsx`) explicitly sets `scrollback: 0` (tmux owns history). The primary work is wiring existing plumbing together, not building from scratch.

The dominant trade-off across all four areas is **simplicity vs. protocol overhead**: using the existing bidirectional `StreamTerminal` stream for scrollback history is elegant but complex; a separate unary `GetScrollback` RPC is simpler and more testable. Similarly, using the OTel JS SDK unifies with the backend's trace model but adds bundle size and setup complexity vs. a lightweight custom JSON endpoint.

## Options Surveyed

(Based on reading: `session/scrollback/buffer.go`, `session/scrollback/manager.go`, `web-app/src/components/sessions/XtermTerminal.tsx`, `web-app/src/components/sessions/SessionWizard.tsx`, `web-app/src/components/ui/AutocompleteInput.tsx`, and the proto definitions directory.)

## Trade-off Matrix

| Concern | Stream Extension | Separate Unary RPC | OTel HTTP Proxy | Custom JSON Endpoint | touch-action: none | overscroll-behavior |
|---|---|---|---|---|---|---|
| Lazy scrollback latency | Low (in-stream) | Medium (new RPC) | N/A | N/A | N/A | N/A |
| Bandwidth efficiency | High (delta) | Medium (full) | N/A | N/A | N/A | N/A |
| Backend complexity | High | Low | Low | None | None | None |
| Browser complexity | Medium | Low | Medium | Low | High | Low |
| Backward compatible | Yes | No (new endpoint) | Yes | Yes | Yes | Yes |
| Implementation risk | Medium | Low | Medium | Low | Medium | Low |

## Risk and Failure Modes

### Lazy Scrollback
- **Cursor corruption:** If ANSI data with cursor-movement sequences is prepended while the terminal is scrolled up, subsequent writes render at stale coordinates. Mitigation: send `\u001b[H` (cursor home) before historical data; replay from sequence 0.
- **Sequence collisions:** If buffer overflows during prepend, out-of-order chunks appear. Mitigation: enforce `fromSeq >= oldestSequence` on server; reject requests with gaps.
- **Client memory exhaustion:** If user scrolls up 1000 times, each fetch adds to DOM. Mitigation: cap client-side prepended history to ~10,000 cells; trim oldest on further scrolls.
- **xterm private API breakage:** Using `_core.viewport` to scroll xterm manually breaks on version upgrade. Mitigation: check for public API in xterm v5+ before using private API.

### Frontend OTel
- **Bottleneck under load:** Telemetry endpoint becomes a bottleneck. Mitigation: batch on client; sample at 50% for non-errors; per-window rate limit.
- **PII leakage:** Terminal output or file paths in span names. Mitigation: scrub path names; never log full command; use anonymized session IDs.
- **OTLP collector unavailable:** Browser spans dropped silently. Mitigation: queue spans in IndexedDB; flush on recovery.

### Touch Scroll
- **Momentum scroll stops abruptly:** Custom handler breaks native feel. Mitigation: use CSS `scroll-behavior: smooth` + throttle pointer move events to 16ms (60fps).
- **Conflicts with xterm.js internal handlers:** Mitigation: test with real mobile hardware.

### Branch Autocomplete
- **Slow on large repos:** `git for-each-ref` or go-git on 100k+ refs can take 500ms–2s. Mitigation: cache in-memory; add 1s timeout with fallback to empty list.
- **Stale results race:** User navigates away before `ListBranches` returns; request completes for wrong repo. Mitigation: abort with `AbortController` when repo path changes.

## Migration and Adoption Cost

### A. Lazy Scrollback (HIGH EFFORT — 3–4 weeks)
- Backend: Wire `ScrollbackRequest` handler in `StreamTerminal` (~1 week)
- Frontend: Detect scroll-to-top via scroll event on xterm viewport; fire RPC (~1 week)
- Testing: Sequence collision, out-of-bounds, cursor state (~2 weeks)
- Rollout: Feature-flag behind `ENABLE_LAZY_SCROLLBACK=true`

### B. Frontend OTel (MEDIUM EFFORT — 2 weeks)
- Setup `@opentelemetry/sdk-web`, `@opentelemetry/exporter-trace-otlp-http` (~3 days)
- Implement `/api/telemetry` proxy endpoint in Go (~3 days)
- Instrument XtermTerminal, SessionWizard, StreamTerminal attach latency (~1 week)
- Rollout: Opt-in via `ENABLE_BROWSER_TELEMETRY=false` (safe default)

### C. Touch Scroll Isolation (LOW EFFORT — 1 week)
- CSS: Add `overscroll-behavior: contain` + `touch-action: none` to `XtermTerminal.module.css` (~1 day)
- Manual scroll handler (if scrollback re-enabled): ~200 lines TypeScript (~3 days)
- Testing on iPad, iPhone, Android Chrome (~2 days)

### D. Branch Autocomplete (LOW EFFORT — 1 week)
- Backend: Add `ListBranches(repoPath string)` unary RPC in `session_service.go` (~2 days)
- Frontend: Wire `useBranchSuggestions` hook in `SessionWizard.tsx` using existing `AutocompleteInput` component (~1 day)
- Testing: git repo with >1000 branches; timeout scenarios (~2 days)

## Operational Concerns

### Lazy Scrollback
- Track: `scrollback_request_latency_ms`, `scrollback_bytes_transferred`, `sequence_collision_errors`
- Limits: Max 500 lines per request; reject `limit > 10000`; reject `fromSeq < oldestSequence - 100`
- CircularBuffer evicts oldest on overflow; no manual cleanup needed
- Alert if scrollback request latency > 1s

### Frontend OTel
- Start with 10% sample rate for non-error spans; 50% for errors
- 7-day retention in OTLP backend
- Automatically strip `/home`, `/Users`, `/workspace` from span attributes
- Alert if telemetry endpoint error rate > 5%

### Branch Autocomplete
- Client-side localStorage cache with 5-minute TTL per repo path
- Track: `branch_list_latency_ms`, `git_command_errors`, `fallback_count`
- Max 1000 branches returned; inform user if truncated

## Prior Art and Lessons Learned

**Lazy Scrollback:**
- Mosh (mobile shell) uses sequence-based snapshots + diff compression for slow networks — same sequence model as our `CircularBuffer`
- Kitty terminal uses ringbuffer + OSC protocol for terminal integration
- Lesson: Keep sequences immutable once assigned; never reuse; use uint64 to avoid wraparound
- Lesson: Server-side replay + full redraw is simpler than client-side prepend (but slower)

**Frontend OTel:**
- Browser OTel community standard: `@opentelemetry/sdk-web` + `@opentelemetry/sdk-trace-web`
- Lesson: Never export traces synchronously; always batch (improves UX, reduces dropped data)
- Lesson: Use IndexedDB fallback for offline scenarios

**Touch Scroll:**
- iOS Safari: `touch-action: manipulation` is safer than `none` (allows native double-tap zoom)
- Android Chrome: `overscroll-behavior: contain` is unreliable on scrollable nested divs
- Lesson: Test on real devices, not just DevTools simulation

**Branch Autocomplete:**
- GitHub CLI (`gh`) caches branches locally via `git for-each-ref` (much faster than `git branch`)
- Lesson: Use `--format=%(refname:short)` to avoid branch list parsing
- Lesson: Separate remote and local branches in response; don't merge on server

## Open Questions

- [ ] Does CircularBuffer persist sequence numbers across server restarts? — blocks: client-side sequence state after restart; may need a "full redraw" flag on reconnect
- [ ] What xterm.js version is installed? v4.x has `private _core.viewport`; v5.x may have public API — blocks: touch scroll implementation approach [TRAINING_ONLY — verify]
- [ ] Is `TerminalDiff` fully implemented in backend, or just proto definitions? — blocks: whether scrollback prepend must use raw `TerminalOutput`
- [ ] How does the frontend currently send `FlowControl.paused=true`? — blocks: whether lazy scrollback may starve PTY during large prepends
- [ ] Can browser `@opentelemetry/sdk-web` export to gRPC OTLP? — answer: no; requires HTTP/OTLP — blocks: telemetry pipeline design

## Recommendation

**Recommended approach per area:**

**A. Lazy Scrollback — Separate Unary RPC** (not stream extension)
- Add `GetScrollback(sessionId, fromSeq, limit)` as a clean unary RPC
- On initial attach: `StreamTerminal` sends only last 500 lines (fromSeq = maxSeq - 500)
- On scroll-to-top trigger in xterm: call `GetScrollback(fromSeq=currentOldestSeq - 500, limit=500)` and prepend to terminal
- Simpler to test and cache than extending the bidirectional stream
- The `CircularBuffer.GetLastN()` and `GetRange()` methods are already there — this is a one-day backend job

**B. Frontend OTel — Lightweight Custom JSON Endpoint First**
- Skip `@opentelemetry/sdk-web` initially; POST structured JSON events to `/api/telemetry`
- Backend logs and optionally forwards to OTLP
- Instrument: session attach latency (time from click to first terminal output), RPC round-trip (StreamTerminal first byte), page navigation events
- Migrate to OTel SDK later if correlation with backend traces is needed
- **Why**: Avoids bundle size risk; gets actionable data in days not weeks

**C. Touch Scroll — CSS First, Then JS**
- `overscroll-behavior: contain` on `.xterm-viewport` (or terminal wrapper div) — CSS-only, immediate
- If scrollback is re-enabled: add `touchstart`/`touchmove` listener with manual scroll delta calculation using `terminal.scrollLines()`
- Do NOT use `touch-action: none` — breaks text selection on mobile

**D. Branch Autocomplete — New `ListBranches` Unary RPC**
- `ListBranches(repoPath: string, includeRemote: bool)` → `{ localBranches: string[], remoteBranches: string[] }`
- Wire into `SessionWizard.tsx` using existing `AutocompleteInput` component
- Shell out to `git for-each-ref refs/heads refs/remotes/origin --format='%(refname:short)'` with a 2s timeout and in-memory 5-minute cache
- This mirrors how the existing path-completion RPC works

## Web Search Results (2026-04-16)

**xterm.js public scrollback API (verified):** No public scroll API exists in v4 or v5. Mobile scrolling remains an unresolved upstream issue (GitHub #5377, July 2025). The `_core.viewport` private API approach remains the only option; pin xterm.js version when using it.

**OTel JS bundle size (verified):** ~300 KB uncompressed, ~60 KB gzipped for the full sdk-web. OTel SDK 2.0 (2025) improved tree-shaking. Dynamic import is strongly recommended to avoid blocking first paint. Confirms the lightweight custom JSON endpoint approach is the better first step.

**overscroll-behavior: contain (verified):** Added in **Safari 16** (not 15). iOS 15 users will not benefit. Use CSS as a baseline for iOS 16+; add JS touch event fallback for iOS 15. `overscroll-behavior` alone is insufficient for xterm.js since xterm doesn't have a native scrollable viewport.

**git for-each-ref performance (verified):** p90 ~75ms for 100 branches. Using `--contains HEAD` causes severe slowdown. Use `git for-each-ref refs/heads --format='%(refname:short)'` (no `--contains`). Add 2s timeout as defense for repos with 1000+ branches.

**xterm.js touch scroll (verified):** Issue #5377 (July 2025) confirms touch scroll is an open unresolved upstream issue. The only working approach is application-level touch event interception + manual `terminal.scrollLines()` calls. Do NOT rely on CSS-only fixes.

## Pending Web Searches

1. `xterm.js v5 public scrollback API scrollLines` — verify if v5 exposes public scroll API [TRAINING_ONLY — verify]
2. `opentelemetry js sdk web bundle size 2025 minified gzipped` — confirm bundle impact before committing
3. `iOS Safari overscroll-behavior contain nested div 2025` — verify support level
4. `git for-each-ref performance 1000 branches seconds latency` — benchmark data for branch listing
5. `xterm.js touchstart touchmove mobile scroll preventDefault` — verify correct event interception approach
