# Research: request fan-out call-site forensics (Agent 2)

Labels: VERIFIED = command run or code read at the cited path; INFERRED = deduced, not proven.
HAR = `~/Downloads/stapler-squad-backend.har` (parsed with python3 -I, offsets relative to first entry).
All paths are under `web-app/src/`.

## 1. Root pattern (VERIFIED)

The page at t=6.0 s mounts about 8 `SessionDetail` subtrees at once. Each subtree independently fires roughly 13 unary RPCs, and nothing is deduped, cached or aborted across instances. The HAR shows this as 8 distinct sessionIds, each getting an identical RPC set within ~100 ms:

| Per session mount (HAR t=6.04-6.14) | Count per session |
|---|---|
| GetVCSStatus | 2 |
| GetSessionDiff | 1 |
| ListFiles ("." and "docs") | 2 |
| GetFileContent (same path for every session, 404 on the sessions where it does not exist) | 1 |
| GetFileTransportCapabilities | 1 |
| ListWorkflows | 1 |
| ListShells | 1 |
| StatFile (later, t=7.2 / 9.6 / 12.1, 3 paths) | 3 |

Plus ListProgramsConfig x58 (see 2.1). Of the 161 status-0 entries, every ListFiles/GetVCSStatus/GetSessionDiff/ListWorkflows/ListProgramsConfig/LogClientEvents/StatFile call blocked for ~30 s: it burst at t=6 into a pipe already holding 6 slow calls.

Per-endpoint start-offset histogram (VERIFIED, HAR): ListProgramsConfig 58 all in second 6; LogClientEvents 42/8/6 in seconds 6/7/8; GetFeatureFlags 14 all in second 2; ListFiles 16 in second 6 then 1 each at 7, 9, 12; GetVCSStatus 16 and GetSessionDiff 8 and ListWorkflows 8 and GetFileTransportCapabilities 8 and GetFileContent 8 all in second 6; ListShells 4 in second 6 (the other 4 never connected, so are not in the HAR with a body); StatFile 3/3/2 at 7/9/12.

Why 8 panes/sessions mount together is INFERRED (a restored window, `?session=...&window=00c570b1`, with ~8 panes, or a window switch to such a window). The HAR shows the 8 sessionIds but not the layout. The 14 StreamTerminal WebSockets and 14 console-interceptor chain depth (2.2) show 14 `TerminalOutput` instances, more than 8 because `SessionDetailView` also keeps pooled terminals.

## 2. Per-RPC forensics

### 2.1 ListProgramsConfig (58)
- Single caller: `lib/hooks/useAvailablePrograms.ts:19` (`client.listProgramsConfig({})` in a mount effect, no signal, `// abort-signal-exempt`). `components/settings/ProgramsManager.tsx:124` is the settings page only.
- Trigger: mount of every consumer. No module cache, no in-flight dedupe, so N consumers = N calls. VERIFIED.
- Consumers (VERIFIED by grep): `sessions/SessionActionsOverflow.tsx:211`, `sessions/SessionDetailView.tsx:510`, `sessions/OmnibarCreationPanel.tsx:328`, `workflows/WorkflowForm.tsx:71`.
- `SessionActionsOverflow` is rendered unconditionally per `SessionRow.tsx:681`, `SessionCard.tsx:1277`, and per pane in `pane/PaneHeader.tsx:66`. So one call per session-list row/card, plus one per pane header, plus one per `SessionDetailView`. VERIFIED that each renders it; INFERRED that the 58 is rows + pane headers + details (sessions list size not in HAR).
- Per window switch / pane mount: every row, header and detail remounted re-fires it (the data is static host config). Highest-value dedupe target: it needs one module-level cached promise.
- Failure handling today: on error it falls back to `fetch("/api/server-info")` per caller (another uncached call per instance, so 58 failures -> 58 more); the `mounted` flag guards setState but the request is not aborted.

