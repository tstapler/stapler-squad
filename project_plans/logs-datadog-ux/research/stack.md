# Stack Research — Logs Datadog UX

Date: 2026-05-14

## 1. Virtualization Libraries

### Candidates Evaluated

| Library | Version | gzipped | Stars | Last release |
|---|---|---|---|---|
| `@tanstack/virtual` (TanStack Virtual v3) | 3.x | ~4 KB | 9k+ | Active |
| `react-window` | 1.8.x | ~6 KB | 16k | Low (2023) |
| `react-virtuoso` | 4.x | ~32 KB | 4.5k | Active |

### Comparison Matrix

| Criterion | `@tanstack/virtual` | `react-window` | `react-virtuoso` |
|---|---|---|---|
| Dynamic / variable row heights | Yes — `measureElement` callback; optimal for expanded rows | Fixed-height only (VariableSizeList requires manual height tracking) | Yes — automatic via ResizeObserver |
| Streaming append (live tail) | Excellent — headless, you control DOM; trivial to call `scrollToIndex(-1)` | Possible but requires ref gymnastics | Built-in `followOutput` prop — handles live tail out of the box |
| Horizontal scroll + sticky gutter | Yes — headless; layout fully under developer control | Awkward — wrapper/outerElement must be customized | Requires workaround; `TableVirtuoso` supports sticky columns only via `fixedHeaderContent` |
| Mobile touch (iOS/Android) | Excellent — uses native scroll container; no synthetic scroll | Excellent — same model | Good — uses native scroll but has known iOS rubber-band edge cases |
| Bundle size (gzipped) | ~4 KB | ~6 KB | ~32 KB |
| TypeScript quality | First-class (written in TS) | Good | Good |
| Headless (own markup) | Yes — renders nothing, only measures/positions | No — renders its own div wrappers | No — renders its own wrapper components |

### Recommendation: `@tanstack/virtual`

**Rationale:**

1. **Dynamic heights are a hard requirement.** Expandable rows mean row heights change at runtime. `react-window` cannot handle this without manual bookkeeping that is brittle under live-tail append. TanStack Virtual's `measureElement` ref callback adapts automatically via `ResizeObserver`.

2. **Horizontal scroll with sticky gutter.** The gutter (line number + level badge) must not scroll horizontally while the message body does. This requires a specific DOM structure that headless `@tanstack/virtual` makes trivial — a CSS Grid wrapper with `position: sticky` columns and an inner horizontally-scrolling region. `react-window` and `react-virtuoso` both inject their own scroll containers and fight this pattern.

3. **Small bundle.** At ~4 KB gzipped it is the smallest option. The project has a 5 MB total JS budget (from `package.json` size-limit config) and every KB matters for mobile.

4. **No existing virtualization code in the project.** No `@tanstack/virtual`, `react-window`, or `react-virtuoso` is installed. This will be a new dependency regardless of choice; TanStack Virtual's approach is the most future-proof given the project's existing use of TanStack-adjacent patterns (the package set suggests familiarity: `react-hook-form`, `zod`, etc.).

**`react-virtuoso` trade-off:** Its `followOutput` live-tail API is attractive, but the 32 KB bundle cost and reduced layout control are disqualifying for the sticky-gutter requirement.

---

## 2. Search and Highlight

### Candidates Evaluated

| Option | Description | gzipped |
|---|---|---|
| Native `<mark>` + regex split | Zero-dep; built into HTML | 0 KB |
| `react-highlight-words` | React component wrapping word tokenization | ~3 KB |
| `fuse.js` (already installed) | Fuzzy search + match offsets | ~5 KB |

### Comparison Matrix

| Criterion | Native mark+regex | `react-highlight-words` | `fuse.js` |
|---|---|---|---|
| Exact substring match | Yes | Yes | Via `isFuzzy: false` + `ignoreLocation: true` |
| Fuzzy match | No | No | Yes |
| Match offset info (for highlight) | Yes (regex.exec) | Yes (internal) | Yes (`.matches[].indices`) |
| Performance at 10k lines | Excellent — regex on pre-filtered strings | Same | Fuse index builds ~10ms for 10k strings; search ~5ms |
| Bundle size | 0 KB | ~3 KB | Already installed |
| Case-insensitive | Yes | Yes | Yes |
| Highlight wraps in `<mark>` | Manual | Built-in | Manual (from indices) |

### What is already installed

**`fuse.js` v7.3.0** is already a production dependency, used in `useSessionSearch.ts` for session fuzzy search. No new dependency is needed.

However, for log search, **exact substring matching is preferred** — users expect log grep semantics, not fuzzy session-title search. Fuse.js supports this with `threshold: 0` and `useExtendedSearch: true` (`'"exact phrase"` syntax), but the simpler implementation is:

1. **Filter rows:** Run a case-insensitive `String.prototype.includes()` (or a compiled `RegExp`) over the 10k log rows. At 10k rows of ~100-char strings, this completes in < 5ms without any library.
2. **Highlight matches:** Use a regex `.split()` + `.map()` to wrap matches in `<mark>` elements. This is zero-dependency.
3. **Fuzzy/advanced search (future):** Fuse.js is on-hand if fuzzy matching is ever needed — no new dep required.

