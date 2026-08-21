# Build vs. Buy: CI Status in Diff Viewer

## 1. Existing OSS library (e.g. `github.com/google/go-github`)

**Evidence**: `go.mod` (repo root) has no GitHub SDK dependency at all — `grep -n "github.com/google/go-github\|go-github" go.mod` returns nothing. The full `require` block (`go.mod:9-33` direct, `:53-170` indirect) contains no GitHub API client library. All GitHub integration in this repo is hand-rolled: either shelling out to the `gh` CLI via `executor/safeexec`, or making raw `net/http` calls to the REST/GraphQL endpoints.

**Three existing CI-status fetchers, confirmed distinct**:

| File | Mechanism | Input shape | Normalizer | Output vocabulary |
|---|---|---|---|---|
| `github/client.go:256,335-373` (`GetPRInfoCtx` → `getCheckConclusion`) | `safeexec` subprocess: `gh pr view --json statusCheckRollup` | Array of `ghStatusCheckItem{Name,Context,State,Status,Conclusion}` (`:113-120`) | `getCheckConclusion()` aggregates across all checks with a conclusion→status→state fallback per item | `"failure"` / `"pending"` / `"success"` / `"neutral"` / `""` |
| `github/user_pr_cache.go:563-568,663-674` (`fetchUserPRsForToken` → `normalizeCheckState`) | Native HTTP GraphQL POST (no subprocess) | Single pre-aggregated `commits.nodes[0].commit.statusCheckRollup.state` string — GitHub itself does the rollup server-side | `normalizeCheckState()` maps `SUCCESS`/`FAILURE`/`ERROR`/`PENDING`/`EXPECTED`, else lowercases passthrough | `"success"` / `"failure"` / `"pending"` / lowercase(anything else) |
| `session/backlog_plugin_github_prs.go:161-193` (`fetchCILabel`) | Native HTTP REST GET `/repos/{owner}/{repo}/commits/{sha}/check-runs` | Array of `githubCheckRun{Conclusion string}` — **only decodes `conclusion`, not `status`**, so in-progress checks are invisible to it | None — a bare boolean scan for `conclusion == "failure" \|\| "timed_out"` | Binary: `"pr:ci-failing"` label or `""` (no success/pending states at all) |

**Are these consolidatable, or is that itself real work?** Partially each — not a single drop-in extraction, but not a from-scratch rebuild either:

- `client.go`'s `getCheckConclusion` is the richest and most correct normalizer (it's the only one of the three that actually implements a real state machine over individual checks: fallback chain conclusion→status→state, explicit handling of `failure`/`error`/`action_required`/`timed_out` as failure, `in_progress`/`queued`/`pending` as pending, everything else — including `skipped`/`cancelled` which aren't explicitly named — collapsing to `"neutral"`).
- `user_pr_cache.go`'s input is fundamentally different (GraphQL already hands back one pre-aggregated string, not a per-check array), so it can't reuse `getCheckConclusion`'s aggregation loop as-is — but it *can* be made to emit the same output vocabulary (`success`/`failure`/`pending`/`neutral`) instead of its own passthrough default, which today would leak raw GitHub enum values like `EXPECTED`/`STARTUP_FAILURE` unlowercased-but-unmapped into the app.
- `backlog_plugin_github_prs.go`'s `fetchCILabel` hits the same REST Check Runs endpoint shape as an array-of-checks and is the weakest of the three (missing `status`, no pending detection, no shared vocabulary) — it's the one genuinely due for an upgrade: decode `status`+`details_url` in addition to `conclusion`, and delegate to a shared normalizer.
- **None of the three currently capture a check/run URL** (`details_url` / `html_url` on the check-run object). This is a real gap for AC #2 ("badge links out to the GitHub Actions run") — it's new code regardless of which fetcher gets reused, not something to consolidate away.

**Verdict: Recommended — reuse/consolidate, do not add a new SDK dependency.** The requirements doc's own framing (`requirements.md:51-52`, AC #3) is correct: build a single canonical CI-status type + normalizer (modeled on `client.go`'s `getCheckConclusion`, since it's the most complete state machine already in the repo), have the REST Check Runs path (`fetchCILabel`) upgraded to decode `status`+`details_url` and funnel through it, and have the GraphQL path keep its pre-aggregated shortcut but map through the same output vocabulary. This is real consolidation work (not a one-line extract), but it is strictly smaller than either (a) adding `go-github` as a fourth dependency and a fourth fetch path, or (b) leaving three divergent implementations to rot further out of sync.

## 2. SaaS / managed status-aggregator API

**Verdict: Not recommended / N/A.** GitHub Actions is already the target system being queried — there is no third party in the loop to aggregate. The repo already authenticates to GitHub via `gh`'s stored credentials and PATs (`github/client.go`, `github/user_pr_cache.go`'s `collectAllTokens`), so there's no new auth surface a SaaS aggregator would simplify. Confirmed no existing dependency on any CI-status SaaS (Buildkite Analytics, Codecov status API, etc.) anywhere in `go.mod` or `web-app/package.json`.

## 3. LLM-generated vs. battle-tested normalization logic

