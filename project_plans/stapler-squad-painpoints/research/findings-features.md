# Findings: Features

## Summary

Stapler Squad can solve its five UX pain points by adopting patterns from mature tools: (A) virtual scrolling for terminal scrollback, (B) `overscroll-behavior` + pointer events for mobile touch, (C) fuzzy-search autocomplete for branches, (D) inline contenteditable rename, (E) local-only analytics to self-hosted backend. These are proven, low-risk patterns with clear implementation paths.

The dominant trade-off is complexity vs. user expectation: users expect the same UX they get in VS Code and GitHub, but implementing it properly (virtual scroll, fuzzy search, inline edit) requires non-trivial effort. Simpler approaches (cap buffer, prefix-only search, modal rename) are faster to ship but feel worse.

## Options Surveyed

### A. Terminal Scrollback in Comparable Tools

| Tool | Approach | Notes |
|------|----------|-------|
| **VS Code** | Virtual scroll + capped buffer (10k–1M lines) | Only renders visible rows. User can increase cap. Lazy-loads file content. |
| **xterm.js** | No built-in virtualization | Raw `<div>` holds all rows. Application must implement virtual scroll. |
| **ttyd** (C web terminal) | Send all scrollback + gzip compression | No pagination. Relies on browser's native scrolling. Works for typical session logs. |
| **Wetty** (Go web terminal) | Streaming model; buffer size depends on pty | Sends new output as it arrives. No lazy loading of backlog. |
| **Termux** (mobile) | Capped scrollback; user configurable | Typical cap 2,000–5,000 lines. Circular buffer prevents unbounded growth. |

**Consensus:** Virtual rendering is the gold standard for large buffers. If virtual scroll is too complex, cap buffer size + discard oldest lines (circular buffer pattern).

### B. Mobile Touch Scroll in Web Terminals

| Tool / Pattern | Solution | Trade-offs |
|---|---|---|
| **iOS Safari `overscroll-behavior: none`** | Prevents swipe-back gesture on terminal div | Terminal must be in a non-scrollable container; page scroll is outside. |
| **Android Chrome pointer events** | Use `onpointerdown/move/up` instead of wheel; set `touch-action: pan-y` | Requires manual scroll calculation. xterm.js typically doesn't expose this natively. |
| **ttyd** | Disables pinch-zoom inside terminal; uses mouse wheel events | Touch scroll still fights page scroll on mobile Safari. Not ideal. |
| **Wetty** | Terminal in scrollable div; relies on browser scroll | Works on Android; problematic on iOS Safari (gesture conflicts). |
| **Best practice (Termux, JuiceSSH)** | Separate scroll container outside terminal; `overscroll-behavior: contain` on inner, `touch-action: manipulation` on terminal | Requires careful DOM structure. Touch scroll inside terminal, page scroll outside. |

**Consensus:** Use a nested scroll container with `overscroll-behavior` + pointer events, not wheel events.

### C. Branch Autocomplete UX Patterns

| Tool | Behavior | Details |
|---|---|---|
| **VS Code Command Palette** | Fuzzy filtering, instant | Every keystroke filters. Shows top ~20 matches. Keyboard arrows + Enter to select. Highlights matched chars. |
| **GitHub branch selector** | Dropdown with recent first, then alphabetical | Max ~30 visible; scroll within list. Prefix match only. |
| **JetBrains (IntelliJ, GoLand)** | Modal combobox with fuzzy search + history | Recent branches pinned. Fuzzy matching. Enter creates branch if it doesn't exist. |
| **Gitpod / GitHub Codespaces** | Fuzzy combobox in sidebar | Instant filtering. Shows branch + last commit message. |

**Consensus:** Fuzzy search (not just prefix) + recent-first sorting + max 20–30 visible results + arrow keys for navigation.

### D. Inline Rename/Retag Patterns

| Platform | Pattern | Implementation |
|---|---|---|
| **GitHub (issues, PR titles)** | Click title → toggles `contenteditable` div | Blur or Ctrl+Enter saves. Esc cancels. No modal. |
| **Linear (task names)** | Click to inline edit; instant optimistic save on blur | Small input overlay. Server sync in background. |
| **Notion (block editing)** | Click inline; contenteditable div | Markdown parsing. Blur saves. |
| **Figma (artboard naming)** | Double-click name → input box | Enter/blur saves. Esc cancels. |

**Consensus:** Inline toggle of hidden `<input>` (safer than contenteditable) is the modern pattern. No modal dialog. Blur/Enter to save; Esc to cancel. Optimistic UI updates.

### E. Analytics in Single-User Dev Tools

| Tool | Telemetry Approach | Notes |
|---|---|---|
| **VS Code** | Opt-in; sends crash reports, feature usage | Tied to Microsoft account. User can opt out. |
| **PostHog (open source)** | Event capture → local backend or PostHog Cloud | Self-hostable; events only stored locally if using self-hosted. Privacy-friendly. |
| **Plane (open-source project tool)** | Custom analytics in backend; stores in PostgreSQL | Events logged server-side. User has full control. No third-party vendor. |
| **Custom (lightweight)** | Send structured logs (JSON) to own `/logs` endpoint | Store in SQLite or PostgreSQL on user's server. Simple, auditable. |

