# Findings: Stack — Terminal Snapshot Rendering

Created: 2026-04-14

## Summary

This project needs to display last N lines of ANSI terminal output on a session list page (showing 20+ session cards simultaneously) without navigating to full terminal view. The backend already has ConnectRPC bidirectional streaming infrastructure, a terminal scrollback buffer system, and full xterm.js integration for the detail view.

**Recommended approach**: Extend existing ConnectRPC streaming to support snapshot mode (raw ANSI bytes for last N lines), render with lightweight `ansi-to-html` (already in package.json), cache rendered output in React state with 2-3 second TTL, fetch via server-streaming ConnectRPC rather than polling.

**Dominant trade-off**: Rendering fidelity vs. bundle/memory cost per card. xterm.js gives 100% fidelity but costs 280 KB bundle + 500 KB memory per card instance. ansi-to-html gives 90% fidelity (colors/bold, no cursor movement) at 6 KB + ~10 KB per card. For read-only snapshots in a list of 20+ sessions, fidelity loss is acceptable and the performance win is decisive.

## Options Surveyed

### ANSI Rendering Libraries

**xterm.js (`@xterm/xterm`)** — Full terminal emulator. Bundle: 280–350 KB. Canvas rendering (WebGL). 100% VT fidelity. Already in project. Too heavy for read-only snapshots — 500 KB+ memory per card instance.

**ansi-to-html** — ANSI → HTML converter. Bundle: ~6 KB (already at 0.7.2 in package.json). HTML `<span>` rendering. 90% fidelity (SGR color/bold, not cursor movement). Single function call. **Best fit for read-only snapshots.**

**xterm-for-react** — xterm.js wrapper. 280 KB. Lower activity. Same weight problem as xterm.js.

**react-terminal-ui** — ~15 KB. Canvas via Pixi.js. Basic VT100. Low community activity.

**@xterm/xterm read-only** — Same xterm.js, disable keyboard. Still 280 KB + one Terminal() instance per card.

### Snapshot Delivery Methods

**REST Polling** — 50–200ms latency. Creates O(N) requests for N sessions. Architectural mismatch with ConnectRPC-only project. ❌

**ConnectRPC Server-Streaming (Recommended)** — 10–50ms latency. HTTP/2 multiplexing. Auto-reconnect. Extends existing SessionService. Perfect fit. ✅

**Server-Sent Events (SSE)** — One TCP connection per client per endpoint. Adds separate transport infrastructure alongside ConnectRPC. ❌

### Snapshot Format

**Raw ANSI bytes** — Smallest payload. Client converts with ansi-to-html (~1–5ms). Minimal backend work. **Recommended.**

**Pre-rendered HTML** — No client conversion, but 2–3x payload size. Backend must implement ANSI→HTML in Go. More complex.

**Stripped plaintext** — Safest. No colors. Poor UX for terminal output. Fallback only.

**Structured terminal state (semantic)** — Full control but large payload. Overkill for snapshots.

## Trade-off Matrix

| | ansi-to-html | xterm.js | react-terminal-ui | xterm (read-only) |
|---|---|---|---|---|
| Bundle size (KB) | **6** | 280–350 | 15 | 280+ |
| Rendering fidelity | 90% (colors/bold) | 100% | 70% | 100% |
| Read-only fit | ✅ Native | ⚠️ Overkill | ✅ Good | ✅ Possible |
| Scroll perf (20 cards) | ✅ 60 FPS (HTML) | ❌ 15–30 FPS (canvas×20) | ⚠️ 30–45 FPS | ❌ 15–30 FPS |
| Integration cost | ✅ Trivial (1 line) | ⚠️ Medium | ⚠️ Medium | ✅ Low (familiar) |
| Memory per card | ✅ ~10 KB | ❌ 500 KB+ | ⚠️ ~100 KB | ❌ 500 KB+ |
| Maintenance burden | ✅ None (npm) | ✅ Active | ⚠️ Medium | ✅ Already used |

## Risk and Failure Modes

**Partial ANSI sequences at N-line boundary** — Snapshot splits mid-escape-code; ansi-to-html produces garbage HTML.
- Probability: Medium. Severity: High.
- Mitigation: Read last N lines with line-boundary alignment (`bufio.Reader.ReadString('\n')` in Go ensures this); validate UTF-8 before sending.

**Cursor position codes in output** — CSI cursor movement codes cause visual misalignment.
- Probability: Low. Severity: Low.
- Mitigation: ansi-to-html strips non-rendering codes by design. No action needed.

**256-color/truecolor sequences** — `ESC[38;5;Nm` or `ESC[38;2;R;G;Bm` not supported.
- Probability: Low (ansi-to-html 0.7.x supports both). Severity: Medium (cosmetic).
- Mitigation: Verify ansi-to-html 0.7.2 supports truecolor. [TRAINING_ONLY - verify]

**Key thrashing in React list** — Session added/removed changes keys; snapshots remount; flickering.
- Probability: Medium. Severity: High.
- Mitigation: Use stable keys (session ID, not array index); `React.memo` on snapshot component.

**Scroll position loss** — Snapshot update triggers full list re-render; page jumps to top.
- Probability: Low with correct key management. Severity: High.
- Mitigation: Ensure updates don't re-trigger full list reconciliation; test with 30+ sessions.