This is not a novel algorithm, but the check-conclusion state machine is exactly the kind of "looks trivial, isn't" logic worth being careful with (per the repo's own `.claude/rules/interface-pollution-checklist.md` spirit — don't casually rewrite something that already encodes real edge cases). Evidence of nontrivial edge cases already discovered and handled in this codebase:

- GitHub's REST Check Runs `conclusion` field has 8 documented values: `success`, `failure`, `neutral`, `cancelled`, `skipped`, `timed_out`, `action_required`, and `null` (still running). `client.go:335-373`'s `getCheckConclusion` handles 5 of these explicitly (`failure`/`error`/`action_required`/`timed_out` → failure, `success` → success) and implicitly collapses the rest (`skipped`, `cancelled`, unset) into `"neutral"` via its `default` branch — this is a real, tested judgment call already made in the repo, not something to silently re-derive.
- The same function also has to reconcile three overlapping legacy fields (`State` from the GraphQL-flavored `statusCheckRollup`, `Status` from the Checks API, `Conclusion`) because `gh pr view --json statusCheckRollup` mixes both old commit-status and new check-run semantics in one array (`ghStatusCheckItem`, `client.go:113-120`) — this fallback chain (`if c == "" { c = st }`) is itself evidence of a real integration bug once hit and fixed.
- `backlog_plugin_github_prs.go`'s simpler binary version is proof of what happens when this state machine is *re-derived* casually for a narrower use case: it silently drops pending-state detection because it never asked for `status` in the first place. That's a concrete argument for reusing the existing, more complete normalizer rather than writing a fourth ad hoc version for the diff-viewer badge.

**Verdict**: Reuse `getCheckConclusion`'s state machine (extended with `details_url` capture) rather than writing a new one. Extending/generalizing it is low-risk, low-effort; re-deriving the failure/pending/success/neutral mapping from scratch risks reintroducing the exact gap already visible in `fetchCILabel`.

## 4. Existing frontend badge/status UI to adapt

**Evidence**: `web-app/src/components/sessions/GitHubBadge.tsx` already exists and is closely adjacent to this feature's needs:

- It already accepts a `checkConclusion?: string` prop (`GitHubBadge.tsx:29`) and surfaces it in the tooltip today: `if (checkConclusion) tooltipParts.push(\`CI: ${checkConclusion}\`)` (`:116`) — the plumbing for a CI status string reaching this component already exists, just not yet rendered as its own visual badge/link.
- It already has a `PRPriority` union (`"blocking" | "ready" | "pending" | "draft" | "complete" | "no_pr" | "auth_error" | "error"`) with dedicated CSS variant classes per state (`prBadgeBlocking`, `prBadgeReady`, etc., imported from `GitHubBadge.css`) — the same variant-per-state pattern (vanilla-extract `recipe`, per ADR-009 / `.claude/rules/css-architecture.md`) is directly reusable for a CI-specific state set (passing/failing/pending/no-checks).
- The comment `// PR status props (populated by PRStatusPoller)` (`:23`) confirms there's already a live polling component (`PRStatusPoller`, referenced in `web-app/src/gen/session/v1/types_pb.ts` and consumed here) feeding this badge — this is the existing "real-time-ish" plumbing the requirements doc (`requirements.md:65-68`) wants CI status to piggyback on via `WatchSessions`/`WatchReviewQueue`, rather than a new poll loop.
- It currently only links to the **PR** (`resolvedPrUrl`, `:97-99`), not a specific **check run**, so AC #2 ("badge links out to the corresponding GitHub Actions run/check page") needs either a second small badge/icon next to the existing PR badge, or a click-target change — this is incremental UI work on an existing, well-structured component, not new UI from scratch.

**Verdict: Recommended — extend `GitHubBadge.tsx`**, adding a CI-specific visual state (or a small adjacent CI badge) using the same variant/CSS-recipe pattern already established, rather than building a new standalone status-badge component. `DiffViewer.tsx` (`web-app/src/components/sessions/DiffViewer.tsx:11`) does not currently import or render `GitHubBadge`, so wiring it into the diff viewer header is new integration work, but the badge component itself is reusable in place.

## Summary Table

| Option | Verdict |
|---|---|
| Add `google/go-github` (or similar SDK) | Not recommended — no existing dependency, and the repo has a consistent pattern of subprocess/native-HTTP calls (`gh` CLI + raw REST/GraphQL) it would introduce a second, inconsistent integration style for no functional gain |
| Consolidate 3 existing CI-fetch implementations into one shared normalizer/type | Recommended — real work (input shapes differ), but strictly smaller than a 4th implementation; `client.go`'s `getCheckConclusion` is the strongest base to generalize from |
| Third-party CI-status SaaS/aggregator | Not recommended / N/A — GitHub Actions is already the direct target, no aggregation need |
| Write new normalization logic from scratch | Not recommended — the check-conclusion state machine already has real edge-case handling in `getCheckConclusion`; re-deriving it risks repeating the gap visible in `fetchCILabel` (missing pending-state detection) |
| Build new frontend CI badge component | Not recommended — extend `GitHubBadge.tsx`, which already has a `checkConclusion` prop, a per-state variant/CSS pattern, and polling plumbing (`PRStatusPoller`) to build on |