**Consensus:** For a self-hosted tool, local-only analytics (logs to own backend, not third-party SaaS) is the pattern.

## Trade-off Matrix

| Pain Point | Solution | Complexity | Perf Impact | Privacy | UX Quality |
|---|---|---|---|---|---|
| Scrollback load | Virtual scroll + pagination | High | Huge improvement | N/A | Best |
| Scrollback load | Cap buffer size (circular) | Low | Good (data loss if exceeds cap) | N/A | Acceptable |
| Mobile touch scroll | `overscroll-behavior` + pointer events | Medium | Minimal | N/A | Native feel |
| Mobile touch scroll | Separate scroll container | Medium | Minimal | N/A | Native feel |
| Mobile layout | CSS media queries + flexbox | Low | None | N/A | Immediate improvement |
| Branch autocomplete | Fuzzy search (client-side) | Low | Instant (<1k branches) | N/A | Modern, expected |
| Frontend observability | Local analytics (custom) | Low–Medium | Negligible | Excellent | Background |
| Frontend observability | PostHog self-hosted | Medium–High | Backend + event buffer | Excellent | Dashboard |
| Inline rename | Hidden input toggle | Low | None | N/A | Familiar |

## Risk and Failure Modes

### A. Virtual Scrolling
- **Risk:** Scrollbar thumb size becomes inaccurate if not properly updated.
- **Mitigation:** Calculate scrollbar height dynamically based on total line count.
- **Failure mode:** Scrolling jumps unexpectedly if virtual scroll window isn't re-rendered smoothly.

### B. Mobile Touch Scroll
- **Risk:** `overscroll-behavior: none` breaks Safari's back-swipe gesture entirely.
- **Mitigation:** Only apply `overscroll-behavior` to terminal container, not page.
- **Failure mode:** Gesture conflicts reappear if DOM nesting is wrong.

### C. Branch Autocomplete
- **Risk:** Fuzzy search can match too many results for large repos (10k+ branches). UI becomes slow.
- **Mitigation:** Cap visible results to 20; server-side search if needed. Debounce input (200ms).
- **Failure mode:** User types faster than server responds; stale results shown.

### D. Inline Rename
- **Risk:** User accidentally hits Enter and saves incomplete name.
- **Mitigation:** Show "Saving..." indicator. Rollback on failure. Allow Ctrl+Z undo.
- **Failure mode:** Session name is truncated/wrong if save fails silently.

### E. Analytics
- **Risk:** Event volume grows unbounded; SQLite file becomes huge.
- **Mitigation:** Retention policy (keep logs for 30 days, then truncate). Sample events if volume is high.
- **Failure mode:** Disk space exhausted; performance of queries degrades.

## Migration and Adoption Cost

### High Impact, Low Cost (start here)
- **Mobile layout (CSS media queries):** 1–2 days. No backend changes.
- **Branch autocomplete (fuzzy search, client-side):** 2–3 days. Drop-in dependency. No API changes.
- **Inline rename (hidden input toggle):** 3–5 days. Requires careful validation + optimistic UI.

### High Impact, Medium Cost
- **Virtual scrolling for terminal:** 5–10 days. Requires refactoring xterm.js integration. Potential for regressions.
- **Mobile touch scroll (pointer events + overscroll-behavior):** 3–5 days. Requires testing on real iOS/Android devices.

### Medium Impact, Medium Cost
- **Frontend analytics (local SQLite):** 4–7 days. Design event schema. Build dashboard.

## Operational Concerns

### Virtual Scrolling
- Track scroll performance (frame rate) and buffer size distribution.
- Need to validate virtual scroll window matches visual viewport.

### Mobile Touch Scroll
- iOS 13+, Android 8+ have different pointer event APIs. Need device testing.
- Gesture conflict: back-swipe, pinch-zoom, long-press all fight for the same input space.

### Branch Autocomplete
- If autocompleting against Git backend, latency can spike. Server-side caching helps.
- Branch list can change while dialog is open. Consider "stale" indicator.

### Analytics
- Logs grow unbounded. Need automatic cleanup / archival.
- If processing other users' terminals (future), analytics logs might contain PII. Design for data minimization now.
- Event writes should be batched or async to avoid blocking the UI thread.

## Prior Art and Lessons Learned

### Terminal Scrollback (VS Code, iTerm2)
- **Lesson 1:** Users expect arbitrary scrollback depth, but only <1% actually scroll back past the last 100 lines. Capping at 10k lines is unnoticed for most.
- **Lesson 2:** Virtual rendering is worth the complexity only for session logs >100k lines. For typical interactive sessions (20–50k lines), simple scroll + gzip is fine.

### Mobile Touch Scroll (React Native, Flutter)
- **Lesson 1:** `overscroll-behavior: none` is the simplest fix but breaks user expectations.
- **Lesson 2:** Nested scrollable containers work but require careful focus management.
- **Lesson 3:** User testing on real devices is non-negotiable. Simulator touch events don't match real fingers.