### 2.2 LogClientEvents (56)
- Single caller: `lib/hooks/useBrowserLogStream.ts:137` `flush()`, installed from `components/sessions/TerminalOutput.tsx:965` (one hook per `TerminalOutput`). Separate, not in the HAR: `components/telemetry/ClientLogShipper.tsx` + `lib/telemetry/clientLogShipper.ts` use `fetch` POST on an interval (different endpoint, `ENDPOINT` constant; not verified to be counted).
- Trigger: each `TerminalOutput` mount patches `console.log/warn/error/debug` in an effect with `[]` deps (lines ~120-165 of the hook), buffers entries, flushes at 50 entries or every 5 s.
- ROOT CAUSE (VERIFIED, HAR): the interceptors chain. Each new instance captures the current (already patched) `console.log` as its `originals.log`, so one `console.log` call is enqueued by every mounted instance. HAR: 56 requests x 50 entries = 2800 entries but only 266 unique (timestamp, message) pairs; 139 of those appear exactly 14 times, matching 14 flush-owner sessionIds (200 entries each). The file's own NOTE (lines ~38-44) already predicts this and says a ref counter "should be added if multi-mount becomes a real scenario". It has become one.
- Content (VERIFIED): the logs are mostly `[XtermTerminal]`/`[TerminalOutput]`/`[TerminalDimensionCache]` init chatter, e.g. "WebGL renderer enabled" x112, "Pre-sizing deferred: container layout not yet resolved" x84. The shipper itself generates load proportional to terminal mounts squared, in the same second the 8 detail views mount.
- Per window switch: every remount of a `TerminalOutput` adds another chain link (cleanup unwinds, but during overlap on switch both old and new are patched).
- Fix constraints: must keep `console.error`/`onerror`/`unhandledrejection` capture; `beforeunload` beacon path currently registered per instance too (N beacons on unload); flush must not recurse into console; the last buffered entries should still ship.

### 2.3 GetFeatureFlags (14 at t=2.77 s)
- Callers (VERIFIED by grep, non-test): `lib/contexts/FeatureFlagsContext.tsx:105` (provider `fetchFlags`, mount effect at ~line 117, also `refresh`/`reload`/after failed `setFlag`) and `lib/hooks/useFeatureFlagValue.ts:30` (module cache 30 s TTL plus module-level `inFlight` dedupe, 30 s interval per hook instance). Consumers of the context: Header, BottomNav, DrawerNav, UpdateBadge, SessionMonitor, HandoffSummarySection, settings pages, `useFeatureFlag` (reads context only, no call). Consumers of `useFeatureFlagValue`: NotificationPanel, BackgroundActivity, NotificationItem (x2 per item), `useBackgroundActivity`.
- HAR (VERIFIED): 14 calls start in the same millisecond; all share one initiator call site (`4375-...js:4070`, minified). The `blocked` column is a clean signature of the 6-connection limit: 6 calls blocked 22 ms, then 724, 929, 1300, 1600, 2015, 2020, 2417, 2422 ms as slots free, each holding the server ~1.6-2.0 s.
- WHY 14 is NOT explained by the code as read (honest gap): `useFeatureFlagValue` should collapse to 1 via `inFlight`, and the provider is mounted once in `app/Providers.tsx:47`. A single minified call site that fires 14x at once is consistent with (a) 14 provider instances or remounts, (b) the dedupe failing, or (c) a duplicate-module-instance of `useFeatureFlagValue.ts` across chunks. Not verifiable from the HAR (stack frames are minified, the matching build chunks are not in `web-app/out`). Needs a one-line `console.trace` in `fetchFlags`/`loadFlags` on a fresh capture. Treat the cause as INFERRED-unknown; do not plan a fix before this check.
- Each call is ~1.6-2 s of server time, so dedupe matters twice: it removes the client queue and the server work.
- Edge cases for a fix: the provider has a `seq`/`minAcceptable` ordering guard so a late response can not overwrite a post-write read-back; `useFeatureFlagValue` returns `fallback` (default OFF) until loaded and keeps last-known value on a failed re-read (kill-switch semantics); its per-call `AbortSignal.timeout(10s)` is deliberate because the in-flight promise is shared. Merging both onto one store must keep these.

### 2.4 ListFiles (19)
- Caller: `lib/hooks/useFileService.ts:52` `fetchDirectoryFiles`, used by `components/sessions/FileTree.tsx:499` (`loadDirectory`), `:536` (root load effect on mount / session / `includeIgnored` change) and `:840`. No signal, no cache, no dedupe. VERIFIED.
- Why it runs for hidden tabs (VERIFIED): `components/sessions/SessionDetailView.tsx:1213` renders `<FilesTab>` unconditionally (`aria-hidden`, `data-active`) regardless of `activeTab`, so every mounted detail view loads its file tree even when the user is on the terminal tab.
- Why "docs" and "docs/brainstorms" follow ".": `SessionDetailView.tsx:493-495` seeds `filesSelection.path` from `window.location.hash` (`#file=docs/brainstorms/nkp-explorer-mockup.html`) for EVERY mounted view, then `lib/hooks/useFilesDeepLink.ts:23` calls `tree.revealPath(initialSelectedPath)` which expands docs and docs/brainstorms. VERIFIED: the same path hit 8 different sessionIds in the HAR (GetFileContent 404 on sessions whose worktree has no such file). The comment at line 491 says a switch to another session never inherits it, but the lazy initializer runs once per mount, so each fresh mount inherits the hash. INFERRED that the intent was "hash applies only to the focused session".
- Per window switch: full tree reload per pane, including the `docs` expansions.

