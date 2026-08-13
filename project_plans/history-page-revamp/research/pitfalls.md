# History Page Revamp — Known Failure Modes & Pitfalls

**Research date**: 2026-04-12
**Scope**: Virtual scrolling, git worktree fork, ANSI preview stripping, stale cache invalidation, Next.js App Router + ConnectRPC data fetching.

---

## 1. Virtual Scroll — Dynamic Height Expand/Collapse

### Failure modes

**Upward scroll stutter and jump (unresolved in TanStack Virtual)**
When items have dynamic heights and the user scrolls backward (upward), the virtualizer continuously re-measures elements as they enter the viewport. Each measurement produces a size delta that triggers a scroll-position correction, which then triggers another measurement. On macOS Chrome and iOS Safari this compounding cycle produces severe stuttering. Downward scrolling is unaffected because the correction direction does not conflict with the scroll direction.
- Source: [Issue #659](https://github.com/TanStack/virtual/issues/659), [Issue #832](https://github.com/TanStack/virtual/issues/832)

**Item jumps upward on expand (was a regression, now partially fixed)**
Expanding an item that is partially obscured by the top of the scroll window causes the item to move upward instead of growing downward. The root cause is the auto-adjustment logic that fires on every size change and pushes the scroll position to compensate — but when the expanding item straddles the viewport edge, the correction overshoots. PR #1002 ("Adapt default logic to adjust scroll position only on backward scrolling") was merged to narrow the trigger condition, but the fix requires callers to keep `shouldAdjustScrollPositionOnItemSizeChange` in its default state.
- Source: [Issue #562](https://github.com/TanStack/virtual/issues/562)

**`scrollToIndex` unreliable with dynamic heights**
Programmatic scrolling to a specific index (e.g., "jump to session 500") is unreliable when sizes are unknown. During smooth-scroll animation the virtualizer deliberately skips measuring off-screen items to avoid mid-animation jumps, so it lands near — but not exactly at — the target. The final position can be off by the sum of estimation errors for all unmeasured items between the current position and the target.
- Source: [Issue #468](https://github.com/TanStack/virtual/issues/468), [TanStack Virtual Docs](https://tanstack.com/virtual/latest/docs/api/virtualizer)

**Keyboard navigation is unaddressed by the library**
TanStack Virtual ships no built-in keyboard navigation. Focus management, `aria-rowindex`, and `aria-setsize` must be wired manually. When an item expands and shifts the DOM, focused elements can lose focus entirely if the virtualizer unmounts and remounts the row.

### Mitigations

1. **Overestimate `estimateSize`**: return the largest plausible row height (e.g., the fully-expanded preview height) so that scroll-position math under-corrects rather than over-corrects. Shrinking is less disruptive than growth.
2. **Use `shouldAdjustScrollPositionOnItemSizeChange` surgically**: set it to `() => false` while an expand animation is in flight, then reset it once `ResizeObserver` reports the stable size. This prevents the correction loop during the transition.
   ```tsx
   const adjustRef = useRef(true);
   const virtualizer = useVirtualizer({
     shouldAdjustScrollPositionOnItemSizeChange: () => adjustRef.current,
     ...
   });
   function handleExpand(index) {
     adjustRef.current = false;
     setExpanded(index);
     // reset after one paint cycle
     requestAnimationFrame(() => { adjustRef.current = true; });
   }
   ```
3. **Use block-translation positioning**: position the rendered block by translating the container by `items[0].start` rather than absolutely positioning each item. This reduces per-item reflow cost during batch re-measurement.
4. **Increase `overscan`** to 10–20 for pages with unpredictable heights. This pre-renders items before they enter the viewport and reduces the chance of a visible measurement flash.
5. **Keep `measureElement` stable**: attach it via a stable `useCallback` ref on the DOM node, not an inline function. Recreating the ref on every render triggers spurious measurements.
6. **Keyboard focus**: maintain a `focusedIndex` in state, scroll to it on arrow key events via `virtualizer.scrollToIndex`, and restore focus after React reconciles using `useEffect(() => { rowRef.current?.focus() }, [focusedIndex])`.

---

## 2. Git Worktree Fork Conflicts

### Failure modes

**Branch already checked out in another worktree**
`git worktree add <path> <branch>` fails with:
```
fatal: '<branch>' is already checked out at '<other-path>'
```
Git enforces single-ownership: a branch can only be the HEAD of one worktree at a time. This is the most common error when a user tries to fork/resume a session whose branch is still live in another active session.
- Source: [git-scm.com docs](https://git-scm.com/docs/git-worktree), [vibe-kanban issue #306](https://github.com/BloopAI/vibe-kanban/issues/306)

**Stale worktree metadata — "reference is already checked out" after directory deletion**
If a worktree directory is removed from the filesystem without running `git worktree remove`, git's internal refs still show the branch as checked out. Subsequent `git worktree add` calls for the same branch fail even though no actual worktree exists at that path. `git worktree list` shows the dead entry with `(bare)` or a missing-path indicator.
- Source: [vibe-kanban issue #306](https://github.com/BloopAI/vibe-kanban/issues/306)

**Detached HEAD in the source session**
If the source session's worktree is in detached HEAD state (e.g., checked out a specific commit rather than a branch), there is no branch name to fork from. `git worktree add -b <new-branch> <path>` must be used with an explicit commit SHA instead of a branch ref, and the caller must decide what "resume from detached HEAD" means semantically.

**New branch lands on wrong base commit**
A secondary failure mode found in the wild: when `git worktree add -b <new> <path>` is issued without an explicit starting-point commit, git defaults to HEAD of the current working tree's branch — which may itself be behind `origin/main`. The new worktree starts from a stale base, not the intended source-session snapshot.
- Source: [vibe-kanban issue #306](https://github.com/BloopAI/vibe-kanban/issues/306)

### Mitigations

1. **Detect before acting**: before calling `git worktree add`, run `git worktree list --porcelain` and parse the output. If the target branch appears and its path exists, surface a UI error. If the path is missing, run `git worktree prune` first.
2. **Always fork with a unique generated branch name**: never reuse the source session's branch name. Append a timestamp or UUID: `resume/<source-branch>-<timestamp>`. This sidesteps the checked-out conflict entirely.
   ```go
   newBranch := fmt.Sprintf("resume/%s-%d", sourceBranch, time.Now().Unix())
   cmd := exec.Command("git", "worktree", "add", "-b", newBranch, worktreePath, commitSHA)
   ```
3. **Pass an explicit commit SHA** as the starting point so the new worktree always reflects the exact snapshot of the source session, regardless of what HEAD is doing elsewhere.
4. **Prune stale entries proactively**: run `git worktree prune` at application startup and before any fork operation. It is idempotent and fast.
5. **Handle detached HEAD explicitly**: detect `git rev-parse --abbrev-ref HEAD` returning `HEAD` and present a UI warning. The fork should create a new branch from the SHA, not from a branch ref.
6. **Lock file errors**: if `.git/worktrees/<name>/locked` exists, the worktree is locked and cannot be removed. Check for and surface this file path in error messages.

---

## 3. ANSI Escape Codes in Message Previews

### Failure modes

**OSC sequences survive naive SGR-only strippers**
Most regex-based strippers target `ESC[...m` (SGR / color codes). OSC sequences (`ESC]...\x07` or `ESC]\x1b\\`) are structurally different and pass through incomplete strippers unchanged. In terminal output, OSC 8 hyperlinks (`ESC]8;;https://...\x07`) are common from tools like Claude. Rendered as plain text, they appear as garbage characters around URLs.
Example raw bytes that survive SGR-only stripping: `\x1b]8;;https://example.com\x07click here\x1b]8;;\x07`

**BEL-terminated vs ST-terminated OSC ambiguity**
For historical reasons xterm accepts both `\x07` (BEL) and `\x1b\\` (ST, String Terminator) to end OSC sequences. A stripper that only looks for ST will leak the BEL-terminated variant and vice versa. Claude's terminal output uses BEL-terminated hyperlinks.
- Source: [ANSI escape code — Wikipedia](https://en.wikipedia.org/wiki/ANSI_escape_code)

**Incomplete / truncated sequences at buffer boundaries**
When terminal output is chunked (e.g., a scrollback ring buffer cut mid-sequence), the tail of one chunk may be `ESC[` with no final byte yet. A naive stripper's regex matches only complete sequences, so the dangling `ESC[` leaks into the rendered text. Concatenation at chunk boundaries is required before stripping.

**Colon-separated RGB color codes**
Modern terminals use `ESC[38:2::R:G:Bm` (colon-separated, not semicolon) for 24-bit color. Older strip-ansi regex patterns only accept semicolons as parameter separators and miss these, rendering the raw codes as text. The `ansi-regex` package (used by `strip-ansi`) does handle colon separators, but pinning old versions (pre-6.x) risks this gap.
- Source: [DeepWiki ansi-regex CSI sequences](https://deepwiki.com/chalk/ansi-regex/4.2-csi-sequences)

**DCS, APC, PM sequences (not matched by ansi-regex)**
`ansi-regex` intentionally does not match DCS (Device Control String), APC (Application Program Command), or PM (Privacy Message) sequences to avoid false positives. These can appear in tmux output (`ESC P...ST`) and will survive stripping. While typically invisible in terminals, they may render as `^[P...` garbage in plain text.

**Cursor-movement and erase sequences in previews**
Terminal output from interactive CLI tools includes cursor movement (`ESC[A`, `ESC[2K`), erase-to-end-of-line, and carriage-return overwrite sequences. Stripping the ANSI codes removes the control characters but leaves the overwritten content (e.g., progress bars that rewrite the same line appear as many concatenated lines of partial text). The last written state of each line must be reconstructed, not just the raw bytes stripped.

### Mitigations

1. **Use a full-spectrum stripper**: use `strip-ansi` v7+ (ESM) or the Go equivalent `github.com/acarl005/stripansi` / `github.com/muesli/termenv` which handle SGR, OSC (BEL and ST terminated), CSI cursor ops, and colon-separated RGB parameters.
2. **Strip before chunking, or reassemble before stripping**: never strip mid-sequence. If using a ring buffer, ensure sequences are atomic within a buffer segment, or concatenate adjacent segments before passing to the stripper.
3. **Process line overwrites before stripping**: if the preview is meant to show final output state, replay the raw bytes through a minimal VT100 line-buffer (just CR/LF/backspace/erase handling) to reconstruct the final visible line content, then strip ANSI from that. Libraries like `vt10x` (Go) or `xterm.js` headless mode can do this.
4. **Truncate safely**: when truncating to a preview length (e.g., last 3–5 messages), truncate on byte boundaries after stripping, not before, to avoid cutting inside a multi-byte sequence.
5. **Test corpus**: include tmux output, `claude` CLI output, and `git diff --color` output in your test fixtures since these exercise the full range of problematic sequences.

---

## 4. Stale History / Cache Invalidation

### Failure modes

**TTL cache serves old data while new sessions complete**
If the history list is fetched server-side and cached with a TTL (e.g., Next.js `unstable_cache` or `fetch` with `revalidate: 60`), a session that completes while the page is open will not appear until the next revalidation interval. The user sees a list that is missing their most recent work.

**Next.js Router Cache prevents re-fetch after revalidation (known bug)**
Next.js App Router maintains a client-side Router Cache with a 30-second TTL for dynamic routes. After the first cache invalidation cycle, a bug (present through Next.js 14.0.x, fixed in PR #61573) caused the stale entry to remain stale indefinitely — fetching fresh data on every navigation instead of re-caching it. Symptoms: the page flickers on every navigation as data re-fetches, even after revalidation should have settled.
- Source: [Next.js issue #58723](https://github.com/vercel/next.js/issues/58723), [Next.js caching discussion #54075](https://github.com/vercel/next.js/discussions/54075)

**`router.refresh()` polling defeats Server Component caching**
A common workaround is to call `router.refresh()` on a timer inside `useEffect` to keep the history list current. This bypasses the Router Cache, forces a full round-trip on every poll interval, and negates the performance benefits of Server Components. It also produces a full re-render of every component in the subtree on each tick.

**ConnectRPC streaming race with cached initial load**
If the page loads the history list via a cached Server Component fetch but then opens a ConnectRPC streaming subscription for live updates, the two data sources can diverge. The initial render shows the cached snapshot; the stream sends incremental updates on top of it. If the cache is 45 seconds stale at load time, the first few stream events may reference session IDs or state transitions that the client has never seen.

### Mitigations

1. **Tag-based revalidation for explicit invalidation**: use `fetch` with `{ next: { tags: ['history'] } }` and call `revalidateTag('history')` from the Server Action that creates or completes a session. This gives immediate invalidation without polling.
   ```typescript
   // server action
   export async function forkSession(id: string) {
     "use server";
     await backendFork(id);
     revalidateTag('history');
   }
   ```
2. **Short TTL + stale-while-revalidate for background refresh**: use `revalidate: 30` with a `<Suspense>` wrapper so the page loads instantly from cache while the background revalidation runs. For a history page this is an acceptable tradeoff (sessions complete on the order of minutes, not seconds).
3. **Optimistic update on fork/resume action**: when the user triggers fork/resume, add the new session to local state immediately before the server round-trip completes. On error, roll back. This is the most responsive UX and decouples perceived freshness from actual cache TTL.
4. **ConnectRPC: full snapshot on connect, deltas via stream**: the backend should send the full current session list as the first message on a new streaming connection, then send only changed/added sessions as subsequent messages. This ensures the client is never dependent on cache coherence between the SSR snapshot and the stream.
5. **Avoid `router.refresh()` polling**: if live updates are needed, use a ConnectRPC server-streaming RPC or a WebSocket. If SSR freshness is needed, use `revalidateTag`. Never combine both with a polling loop — this creates triple-fetch scenarios.
6. **Upgrade Next.js past 14.0.x**: the Router Cache stale-after-first-invalidation bug (issue #58723) is fixed. Pinning to an affected version will cause persistent stale-data symptoms regardless of revalidation logic.

---

## 5. Next.js App Router + `useEffect` with ConnectRPC

### Failure modes

**`useEffect` infinite loop from unstable client ref**
If the ConnectRPC transport or client is constructed inside the component body (not in a `useRef` or module-level singleton), it gets a new object identity on every render. Any `useEffect` that lists the client as a dependency re-runs on every render, triggering a fetch, which updates state, which triggers a re-render, which recreates the client, which triggers the effect again.
```tsx
// BAD — new transport object every render
function HistoryPage() {
  const transport = createConnectTransport({ baseUrl: '/api' }); // new ref each render
  useEffect(() => {
    fetchHistory(transport); // runs on every render
  }, [transport]); // ← transport is always "new"
}
```

**`useEffect` infinite loop from object/array deps**
Passing an options object literal or array literal as a `useEffect` dependency always fails reference equality. ConnectRPC request objects (e.g., `new ListSessionsRequest({ limit: 50 })`) constructed inline exhibit this.

**Hydration mismatch from streaming SSR + client state**
Server Components render the initial history snapshot. If a Client Component immediately fetches via ConnectRPC in a `useLayoutEffect` or `useEffect` and updates state before hydration completes, React 18's concurrent hydration can produce a mismatch between server HTML and the first client render.

**Double-fetch in StrictMode**
React 18 StrictMode (active in Next.js dev mode) intentionally mounts/unmounts/remounts components. A `useEffect` that initiates a ConnectRPC streaming call will open the stream, close it, and open it again. If the backend does not handle rapid connect/disconnect gracefully, this produces spurious errors or duplicate state entries.

**`startTransition` / Suspense boundary interaction**
`useEffect` does not participate in Suspense. If the history fetch is wrapped in a `<Suspense>` for loading state, but the actual fetch is triggered by `useEffect` rather than an async Server Component or `use(promise)`, the Suspense boundary never activates and there is no loading indicator.

### Mitigations

1. **Construct the transport once**: create the ConnectRPC transport at module level or in a React context provider. Never construct it inside a component body.
   ```tsx
   // module-level singleton
   const transport = createConnectTransport({ baseUrl: '/api' });
   
   // or in a context
   const TransportContext = createContext(transport);
   ```
2. **Stable dependency identity**: use `useRef` for the client object. If using `useCallback`/`useMemo` to create request objects, ensure all inputs are primitives.
   ```tsx
   const clientRef = useRef(createPromiseClient(SessionService, transport));
   useEffect(() => {
     const client = clientRef.current; // stable ref, not a dep
     // fetch...
   }, []); // no client dependency needed
   ```
3. **Prefer Server Component data loading over `useEffect` for initial load**: fetch the history list in a `async` Server Component and pass it as props or children. Reserve Client Component + `useEffect` only for live-update subscriptions that cannot be done server-side.
4. **Handle StrictMode double-invocation**: return a cleanup function from `useEffect` that cancels/aborts the ConnectRPC stream. Use `AbortController`:
   ```tsx
   useEffect(() => {
     const ac = new AbortController();
     streamHistory(client, { signal: ac.signal });
     return () => ac.abort();
   }, []);
   ```
5. **Use `useSyncExternalStore` for live history state**: wrap the ConnectRPC stream in a store abstraction and use `useSyncExternalStore` rather than `useState` + `useEffect`. This avoids the tearing problem with concurrent rendering and gives a clear subscribe/unsubscribe contract.
6. **Hydration safety**: if the Server Component renders the initial session list and the Client Component subscribes to updates, initialize client state from the server-rendered props, not from a fresh fetch. This avoids the hydration mismatch.
   ```tsx
   // Client Component
   function LiveHistoryList({ initialSessions }: { initialSessions: Session[] }) {
     const [sessions, setSessions] = useState(initialSessions); // ← seeded from server
     useEffect(() => { /* subscribe to stream, merge updates */ }, []);
   }
   ```

---

## Sources

- [TanStack Virtual Issue #562 — item jumps upward on expand](https://github.com/TanStack/virtual/issues/562)
- [TanStack Virtual Issue #659 — scroll stutter with dynamic heights](https://github.com/TanStack/virtual/issues/659)
- [TanStack Virtual Issue #832 — lag/stutter in dynamic height data grid](https://github.com/TanStack/virtual/issues/832)
- [TanStack Virtual Issue #468 — scrollToIndex with dynamic height](https://github.com/TanStack/virtual/issues/468)
- [TanStack Virtual Discussion #1013 — shouldAdjustScrollPositionOnItemSizeChange for chat UIs](https://github.com/TanStack/virtual/discussions/1013)
- [TanStack Virtual Docs — Virtualizer API](https://tanstack.com/virtual/latest/docs/api/virtualizer)
- [git-worktree documentation — git-scm.com](https://git-scm.com/docs/git-worktree)
- [vibe-kanban Issue #306 — "reference is already checked out" with stale worktrees](https://github.com/BloopAI/vibe-kanban/issues/306)
- [ANSI escape code — Wikipedia](https://en.wikipedia.org/wiki/ANSI_escape_code)
- [ansi-regex CSI sequences — DeepWiki](https://deepwiki.com/chalk/ansi-regex/4.2-csi-sequences)
- [chalk/strip-ansi — GitHub](https://github.com/chalk/strip-ansi)
- [Next.js App Router caching deep-dive — Discussion #54075](https://github.com/vercel/next.js/discussions/54075)
- [Next.js Router Cache not re-caching after invalidation — Issue #58723](https://github.com/vercel/next.js/issues/58723)
- [Next.js caching documentation](https://nextjs.org/docs/app/deep-dive/caching)