### Recommendation: Zero-dependency regex filter + `<mark>` highlight

Use the native browser string API for filtering. Wrap matched substrings in `<mark>` tags via a utility function. Reserve Fuse.js for a later "smart search" mode (fuzzy, multi-field).

**Performance:** `String.prototype.includes` on 10k strings: < 5ms. RegExp split+map for highlight rendering: only applied to visible (virtualized) rows, typically 20–50 at a time. Total path: well under the 100ms NFR-1 requirement.

---

## 3. What is Already Available in the Project

| Feature | Library | Already installed? | Notes |
|---|---|---|---|
| ANSI rendering | `ansi-to-html` v0.7.2 | Yes | Used in `useTerminalSnapshot.ts` with `escapeXML: true` for safe HTML output. Log messages from the application logger may contain ANSI codes (the stress test generator in `src/lib/test-generators/log-lines.ts` emits ANSI color codes). The logs page currently strips nothing — consider running log lines through `ansi-to-html` or stripping codes server-side. |
| Fuzzy search | `fuse.js` v7.3.0 | Yes | Can be used for advanced search mode at no additional cost |
| ConnectRPC streaming | `@connectrpc/connect` v2.1.1 | Yes | Existing `getLogs` RPC + live tail pattern in `useLiveTail` hook |
| Live tail hook | `useLiveTail` | Yes (internal) | Polling-based; used in logs page already |
| Debounce | `useDebounce` | Yes (internal) | In `src/lib/hooks/useDebounce.ts`; used for search input |
| CSS framework | `@vanilla-extract/css` v1.20.1 | Yes | All new component styles must go in `.css.ts` files |
| `<mark>` element | Native | Browser built-in | Supported in all target browsers including iOS Safari |

---

## 4. ANSI Escape Sequence Assessment

**Terminal session logs** (xterm.js output): Full ANSI codes present — handled by xterm.js rendering; not applicable to the log viewer which shows structured application logs, not terminal PTY output.

**Application logs** (`~/.stapler-squad/logs/`): The Go backend log entries are likely plain text (slog/zap structured JSON). The stress-test generator emits ANSI codes, but production app logs from `getLogs` RPC are structured `LogEntry` protobuf messages with `level`, `message`, `source`, `timestamp` fields — the message field is expected to be plain text.

**Recommendation:** Do not add ANSI stripping to the log viewer component. If ANSI codes appear in production log messages, strip them server-side in the `getLogs` handler using a simple regex (`\x1b\[[0-9;]*m`). `ansi-to-html` is available if rendering colored ANSI output becomes a requirement.

---

## 5. Risks and Trade-offs

| Risk | Severity | Mitigation |
|---|---|---|
| `@tanstack/virtual` has no `followOutput` — live tail scroll must be hand-rolled | Low | `scrollToIndex` + `isScrolling` observer is well-documented; project already does scroll management in `TerminalOutput.tsx` |
| Dynamic row heights require `measureElement` ref on each row; expanded rows cause layout reflow | Medium | Debounce measurement with `requestAnimationFrame`; only measure visible rows |
| Horizontal scroll + sticky gutter is a non-trivial CSS layout | Medium | CSS Grid with `position: sticky` left column is well-supported on iOS Safari 15+ and Android Chrome 80+ |
| `@tanstack/virtual` + vanilla-extract styling requires custom CSS class names on virtualizer items | Low | Headless model means no style conflicts |
| `ansi-to-html` uses `dangerouslySetInnerHTML` — XSS risk if `escapeXML: false` | High if misconfigured | Always use `escapeXML: true` (current project pattern in `useTerminalSnapshot.ts`) |
| No existing virtual scroll component to reuse | Low | This is the first virtualized list; the implementation becomes the canonical pattern |
| iOS Safari rubber-band overscroll can misreport scroll position | Low | Use `overscroll-behavior: contain` on the scroll container; test on real device |

---

## 6. Recommended Stack Summary

| Concern | Library / Approach | New dep? |
|---|---|---|
| Virtual scrolling | `@tanstack/virtual` v3 | Yes (new) |
| Search filtering | Native `String.includes` + RegExp | No |
| Search highlighting | Native `<mark>` + regex split in React | No |
| Fuzzy/advanced search (future) | `fuse.js` (already installed) | No |
| ANSI in log messages | Strip server-side OR ignore (app logs are plain text) | No |
| Live tail scroll | Custom hook using TanStack `scrollToIndex` + scroll event | No |
| CSS styling | `@vanilla-extract/css` + `vars` tokens (project standard) | No |

**New dependency count: 1** (`@tanstack/virtual`). All other capabilities are met by existing dependencies or native browser APIs.

### Why not use xterm.js for this?

xterm.js (already installed) renders PTY terminal output — it is optimized for cursor-addressable, full-screen terminal emulation. A log viewer needs:
- Row-level interaction (click to expand, tap to copy)
- Structured column layout (timestamp, level badge, message)
- In-React search highlighting
- Touch-native horizontal swipe gestures

xterm.js provides none of these — it renders to a canvas/DOM controlled by the terminal emulator, not React. The correct tool is a virtualized React list.