### 2.5 GetVCSStatus (16) and GetSessionDiff (8)
- Callers (VERIFIED): `lib/hooks/useSessionVcs.ts:106` (status) and `:137` (diff) through `SessionVcsProvider` (one per `SessionDetail`, `SessionDetail.tsx:148`); `lib/hooks/useVcsStatus.ts:39` `prefetchVcsStatus` and `:100`; `components/backlog/BacklogItemDetail.tsx:290` (`useVcsStatus`, 15 s poll), `components/backlog/BacklogFileBrowserModal.tsx:46` (`useSessionVcs`), `app/fanout/page.tsx:118`.
- The 2:1 ratio per session is a deduping bug (VERIFIED by reading both): `SessionDetail.tsx:113-116` calls `prefetchVcsStatus` on mount (cache written only after the response, no in-flight sharing), and `useSessionVcs` ignores `vcsCache` entirely and fetches again. The prefetch comment ("so tabs load instantly") is false: the consuming hook never reads that cache. `useVcsStatus` does read it, but only the backlog uses it.
- Triggers in `useSessionVcs`: mount; whenever the Redux session `status` or `updatedAt` changes (every watch-stream update of that session refetches); 60 s interval when `isActive`, 600 s otherwise (skips when `document.hidden`); diff once on mount. `useAbortableRequest` cancels the previous in-flight on re-run/unmount (good). Stops permanently on NotFound.
- Server cost visible in HAR: GetVCSStatus 3-4.5 s each. They occupy slots longest.
- Per window switch: 3 calls per pane, plus a refetch on the next `updatedAt` tick.

### 2.6 GetFileContent (8), StatFile (8), GetFileTransportCapabilities (8)
- GetFileContent: `lib/files/transport/fullFileTransport.ts:30` (signal passed) and legacy `lib/hooks/useFileService.ts:86` via `FileContentViewer.tsx:224`. The 8 calls (404 or blocked) are the hash-seeded path (2.4).
- GetFileTransportCapabilities: `lib/hooks/useFileTransport.ts:47` inside `createDefaultFileTransport`, built by `FileTransportProvider` (`useMemo`, one router per mounted `FilesTab`). Comment says "one cached page-lifetime fetch"; it is cached per router, so per FilesTab mount, not per page. No signal. Capabilities are a server-global property (same answer for every session) but the RPC is keyed by `sessionId`: candidate for a module-level cache keyed by `capsEpoch`.
- StatFile: `lib/files/transport/wireMapping.ts:83`, called from `useOpenBuffers.ts:118` (stat each newly restored "loading" buffer once; aborts on unmount), `useBufferFreshness.ts:112` and `useFilesV2.ts:58`. The three paths per session are the persisted open buffers. `useBufferFreshness` is passed `enabled: true` (`useFilesV2.ts:22`) and polls every `POLL_MS = 10_000` (1 s confirm) for the active buffer of every mounted FilesTab, gated only by `document.visibilityState`, not by Files-tab visibility. Net: a steady 1 StatFile/10 s per mounted session even while the user views a terminal. VERIFIED by code; HAR shows only the first burst (t=7.2/9.6/12.1, spaced 2.4 s: consistent with the 6-slot queue draining, INFERRED).

### 2.7 ListWorkflows (8) and ListShells (4)
- ListWorkflows callers (VERIFIED): `lib/hooks/useWorkflows.ts:102`, used by `SessionDetailView.tsx:428` (one per detail view), `lib/contexts/OmnibarContext.tsx:74` (global), `TriggersPanel`, `WorkflowsPanel`. Per instance: fetch on mount, 90 s interval, and refetch on `visibilitychange`; no cache. A live `WatchWorkflows` socket already exists (9 x WatchWorkflows in the HAR), so the list is also being pushed. Inference: SessionDetailView only needs workflows for attribution, a read of a shared store would suffice.
- ListShells callers (VERIFIED): `lib/hooks/useShells.ts:73` (per `SessionDetailView.tsx:446`, a mount effect) and `lib/hooks/useSessionService.ts:1055`. `useShells` builds its own transport per instance with `createSessionWatchTransport` (not the `getConnectTransport` singleton); unary still goes over fetch (`watch-ws-transport.ts:92-96`), so it shares the 6-slot pool but bypasses the shared interceptors. Server time 4-6 s (ListShells max 24 s in HAR including queue), the slowest steady call.