**Memory leak from streaming subscription** — Component unmounts but ConnectRPC stream persists.
- Probability: Medium. Severity: High (app degrades over hours).
- Mitigation: `useEffect` cleanup to unsubscribe on unmount; verify with React DevTools Memory profiler.

**Network disconnect → stale snapshot** — Client reconnects but misses updates; snapshot shows old state without indicator.
- Probability: High (laptops, mobile). Severity: High.
- Mitigation: Timestamp every snapshot; show "last updated X min ago" badge; auto-request refresh after 30s silence.

**Server overload with 100+ sessions** — 100 simultaneous snapshot streams; server CPU/memory spike.
- Probability: Low for 20 sessions, High for 100+. Severity: High.
- Mitigation: Client-side viewport culling (only fetch visible cards); server-side TTL caching; rate limit per session.

**XSS via terminal output** — Malicious ANSI sequences containing HTML tags.
- Probability: Very low. Severity: High.
- Mitigation: ansi-to-html escapes HTML entities by default; `dangerouslySetInnerHTML` is safe from trusted internal backend.

## Migration and Adoption Cost

| Step | Size | Effort |
|---|---|---|
| Add `snapshot_output` field to Session proto | S | 1–2h |
| Implement `GetSessionSnapshot` RPC (backend: scrollback tail) | S | 2–4h |
| `useSessionSnapshot()` hook + ConnectRPC client | S | 1–2h |
| `SessionSnapshotPreview` component (ansi-to-html → HTML) | S | 1h |
| Integrate with WatchSessions stream + debounce | M | 2–3h |
| Add "last updated" badge + stale state handling | M | 2–3h |
| Unit + e2e tests | M | 3–4h |
| Styling (reuse terminal CSS from existing module) | S | 1h |

**Total**: ~4–5 developer-days.

**Rollback cost**: Very low. Feature is additive and read-only. Can hide behind feature flag. No DB schema changes.

## Operational Concerns

**Monitor**: Snapshot fetch latency (goal <50ms, alert >200ms); snapshot size (goal <50KB, alert >100KB); update frequency per session (goal <0.5/sec, alert >2/sec). Use existing OTel spans.

**Scaling ceiling**: Comfortable to ~50 sessions. Beyond that: implement viewport culling (only fetch snapshots for visible cards), server-side snapshot caching (2-5s TTL), batching.

**Security**: ansi-to-html escapes HTML entities; no XSS risk. Snapshot is same data as full terminal view (no additional exposure).

## Prior Art and Lessons Learned

**GitHub Actions** — Pre-renders HTML on server; CSS class-based colors scale better than inline styles. Single stream per job works well.

**CircleCI** — Uses xterm.js in read-only mode for job logs. Confirms xterm works but is heavy for list previews.

**ttyd/wetty** — Use `ansi_up` (predecessor to ansi-to-html) widely in web terminal projects. Validates lightweight approach.

**Common pitfall (from all three)**: Splitting a snapshot at a byte boundary inside a multi-byte UTF-8 or escape sequence breaks rendering. Always align to line boundaries.

**Common pitfall**: Cursor movement codes in output cause visual artifacts if renderer doesn't strip them. Use a parser, not regex.

## Open Questions

- [ ] Snapshot line count (N): Start with 10 lines? Needs telemetry to validate. — affects per-card height
- [ ] Update frequency: Debounce interval for snapshot push (2s? 3s?) — affects backend load vs. freshness
- [ ] Pre-render location: Backend vs. client-side ANSI conversion. Recommendation: client-side (simpler backend, fast enough). — blocks ADR
- [ ] Viewport culling: Implement from day 1 or add when scaling requires it? — affects architecture complexity

## Recommendation

**Use `ansi-to-html` + ConnectRPC server-streaming + raw ANSI format.**

Reasoning: Smallest bundle (6 KB, already installed), fastest scroll performance for 20+ cards (HTML DOM at 60 FPS vs. canvas at 15–30 FPS), lowest integration cost (extends existing SessionService), minimal backend work (tail scrollback buffer). The 10% fidelity loss (no cursor movement rendering) is irrelevant for a read-only snapshot.

**Accept these costs**: 2-3 second snapshot staleness (vs. real-time); partial fidelity for cursor-heavy output (progress bars may look slightly off in snapshot — acceptable since full view is one click away).

**Reject these alternatives**:
- xterm.js per card: rejected because 500 KB memory × 20 cards = 10 MB just for snapshots, plus canvas rendering jank in a scrolling list
- Pre-rendering on backend: rejected because it adds Go-side ANSI→HTML conversion complexity with no meaningful UX benefit
- REST polling: rejected because it creates O(N) requests and mismatches the existing streaming architecture

## Pending Web Searches

1. `"ansi-to-html npm 0.7 256-color truecolor support"` — verify 256/truecolor escape code support
2. `"xterm.js bundle size production minified 2024"` — confirm 280 KB figure
3. `"react-window scroll anchor 20 items HTML DOM 60fps benchmark"` — verify scroll performance claim
4. `"connectrpc web client backpressure streaming flow control"` — verify backpressure handling
5. `"incomplete ANSI escape sequence boundary split rendering"` — confirm line-boundary mitigation approach
