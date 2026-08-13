# History Page Revamp — Stack Research

**Date**: 2026-04-12
**Goal**: Render initial history list in ≤200ms; scroll 200+ sessions without jank.
**Stack**: Next.js App Router (React 18) + Go backend (ConnectRPC)

---

## 1. Virtualization Library

### Recommendation: TanStack Virtual v3

**Decision**: Use `@tanstack/react-virtual` v3.

### Comparison

| Criterion | react-window | react-virtuoso | TanStack Virtual v3 |
|---|---|---|---|
| Bundle size (minzipped) | ~6 KB | ~25 KB | ~10–15 KB |
| Next.js App Router compat | Yes (RSC-safe, no bundler config) | Yes | Yes (headless, no DOM assumptions) |
| Dynamic/variable item heights | Manual (VariableSizeList) | Built-in, automatic | Built-in (`measureElement`) |
| Scroll position on expand/collapse | Requires manual scroll correction | Built-in `followOutput` / stable anchoring | Stable with `scrollToIndex` + dynamic measurement |
| Keyboard navigation | Not built-in; must implement | Partial built-in | Not built-in; must implement (headless) |
| Maintenance status (2025) | Low activity; last major release 2020 | Actively maintained | Actively maintained (v3.13.23, updated April 2026) |
| API surface | Component-based | Component-based | Hook-based, headless |

### Key Tradeoffs

**TanStack Virtual wins on flexibility and maintenance.** Its headless hook API (`useVirtualizer`) means zero DOM opinions — the component tree is entirely yours, which is ideal for App Router where server and client components must coexist. There is no bundler or Babel config required.

The headless design means **keyboard navigation must be implemented manually** (e.g., `aria-activedescendant` + `onKeyDown` on the scroll container). This is ~20 lines of code and is the standard approach used in accessible list implementations.

**react-window** is the smallest but is effectively unmaintained for new features (no variable-height-with-dynamic-measurement support without the separate `react-virtualized-auto-sizer` helper). It does not handle items that change height at runtime (e.g., session cards that expand to show details).

**react-virtuoso** is the most batteries-included (built-in sticky headers, grouping, follow-output). If the history list needs grouped/sticky section headers (e.g., "Today / Yesterday / This week"), react-virtuoso is worth the extra ~15 KB. Otherwise TanStack Virtual is leaner.

**For this use case** (flat list of session cards, expandable detail rows, no sticky groups), TanStack Virtual is the correct pick.

