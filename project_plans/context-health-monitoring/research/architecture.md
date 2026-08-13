# Architecture Research: Context Health Monitoring

Builds directly on `project_plans/detector-plugins/research/architecture.md` (hereafter
"detector-plugins arch"), which mapped `session/detection`'s three layers (`dtypes`,
`binaries`, `detection` registry/`PatternSet`/`StatusDetector`) in detail. That research is
**not re-derived here** — cited by file:line where relevant. Also checked
`project_plans/review-queue-event-driven/research/architecture.md` and
`project_plans/session-status-display/research/architecture.md`; neither needed re-citing
beyond what's covered independently below (the status-display doc covers `DetectedStatus`
proto wiring, which this doc re-derives from source directly since it's the load-bearing
precedent for Q2).

## 1. Which pattern fits: detector-pipeline vs. post-hoc analyzer vs. something else

**Recommendation: a new sibling post-hoc analyzer, structurally modeled on
`session/tokens` (`TokenStore`/`Parser`/`Associator`) — not a plugin inside
`session/detection`'s `PatternSet`/`DetectorRegistry` pipeline.**

### Why not inside the detection pipeline

`PatternSet.MatchLines` (`session/detection/pattern_set.go:69-141`, per detector-plugins
arch §1.1) runs a **fixed priority chain that classifies terminal text into exactly one of
ten status categories** (Error, Ready, Idle, etc.) per detection call. `BinaryDetector`
(`session/detection/dtypes/dtypes.go:29-33`) and the plugin extension point that
detector-plugins arch designs (`MergedRegistry`, hot-swappable `DetectorRegistry`) are both
about **adding more regex patterns into that same ten-category classification**, not about
computing an orthogonal, cross-call signal. ContextHealth needs two things `PatternSet` was
never built to carry:

1. **Tool-call identity + argument similarity across consecutive calls** — `PatternSet` has
   no concept of "which tool was invoked" at all; it only classifies status categories from
   raw terminal text. There is no existing extension point for this in `session/detection`.
2. **A rolling/stateful judgement across N recent turns** — `StatusDetector` is stateless
   per call (`detectFromText` reads an immutable `PatternSet` via `atomic.Pointer.Load()`,
   detector.go:250-251 per detector-plugins arch §1.1); `DetectionEventSink`
   (`session/detection/event_sink.go:1-41`, `events.go:8-16`) *does* keep a per-session ring
   buffer of the last 2000 detection calls, but each `DetectionEvent` only records the
   matched **status category** and a 512-byte `TailSnippet` of cleaned text
   (`events.go:9-16,25`), not a structured tool name or arguments — insufficient granularity
   for "same tool called 3+ times with similar args."

### Why a post-hoc analyzer over an existing stream — and which stream

Three existing per-session streams were evaluated as the substrate for the two heuristics:

| Candidate stream | Confirmed by | Granularity | Fits which heuristic |
|---|---|---|---|
| Live PTY/tmux terminal text (`session/detection`) | detector-plugins arch §1.1 | Status category only; no tool identity | Neither directly — would need a *new* regex layer to parse tool-invocation banner lines, and args are never rendered in full to the terminal |
| Claude JSONL transcript (`session/tokens`) | `session/tokens/parser.go:122-191`, `jsonl_types.go:34-45` | Per-turn `tool_use` blocks with **exact tool `Name` and full `Input` (json.RawMessage)** already parsed (currently discarded after counting, per the privacy guarantee in `doc.go:1-19` and `types.go:5-7`); assistant `Text` blocks also parsed per-content-block (`jsonlContent.Text`, `jsonl_types.go:41`) but not currently retained | Both — tool name+input hash for loop detection, text regex for confusion detection | Both, but **Claude Code only** |
| PostToolUse HTTP hook (`server/services/hook_receivers.go:70`, `hook_injector.go:16-43`) | Live event, not polled from disk; carries `ToolName`+`ToolInput` per `classifier.PermissionRequestPayload` (`pkg/classifier/classifier.go:50-58`) | Full args, live (no fsnotify latency) | Loop detection only (no assistant text) |

