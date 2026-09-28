# Research: Build vs. Buy — backlog-diagnose-and-nudge

## 1. Existing OSS agent for "diagnose a stuck automation, take bounded corrective action"

Searched for self-healing CI/incident-response agents and AI-SRE/on-call agent products (2026 landscape):

- **HolmesGPT** (Robusta + Microsoft, CNCF Sandbox, Apache 2.0) — investigates production incidents across Kubernetes/VMs/cloud/DB/SaaS, posts root-cause findings to Slack. [github.com/HolmesGPT/holmesgpt](https://github.com/HolmesGPT/holmesgpt)
- **K8sGPT** (CNCF Sandbox, Apache 2.0) — rule-based scanner over Kubernetes objects, LLM only explains findings, not general-purpose. [k8sgpt.ai](https://k8sgpt.ai)
- **Keep** — alert-management/correlation platform, not a diagnose-and-act agent.
- Several narrow "self-healing CI/CD" GitHub projects (`senthilkumaranT/self-healing-ci-cd-pipeline`, `chitanya-chitturi/self-healing-ci-agent`) — hobby-grade, built around GitHub Actions webhook → LLM → auto-PR, no tmux/session concept.

**Pros:** Mature governance patterns worth stealing (detect→triage→diagnose→plan→approve→remediate→verify→learn loop; audit trail; escalate-to-human path) — HolmesGPT in particular is a good reference architecture to read, not adopt.

**Cons:** Every credible OSS option (HolmesGPT, K8sGPT, Aurora) is built around Kubernetes/observability data sources (Prometheus, kubectl, cloud APIs) as its diagnostic surface. None understand tmux sessions, git worktrees, or this repo's `Instance`/`Snapshot()` model. Adopting one means writing a full custom "data source" integration layer that is as much work as the diagnostic-context-bundle assembly this project already scopes, while also importing an entire agent-orchestration framework (most wrap LangGraph/their own DAG runner) that duplicates `session/headless.PoolClient` and `AutonomousDriver`, which this repo already has and which are Claude-Code-CLI-native.

**Verdict: Not recommended.** Read HolmesGPT's loop-stage vocabulary for the plan doc's terminology; do not vendor the code.

## 2. SaaS/managed equivalents

PagerDuty AIOps, incident.io AI, Cleric.ai, Resolve.ai-class products offer "AI on-call engineer" diagnosis, typically $/seat or $/incident pricing, and expect telemetry (logs, traces, metrics) piped to their cloud.

**Pros:** Zero build cost for the orchestration/governance shell; some offer Slack-native nudge/approve UX out of the box.

**Cons:**
- This is explicitly a **single-user, fully local, self-hosted** tool (requirements.md's Constraints section). Every one of these products requires shipping session/backlog data — which includes tmux scrollback, git diffs, and potentially secrets in logs — to a third-party cloud. That is a real trust/architecture departure, not a config toggle, and the requirements explicitly flag this.
- None of them understand this repo's domain objects (`BacklogItem`, `Instance`, worktrees) — integration would mean building a custom connector anyway, at which point the SaaS layer adds cost and data-residency risk without removing meaningful build work.
- Per-incident/seat billing is a poor fit for a single-developer nightly-cron-style workload.

**Verdict: Not recommended.**

## 3. LLM-generated vs. battle-tested library, per sub-problem

### 3a. Token counting / budget enforcement
Requirements.md says "capped at ~250,000 tokens via `session/tokens`." **This is a mismatch worth correcting before planning.** `session/tokens` (see its package doc, `session/tokens/doc.go:1-20`) is a **post-hoc JSONL usage-analytics parser** — it reads token counts *already recorded* in Claude Code's own transcript files after a run completes (`ParseFile`, `TokenStore`, `ComputeFindings`, `PricingTable`). It has no function that estimates the token count of an arbitrary string *before* sending it — there is nothing like `EstimateTokens(text string) int` anywhere in that package (confirmed via `grep -n "^func" session/tokens/*.go`).

The actual precedent for pre-flight budget enforcement already in this repo is `BuildTokenBudgetedPrompt` in `session/backlog_context.go:312-332`: a `len(output)/4` heuristic, with a documented budget (4000 tokens) and a two-pass reduction strategy (drop prior sessions, then truncate description to 500 chars) when over budget.

Web research confirms `len/4`-style heuristics are the right order of magnitude for Claude specifically: Anthropic's tokenizer is proprietary and unpublished, tiktoken (OpenAI's library) undercounts Claude text by 15-30% and should not be used ([Token counting — Claude Platform Docs](https://platform.claude.com/docs/en/build-with-claude/token-counting); [Token Counting Done Right: Stop Using tiktoken for Claude](https://dev.to/pavelespitia/token-counting-done-right-stop-using-tiktoken-for-claude-383c)). Anthropic's only accurate option is the server-side `/v1/messages/count_tokens` API — this repo's headless calls go through `session/headless.Pool.CallBlocking`, which shells into the `claude` CLI rather than calling the Anthropic API directly (no `anthropic-sdk` in `go.mod`), so wiring the real count_tokens endpoint would mean adding a new direct-API HTTP dependency keyed off the already-present `config.AnthropicAPIKey` (`config/config.go:401`) purely for pre-flight counting — a nontrivial new integration for a bound-checking feature.

**Verdict: Fork/adapt `BuildTokenBudgetedPrompt`'s heuristic, generalize it for the much larger (250k) and more heterogeneous bundle.** Do not adopt tiktoken (measurably wrong for Claude). Do not route through `session/tokens` (wrong shape — analytics, not pre-flight estimation) — the requirements.md wording should be corrected to point at `backlog_context.go`'s pattern instead, generalized (per-section budgets/caps for description, AC, history, session snapshot, logs, diff, not one flat len/4 over the whole bundle). Calling the real Anthropic count_tokens API is **Viable** as a stretch improvement (more accurate margin) but not required for v1 given the existing heuristic already ships in production for a similar purpose.

### 3b. Context compaction/summarization
`HandoffSummaryGenerator` (`session/handoff_summary_service.go`) is real, shipped (PR #612), and battle-tested for its actual job: compacting the **middle portion of one session's own transcript** into a handoff summary for a **new context window continuing the same task** — its core call, `GenerateHandoffSummary` (`session/headless/features.go:439`), takes `head, middle, tail []HandoffTranscriptMessage`, i.e. a single ordered message stream, not an arbitrary heterogeneous bundle.

The diagnostic-context bundle in scope here is **not** one session's transcript — it's item description + AC + status/history + review verdicts + a *linked* session's `Snapshot()` state + recent logs + a git diff. `HandoffSummaryGenerator` is a correct, direct reuse for exactly one component of that bundle (compacting the linked session's own scrollback/transcript, if that specific piece is over its sub-budget) — it is not a universal bundle compactor, and the plan should not assume calling it once over the whole assembled bundle will work, since its system prompt and message-shaped input assume transcript continuity semantics ("what came before/after the portion you are summarizing").

**Verdict: Recommended, reuse is correct — but scope it precisely.** Use `HandoffSummaryGenerator` only for the session-transcript component of the bundle; use simple truncation/prioritization (same two-pass style as `BuildTokenBudgetedPrompt`) for the other bundle sections (git diff, item history, review verdicts). Native `/compact` (the new diagnostic agent's own context, per requirements) is correctly out of scope for building — it's a Claude Code CLI built-in, nothing to fork.

### 3c. Idle/liveness detection
`detection.StatusIdle` is genuinely reused elsewhere for exactly this kind of gate — e.g. `session/autonomous_driver.go:590`, `server/adapters/review_queue_adapter.go`, `session/session_goal.go`. It is a real, general per-instance status classification (`session/instance_status.go`), not something narrowly built for one caller.

One caveat carried over from prior-session memory (not re-verified in this pass): `detection.StatusReady` is documented elsewhere as dead code — never produced by `MatchLines` — so any new gating logic should check the actual `MatchLines`/detector implementation before assuming every status constant in that package is live, rather than copying `autonomous_driver.go:590`'s `StatusIdle || StatusReady || StatusSuccess` triple blindly.

**Verdict: Recommended**, reuse `detection.StatusIdle` as the requirements already state — it's a proven, actively-used gate, not a stretch.

## 4. Fork or adapt an existing in-repo implementation

- **Dispatch/notification plumbing.** All the MCP tools requirements.md names as the dispatch surface already exist verbatim and are wired: `create_session` and `resume_session` (`server/mcp/tools_lifecycle.go:54,77`), `create_session_for_pr` (`server/mcp/tools_github.go:77`), `write_to_session` / `steer_session` (`server/mcp/tools_terminal.go:83,139`), `create_backlog_item` / `post_backlog_update` (`server/mcp/tools_backlog.go:2868,2907`). This is the strongest, lowest-risk reuse in the whole feature — the new diagnostic agent is fundamentally "another AutonomousDriver-style consumer of the same MCP tool surface a human/Claude session already uses," not new plumbing.
- **Bounded corrective-action / retry-with-backoff precedent.** `session/backlog_lifecycle_triage.go` already implements almost exactly the shape this feature needs at a smaller scope: `reconcileOrphanedTriageItems`, `reconcileOrphanedTriageRemediation`, and `retryOrphanedTriageWithBackoffGate` (`session/backlog_lifecycle_triage.go:172,353,384`) — a listener that finds items stuck in one specific state (orphaned triage) and takes a bounded, gated corrective action (retry with backoff), not just visibility. This is a much closer structural analog to "diagnose stuck, take bounded action" than any OSS project surfaced in section 1, and is a strong fork/adapt candidate for the new feature's own orphan/nudge loop shape — same backoff-gate idea, generalized from "orphaned triage session" to "stuck backlog item + linked session" more broadly.
- **Turn-cap precedent.** `Config.AutonomousMaxTurnsOrDefault()` (`config/config.go:935-946`) — default 60, hard ceiling 200, config-overridable, zero/negative falls back to default — is a clean, small, directly-copyable pattern for the new nudge cap/cooldown config field; no need to invent a new config shape.
- **Stale retry-session cleanup.** Confirmed present: `session/backlog_lifecycle_superseded_test.go` and superseded-session-retirement code exist in `session/backlog_lifecycle*.go` (PRs #808/#810 per requirements.md), extending it is straightforward rather than parallel-building.

**Verdict: Recommended.** This is the one area where "reuse" is unambiguously correct and low-risk across every sub-piece requirements.md names, plus one additional strong candidate (`backlog_lifecycle_triage.go`'s orphan-reconciliation loop) it didn't explicitly name but should be read before designing the new diagnose-and-nudge loop from scratch.

## Summary table

| Option | Verdict |
|---|---|
| Adopt an OSS self-healing-CI / AI-SRE agent (HolmesGPT, K8sGPT, etc.) | Not recommended — built for k8s/observability data sources, not tmux/session/git |
| SaaS AI-on-call / AIOps product | Not recommended — violates local-only constraint, wrong billing model, still needs a custom connector |
| `tiktoken`-style tokenizer library | Not recommended — measurably wrong for Claude (15-30% undercount) |
| Anthropic `count_tokens` API for pre-flight budget | Viable stretch improvement, not required for v1 |
| Reuse/generalize `BuildTokenBudgetedPrompt`'s `len/4` heuristic | Recommended (requirements.md's "via `session/tokens`" wording should be corrected to point here instead) |
| Reuse `HandoffSummaryGenerator` for the whole bundle | Not recommended as stated — it's transcript-shaped, not bundle-shaped |
| Reuse `HandoffSummaryGenerator` for the session-transcript sub-piece only | Recommended |
| Reuse `detection.StatusIdle` | Recommended |
| Reuse existing MCP dispatch tools (`create_session`, `resume_session`, `create_session_for_pr`, `write_to_session`, `steer_session`, `create_backlog_item`, `post_backlog_update`) | Recommended |
| Fork `backlog_lifecycle_triage.go`'s orphan-reconciliation/backoff-gate loop as the shape for the new diagnose-and-nudge loop | Recommended |
| Reuse `AutonomousMaxTurnsOrDefault()` pattern for nudge cap/cooldown | Recommended |
| Extend existing superseded-session retirement (PR #808/#810) rather than a parallel mechanism | Recommended |
