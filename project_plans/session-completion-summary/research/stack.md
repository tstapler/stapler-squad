# Research: Technology Stack — Session Completion Summary

Agent 1 (Stack). Scope: identify existing libraries/patterns in this repo that
cover the five stack questions FR-1..FR-7 raise. Per repo convention
(CLAUDE.md ponytail/laziness), no new dependency is proposed unless nothing
suitable already exists in `go.mod` / `web-app/package.json`.

## Summary

Every piece this feature needs already has a precedent in the codebase:
markdown assembly via plain string building, async fire-and-forget via `go
func()` + recover, per-key dedup via `singleflight.Group` (already a
dependency, used 4x), LLM narrative generation via the existing
`session/headless.Pool`, clipboard copy via the native Clipboard API with a
`document.execCommand` fallback (already implemented twice), and the ent
"independent entity" shape via `AnalyticsEvent`'s plain `session_id` string
field. **No new Go module or npm package is needed.**

---

## (a) Markdown document generation/templating — Go

**No templating library is used anywhere in this Go codebase.** A repo-wide
search (`rg "text/template|html/template"` across `*.go`) returns zero hits,
and there is no goldmark/blackfriday-direct/markdown-builder dependency in
`go.mod` (blackfriday is only an *indirect* transitive dep of `go-md2man`,
unrelated to app code).

The established pattern for markdown-emitting code is **hand-built strings**:

- `session/headless/features.go`'s `prDescriptionSystemPrompt` (a raw Go
  string literal instructing the LLM to "Output ONLY the PR body as
  Markdown") and its companion `userPrompt` assembly around
  `DraftPRDescription` (`session/headless/features.go:274`).
- `session/backlog_review.go:612` — comment explicitly about closing a
  markdown code fence when interpolating a diff into an LLM prompt, done via
  plain string concatenation.

