# Findings: Pitfalls

## Summary

Stapler Squad faces production risks in four areas: (1) xterm.js lazy scrollback may experience rendering corruption when prepending old ANSI lines while terminal is scrolled up; (2) OTel JS adds unmeasured bundle overhead with known React 19 and iOS Safari PerformanceObserver issues; (3) mobile touch scroll inside xterm lacks `overscroll-behavior` containment and is broken on iOS; (4) git branch listing via go-git may time out on large repos and unexpectedly include remote-tracking branches.

The three highest-urgency risks are: **branch listing timeout** (affects session creation for any large repo), **mobile chrome hiding on touch scroll** (breaks mobile UX fundamentally), and **iOS keyboard viewport shrink** (makes the terminal unusable after typing in any input). All three have low-cost mitigations.

## Options Surveyed

Areas analyzed:
- A: xterm.js virtual/lazy scrollback pitfalls
- B: OTel JS SDK production pitfalls
- C: Mobile touch scroll pitfalls
- D: Git branch listing performance and correctness
- E: Keyboard navigation in dialog pitfalls

## Trade-off Matrix (severity × likelihood × mitigation effort)

| Risk | Severity | Likelihood | Mitigation Effort | Urgency |
|------|----------|-----------|------------------|---------|
| A1: Cursor corruption + lazy scrollback ANSI | High (9/10) | Medium (5/10) | Medium | Medium |
| A2: xterm scrollback buffer overflow | High (8/10) | Low (3/10) | Low | Low |
| A3: Write while terminal scrolled up | Medium (7/10) | Low (2/10) | Low | Low |
| A4: ANSI parsing throughput jank | Medium (6/10) | Low (3/10) | Low | Low |
| B1: OTel JS bundle size | Medium (5/10) | Medium (5/10) | Low | Medium |
| B2: OTel + React 19 strict mode double-init | Low (4/10) | Low (3/10) | Medium | Low |
| B3: OTel PerformanceObserver + iOS Safari | Medium (5/10) | Medium (4/10) | Low | Low |
| C1: Branch listing timeout (1000+ branches) | High (7/10) | Medium (6/10) | Low | **High** |
| C2: go-git returns remote-tracking branches | Medium (6/10) | Medium (5/10) | Low | Medium |
| C3: Race: user creates session before branches load | Medium (6/10) | Low (4/10) | Medium | Medium |
| D1: Touch scroll triggers browser chrome hiding | High (8/10) | High (7/10) | Medium | **High** |
| D2: `touch-action: none` breaks text selection | Medium (5/10) | Low (2/10) | Low | Low |
| D3: `mouseTracking: 'any'` interferes with touch | Medium (5/10) | Low (3/10) | Low | Low |
| D4: iOS keyboard shrinks viewport (terminal unusable) | High (7/10) | High (6/10) | Medium | **High** |
| D5: Text selection vs. scroll gesture conflict | Medium (6/10) | Medium (5/10) | Medium | Medium |
| E1: Focus loss after re-render in dialog | Medium (5/10) | Medium (4/10) | Low | Low |
| E2: Combobox dropdown steals Tab key | High (6/10) | Medium (5/10) | Medium | Medium |

## Risk and Failure Modes

### A1. xterm.js Cursor Corruption + Lazy Scrollback [TRAINING_ONLY — verify]

**Failure Mode:** When streaming ANSI output to xterm.js that includes cursor positioning sequences (e.g., `ESC[H`, `ESC[10;5H`), xterm.js tracks internal cursor position. If lazy scrollback prepends old lines via `terminal.write()` while the user is scrolled up, the next server output containing cursor movement sequences renders at stale cursor coordinates — text overwrites or misaligns.

**Trigger:** Lazy scrollback active; user scrolled up; server output contains cursor positioning ANSI (e.g., `git diff`, `vim`, `htop`).

**Current Protection:** Stapler currently sets `scrollback: 0` and manages all history in the backend circular buffer — no prepending occurs, so this is safe today. The risk activates only when lazy scrollback is implemented.

**Mitigation:**
- Never prepend lines while terminal is scrolled up; queue them
- Send `\u001b[H` (cursor home) before historical data
- After prepend, explicitly reposition cursor to saved location
- Use xterm's `saveState` / `restoreState` escape sequences around historical writes

---

### A2. xterm.js Scrollback Buffer Overflow [TRAINING_ONLY — verify]

