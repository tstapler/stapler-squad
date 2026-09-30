# Stack Research: session-classifier-pipeline

## 1. Rule-based matching engine — `pkg/classifier/classifier.go`

- `Rule` struct (`pkg/classifier/classifier.go:365-399`) is tool-use-domain-specific: `ToolName`,
  `ToolPattern *regexp.Regexp`, `ToolCategory`, `Criteria *CommandCriteria`, `CommandPattern
  *regexp.Regexp`, `FilePattern *regexp.Regexp`, `RequireCIPassing`, `MinSessionIdleMinutes`, plus
  shared/generic fields: `ID`, `Name`, `Decision ClassificationDecision`, `RiskLevel`, `Reason`,
  `Alternative`, `Priority int`, `Enabled bool`, `Source string` (aliased by typed `RuleSource`
  constants `SourceSeed`/`SourceUser`/`SourceClaudeSettings`, `classifier.go:401-411`).
- `RuleBasedClassifier` (`classifier.go:413-434`) holds `rules []Rule` sorted by `Priority`
  descending, guarded by `deadlock.RWMutex` (drop-in `sync.RWMutex` replacement, deadlock-detecting
  build — `github.com/linkdata/deadlock v0.5.5` in go.mod). `ReplaceRules`/`AddRules`/`Rules()` give
  atomic bulk-replace and read access; `Classify`/`classifyInternal`/`classifySingle` do the actual
  per-payload matching against `matchesRule`.
- Rules are plain `regexp.Regexp` (stdlib `regexp`, no third-party regex lib) matched with `.MatchString`.
- Per the requirements' Feasibility Risk, `Rule` as it stands is NOT reusable as-is for session
  tagging — it has no session-shaped fields (name/branch/path/program/tags) and its `Classify`
  orchestration returns a single `ClassificationResult` (decision-output), not tag-output. The
  requirements' Rabbit Hole is confirmed: a shared "rule core" (ID/Name/Priority/Enabled/Source +
  generic matching primitives) needs factoring out from the tool-use-specific fields, rather than
  bolting session fields onto the existing `Rule` struct.
- `SeedRules()` (`classifier.go`, near end) is the existing precedent for a seed-rule-set
  constructor style to mirror for tagging seed rules.

## 2. ent schema/migrations — `session/ent/schema/approvalrule.go`