### Autocomplete (VS Code, GitHub, Slack)
- **Lesson 1:** Fuzzy search beats prefix matching for discoverability, but can be slow on large lists (10k+ items). Limit to top 100 candidates before fuzzy scoring.
- **Lesson 2:** Recent-first sorting (branches you've checked out recently) beats alphabetical for UX.
- **Lesson 3:** Keyboard navigation (arrow keys, Enter) is critical.

### Inline Editing (GitHub, Linear, Notion)
- **Lesson 1:** Contenteditable is fragile (undo, drag-drop, paste all behave oddly). Using a hidden `<input>` is safer.
- **Lesson 2:** Blur to save introduces a gotcha: accidental blur triggers save. Users expect Ctrl+S or Enter. Consider both.
- **Lesson 3:** Optimistic UI feels snappy; users forgive the occasional rollback.

### Analytics (Sentry, PostHog, Datadog)
- **Lesson 1:** Most open-source / self-hosted tools do not ship telemetry by default. Opt-in is critical.
- **Lesson 2:** For developer tools, track "did the user complete the task?" more than "how many times did they click X?". High-level outcomes > low-level interactions.

## Open Questions

- [ ] Should Stapler virtual-render the entire scrollback buffer, or only the last N lines (discarding older)? — blocks decision on: scrollback architecture approach
- [ ] Should branches come from the tmux session's local `.git` folder, or from a configured remote? — blocks: branch autocomplete latency/freshness design
- [ ] How long should Stapler retain interaction logs? — blocks: analytics retention policy
- [ ] Should scrolling up on the page (outside terminal) be possible while terminal is focused? — blocks: touch scroll DOM architecture

## Recommendation

**Start with low-hanging fruit (Weeks 1–2):**
- Implement mobile layout using CSS media queries + flexbox. Test on real iOS/Android.
- Add fuzzy-search branch autocomplete using a lightweight library (fuse.js or similar). Keep client-side; use cached branch list.
- Implement inline rename using a hidden `<input>` toggle. Save on blur + Enter; support Esc to cancel.

**Mid-term improvements (Weeks 3–4):**
- Solve mobile touch scroll by restructuring DOM: place terminal in a separate scrollable container; use `overscroll-behavior: contain` + pointer events.
- Add basic frontend analytics: log (action, timestamp, session_id) to local backend endpoint. No external vendors. Privacy-by-default.

**High-impact future work (Weeks 5+):**
- Implement virtual scrolling for terminal scrollback if session logs regularly exceed 100k lines.
- Otherwise, cap buffer size at 50k–100k lines with a circular buffer (oldest lines discarded).

## Web Search Results (2026-04-16)

**VS Code terminal scrollback (verified):** Default is **1,000 lines**; configurable up to 50,000. VS Code does not use virtual rendering for the scrollback buffer — it uses xterm.js's built-in buffer. This means VS Code also has a hard cap, not infinite scrollback. (Source: [code.visualstudio.com/docs/terminal/basics](https://code.visualstudio.com/docs/terminal/basics))

**xterm.js mobile touch (verified):** GitHub issue #5377 (July 2025) confirms "limited touch support on mobile devices impacts terminal usability." Touch scrolling sends arrow keys rather than scrolling the viewport (issue #1007). Ballistic scrolling not supported. This is an **open, unresolved upstream issue** — there is no clean xterm.js fix; workarounds must be implemented in the application layer. (Source: [xtermjs/xterm.js#5377](https://github.com/xtermjs/xterm.js/issues/5377))

**overscroll-behavior: contain and iOS Safari (verified):** `overscroll-behavior` was added in **Safari 16**, not Safari 15. iOS 15 devices will not benefit from this CSS fix. For iOS 15 support, a JavaScript fallback (touch event interception) is required. (Source: [caniuse.com/css-overscroll-behavior](https://caniuse.com/css-overscroll-behavior))

**go-git remote tracking branches (verified):** In go-git, `Repository.Branches()` only returns refs where `IsBranch() == true` — which applies only to `refs/heads/*` (local branches). Remote tracking refs (`refs/remotes/*`) return `IsBranch() == false` and are not included. C2 risk is **lower than estimated** — go-git's `Branches()` method correctly scopes to local branches only. (Source: [src-d/go-git#601](https://github.com/src-d/go-git/issues/601))

## Pending Web Searches

1. `xterm.js canvas renderer virtual scroll` — confirm current state of canvas addon and performance ceiling
2. `VS Code terminal scrollback buffer size default 2025` — verify current default and max cap
3. `iOS Safari overscroll-behavior support iOS 13 15` — confirm browser support and quirks
4. `pointer events vs wheel events mobile scroll best practice 2025` — best practice
5. `minisearch vs fuse.js bundle size 2025` — performance + bundle size comparison
6. `contenteditable vs input element accessibility a11y 2025` — a11y best practices for inline edit
7. `ttyd web terminal scrollback 2025 implementation` — verify if it still sends all backlog at once