**Failure Mode:** The `scrollback` option in xterm.js is a **line count**, not byte count. When the buffer reaches the limit, older lines are evicted. ANSI sequences with cursor movement or double-width CJK characters may cause xterm.js to miscalculate line boundaries, resulting in mangled output when oldest lines are discarded.

**Trigger:** High-output scenario (e.g., `npm build` verbose logging); 1000+ lines arrive within seconds; output contains ANSI codes.

**Current Protection:** `scrollback: 0` is set — xterm.js doesn't evict. Backend circular buffer holds up to 10MB raw PTY data.

**Mitigation:** If scrollback is re-enabled, set limit conservatively (`scrollback: 500`). Log when circular buffer hits 80% capacity as a canary.

---

### A3. Write While Terminal Scrolled Up [TRAINING_ONLY — verify]

**Failure Mode:** If `terminal.write()` is called while the user has scrolled up to view old output, the new data is written at the cursor position (end of buffer) but the viewport doesn't auto-scroll. User doesn't see new output and doesn't realize the command is still running.

**Current Protection:** Stapler disables xterm scrollbar (`overflow-y: hidden` in CSS). Scrolling is tmux-native (Ctrl+B `[`), not web-native — so this can't happen today.

**Mitigation if scrollbar is added:** Auto-scroll to bottom on new output unless user has scrolled more than 5% from bottom.

---

### A4. ANSI Parsing Throughput Jank [TRAINING_ONLY — verify]

**Failure Mode:** xterm.js ANSI parsing is O(n) per byte. Writing 10K+ lines of ANSI output can consume 50–200ms on a modern machine, causing UI stalls. WebGL renderer helps but adds ~50MB GPU context memory.

**Current Protection:** `StreamTerminal` already chunks PTY reads to 1KB. At realistic PTY speeds (100KB/sec), this is ~10 chunks/sec — manageable.

**Mitigation:** Monitor `TerminalMetrics` output. If total terminal load time exceeds 5 seconds, add `setTimeout` delays between writes to yield to the event loop.

---

### B1. OTel JS Bundle Size [TRAINING_ONLY — verify]

**Failure Mode:** `@opentelemetry/sdk-web` + `@opentelemetry/exporter-trace-otlp-http` total ~150KB minified, ~45KB gzipped. If imported in `Providers.tsx` or `layout.tsx` without dynamic import, the Next.js main chunk may exceed the project's bundle size budget, causing build failures.

**Trigger:** OTel SDK imported at the top of the module tree; no tree-shaking excludes unused OTel modules (OTel has known barrel export issues).

**Mitigation:**
- Measure baseline bundle before integrating: `npm run build && npm run size-limit`
- Use dynamic import: `await import('@opentelemetry/sdk-web')` — lazy-loads after first paint
- Alternative: `web-vitals` (5KB) for Core Web Vitals only; custom JSON logging for interaction events

---

### B2. OTel + React 19 Strict Mode Double-Init [TRAINING_ONLY — verify]

**Failure Mode:** React 19 strict mode renders components twice in dev. If OTel is initialized inside a `useEffect` without a ref guard, `PerformanceObserver` subscriptions may double-register, causing duplicate spans or missed initialization on second render.

**Mitigation:** Initialize OTel with a module-level singleton, not inside React lifecycle. Or use `useRef` guard:
```typescript
const otelInitialized = useRef(false);
useEffect(() => {
  if (otelInitialized.current) return;
  otelInitialized.current = true;
  initializeOTel();
}, []);
```

---

### B3. OTel PerformanceObserver + iOS Safari [TRAINING_ONLY — verify]

**Failure Mode:** iOS Safari 13–15 have incomplete PerformanceObserver API. `PerformanceObserver('navigation')` may throw `TypeError: unsupported entryTypes`, causing OTel initialization to fail and all traces to be silently dropped.

**Mitigation:** Wrap OTel init in try-catch, gracefully degrade to no-op:
```typescript
try {
  initializeOTel();
} catch (e) {
  console.warn('OTel init failed, continuing without traces', e);
}
```

---

### C1. Git Branch Listing Timeout on Large Repos [TRAINING_ONLY — verify]

**Failure Mode:** `go-git` `Repository.Branches()` iterates all refs in `.git/refs/` and `.git/packed-refs`. On a repo with 1000+ branches, this can take 500ms–2s on spinning disk or high-latency network mounts. Frontend branch dropdown hangs.

**Trigger:** Repository has 1000+ local branches (old CI system, legacy monorepo). User opens SessionWizard and tries to select a branch. `ListBranches` RPC is invoked. go-git iterates all branches without pagination or caching.