**Recommendation: extend the `session/tokens` package**, i.e. add a `ContextHealthAnalyzer`
(or extend `Parser`/`ParseResult`) that, while already walking `jsonlContent` blocks per
turn (`parser.go:163-181`), additionally:
- retains a short rolling window of `(ToolName, hash(Input))` tuples per session to detect
  3+ consecutive same-tool-similar-args calls (loop heuristic), and
- runs a small fixed regex set (mirroring `skill_detector.go:11-58`'s existing
  text-scanning pattern) over `jsonlContent.Text` blocks for apology/error-style phrases
  (confusion heuristic),

then folds the result into a new field on `ParseResult` (alongside `ToolUsage`,
`SkillActivations` — `types.go:8-26`), surfaced through the same `TokenStoreReader`
interface (`types.go:56-63`) and `TokenStore.Subscribe()` channel (`store.go:126-135`) that
`InsightsService.WatchInsights` already consumes (`insights_service.go:491-531`).

This is the closest existing precedent for "compute a derived per-session signal from a
background JSONL-parsing pipeline, cache it, and notify subscribers on change" — and it's a
**functioning pattern already wired end-to-end today** (fsnotify → worker pool → cache →
subscriber channel → ConnectRPC stream), unlike the detection package's plugin/hot-reload
mechanism, which detector-plugins arch found has **zero existing hot-reload precedent** in
`session/detection` itself.