Full existing schema to mirror (`session/ent/schema/approvalrule.go:1-101`):
- Fields: `rule_id` (unique, not-empty), `name`, tool-domain optional strings (`tool_name`,
  `tool_pattern`, `tool_category`, `command_pattern`, `file_pattern`), `decision int`, `risk_level
  int`, `reason`/`alternative` optional strings, `priority int` (default 0), `enabled bool`
  (default true), `source string` (default `"user"`), `created_at`/`updated_at` timestamps
  (`Immutable()` / `UpdateDefault(time.Now)`), plus structured `CommandCriteria` fields stored as
  `field.JSON([]string{})` with `Optional().Default([]string{})` (so SQLite migrations of old rows
  without these columns don't hit a NOT NULL constraint), and two scalar bools/int32
  (`safe_python_imports_only`, `require_ci_passing`, `min_session_idle_minutes`).
- Indexes: `rule_id`, `priority`, `enabled` (`Indexes()`, lines 94-100). No edges.
- A tagging-rule schema (e.g. `session/ent/schema/tagrule.go`) should mirror this shape but swap
  the tool-use fields for session-tagging fields (name/branch/path/program regex patterns, a
  dependency-tag match list) and a `tag`/`tags` output field instead of `decision`/`risk_level`.
  Per repo `CLAUDE.md`: after editing the schema, regenerate with
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (NOT
  the flagless form — it breaks `UpsertRule`-style methods) and do not commit generated
  `session/ent/*.go` output — `.gitignore` already excludes it.

### CRUD/service layer to mirror for `ApprovalRule`

- `server/services/rules_store.go`: `RulesStore` wraps `*session.Storage`; `All()`, `ToRules()
  []classifier.Rule`, `Upsert(ctx, spec) (RuleSpec, error)`, `Delete(ctx, id) error`,
  `WatchAndReload`, plus spec↔`classifier.Rule` conversion helpers (`specsToRules`,
  `parseDecision`, `parseRiskLevel`, `decisionToInt`/`riskLevelToInt` and their inverses) and a
  `BulkUpsert`.
- `server/services/rules_service.go`: `RulesService` wraps `rulesStore` +
  `classifier.RuleBasedClassifier`; `ListApprovalRules`, `UpsertApprovalRule`,
  `DeleteApprovalRule`, `ReloadClaudeSettingsRules`, `rebuildClassifier` (atomic
  `ReplaceRules` after CRUD), `filterRulesBySource(rules, allowed ...classifier.RuleSource)` (source
  filtering helper, ready to reuse), `specToProto`/`ruleToSpec` proto conversion, plus AI-suggestion
  flow (`GenerateSuggestedRule`, `parseSuggestions`, `validateSuggestion`) and YAML
  export/import (`ExportRules`, `validateYAMLEntry`, `BulkUpsertRules`).
- `server/mcp/tools_rules.go`: MCP tool handlers (`listApprovalRules`, `upsertApprovalRule`,
  `deleteApprovalRule`, `reloadClaudeSettingsRules`) — the same surface exposed to
  `mcp__stapler-squad__upsert_approval_rule` / `list_approval_rules` / `delete_approval_rule` tools
  visible in this session. A tagging-rule CRUD surface should add sibling tools here rather than a
  new MCP namespace, consistent with the requirements' "integrate with, not duplicate" scope note.

## 3. Background debounced pollers

- `session/pr_status_poller.go`: `PRStatusPoller` + `PRStatusPollerConfig` (`PollInterval`,
  `ConcurrentFetches`, `CallTimeout`, `AuthCacheDuration`, `NoPRBackoff`). `Start(ctx)`/`Stop()`
  run a `pollLoop` on a ticker; `SetInstances`/`AddInstance`/`RemoveInstance` manage the polled set;
  `SetOnUpdated(fn func(*Instance))` is the update-callback hook. No content-hash caching here —
  it dedupes via `ETagCache` (HTTP conditional GET) and a `NoPRBackoff` timer instead.
- `session/worktree_pr_poller.go`: `WorktreePRPoller` mirrors the same `Start`/`Stop`/`pollLoop`
  shape but scans worktrees via a `WorktreeSource` interface and caches per-worktree PR lookups in
  a `listCacheEntry` map plus a `setNoPRBackoff` per-key backoff — closest existing precedent for
  "poll independently, don't hit external state every tick."
- `session/review_queue_poller.go` (largest, most relevant to LLM-fallback caching): defines a
  **content-hash-shaped cache already** — `contentCacheEntry` (`review_queue_poller.go:151-158`)
  bundling `cachedContent string` + activity timestamps, stored in a
  `*xsync.Map[string, contentCacheEntry]` (`github.com/puzpuzpuz/xsync/v4 v4.5.0`, already a go.mod
  dependency — lock-free concurrent map, used repo-wide in place of `map + sync.RWMutex`) inside
  `pollerContentProvider`. `EvictInstance(title)` removes a session's cache entry on
  session-removal. This is the direct structural precedent for the classifier's LLM-result cache:
  key by session ID (or a stable session identifier), value bundling a content hash +
  last-classified tag + timestamp, in an `xsync.Map`, evicted alongside session teardown.
- None of the three pollers implement "debounce" via a leading/trailing timer per-item (e.g.
  golang.org/x/time/rate or a debounce library) — they use a fixed poll interval
  (`PollInterval`/ticker) plus per-item backoff timers (`NoPRBackoff`, `setNoPRBackoff`). A new LLM
  poller for session tagging should follow this same pattern: fixed `PollInterval` tick +
  content-hash-based skip (not a true edge-triggered debounce library) — consistent with what's
  already in the codebase, and directly satisfies "don't re-invoke the LLM on every render/poll
  when the content hash hasn't changed."
- No shared poller base/interface was found (`grep` for a common `Poller` interface came back
  empty) — each poller is copy-paste-and-adapt from the previous one. The requirements' rabbit hole
  ("reuse vs. duplicate is a real decision") should be resolved as "duplicate the pattern," matching
  existing practice, not "introduce a new base type," unless Phase 3 planning decides the fourth
  instance justifies extraction.

## 4. LLM invocation — two distinct existing paths

### `session/headless` package (subprocess `claude` CLI pool)

- `session/headless/pool.go`: `Pool` manages named "feature-key" sessions over the `claude` CLI
  subprocess (`claudeBin`), with session reuse for prompt-cache locality, `MaxCallsPerSession`
  rotation, and a `concurrencySem` for bounded concurrent subprocess calls. `PoolConfig.DefaultModel
  string` sets a pool-wide default model.
- `session/headless/caller.go`: `CallOptions.Model string` (`caller.go:29-30`) — "overrides the
  pool's `DefaultModel` for this call only," forwarded to the subprocess as `--model <value>`
  (`caller.go:188-198`). **Model selection is supported and already plumbed through as an arbitrary
  string** — passing `"haiku"` (or a full model ID) per-call is a one-line `CallOptions{Model:
  "haiku"}` change at any call site, no new plumbing needed.
- `session/headless/features.go`: `GenerateSessionCompletionNarrative(ctx, pool, sessionTitle,
  sessionGoal, diff, decisionsSummary) (string, float64, error)` (`features.go:345-364`) is prose-
  only — it builds a text prompt and returns `pool.CallBlocking(...)`'s raw string plus a `cost
  float64` (USD) side-output via callback. **No structured/JSON output mode exists in this
  package** — every `features.go` helper returns freeform prose. A new tag-classification helper
  here would need its own prompt asking for a single tag/JSON and would have to parse the raw
  string itself (no schema enforcement, no `--output-format json` flag observed in `caller.go`).

### `server/services` direct Anthropic API client (separate from `headless`)

- `server/services/anthropic_client.go`: a second, independent LLM path — calls
  `https://api.anthropic.com/v1/messages` directly (not via the `claude` CLI), with
  `anthropicModel = "claude-haiku-4-5-20251001"` (`anthropic_client.go:13-14`) as its **already-
  Haiku default model**. This is the AIClient behind `RulesService.GenerateSuggestedRule`
  (`server/services/rules_service.go:906-951`), gated on `ANTHROPIC_API_KEY` being set.
  `rs.aiClient.Complete(ctx, systemPrompt, userPrompt) (string, error)` — also prose-returning at
  the interface level, but the existing caller (`GenerateSuggestedRule`) already prompts for JSON
  and hand-parses it: `parseSuggestions(rawJSON)` (`rules_service.go:1025-1054`) does
  `json.Unmarshal` with a markdown-code-fence-stripping fallback (`cleaned` variable) when the
  model wraps its JSON in ```` ```json ```` fences. **This is the closest existing precedent for
  "structured tag output"** — prompt-engineered JSON + manual unmarshal/repair, not a native
  structured-output/tool-use schema constraint from the Anthropic API. A tag-classification LLM
  step can reuse either this `AIClient`/`anthropic_client.go` path (already defaults to Haiku, no
  subprocess/CLI dependency, needs `ANTHROPIC_API_KEY`) or extend `headless` with a new
  JSON-prompting feature function following `parseSuggestions`'s pattern — Phase 3 planning should
  pick one; there is no third "structured output" mechanism to discover.
- `server/services/cli_ai_client.go` exists alongside `anthropic_client.go` implementing the same
  `AIClient` interface — likely a CLI-shelling alternative implementation (not fully inspected;
  worth a quick look in Phase 3 to confirm it isn't the preferred path instead).

## 5. Content-hash caching primitives

- `crypto/sha256` (stdlib) is the established hashing primitive repo-wide for this kind of
  cache-key derivation: `internal/history/models.go:29`, `config/workspacepath/workspacepath.go:117`,
  `session/sshremote/approval_relay.go:74`, `pkg/analytics/escape_code_parser.go:273`,
  `session/pipeline_engine.go` (imports `crypto/sha256`). No third-party hashing library is used for
  this purpose (`session/workspace/lock_postgres.go` uses `hash/fnv`, but for a different,
  non-content-addressing purpose — advisory lock keys). The LLM-fallback cache should hash
  `sha256.Sum256([]byte(<content>))` over whatever fields Phase 3 pins down as "content" and store
  the resulting hex string as the cache-comparison key, consistent with existing call sites.

## 6. Module versions (go.mod highlights)

- `go 1.26.6` (module Go version)
- `entgo.io/ent v0.14.5` — ent ORM, used for all schema/migrations including `ApprovalRule`
- `github.com/linkdata/deadlock v0.5.5` — drop-in deadlock-detecting `sync.RWMutex`/`sync.Mutex`
  replacement, used by `RuleBasedClassifier.mu` and elsewhere
- `github.com/puzpuzpuz/xsync/v4 v4.5.0` — lock-free concurrent map, used by
  `review_queue_poller.go`'s content cache; recommended for the new LLM-result cache too
- `golang.org/x/sync v0.22.0` — available if a `singleflight`/`errgroup` pattern is needed for
  coalescing concurrent LLM calls on the same session
- `golang.org/x/time v0.15.0` — available for rate-limiting the LLM poller's outbound calls if
  needed (not currently used by any existing poller for this purpose — they use fixed ticker
  intervals plus per-item backoff instead)
- Regex matching throughout `pkg/classifier` uses stdlib `regexp` only — no third-party regex
  engine dependency to account for

## Session for Phase 3 (open questions this research feeds)

- **Rule struct shape**: confirmed a shared "rule core" (ID/Name/Priority/Enabled/Source +
  generic string/regex matching) needs factoring out of `Rule`; the tool-use fields
  (`ToolName`/`Criteria`/`RequireCIPassing`/etc.) and new session-tagging fields
  (name/branch/path/program patterns + tag-dependency list) should live in separate
  domain-specific structs alongside the shared core, not one struct with both.
- **LLM invocation path**: two real candidates exist — `session/headless` (CLI subprocess pool,
  already supports `Model` override, no JSON/structured mode) vs. `server/services.AIClient` /
  `anthropic_client.go` (direct Anthropic API, already defaults to Haiku, existing JSON-prompt +
  parse precedent via `parseSuggestions`). Recommend Phase 3 pick `anthropic_client.go`'s path
  given it already defaults to Haiku and already has a working JSON-output precedent — using
  `headless` would require adding both model-override wiring (trivial) and a net-new JSON-parsing
  helper (duplicating `parseSuggestions`'s fence-stripping logic) for no clear benefit, unless there
  is a reason to prefer the CLI-subprocess pool's session-reuse/prompt-caching behavior for this
  workload. Should also inspect `server/services/cli_ai_client.go` (not yet read in depth) before
  finalizing.
- **Debounce/poller lifecycle**: no shared poller base exists; follow the copy-adapt pattern from
  `pr_status_poller.go`/`worktree_pr_poller.go`, using `xsync.Map`-backed content-hash caching per
  `review_queue_poller.go`'s `contentCacheEntry` precedent, and a fixed-interval ticker (not an
  edge-triggered debounce library) consistent with existing pollers.
- **Content-hash definition**: not resolved here (explicitly deferred to Phase 3 per requirements)
  — but the hashing *mechanism* (`sha256.Sum256`) is settled by existing convention.
