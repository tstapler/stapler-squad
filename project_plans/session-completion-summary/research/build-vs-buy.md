# Build vs. Buy: Session Completion Summary

Agent 6 research. Bias: reuse what's already in this codebase (ponytail: stdlib →
native → already-installed dep → one line → minimum code, before any new dependency).

## 1. Markdown generation

No markdown-templating library exists anywhere in the Go codebase. The one directly
analogous prior art is `server/services/rule_prompt_builder.go` (`DefaultRulePromptBuilder`,
used to assemble LLM prompt context from structured data) — it builds text with plain
`strings.Builder` + `fmt.Sprintf`, no templating engine at all
(`server/services/rule_prompt_builder.go:20,110`). `text/template` is not used for
markdown assembly anywhere in `server/` or `session/`. The `pr-description-generator`
agent is a Claude Code prompt-driven agent (`.claude/agents/pr-description-generator.md`),
not reusable Go code — it informs *tone/structure* of the narrative section, not the
implementation mechanism.

- **Build with `strings.Builder`** — Pros: zero new dependency, mirrors the one existing
  markdown/prompt-assembly precedent in this repo exactly, trivial to unit test (assert on
  output string). Cons: manual escaping of user-controlled content (branch names, commit
  messages) needed to avoid breaking GFM table/code-fence structure. **Verdict: Recommended.**
- **`text/template`** — Pros: separates structure from data, marginally more readable for a
  long multi-section doc. Cons: adds a new pattern the codebase doesn't otherwise use for
  this purpose, template-injection-shaped bugs (unescaped `{{}}` collisions) are new surface
  area for zero real benefit at this document's size (~6 fixed sections). **Verdict: Viable**
  only if the doc grows enough sections that `strings.Builder` control flow gets unwieldy;
  not needed at current scope.
- **Markdown-specific Go library (e.g. `goldmark`, `blackfriday`)** — these *parse/render*
  markdown to HTML; they don't help *generate* markdown text. Not applicable to this
  problem. **Verdict: Not recommended.**

## 2. LLM narrative generation

Two candidate abstractions exist; only one is real code today.

**`session/headless.Pool`** (`session/headless/pool.go`, `caller.go`) is a working,
already-integrated "call an LLM, get text + cost back" abstraction:
`PoolClient.CallBlocking(ctx, key FeatureKey, systemPrompt, userPrompt string, opts CallOptions) (string, float64, error)`
(`session/headless/client.go:8`) — returns the completion text *and* `CostUSD` in one
call. It's used today for backlog triage (`session/backlog_lifecycle.go`,
`server/services/backlog_service_triage.go`) and is wired through `server/dependencies.go`
with a nil-safe fallback (pool absent → feature silently no-ops, matching FR-2's
"deterministic non-LLM fallback" requirement almost exactly). It shells out to the local
`claude` CLI binary rather than calling the Anthropic API directly — heavier per-call
(subprocess spawn) but zero new dependency, already handles session reuse, concurrency
limits, and circuit-breaking (`session/headless/pool.go:34-79`).

**`AnthropicAIClient`** was *designed* in `project_plans/ai-rule-generation/decisions/ADR-001-rule-ai-provider-interface.md`
(a `RulePromptBuilder`/`AIClient` interface pair calling the Anthropic Go SDK directly) but
was **never implemented** — confirmed by `find server/services -iname "*anthropic_ai_client*"`
returning nothing, and no `anthropic-sdk-go`-shaped module in `go.mod`. It exists only as a
plan.

