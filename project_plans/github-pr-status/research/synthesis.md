# Research Synthesis: GitHub PR Status Integration

Date: 2026-04-09
Inputs: features.md, architecture.md, pitfalls.md

## Key Decisions From Research

### 1. Poller: Workspace-level shared ticker at 60-second interval with ETag caching
From architecture.md + supplemental research: `ReviewQueuePoller` pattern is proven. One goroutine, shared `time.Ticker`, iterate all sessions per tick. **Not** per-session goroutines.

**Key finding:** GitHub REST API supports `ETag` / `If-None-Match` conditional requests. A `304 Not Modified` response costs **zero rate limit quota**. This means:
- Poll every 60s (aggressive) — safe because unchanged PRs return 304 for free
- Only PRs that actually changed consume rate limit quota
- Store ETag per `(owner, repo, prNumber)`; pass via `gh api --header "If-None-Match: <etag>"`
- Interval: 60s default (configurable), down from original 300s estimate
- Semaphore: max 5 concurrent `gh` calls to respect secondary rate limits

**No push alternative:** GitHub has no GraphQL subscriptions, no `gh webhook forward` for local tools. `gh run watch` itself uses polling. ETag conditional polling is the best available approach.

### 2. Status taxonomy: Derive `PRPriority` server-side
From features.md: compute a single `priority` enum in Go before sending to UI.
- `blocking` | `ready` | `pending` | `draft` | `complete` | `no_pr` | `error`
- Client receives the derived enum + counts, not raw review arrays
- Derivation logic: CHANGES_REQUESTED or CI FAILURE → blocking; APPROVED + SUCCESS → ready

### 3. Auto-discovery: `gh pr list --head <branch>` for branch-based sessions
From pitfalls.md + requirements: sessions created from PR URLs already have PR number stored. For all other sessions, run discovery query per branch to populate `GitHubPRNumber`.

### 4. gh CLI auth: non-fatal degradation
From pitfalls.md: if gh is unavailable/unauthenticated, poller sets `PRStatus.State = "auth_error"`, pauses all polling, surfaces one-time UI banner. Does not crash or keep retrying.

### 5. Fork detection: detect-and-skip for Phase 1
From pitfalls.md: Fork PRs require querying upstream with `owner:branch` head syntax. Too complex for Phase 1. Detect fork via `gh api repos/:owner/:repo --jq '.isFork'`, set `no_pr` state, surface "Fork — upstream PR lookup coming soon" message.

### 6. Terminal states: stop polling on MERGED/CLOSED
From pitfalls.md + features.md: Once `MERGED` or `CLOSED`, mark `PRStatusTerminal = true`. Poller skips terminal sessions. Saves API quota.

### 7. UI: Extend `GitHubBadge`, don't replace it
From features.md: Add `priority` and `status` props to existing `GitHubBadge.tsx`. Color-coded pill (red/green/yellow/gray) + optional inline counts (`✓ 2 · ✗ 1`). No badge shown when `no_pr`.

---

## New Fields Required

### On `session.Instance` (Go)
```go
// Add to existing GitHub block in instance.go
GitHubPRState          string    `json:"github_pr_state,omitempty"`           // open/merged/closed
GitHubPRIsDraft        bool      `json:"github_pr_is_draft,omitempty"`
GitHubPRPriority       string    `json:"github_pr_priority,omitempty"`        // derived: blocking/ready/pending/etc.
GitHubApprovedCount    int       `json:"github_approved_count,omitempty"`
GitHubChangesReqCount  int       `json:"github_changes_req_count,omitempty"`
GitHubCheckConclusion  string    `json:"github_check_conclusion,omitempty"`   // success/failure/pending/etc.
GitHubPRStatusTerminal bool      `json:"github_pr_status_terminal,omitempty"`
LastPRStatusCheck      time.Time `json:"last_pr_status_check,omitempty"`
```

### On proto `Session` message
Add fields mirroring the above (next available field numbers after 32):
```protobuf
string github_pr_state = 33;
bool   github_pr_is_draft = 34;
string github_pr_priority = 35;
int32  github_approved_count = 36;
int32  github_changes_req_count = 37;
string github_check_conclusion = 38;
google.protobuf.Timestamp last_pr_status_check = 39;
```

### On `github.PRInfo` (extend existing struct)
```go
ReviewDecision       string // APPROVED / CHANGES_REQUESTED / REVIEW_REQUIRED / etc.
ApprovedCount        int
ChangesRequestedCount int
CheckConclusion      string // SUCCESS / FAILURE / PENDING / IN_PROGRESS / NEUTRAL
CheckStatus          string // completed / in_progress / queued
```

Add to `gh pr view --json` fields: `reviews,statusCheckRollup,reviewDecision`

---

## Implementation Phases

### Phase A: Backend plumbing (no UI changes)
1. Extend `github.PRInfo` struct with review/CI fields
2. Update `GetPRInfo()` to fetch `reviews,statusCheckRollup,reviewDecision`
3. Add `DerivePRPriority()` function in `github/` package
4. Add PR auto-discovery: `GetPRForBranch(owner, repo, branch string) (*PRInfo, error)` using `gh pr list --head <branch>`
5. Add fork detection: `IsForkRepo(owner, repo string) bool`
6. Add new Instance fields + storage field update method
7. Add `session/pr_status_poller.go` — workspace-level ticker, 5-min interval

### Phase B: Wire poller into server
1. Instantiate `PRStatusPoller` in `server/server.go` alongside `ReviewQueuePoller`
2. Call `SetInstances()` when sessions load
3. On first tick: run auto-discovery for sessions with no `GitHubPRNumber` set
4. Emit `EventSessionUpdated` via EventBus on status change

### Phase C: Proto + web UI
1. Add proto fields to `Session` message in `types.proto`
2. Run `make generate-proto`
3. Extend `GitHubBadge.tsx` — add `priority`, `approvedCount`, `changesRequestedCount`, `checkConclusion` props
4. Add CSS styles for priority colors in `GitHubBadge.css.ts` (vanilla-extract)
5. Update `SessionCard.tsx` to pass new fields from `Session` proto

---

## Risk Register

| Risk | Mitigation | Phase |
|---|---|---|
| Rate limit from polling 100+ sessions | Semaphore (max 5 concurrent), 5-min interval, cache | A |
| `gh` not installed / unauthenticated | `CheckGHAuth()` cached, `auth_error` state, UI banner | A |
| Fork branches return empty PR lookup | Fork detection → skip gracefully | A |
| Session deleted during fetch | Re-check existence after fetch before writing | A |
| `gh` CLI hangs (network) | `exec.CommandContext` with 8s timeout | A |
| PR has no CI checks | `checkConclusion = null` → omit CI badge | C |
| `CHANGES_REQUESTED` from dismissed review | GitHub API: dismissed reviews don't count — `reviewDecision` field handles this correctly | A |

---

## Open Questions (for planning phase)

1. Where exactly does auto-discovery run — on session creation, or lazily on first poll tick? **Recommendation:** Lazy (first tick), to avoid blocking session creation.
2. How does the `PRStatusPoller` get access to the `EventBus`? Inject via constructor (same pattern as `ReviewQueuePoller`).
3. Should `GitHubBadge` show the PR number when status is `complete` (merged)? Or hide it? **Recommendation:** Show `✅ #123 Merged` in dim style — useful context, not noisy.
4. What happens if a branch matches multiple PRs (rare but possible)? Use the most recent by `updatedAt`.