**Mitigation:**
1. Add timeout: `ctx, cancel := context.WithTimeout(ctx, 2*time.Second)` in the handler
2. Return partial results if timeout: `{ branches: first_100, truncated: true }`
3. Cache results in-memory for 60s to avoid repeated enumerations
4. Switch to `git for-each-ref --format='%(refname:short)' refs/heads` shell command (faster than go-git library for this use case)

---

### C2. go-git Returns Remote-Tracking Branches Unexpectedly [TRAINING_ONLY — verify]

**Failure Mode:** `go-git` `Repository.Branches()` may iterate refs under both `refs/heads/` (local) and `refs/remotes/` (remote tracking). If the intent is to list only local branches for checkout, the API returns ~2x more results, and user sees `origin/main` listed twice (once as remote, once as local tracking branch).

**Mitigation:** Explicitly filter on the backend:
```go
if strings.HasPrefix(ref.Name().String(), "refs/remotes/") {
  continue
}
```
Or use `git for-each-ref refs/heads` which is scoped to local branches only.

---

### C3. Race Condition: User Creates Session Before Branches Load [TRAINING_ONLY — verify]

**Failure Mode:** User opens SessionWizard, types repo path, and rapidly clicks "Create Session" before `ListBranches` RPC completes. Session is created with stale or empty branch info. Backend attempts to checkout a branch that doesn't exist, causing checkout failure.

**Mitigation:**
1. Disable "Create Session" button until `ListBranches` completes (show loading spinner in dropdown)
2. If user navigates away before load completes, cancel RPC via `AbortController`
3. Validate branch name against loaded branch list before submitting

---

### D1. Touch Scroll Triggers Browser Chrome Hiding [TRAINING_ONLY — verify]

**Failure Mode:** On iOS/Android, the browser's address bar ("chrome") auto-hides when the user scrolls down. If the terminal container is fixed-position with `overflow: hidden` and the user swipes inside it, the browser interprets the touch as a page scroll (not terminal scroll), triggering chrome hiding. This leaves the user unable to scroll the terminal and causes jank as the viewport resizes when chrome appears/disappears.

**Current State:** Stapler disables xterm viewport scrolling (`overflow-y: hidden` in CSS). No native scrolling means no touch scroll conflict today — **unless** user is on a page with a scrollable parent element (e.g., session list is scrollable, terminal is embedded). In that case, touch scroll propagates to parent.

**Mitigation:**
```css
.terminal {
  overscroll-behavior: contain;  /* prevents scroll chaining to parent */
}
```
Supported iOS 13+, Android 9+. Do NOT use `preventDefault()` on `touchmove` — iOS Safari may ignore it for chrome hiding.

---

### D2. `touch-action: none` Breaks Text Selection [TRAINING_ONLY — verify]

**Failure Mode:** `touch-action: none` disables all touch gestures, including long-press text selection on mobile.

**Mitigation:** Use `touch-action: manipulation` instead (allows double-tap zoom but not pinch-to-zoom; preserves text selection). Or `touch-action: pan-x pan-y` (allows panning, preserves most gestures).

---

### D3. `mouseTracking: 'any'` Interferes with Touch [TRAINING_ONLY — verify]

**Failure Mode:** If `mouseTracking: 'any'` is enabled in xterm.js, it intercepts touch events and sends them as mouse events to the PTY. Applications like `vim` or `less` receive unexpected mouse events when user is trying to scroll, causing them to jump the cursor or trigger mode changes.

**Current Protection:** Default is `mouseTracking: 'none'`. No risk unless admin explicitly enables mouse tracking.

**Mitigation:** Document that `mouseTracking: 'any'` must not be enabled on mobile. Detect touch-capable devices via `window.matchMedia('(hover: none)')` and disable mouse tracking.

---

### D4. iOS Keyboard Shrinks Viewport — Terminal Unusable [TRAINING_ONLY — verify]

**Failure Mode:** On iOS, when the virtual keyboard appears (user taps an input field), the viewport shrinks. The terminal fills the viewport, so it shrinks or becomes unusable. After keyboard hides, xterm.js must re-fit but may render at stale coordinates.

**Current State:** `TerminalOutput.tsx` tracks `isKeyboardVisible` state, but implementation is incomplete — it only tracks state, doesn't prevent rendering jank or adjust terminal dimensions.