**Recommendation:** build the deterministic sections (diff stat, decisions
breakdown, timeline, token/cost) with stdlib `strings.Builder` /
`fmt.Sprintf`, matching the rest of the codebase, rather than introducing
`text/template` (stdlib, zero-cost to add, but would be the *first* use of
templating in this Go codebase and doesn't match convention) or a markdown
library. The narrative section is LLM-generated markdown text spliced in
directly (see (c)) — no rendering step needed since it's stored/exported as
raw markdown, never HTML-rendered server-side (`react-markdown` +
`remark-gfm` already handle client-side rendering, see FR-4/UI section
below).

## (b) Async / background execution with per-key in-flight dedup (FR-7)

**Fire-and-forget dispatch pattern** (already used pervasively for
lifecycle-triggered background work): plain `go func() { ... recover() ...
}()`, e.g. `session/backlog_lifecycle.go` (10 call sites: lines 916, 1103,
1121, 1469, 1979, 2247, 2446, 3515) and its documented "own recover()"
convention at line 1407-1415, layered under the whole-tick `recover()` in
`server/dependencies.go`. This is the pattern to hang summary generation off
of `instanceBacklogListener.OnLifecycleEvent` (line 797), which already
subscribes to both `EventExited` and `EventStopped` identically — the exact
hook FR-1 needs, including the existing convention for excluding a specific
reason string (compare to how `reconcile-session-missing`
(`session/review_queue_poller.go:451`) already fires `EventExited` with a
distinguishable reason so listeners can special-case it).

**Per-key in-flight dedup**: `golang.org/x/sync/singleflight` — **already a
direct dependency** (`golang.org/x/sync v0.20.0` in `go.mod`; the module
provides `singleflight` as a subpackage, no separate module needed) and
already used for exactly this "coalesce concurrent/duplicate calls for the
same key" shape in 4 places:
- `github/client.go:34` (`ghAuthGroup singleflight.Group`)
- `github/user_pr_cache.go:120-121` (`loginGroup`, `refreshGroup`)
- `session/tmux/tmux.go:118,128` (`existsSF`, `noCacheSF`)
- `session/unfinished/gogit_vcs_reader.go:490-496` + generic helper
  `sfDo[T any](sf *singleflight.Group, key string, fn func() (T, error))
  (T, error)` at line 619 — a ready-made generic wrapper pattern to copy.

`singleflight.Group.Do(sessionID, generateFn)` directly satisfies FR-7:
concurrent duplicate `EventExited` fires and repeated Regenerate clicks for
the same session ID collapse into one in-flight generation, and all callers
receive the same result. No new dependency needed.

**Status/state tracking** (if a per-session generation-status map is needed
independent of the singleflight call, e.g. to answer "is generation
currently running" from an RPC without blocking on the call): `xsync.Map`
(`github.com/puzpuzpuz/xsync/v4`, already a dependency) is the established
lock-free map choice — see `session/instance_status.go:26`
(`controllers *xsync.Map[string, *ClaudeController]`) and
`session/shell_registry.go` for the wrapping convention (never hold the
`Compute` closure across I/O).

**Double-checked locking gotcha**: if the generation path does
check-cache → miss → compute → re-lock → conditional-store, follow
`.claude/rules/go-double-checked-locking.md` exactly: return the
locally-computed value after the write-lock section, not a re-read of the
shared slot (canonical example: `IsDirty` in `session/git/worktree_git.go`).
This applies if summary generation ever races two goroutines computing the
same summary and one write "loses" — return what *this* goroutine computed.

## (c) LLM narrative generation

**This repo already has a purpose-built headless-LLM-call abstraction**:
`session/headless.Pool` (`session/headless/pool.go`, `caller.go`,
`features.go`). It shells out to the `claude` CLI in headless mode, manages
session reuse for prompt-cache efficiency, and bounds concurrency
(`PoolConfig.MaxConcurrentSessions`, default 5).

Relevant surface:
- `Pool.CallBlocking(ctx context.Context, key FeatureKey, systemPrompt,
  userPrompt string, opts CallOptions) (string, float64, error)`
  (`session/headless/caller.go:480`) — returns the completion text, an
  estimated cost (float64, USD), and an error. The `float64` return is
  already exactly the "cost" data point needed to fold into FR-2's
  token-usage/cost section (though the primary cost source for the summary
  itself should stay `session/tokens` parsing per the requirements'
  grounding section — this is the cost of the narrative-generation call
  itself, a secondary/internal figure, not to be conflated).
- `FeatureKey` constants already declared in `session/headless/features.go:14-22`
  include `FeatureKeySummarize FeatureKey = "summarize"` — a pre-existing,
  unused-by-this-feature-yet key that is semantically the closest match; a
  new distinct key (e.g. `FeatureKeySessionCompletionSummary`) is still
  advisable so per-feature session rotation/call-count tracking in `Pool`
  doesn't mix unrelated narrative styles, but the naming precedent and enum
  location are established.
- **Direct pattern to copy**: `DraftPRDescription(ctx, pool, itemTitle,
  itemDescription, diff, branchName string) (string, error)`
  (`session/headless/features.go:274-290`) — builds a system prompt (stable
  const, markdown-output instruction baked in) + a per-call user prompt,
  calls `pool.CallBlocking(ctx, FeatureKeyPRDescription, systemPrompt,
  userPrompt, CallOptions{})`, and returns just the string (dropping cost).
  A `SummarizeSessionCompletion(ctx, pool, ...)` function following this
  exact shape is the natural implementation for FR-2's narrative step.
- **Graceful degradation for FR-5 comes free**: `CallBlocking` already
  surfaces LLM failures as a plain Go `error` — the caller can catch that,
  skip the narrative section, and still emit deterministic sections (diff,
  decisions, timeline, cost) with an empty-state narrative placeholder,
  satisfying "narrative failures degrade gracefully (still READY)."
- `AIClient` interface (`server/services/ai_interfaces.go:45-47`,
  `Complete(ctx, systemPrompt, userPrompt string) (string, error)`) is a
  *different*, narrower abstraction used by the rules-suggestion feature
  (`RulesService`) — it is not backed by the headless pool and is scoped to
  that feature's `RulePromptBuilder`. Not the right fit here; `session/headless.Pool`
  is the correct precedent since it's already the mechanism for
  backlog/session lifecycle narrative generation (PR descriptions, commit
  messages, acceptance criteria, triage — see the full `FeatureKey` const
  block).

## (d) Clipboard copy — React frontend

**No clipboard library exists or is needed** — `package.json` has no
`clipboard.js`/`copy-to-clipboard`/similar, and the native
`navigator.clipboard.writeText()` + `document.execCommand('copy')` fallback
pattern is already implemented twice:

- `web-app/src/components/logs/ExpandedLogDetail.tsx:19-28` — the cleanest
  reference implementation:
  ```ts
  const handleCopy = () => {
    navigator.clipboard.writeText(text).catch(() => {
      // Fallback for browsers without clipboard API (some mobile WebViews)
      const el = document.createElement("textarea");
      el.value = text;
      document.body.appendChild(el);
      el.select();
      document.execCommand("copy");
      document.body.removeChild(el);
    });
  };
  ```
- `web-app/src/components/logs/ExportButton.tsx:123-130` (`copyToClipboard`)
  — same `navigator.clipboard.writeText` call, simpler (no fallback), used
  behind a "Copy to clipboard" export option — closest UX precedent for
  FR-4's "one-click copy-to-clipboard export as GFM markdown" since it's
  already wired to an export-menu pattern rather than a single inline button.
- `XtermTerminalSelection` (tested in
  `web-app/src/components/sessions/__tests__/XtermTerminalSelection.test.tsx`)
  has a third, keyboard-shortcut-triggered copy path with the same
  `navigator.clipboard.writeText` call plus documented Mac-fallback
  behavior — useful if the Summary tab also wants a keyboard shortcut.

**Recommendation:** reuse the `ExpandedLogDetail.tsx` pattern verbatim (or
extract it to a small shared `copyToClipboard(text: string)` helper if used
in 3+ places after this feature adds a 3rd real call site — see
`.claude/rules/interface-pollution-checklist.md` smell #5, don't
over-abstract for 2 call sites).

For **GFM rendering** of the summary tab itself (not the copy action, but
displaying it before copy), `react-markdown` (`^10.1.0`) + `remark-gfm`
(`^4.0.1`) are already dependencies and already presumably used for other
markdown-rendering surfaces in the app — reuse rather than add a renderer.

## (e) ent schema pattern — independent entity, no cascade with Session

Two precedents exist in `session/ent/schema/`, and they demonstrate the
exact fork this feature must not get wrong:

**Wrong shape to copy** — `DiffStats` (`session/ent/schema/diffstats.go`):
```go
func (DiffStats) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("session", Session.Type).
            Ref("diff_stats").
            Unique().
            Required(),
    }
}
```
A `Required()` `edge.From` ties the child row's referential integrity to the
parent `Session` row via ent's edge/FK machinery — exactly the coupling
FR-3 says must NOT happen ("must outlive the `Session` row").

**Right shape to copy** — `AnalyticsEvent`
(`session/ent/schema/analytics_event.go`): a plain, optional string field,
not an edge:
```go
field.String("session_id").
    Optional(),
```
with a secondary (non-unique) index for lookup:
```go
index.Fields("session_id"),
```
This is the same shape `server/notifications/store.go`'s notification
history store uses per the requirements' own grounding section. Copy this
pattern for the new summary entity (e.g. `SessionSummary`):
- `field.String("id").Unique().NotEmpty().Immutable()` — app-generated UUID,
  following the `uuid.New().String()` convention already used at
  `server/services/approval_handler.go:357,485` (repo has no
  ent-native autoincrement-ID convention in play here; `AnalyticsEvent` also
  uses a plain string ID).
- `field.String("session_id").NotEmpty()` (required here, unlike
  `AnalyticsEvent`'s optional one, since every summary is generated for a
  specific session — but still a plain field, not an edge) +
  `index.Fields("session_id")` for retrieval by session, satisfying "one new
  Summary tab, independently retrievable."
- Structured substructure fields (decisions breakdown, timeline entries) as
  `field.JSON(...)`, mirroring `AnalyticsEvent.Fields()`'s
  `field.JSON("labels", map[string]string{}).Optional()` — avoids adding
  further child tables/edges for what is fundamentally a snapshot document.
- A status field for FR-5's READY/ERROR/PENDING lifecycle: no
  `field.Enum` usage exists anywhere in `session/ent/schema/*.go` today (repo
  convention favors plain validated strings over ent enums elsewhere) — a
  plain `field.String("status")` with app-level validation (constants, not
  an ent enum) matches existing repo style; introducing the first
  `field.Enum` in this schema package would be a minor convention deviation
  worth flagging in the plan/ADR phase rather than deciding unilaterally
  here.
- `field.Time("created_at").Default(time.Now).Immutable()` and a mutable
  `updated_at` (for Regenerate re-runs), following `AnalyticsEvent`'s
  `created_at` pattern.

Regenerate `session/ent` after any schema change with the **required flag**
per `.claude/rules/ent-schema-generation.md`:
```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```
(omitting `--feature sql/upsert` silently breaks `UpsertRule`-style upsert
methods — relevant if the Regenerate action needs an upsert-on-conflict
write path for the summary row).

---

## Dependency Inventory (existing, relevant)

### Go (`go.mod`) — all already present, no additions needed
| Dependency | Version | Use for this feature |
|---|---|---|
| `golang.org/x/sync` | v0.20.0 | `singleflight.Group` for FR-7 per-session dedup |
| `github.com/puzpuzpuz/xsync/v4` | v4.5.0 | optional in-memory generation-status map |
| `github.com/google/uuid` | v1.6.0 | summary row ID generation |
| `entgo.io/ent` | v0.14.5 | new `SessionSummary` schema (plain field, not edge) |
| stdlib `strings`, `fmt` | go 1.26.3 | deterministic markdown section assembly |
| (internal) `session/headless` | n/a (in-repo) | LLM narrative generation via `Pool.CallBlocking` |
| (internal) `session/tokens` | n/a (in-repo) | token usage/cost data per requirements' grounding |

### Frontend (`web-app/package.json`) — all already present, no additions needed
| Dependency | Version | Use for this feature |
|---|---|---|
| `react-markdown` | ^10.1.0 | render the GFM summary in the new Summary tab |
| `remark-gfm` | ^4.0.1 | GFM extensions (tables, task lists) for the summary |
| native `navigator.clipboard` | browser API | FR-4 one-click copy (no package) |

**No new dependency — Go or npm — is recommended.** Everything FR-1
through FR-7 needs already exists in this repo, either as a direct
dependency or as an established internal pattern with a concrete file:line
precedent.