**Caveat (flag for planning phase):** this substrate is Claude-Code-specific
(`session/tokens` parses Claude's JSONL transcript format only — `doc.go:1-2`). If
ContextHealth must also cover Aider/Gemini/OpenCode/Agy sessions (the multi-binary support
`session/detection/binaries` implies), a text-based fallback modeled on the PostToolUse-banner
idea would be needed for non-Claude programs; the requirements summary doesn't state this
scope explicitly and it should be confirmed before planning locks in `session/tokens` as the
sole source.

## 2. Integration Points

- **Per-session state today**: `session/instance.go`'s `Instance` struct (`instance.go:100-160+`)
  holds no analogous "derived analysis" field yet; the nearest precedent,
  `detection.DetectedStatus`, is **not stored on `Instance` as a field** — it's read live via
  `Instance.GetDetectedStatus()` (`session/instance_state.go:261-264`), which delegates to a
  `ClaudeController`'s `atomic.Pointer[detection.StatusDetector]`
  (`session/claude_controller.go:119`, `:631`, `:955`). ContextHealth should follow the same
  shape as the *recommended* new analyzer (§1) rather than living on `Instance` directly: a
  package-level `TokenStore`-like store keyed by session UUID
  (`tokens.TokenStore.GetByUUID`, `store.go:113-117`), not a field mutated in place on
  `Instance`.
- **Reaching the frontend via ConnectRPC**: the proto `Session` message already carries a
  precedent for exactly this shape of field — `DetectedStatus detected_status = 68` and its
  string context field `= 69` (`proto/session/v1/types.proto:222-229`), doc-commented "Maps
  to `detection.DetectedStatus` in Go." The single conversion point that populates it is
  `InstanceToProto` (`server/adapters/instance_adapter.go:15,157-171`) — computed once per
  conversion call from `statusInfo` and assigned via
  `detection.DetectedStatusToProto(statusInfo.ClaudeStatus)` (`:171`). A new
  `ContextHealth`/`ContextHealthReason` field pair should be added the same way: next unused
  proto field numbers (highest currently in use is `100`; detected_status/detected_context
  occupy `68`/`69` — confirm exact next-free number in `types.proto` at plan time), populated
  inside `InstanceToProto` by querying the new analyzer store
  (`contextHealthStore.GetByUUID(inst's conversation UUID)`), mirroring lines `157-171`.
  Because `WatchSessions` (`server/services/session_service.go:2064-2110+`) streams
  `SessionEvent`s built via `InstanceToProto`/`convertEventToProto` off an internal
  `eventBus.Subscribe(ctx)` (`:2072`), **no new streaming RPC is needed** — ContextHealth
  rides the existing `WatchSessions` stream automatically once it's in the proto and set in
  `InstanceToProto`. (`InsightsService.WatchInsights`, `insights_service.go:491-531`, is a
  parallel *aggregate* streaming RPC over `TokenStore.Subscribe()` and is the pattern to copy
  only if ContextHealth is later exposed as a *dashboard-level* rollup rather than a
  per-session badge — out of scope per the requirements' "badge+tooltip on session cards.")
- **Config-driven thresholds**: follow `config.CapacityConfig` exactly
  (`config/types.go:256-274` — struct + `CapacityConfigOrDefault()` zero-value defaulting),
  registered on the root `Config` struct (`config/config.go:330`,
  `json:"capacity,omitempty"`) and defaulted at both fresh-config construction (`:452`) and
  on every load (`:894`). A `ContextHealthConfig{LoopRepeatThreshold int,
  ConfusionPhraseThreshold int, ...}` should be added the same way — new field on `Config`,
  its own `ContextHealthConfigOrDefault()`, called at both of those existing call sites.
- **Frontend badge+tooltip**: `web-app/src/components/sessions/SubStatusChip.tsx` is the
  direct precedent — a small derived-status chip already rendered on `SessionCard.tsx` next
  to `StatusBadge.tsx`. A `ContextHealthBadge` component should follow the same shape
  (colored dot/badge + tooltip text sourced from the new proto reason field), registered in
  `SessionCard.tsx` alongside the existing badges.

## 3. Data Flow and Consistency

**Persistence**: not needed, and should explicitly *not* persist, matching the existing
`DetectedStatus` precedent. Confirmed by inspecting `session/ent/schema/session.go:34` — only
the coarse lifecycle `status` (int enum) is a persisted ent field; there is no
`detected_status` column anywhere in `session/ent/schema/*.go`. Fine-grained/derived signals
in this codebase are consistently **in-memory, recomputed from the live stream, never
written to SQLite** — `ContextHealth` should follow suit: recomputed from the JSONL
transcript (or whichever stream is chosen) on each `InstanceToProto` call / analyzer refresh,
not stored as a durable field. This also sidesteps a stale-health-after-restart problem for
free — on restart, `TokenStore`'s startup walk (`store.go:69-79`,
`walkAndEnqueue`) already re-parses all JSONL files, so health recomputes naturally.

**Concurrency**: the recommended design (§1, §2) needs **no new mutex**. It slots into
`TokenStore`'s existing `sync.RWMutex`-guarded `cache`/`byUUID` maps (`store.go:26-33,113-120`)
and its `atomic`-free worker-pool/inflight (`sync.Map`) design — the new per-session
health computation is just another field computed inside `parseAndCache`
(`store.go:105-129`) under the same lock that already guards writing `cachedEntry`/`byUUID`.
Two anti-patterns to explicitly avoid per the repo's rules:
- **Do not add a new `Manager`/`Service` wrapper type that only forwards to `TokenStore`**
  (interface-pollution checklist item 4, `.claude/rules/interface-pollution-checklist.md`) —
  extend `ParseResult`/`Parser` in place, the same way `SkillActivations` was added
  (`skill_detector.go`) rather than standing up a parallel `ContextHealthService` that just
  calls through to the token store.
- **No double-checked-locking violation** (`.claude/rules/go-double-checked-locking.md`):
  `parseAndCache` (`store.go:105-129`) already returns/stores the locally-parsed `result`
  directly into the cache under `ts.mu.Lock()` — extending it to also compute
  `result.ContextHealth` inline, before the single `Lock`/`Store`, preserves this; the bug
  the rule warns about (re-reading the shared slot instead of the just-computed value) would
  only be introduced if a *new* handler added a "compute now and return it" RPC path that
  re-`Load()`s after a concurrent second parse — not needed here since `InstanceToProto`
  always reads via `GetByUUID` (a fresh read of current state), never "the value I just
  computed."
- **No speculative interface**: don't define a `ContextHealthComputer` interface with one
  implementation up front (checklist item 1) — a concrete function
  `computeContextHealth(result *ParseResult, cfg config.ContextHealthConfig) ContextHealth`
  colocated in `session/tokens` is sufficient; `TokenStoreReader` (`types.go:56-63`) is the
  right *existing* interface boundary to extend (add a `ContextHealth` accessor or fold it
  into `ParseResult`), not a new one.

**In-flight session mid-recompute**: same reasoning as detector-plugins arch §4's "atomic
swap of an immutable snapshot" — `TokenStore.GetByUUID` (`store.go:113-117`) is read fresh
on every `InstanceToProto` call (no long-lived cached copy inside `Instance`), so a health
recompute takes effect on the very next proto conversion / `WatchSessions` broadcast, with no
partial/half-applied state (the whole `ParseResult`, including its new health field, is
built off to the side in `parseAndCache` before the single `Lock`/write, `store.go:118-127`).

## 4. EventStorming Table

Not included. This is a single derived-signal computation (JSONL parse → heuristic → cache →
proto field), not a multi-actor business domain with branching commands/policies — the two
heuristics are pure functions over an already-parsed `ParseResult`, computed inline in the
existing `parseAndCache` flow. Confirmed by inspection of `session/tokens/store.go` and
`parser.go`; no additional actors, sagas, or cross-service commands are involved beyond what
§1–§3 already describe.

## Summary of Concrete File-Level Anchors

| Concern | File:Line |
|---|---|
| Why not the `PatternSet`/`DetectorRegistry` pipeline | `session/detection/pattern_set.go:69-141`, `session/detection/dtypes/dtypes.go:29-33` |
| Per-session detection ring buffer (insufficient granularity) | `session/detection/event_sink.go:1-41`, `session/detection/events.go:8-26` |
| JSONL tool_use + text parsing (recommended substrate) | `session/tokens/parser.go:122-191`, `session/tokens/jsonl_types.go:34-45` |
| Privacy guarantee to respect/extend carefully | `session/tokens/doc.go:1-19`, `session/tokens/types.go:5-7` |
| Existing text-regex-scan-over-JSONL precedent | `session/tokens/skill_detector.go:11-58` |
| `TokenStore` cache/fsnotify/subscriber pattern to extend | `session/tokens/store.go:26-135` |
| `ParseResult` shape to add `ContextHealth` field to | `session/tokens/types.go:8-26` |
| Existing streaming RPC precedent (aggregate-level) | `server/services/insights_service.go:491-531` |
| Per-session streaming RPC that would carry ContextHealth automatically | `server/services/session_service.go:2064-2110` |
| Proto field precedent (`DetectedStatus`/context) | `proto/session/v1/types.proto:222-229` |
| Single conversion point to populate the new proto field | `server/adapters/instance_adapter.go:15,157-171` |
| Config threshold precedent to copy | `config/types.go:256-274`, `config/config.go:330,452,894` |
| Confirmation `DetectedStatus` (and by extension, should-be ContextHealth) is NOT persisted | `session/ent/schema/session.go:34` (only coarse `status` int is a column) |
| Frontend badge+tooltip precedent | `web-app/src/components/sessions/SubStatusChip.tsx`, `SessionCard.tsx` |
| Interface-pollution guard applied | `.claude/rules/interface-pollution-checklist.md` (avoid new forwarding `ContextHealthService`) |
| Double-checked-locking guard applied | `.claude/rules/go-double-checked-locking.md` (n/a as designed — no re-Load-after-write path) |