**Mitigation:**
```typescript
useEffect(() => {
  const viewport = window.visualViewport;
  if (!viewport) return;
  const handleResize = () => {
    xtermRef.current?.fit();
  };
  viewport.addEventListener('resize', handleResize);
  return () => viewport.removeEventListener('resize', handleResize);
}, []);
```
Add 300ms delay after keyboard disappears — iOS doesn't immediately report final dimensions:
```typescript
setTimeout(() => xtermRef.current?.fit(), 300);
```

---

### D5. Text Selection vs. Scroll Gesture Conflict

**Failure Mode:** On iOS/Android, long-press-drag to select text is ambiguous with scroll gesture. Browser may interpret drag as scroll, preventing text selection.

**Mitigation:**
```css
.terminal :global(.xterm) {
  user-select: text;
  -webkit-user-select: text;
}
```
Document: double-tap for word selection, triple-tap for line selection on mobile.

---

### E1. Focus Loss After Re-render in Dialog

**Failure Mode:** React re-renders the session creation dialog after each form field update (e.g., after branch list loads). If the focused element is replaced in the DOM, focus is lost and keyboard navigation breaks.

**Mitigation:** Use stable `id` attributes on form fields. Use `autoFocus` only on the first field. After async operations that update state, restore focus explicitly:
```typescript
useEffect(() => {
  if (branchInputRef.current) branchInputRef.current.focus();
}, [branches]);
```

---

### E2. Combobox Dropdown Steals Tab Key

**Failure Mode:** When a combobox dropdown is open, pressing Tab selects the highlighted option and advances focus — or it may close the dropdown without selecting. This breaks the natural Tab-through-fields flow in the session creation form.

**Trigger:** User tabs through SessionWizard fields; branch combobox opens; Tab closes dropdown unexpectedly or jumps to wrong next field.

**Mitigation:** Configure the combobox library so:
- Tab selects current highlighted option AND advances to next field
- Escape closes dropdown and returns focus to the input
- Arrow keys navigate within dropdown; Tab/Enter confirm

Most headless combobox libraries (Radix, Downshift) support this configuration explicitly.

## Migration and Adoption Cost

**Immediate wins (0–3 days each, fix before implementing lazy scrollback):**
- C1: Branch listing timeout + caching (2–3 hours)
- D1: `overscroll-behavior: contain` CSS (1 hour)
- D4: `visualViewport` resize listener for iOS keyboard (3–5 hours)
- E2: Verify combobox Tab behavior in chosen library (during library selection)

**Before shipping lazy scrollback (1 week):**
- A1: Design and test cursor state management around historical write injection
- A3: Implement auto-scroll-to-bottom policy if scrollbar is added

**Before shipping OTel JS (2–3 days):**
- B1: Measure bundle size; implement dynamic import if needed
- B3: Add try-catch around OTel initialization

## Operational Concerns

- **Branch listing:** Add server-side latency histogram to OpenTelemetry (Go backend already instrumented)
- **Mobile touch issues:** Track session error rates by user-agent to detect iOS/Android-specific failures
- **xterm.js performance:** `TerminalMetrics` logging is already in place — add tracking for lazy scrollback chunk loads when implemented
- **OTel JS:** Start with 10% sample rate; monitor for dropped spans or initialization errors

## Prior Art and Lessons Learned