## 3. Window-switch behaviour (VERIFIED by reading, INFERRED for HAR attribution)
- `app/page.tsx:633` renders only `currentWindow.paneState` through `PaneTilingContainer`, and `pane/PaneSplitRenderer.tsx:388` keys `SessionDetail` on `${pane.id}-${pane.sessionId}`. A switch (or session reassign) unmounts the old subtree and mounts the new one: every hook above restarts and re-fires. `TerminalPool.tsx` keeps xterm instances alive but, by its own header comment, not the data fetches or the `TerminalOutput` WebSocket.
- Per mounted pane the cost is ~13 RPCs plus 1-2 `ListProgramsConfig`, plus 1 interceptor chain link. With P panes the burst is ~15P RPCs against 6 connections, with the slowest calls (ListShells 4-6 s, GetVCSStatus 3-4.5 s, GetFeatureFlags ~2 s) holding slots. This is the mechanism by which the 6-slot limit turns into 30 s `blocked` (the status-0 entries are all `blocked` ~30,000 ms, equal to `DEFAULT_RPC_TIMEOUT_MS = 30_000` in `lib/api/transport.ts`, so the Connect client-side timeout most likely fires while the request is still queued, INFERRED).

## 4. Dedupe / cache inventory

| RPC | Cache | In-flight dedupe | Abort on unmount |
|---|---|---|---|
| ListProgramsConfig | none | none | no |
| LogClientEvents | n/a | n/a (per-instance buffers) | no (intentionally) |
| GetFeatureFlags (provider) | context state | no | no |
| GetFeatureFlags (`useFeatureFlagValue`) | 30 s module TTL | yes | 10 s timeout, not per-component |
| GetVCSStatus (`useSessionVcs`) | none | none | yes |
| GetVCSStatus (`useVcsStatus`/prefetch) | 15 s module TTL | no | prefetch no; hook yes |
| GetSessionDiff | none | none | yes |
| ListFiles | none | none | no |
| GetFileContent / StatFile | per-router chunk cache only for ranges | none | partial (signal passed) |
| GetFileTransportCapabilities | per FilesTab router | no | no |
| ListWorkflows | none | none | no |
| ListShells | none | none | no |

## 5. Edge cases and failure modes a fix must handle
1. Stale data: flags are a kill switch (30 s TTL today); VCS status must refetch on `sessionUpdatedAt`; workflows are pushed over WatchWorkflows but need the 90 s/visibility backstop because that stream can silently stall (comment in `useWorkflows.ts`); deleted sessions return NotFound and must stop polling (both VCS hooks have `stoppedRef`).
2. Abort semantics on a shared promise: aborting one component's request must not cancel other subscribers (`useFeatureFlagValue` bypasses signals for this reason). Use subscriber ref-counting or time-bound shared requests.
3. Unmount/remount churn during a window switch: results landing after unmount must not setState (`mounted`/`signal.aborted` guards exist in some hooks, not in `useAvailablePrograms`' fallback chain or `FileTree.loadDirectory`).
4. Errors: ListProgramsConfig fallback to `/api/server-info` must also be single-flight; a failed shared fetch must not cache failure forever (retry with backoff); `useFeatureFlagValue` must keep returning `fallback` OFF before first load.
5. Hash-seeded path: scoping `#file=` to the focused session only; a 404 on a restored path must not retry per poll.
6. Hidden-surface work: FilesTab is mounted while hidden; any "pause when hidden" gating must still allow the first load and not break `useFilesDeepLink` reveal or the buffer freshness token adoption.
7. Console interceptor singleton: must preserve `unhandledrejection`/`onerror`, keep the sessionId tag meaningful (currently each instance tags its own session id; a global one needs a last-focused-session rule), and flush on `beforeunload` once.
8. Transport: `useShells` builds a separate transport; unifying must keep `createRpcTimingInterceptor` analytics wiring (it adds it when an `AnalyticsContext` exists).
9. Test/SSR envs: several hooks guard `typeof fetch`, partial `@/lib/config` mocks; module singletons need `reset*ForTest` hooks (pattern: `resetFeatureFlagValueCache`).
10. Server cost: even with perfect dedupe, GetVCSStatus 3-4.5 s, ListShells 4-6 s, GetFeatureFlags 1-2 s server times remain; out of scope per requirements but they set how long a slot is held.

## 6. Answers to requirements.md Open Questions
- Who produces the 58 ListProgramsConfig and 56 LogClientEvents? ListProgramsConfig: `useAvailablePrograms` instances (one per session row/card, pane header and detail view), no dedupe. LogClientEvents: per-`TerminalOutput` console interceptors that chain, so each console line is enqueued by all 14 mounted instances (2800 sent entries vs 266 unique).
- Localhost vs remote access: HAR is 100% `localhost:8543` HTTP/1.1 (from requirements); not re-checked here.

## 7. Gaps
- Why 14 identical GetFeatureFlags fire simultaneously is not proven (2.3).
- Rows vs panes split of the 58 ListProgramsConfig is INFERRED.
- Layout/pane count behind the t=6 burst is INFERRED.
- Trace file was not used for this task.
