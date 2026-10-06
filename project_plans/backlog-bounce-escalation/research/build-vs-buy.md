# Build vs. Buy — backlog-bounce-escalation

Scope per requirements.md: (1) multi-reason stuck-state severity signal, (2) durable
"capped-while-bouncing" escalation marker, (3) optional flaky-test-aware review strategy.
Single user, self-hosted, backlog size in the tens of items.

## 1. OSS "escalation policy" library (severity scoring / dedup / flapping detection)

Landscape: Prometheus Alertmanager (grouping/dedup/inhibition/silences), OpenTelemetry
Collector alerting processors, Cabot, Grafana OnCall's escalation-policy engine — all built
for routing alerts to *people* across on-call schedules, with dedup keys, flap-damping
windows, and multi-channel notification fanout.

- **Pros**: battle-tested flapping/dedup algorithms; would eliminate hand-rolling threshold
  logic.
- **Cons**: every one of these is designed around routing to on-call humans/schedules/pager
  channels — none of which exist here (single user, one UI, no oncall rotation). Adopting
  any of them means running a second service (or embedding a heavyweight rule engine),
  standing up its own config/storage model, and translating `backlog_stuck_states` rows into
  its alert model — for a domain that's "count rows WHERE closed_at IS NULL GROUP BY
  item_id" against a Postgres table already queried directly elsewhere in this codebase
  (`server/services/backlog_service_stuck.go`, `session/stuck_decisions.go`).