- **Reuse `session/headless.Pool.CallBlocking`** — Pros: real, tested, already async-callable
  from a service (matches FR-5's async/non-blocking requirement), returns cost data FR-2 also
  needs (token/cost line), already has a documented nil-safe "feature disabled" fallback path.
  Cons: subprocess-per-call overhead (fine for a once-per-session-end generation, not a hot
  path); requires the `claude` CLI binary present, same constraint backlog triage already
  accepts. **Verdict: Recommended.**
- **Build a new direct Anthropic API client (per unimplemented ADR-001)** — Pros: lower
  latency, no subprocess, no CLI binary dependency. Cons: net-new dependency (`go.mod` has no
  Anthropic SDK today), duplicates work `headless.Pool` already does for a near-identical use
  case (single-shot prompt → text), and this ADR was itself deferred/unimplemented for the
  original rule-generation feature — reintroducing it here means solving the same integration
  problem twice instead of once. **Verdict: Viable** only if `headless.Pool`'s subprocess
  latency proves unacceptable in practice; not the starting point.

## 3. Frontend markdown rendering + copy-to-clipboard

`web-app/package.json` already has `react-markdown` (`^10.1.0`) and `remark-gfm`
(`^4.0.1`) as dependencies — no rendering usage found yet in `web-app/src` (`rg` for
`ReactMarkdown|react-markdown|marked(` returned no matches), meaning these are installed
but not yet wired into any component. This is the correct renderer to reach for since it's
already a dependency and already GFM-aware (matches FR-4's "export as GFM markdown").

For copy-to-clipboard, `web-app/src/lib/clipboard.ts` already exports
`copyToClipboard(text: string): Promise<boolean>` — it wraps `navigator.clipboard.writeText`
with an `execCommand('copy')` fallback for plain-HTTP LAN access (where
`navigator.clipboard` is undefined). It's already used across the app (account page, config
page, history/logs pages, terminal context menu, VCS widget header — 10+ call sites).

- **Reuse `react-markdown` + `remark-gfm`** for any in-app markdown preview — Pros: zero new
  dependency, GFM tables/checklists render correctly, already installed for this exact
  purpose. Cons: none material. **Verdict: Recommended.**
- **Reuse `copyToClipboard()` from `web-app/src/lib/clipboard.ts`** rather than calling
  `navigator.clipboard.writeText` directly — Pros: existing tested wrapper, consistent
  behavior/fallback with every other copy affordance in the app. Cons: none.
  **Verdict: Recommended** (do not reintroduce a raw `navigator.clipboard` call or a new
  clipboard package — both would be inconsistent with the established pattern).

## 4. Async job execution / dedup guard

No job-queue library exists in `go.mod` (checked full dependency list: `robfig/cron/v3` is
for cron scheduling, `puzpuzpuz/xsync/v4` is a concurrent map, neither is a job queue).
The app is single-server/single-process; nothing in the codebase uses a distributed queue
(Redis/SQS/etc.) — confirmed no such dependency exists. The established precedent for
per-key async dedup is `session/headless/pool.go`'s `keyMu map[FeatureKey]*sync.Mutex` +
`acquireKeyMu()` (`session/headless/pool.go:80-90`) — a lazily-created per-key mutex map
guarding concurrent calls for the same feature key, which is structurally identical to
"per-session in-flight generation guard" (FR-7), just keyed by session ID instead of
`FeatureKey`.

- **`sync.Map` (or a plain `map[string]*sync.Mutex` behind one `sync.Mutex`, matching
  `headless.Pool`'s exact pattern) + a plain `go func()`** — Pros: zero new dependency,
  directly mirrors an existing, working in-repo pattern for the identical problem shape,
  trivially testable. Cons: none for this document's scale (one generation per session-end
  event, not a high-throughput queue). **Verdict: Recommended.**
- **A job-queue library (e.g. `asynq`, `machinery`, `river`)** — Pros: would add
  retry/backoff/observability UI out of the box. Cons: net-new dependency, net-new
  infrastructure assumption (most require Redis or a dedicated queue table) for a
  single-process app with no other distributed-queue usage anywhere in the codebase — directly
  contradicts the interface-pollution/anti-speculative-abstraction convention
  (`.claude/rules/interface-pollution-checklist.md`) and this being "a single-server,
  single-process app." **Verdict: Not recommended.**

## 5. Diffing / diff-stat computation

`SessionService.GetSessionDiff` (`server/services/session_service.go:2586-2644`) already
does everything this feature needs: for a live session it calls
`instance.UpdateDiffStats()` / `GetDiffStats()`; for a completed session (no longer in
memory) it reconstructs the worktree from stored `InstanceData` and calls `wt.Diff()`
directly — i.e., it already handles the "session ended, diff must still be computable"
case this feature needs. The response (`sessionv1.DiffStats{Added, Removed, Content}`)
already provides insertions/deletions counts and diff content, matching FR-2's "diff stat +
link" requirement. Underlying diff computation lives in `session/git` (`git.DiffStats`,
`git.NewGitWorktreeFromStorage(...).Diff()`), already go-git-based per
`.claude/rules/prefer-go-git-over-subshells.md`.

One gap to flag for the plan phase (not a build-vs-buy question): FR-3 requires the summary
to survive **Session-row deletion**, but `GetSessionDiff`'s completed-session path still
depends on `InstanceData`/worktree metadata existing in storage. If a session row (and its
worktree) is deleted before the summary is generated, `GetSessionDiff` will not have
anything to diff — meaning diff-stat capture needs to happen (or be finalized) at
session-end time, before deletion, not deferred to whenever the summary is first read. No
new diff library is needed either way.

- **Reuse `SessionService.GetSessionDiff` / underlying `git.DiffStats`** — Pros: 100% covers
  the diff-stat need, zero new code for diffing itself, already exercises both live and
  completed-session code paths. Cons: none for diffing itself; sequencing (capture before
  session-row deletion) is a plan-phase concern, not a build-vs-buy one.
  **Verdict: Recommended.**

## Summary table

| Area | Recommendation | New dependency? |
|---|---|---|
| 1. Markdown generation | `strings.Builder`, mirroring `rule_prompt_builder.go` | No |
| 2. LLM narrative | Reuse `session/headless.Pool.CallBlocking` | No |
| 3. Frontend render + clipboard | `react-markdown`+`remark-gfm` (installed, unused) + `lib/clipboard.ts`'s `copyToClipboard()` | No |
| 4. Async dedup guard | Per-session-ID mutex map, mirroring `headless/pool.go`'s `keyMu` | No |
| 5. Diff stats | `SessionService.GetSessionDiff` / `git.DiffStats` as-is | No |

Every area resolves to reusing existing code or stdlib — no new dependency is justified
anywhere in this feature.