**xterm.js:**
- Cloud terminals (VS Code Remote, Gitpod, Replit) disable xterm scrollbar and manage history in backend — same approach as Stapler ✓
- Known: xterm.js + ResizeObserver can cause resize loops on older Edge/Safari — Stapler already debounces resize ✓
- Known: xterm v5.x fixed several rendering bugs present in v4.x (#3688, #3721)

**go-git:**
- Performance on repos with 1000+ branches is a known limitation (GitHub issue go-git#524)
- Workaround: `git ls-remote` or `git for-each-ref` shell exec is faster than go-git library for branch listing
- Caching branch list is standard practice in Git UIs (VS Code, GitHub Desktop)

**Mobile Touch Events:**
- `overscroll-behavior: contain` is well-supported (iOS 13+, Android 9+) and is the recommended approach
- `touch-action` is standard CSS, but Apple's `user-select` requires WebKit prefixes
- Keyboard viewport resize is unavoidable on iOS; workaround is `visualViewport` API

**OpenTelemetry JS:**
- Bundle size is a known pain point; community recommends dynamic import for non-critical paths
- Sentry/Datadog/New Relic recommend instrumenting only critical paths to avoid overhead

## Open Questions

- [ ] Does xterm.js v5+ expose a public scrollLines API? — blocks: touch scroll manual handler approach [TRAINING_ONLY — verify]
- [ ] Does `overscroll-behavior: contain` work on iOS Safari 15+ inside a fixed-position element? — blocks: whether CSS-only mobile fix is sufficient [TRAINING_ONLY — verify]
- [ ] Are there any large repos (1000+ branches) in active use by the user? — blocks: urgency of branch listing timeout fix
- [ ] What is the current `scrollback` setting in `terminalConfig`? — blocks: whether A2/A3 risks are currently active

## Recommendation

**Top 3 urgent fixes (implement before any new feature work):**

1. **C1 — Branch listing timeout:** Add `context.WithTimeout(ctx, 2*time.Second)` and in-memory 5-minute cache to the `ListBranches` handler. ~3 hours of work, eliminates hang for any large repo user.

2. **D1 — Touch scroll chrome hiding:** Add `overscroll-behavior: contain` to the terminal container CSS. ~1 hour. Immediate improvement on all mobile browsers.

3. **D4 — iOS keyboard viewport:** Wire `visualViewport.addEventListener('resize', ...)` in `TerminalOutput.tsx` with a 300ms debounced fit. ~4 hours. Fixes the most common iOS usability failure.

**Before implementing lazy scrollback:** Design cursor state management around prepend (A1). This is the highest-severity risk that could make the feature unusable after shipping.

**Before shipping OTel JS:** Measure bundle size with and without dynamic import (B1). If bundle exceeds limit, use lightweight custom JSON endpoint approach instead.

## Web Search Results (2026-04-16)

**xterm.js mobile touch (verified):** GitHub issue #5377 (July 2025) confirms mobile touch support is a known unresolved upstream issue. Ballistic scrolling not supported. Touch sends arrow keys instead of scrolling viewport (issue #1007). **D1 risk is confirmed high-severity and high-likelihood.** No CSS-only fix exists; application-level touch interception required.

**OTel JS bundle size (verified):** ~300 KB uncompressed, ~60 KB gzipped. OTel SDK 2.0 (2025) improved tree-shaking. **B1 risk is real** — dynamic import mandatory to avoid blocking first paint.

**overscroll-behavior: contain iOS Safari (verified):** Added in **Safari 16, not 15**. iOS 15 (still in significant use) will not benefit from CSS-only fix. **D1 mitigation must include a JS fallback** for iOS 15 users.

**go-git Branches() vs remote tracking refs (verified):** `Repository.Branches()` only returns `IsBranch() == true` refs, which are only `refs/heads/*`. Remote tracking refs return `IsBranch() == false`. **C2 risk is LOW** — go-git correctly scopes to local branches.

**visualViewport and iOS keyboard (verified):** `visualViewport` API is supported in Safari 13+. **Critical caveat: Safari 15 does not trigger a `resize` event when the virtual keyboard appears.** The `VirtualKeyboard` API (Chrome-only) is not supported in WebKit/Safari. For iOS keyboard detection, `window.visualViewport.onresize` works on iOS 16+ but not reliably on iOS 15. The `dvh` viewport units approach is more robust. **D4 risk is confirmed; `visualViewport` approach has caveats.**

**VS Code scrollback (verified):** Default 1,000 lines (not thousands). Max 50,000. VS Code uses xterm.js's own buffer — no application-level lazy loading, just a hard cap. This confirms our approach of lazy loading + hard cap is the right path.

**git for-each-ref (verified):** p90 ~75ms, p99 ~121ms for 100 branches. Performance degrades severely with `--contains HEAD` but not with plain `git for-each-ref refs/heads`. **C1 risk is medium, not high** — plain `for-each-ref` is fast on typical repos.

## Pending Web Searches

1. `xterm.js prepend lines lazy scrollback cursor rendering GitHub issue` — verify if A1 cursor corruption is documented
2. `xterm.js scrollback option line limit bytes behavior` — confirm A2 behavior (line count not bytes)
3. `go-git Repository Branches remote tracking refs remotes` — verify C2: whether go-git includes remotes in Branches()
4. `opentelemetry sdk-web bundle size gzipped 2025` — exact B1 measurements
5. `overscroll-behavior contain iOS Safari 15 fixed position` — verify D1 CSS support
6. `visualViewport resize iOS Safari keyboard detection 2025` — verify D4 API availability and behavior
7. `xterm.js mouse tracking any mode touch events mobile` — verify D3 interference
8. `git for-each-ref 1000 branches latency benchmark go-git` — confirm C1 timing expectations