- **Verdict**: **Overkill — do not adopt.** The requirements doc's own Rabbit Holes section
  already flags this risk ("could balloon into a full scoring/weighting system — keep the
  initial version to a simple count/threshold"). A `COUNT(*) ... GROUP BY item_id HAVING
  COUNT(*) >= N` query plus a small Go struct is the entire "escalation policy" this feature
  needs.

## 2. SaaS/managed incident-alerting API (PagerDuty, Opsgenie, Better Uptime, etc.)

- **Pros**: mature severity/priority modeling, on-call routing, mobile push, SLA timers.
- **Cons**: this is an internal, self-hosted, single-user tool with no oncall — there is no
  "page someone" use case, only "surface a signal in the UI/notification feed I already
  read." Requirements explicitly scope this to the existing `EventBusNotifier`
  (`server/services/backlog_notifier.go`) / durable-row notification pattern used by the
  prior `backlog-stuck-item-visibility` project, not a new external channel. Bringing in a
  paid SaaS dependency for a problem solved by "add a boolean/enum column and check it in
  the reconcile loop" would add an external service, API keys/secrets management, and
  network-failure handling for zero incremental value to a single reader.
- **Verdict**: **Overkill — do not integrate.** No oncall, no multi-recipient routing, no
  external escalation need exists in this product.

## 3. LLM-generated bespoke logic vs. battle-tested library, per sub-piece

### 3a. Counting/aggregating open stuck reasons
Trivial — this is a `GROUP BY item_id` count (or an in-process count over
`OpenStuckStateData` rows already fetched in `ListStuckBacklogItems`,
`server/services/backlog_service_stuck.go:130`). No library of any kind is justified; a
plain SQL aggregate or a `map[string]int` tally is the entire implementation.
**Verdict: bespoke, trivial.**

### 3b. Flaky-test classification
Three realistic options, per the requirements doc's own Rabbit Holes framing ("keyword match
... vs LLM classification step"):

- **Keyword/regex heuristic** (bespoke, cheap): match item title/description against a
  fixed term list (`flaky`, `intermittent`, `-race`, `non-determin`, `TestXxx_flake`, etc.).
  Zero latency, zero LLM cost, fully deterministic and testable with table-driven Go tests.
  Weakness: only as good as the keyword list; won't catch a flaky-test fix whose title
  doesn't self-describe as such.
- **LLM classification call**: this pattern is **already established in this codebase** —
  `server/services/backlog_service_triage.go` routes headless LLM calls through
  `s.pipelineEngine.TriagePromptFor` / `session.BuildHeadlessTriagePrompt` and a
  `headless.CallBlocking` invocation (see `triageCallBudget`, `classifyHeadlessCallError`,
  and the triage flow around backlog_service_triage.go:2133-2440). So "call an LLM to
  classify something about a backlog item" is a proven, already-wired pattern in this
  repo — it is not a new integration, just a new prompt/call site. Cost: added latency (a
  full headless LLM turn) and non-determinism in the classification itself, which is an odd
  fit for a feature whose whole premise is "flaky/non-deterministic signals are unreliable."
- **Dedicated library**: no realistic general-purpose "is this a flaky test fix" classifier
  library exists — this is inherently domain-specific text classification.

**Verdict**: start with the **keyword heuristic** — it directly matches the Rabbit Holes
guidance to keep this cheap and not let it become "a general intent-classification
subsystem," and the existing three live bounce items (`ccbfe7a6`, `e271db3d`, `92d679fd`)
are themselves confirmed-flaky-test titles, i.e. the keyword case would already catch the
motivating examples. Reserve the LLM-call option only if the heuristic proves too noisy in
practice post-ship — and if adopted, reuse the existing `pipelineEngine`/`headless` call
pattern rather than inventing a new LLM integration path.

### 4. Fork or adapt something already in this codebase

`pkg/classifier` (`pkg/classifier/classifier.go`, `escalation.go`) is directly adjacent
prior art: a `RuleBasedClassifier` that inspects a request (there: a tool-use permission
payload) against ordered rules and returns a `ClassificationDecision` (`AutoAllow` /
`AutoDeny` / `Escalate`) plus a `RiskLevel` (`RiskLow`...`RiskCritical`, an ordered `iota`
enum) and a `Reason` string — consumed by `server/services/approval_handler.go` to decide
whether to escalate a request for manual review, with `EscalationReason` /
`EscalationCategory` persisted on the resulting record (`server/services/approval_store.go`).
This is structurally the same shape as "classify a backlog item's stuck state into a
severity level and persist why."

- **Pros of adapting the *pattern* (not the code)**: proven shape for this exact problem —
  ordered rule evaluation → decision + reason string, persisted alongside the record it
  judged. `session/review_queue_poller.go`'s `riskLevelRankTable` also shows an established
  convention for ranking severity strings for display/sort purposes, directly reusable for
  ordering "elevated" vs. "normal" stuck items in the UI.
- **Cons of literally forking `pkg/classifier`**: it's purpose-built for tool-permission
  payloads (`PermissionRequestPayload`, regex command matching via
  `command_parser.go`) — none of that machinery applies to counting stuck-state rows.
  Importing the package wholesale would pull in unrelated surface area for zero reuse.
- **Verdict**: **adapt the pattern, not the package.** Model the new logic as a small,
  package-local function (e.g. in `session/stuck_decisions.go`, alongside
  `isBouncing`/`stuckPRReady`/`abandonedReview`) that takes the open-reason count +
  remediation-attempt state and returns a severity/reason value, in the same style as the
  existing pure predicate functions in that file — plus reuse the
  `RiskLevel`-style ordered-enum and `riskLevelRankTable`-style ranking convention already
  established for `approval_store.go`/`review_queue_poller.go`, rather than inventing a new
  scoring vocabulary.

## Summary

| Option | Verdict |
|---|---|
| OSS escalation-policy library (Alertmanager-style) | Overkill — reject |
| SaaS incident/alerting API | Overkill — reject (no oncall, internal-only) |
| Reason-count aggregation | Bespoke SQL/Go, trivial — no library |
| Flaky-test classification | Bespoke keyword heuristic first; LLM-call fallback reuses existing `pipelineEngine`/`headless` pattern already in `backlog_service_triage.go` if heuristic proves insufficient |
| Adjacent in-repo code | Adapt the `pkg/classifier` / `RiskLevel` / `riskLevelRankTable` *pattern* (ordered decision enum + persisted reason), not the package itself — implement fresh in `session/stuck_decisions.go`'s existing pure-predicate style |
