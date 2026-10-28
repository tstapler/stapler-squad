# GetFeatureFlags call trace (Task 1.1.1b)

Question: which code path fires the 14 identical `GetFeatureFlags` calls in the baseline HAR (`requirements.md`, `architecture.md:21`)?

## Method

- Build: HEAD `8b2198d63` (Next web build via `make web-build`, Go binary to `~/.stapler-squad/manual-builds/manual-2/`).
- Instance: `STAPLER_SQUAD_INSTANCE=claude-manual-ff2`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-2`, `PORT=62873`, fresh (no sessions, no notifications).
- Headless Chromium (Playwright), `localStorage["ssq:perf"]="trace"` set before load (`web-app/src/lib/perf/perfDebug.ts:40-62`). `console.trace` stacks read over CDP `Runtime.consoleAPICalled`; network calls counted via `page.on("request")` on URLs containing `GetFeatureFlags`.
- Scripts (not committed, scratch): `/tmp/w2/{trace,multi,par}.mjs`.

## Call sites (VERIFIED by grep: the only non-test callers of `getFeatureFlags`)

| Site | Trace label | Cache / dedupe |
|---|---|---|
| `web-app/src/lib/contexts/FeatureFlagsContext.tsx:106` (provider `fetchFlags`, run from the mount effect at `:128`) | `FeatureFlagsContext.getFeatureFlags` | none shared with the hook below; one call per provider mount |
| `web-app/src/lib/hooks/useFeatureFlagValue.ts:26` (`loadFlags`, called from each hook instance's mount effect and every 30 s) | `useFeatureFlagValue.loadFlags` | module-level 30 s TTL cache + in-flight promise (`:15-16`, `:27-42`) shared across hook instances within one document |

## Results

Single document, each route loaded fresh (`/`, `/sessions` (redirects to `/`), `/notifications`, `/settings`, `/review-queue`, `/files`, `/backlog`, `/history`), 6 s settle: every route gave the identical result.

| Metric | Value |
|---|---|
| `traceCall` stacks | 3 (1 x `FeatureFlagsContext.getFeatureFlags`, 2 x `useFeatureFlagValue.loadFlags`) |
| `GetFeatureFlags` network requests | 2 |

Both traces come from `commitHookEffectListMount` (mount effects), no timer/retry initiators. The two `loadFlags` calls collapse to one request via the in-flight dedupe; the provider's call is a second, uncoupled request. The two hook consumers are mounted on every route by `NotificationPanel` in `RootShell.tsx:31` (`NotificationPanel.tsx:166` `useFeatureFlagValue` and `NotificationPanel.tsx:367` `useBackgroundActivity` -> `useBackgroundActivity.ts:44`); that attribution is INFERRED from mount conditions, because production-build stacks stop at React's effect loop and do not name the owning component.

Multiple documents loaded concurrently in one browser context (module state is per document):

| Documents | traceCall stacks | Network requests |
|---|---|---|
| 1 | 3 | 2 |
| 3 | 9 | 6 |
| 7 | 21 | **14** |

## Findings

1. Within one document the hook dedupes correctly (VERIFIED: 2 hook traces -> 1 request). Adding `NotificationItem`/tray consumers adds traces, not requests, as long as the cache/in-flight promise holds.
2. The provider and the hook do not share a cache, so every document makes at least 2 identical requests (VERIFIED).
3. 14 requests is exactly 7 documents x 2 (VERIFIED arithmetic: 7 concurrent pages gave 14). No in-document path produced more than 2 requests in any scenario tried.

**Root cause classification (from the three options in Story 1.1.1): dedupe failure.** The two consumers keep independent caches, so the floor is 2 per document, and nothing dedupes across documents. "Multiple provider instances" was not observed inside one document (exactly 1 provider trace per load). "Duplicate module instance" was not observed (the hook's cache dedupes within the document).

## Gaps (what this does not prove)

- UNVERIFIED that the baseline HAR contained 7 documents (7 tabs/windows or reloads inside 2.8 s). The HAR (`requirements.md`) cannot show that; it needs the owner's re-capture with DevTools closed, plus a note of how many dashboard tabs/windows were open. Waiting on owner.
- This run used a fresh instance with no sessions or notifications, not a restored 3-window state. Extra consumers (`NotificationItem` x N) only add traces per the dedupe, but an in-document path that bypasses the dedupe (for example a cache-miss storm after a failed read, since failures are not cached) was not exercised.
- Headless Chromium on an unloaded local server; no 6-connection saturation, so slow-response retry behaviour was not tested.

## Implication for Task 2.1.4b

- In-document fix: one shared flags source (the provider and `useFeatureFlagValue` read/write the same cached promise) gets every document from 2 requests to 1.
- It cannot lower the 14 if they are 7 documents: that needs cross-document coalescing (HTTP-cacheable GET, `BroadcastChannel`/leader tab, or SharedWorker) or acceptance of one request per document.