### Sources
- [TanStack Virtual docs](https://tanstack.com/virtual/latest/docs/introduction)
- [TanStack Virtual npm (v3.13.23)](https://www.npmjs.com/package/@tanstack/react-virtual)
- [TanStack Virtual vs React-Window comparison — Borstch](https://borstch.com/blog/development/comparing-tanstack-virtual-with-react-window-which-one-should-you-choose)
- [TanStack Virtual vs React-Window for sticky table grids — Medium](https://mashuktamim.medium.com/react-virtualization-showdown-tanstack-virtualizer-vs-react-window-for-sticky-table-grids-69b738b36a83)
- [react-window vs react-virtuoso — DEV Community](https://dev.to/sanamumtaz/react-virtualization-react-window-vs-react-virtuoso-8g)
- [TanStack/virtual GitHub discussion #459 — comparison with react-window/react-virtualized](https://github.com/TanStack/virtual/discussions/459)

---

## 2. Pagination Strategy

### Recommendation: Cursor-based pagination via unary ConnectRPC calls

**Decision**: Use cursor-based pagination with a unary RPC (`ListClaudeHistory`). Return a `next_page_token` (opaque cursor string) in the response. Do **not** use server-streaming for the initial page load.

### Analysis

#### Cursor vs. Offset

| Criterion | Offset pagination | Cursor pagination |
|---|---|---|
| Consistency during inserts/deletes | Poor (items shift) | Stable (cursor tracks position) |
| Performance on large datasets | Degrades (full scan to offset) | O(log n) with index seek |
| Complexity | Trivial to implement | Slightly more complex |
| Resumability (back button, restore scroll) | Easy (page number in URL) | Requires storing cursor in URL/state |
| Used by major APIs | Rarely for large lists | GitHub, Twitter/X, Stripe, Facebook |

Cursor pagination is the correct choice here. History sessions are append-only in practice (new sessions are always added at the head), so offset pagination would cause items to shift between page loads, producing duplicates or gaps. Cursor pagination eliminates this.

#### Unary RPC vs. Server Streaming

Server streaming (`ListClaudeHistory` as a stream) has an appealing mental model — push rows to the client as they are read — but it introduces significant complexity:

1. **HTTP/1.1 fallback**: ConnectRPC supports streaming over HTTP/2 but falls back to a long-poll envelope format over HTTP/1.1. In a browser/Next.js context, HTTP/2 is not always negotiated for local dev, making stream cancellation and error recovery harder to reason about.
2. **Initial render latency**: A streaming approach still requires the client to wait for the first message before it can render. A well-tuned unary call returning the first 50 rows is faster to first paint because the response is a single buffered write from the server.
3. **Simplicity**: Unary RPCs compose naturally with React's data-fetching model (SWR, React Query, or direct `fetch`) and require no special streaming client state.

**Recommended page size**: 50 items for the first page. This keeps the Go handler under 10ms for typical SQLite/file-system reads and fits the 200ms render budget with margin.

**Loading strategy**:
- Fetch page 1 on mount (50 items).
- Virtualizer renders immediately; the rest of the list appears as the user scrolls near the bottom (load-more trigger at 80% scroll depth).
- Cache pages in React Query / SWR keyed by cursor token.

#### Proto shape (sketch)

```protobuf
message ListClaudeHistoryRequest {
  int32 page_size    = 1; // default 50, max 200
  string page_token  = 2; // empty = first page
}

message ListClaudeHistoryResponse {
  repeated HistoryEntry entries  = 1;
  string next_page_token         = 2; // empty = last page
  int32  total_count             = 3; // optional, for progress display
}
```

### Sources
- [ConnectRPC Go streaming docs](https://connectrpc.com/docs/go/streaming/)
- [Cursor pagination guide for Go (PostgreSQL/MySQL)](https://bun.uptrace.dev/guide/cursor-pagination.html)
- [How to implement cursor-based pagination in Go REST APIs — OneUptime](https://oneuptime.com/blog/post/2026-01-25-cursor-based-pagination-go-rest-apis/view)
- [ConnectRPC connect-go GitHub](https://github.com/connectrpc/connect-go)

---

## 3. ANSI Stripping

### Recommendation: `strip-ansi` v7 (ESM, pure regex, no native deps)

**Decision**: Use the `strip-ansi` npm package to strip ANSI escape codes before passing text to React. Render the result as plain text (`{text}` in JSX), not HTML. Do not use `dangerouslySetInnerHTML` for previews.

### Analysis

#### Library options

| Package | Minzipped | Approach | Maintenance |
|---|---|---|---|
| `strip-ansi` v7 | ~0.3 KB | Regex via `ansi-regex` | Actively maintained (261M weekly downloads; v7.2.0 published April 2026) |
| `ansi-to-html` | ~5 KB | Converts to `<span>` HTML | Active, but produces HTML (see security note) |
| Custom regex | 0 KB | Write your own | Risk of incomplete coverage |

`strip-ansi` is the correct choice for **preview text** (single-line truncated summaries). It removes all ANSI/VT100 escape sequences via a well-tested regex ([`ansi-regex`](https://www.npmjs.com/package/ansi-regex)) and returns a plain string.

#### Security note

ANSI escape sequences in terminal output can embed arbitrary byte sequences including OSC (Operating System Command) codes that carry URLs or file paths. Converting ANSI to HTML (`ansi-to-html`) and then using `dangerouslySetInnerHTML` creates an XSS vector unless the output is sanitized with DOMPurify afterward. For **preview snippets** (one-line summaries), converting to HTML is unnecessary — stripping and rendering as plain text is the safest path.

If full ANSI color rendering is needed in the expanded session detail view (not the list preview), use `ansi-to-html` **plus** DOMPurify before passing to `dangerouslySetInnerHTML`.

#### Supply chain note

In September 2025, `strip-ansi`, `ansi-regex`, and ~20 other packages were briefly compromised via a phishing attack on their maintainer. The malicious versions were live for ~2.5 hours before removal. All versions currently published have been verified clean. Pin the exact version in `package.json` (e.g., `"strip-ansi": "7.2.0"`) and lock with `package-lock.json` / `yarn.lock` as standard practice.

#### Usage pattern

```ts
import stripAnsi from 'strip-ansi';

// In the list row component — safe plain-text preview
const preview = stripAnsi(entry.lastMessage).slice(0, 120);
// Render: <span>{preview}</span>  ← no dangerouslySetInnerHTML needed
```

### Sources
- [strip-ansi npm](https://www.npmjs.com/package/strip-ansi)
- [ansi-regex npm](https://www.npmjs.com/package/ansi-regex)
- [ansi-regex Snyk security page](https://security.snyk.io/package/npm/ansi-regex)
- [StepSecurity: 20+ popular NPM packages compromised (September 2025)](https://www.stepsecurity.io/blog/20-popular-npm-packages-compromised-chalk-debug-strip-ansi-color-convert-wrap-ansi)
- [React XSS prevention — Pragmatic Web Security](https://pragmaticwebsecurity.com/articles/spasecurity/react-xss-part3)
- [DOMPurify — OWASP XSS cheat sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross_Site_Scripting_Prevention_Cheat_Sheet.html)

---

## Summary Table

| Question | Recommendation | Key reason |
|---|---|---|
| Virtualization library | **TanStack Virtual v3** | Headless, RSC-safe, actively maintained, handles dynamic heights |
| Pagination strategy | **Cursor-based unary RPC, 50 items/page** | Stable ordering, fast first page, simpler than streaming |
| ANSI stripping | **`strip-ansi` v7 + plain JSX render** | Smallest footprint, eliminates XSS risk by avoiding HTML entirely |
