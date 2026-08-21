# GitHub PR Status Integration

**Epic:** Surface real-time GitHub PR status (reviews, CI, lifecycle) on session cards in the web UI.

**Status:** Planning complete | Ready for implementation
**Branch:** `claude-squad-pr-integration`
**Created:** 2026-04-09

## Spec References

- Requirements: `project_plans/github-pr-status/requirements.md`
- Research: `project_plans/github-pr-status/research/synthesis.md`
- ADR-001: Workspace-level poller (`project_plans/github-pr-status/decisions/ADR-001-workspace-level-pr-status-poller.md`)
- ADR-002: ETag conditional polling (`project_plans/github-pr-status/decisions/ADR-002-etag-conditional-polling.md`)
- ADR-003: Server-side priority derivation (`project_plans/github-pr-status/decisions/ADR-003-server-side-priority-derivation.md`)

---

## Dependency Visualization

```
Story 1: Backend - GitHub Client + PR Discovery
  Task 1.1  Extend PRInfo struct
  Task 1.2  Add GetPRForBranch() discovery
  Task 1.3  Add DerivePRPriority()            ----+
  Task 1.4  Add IsForkRepo() detection             |
  Task 1.5  Add ETag cache + gh api wrapper         |
  Task 1.6  Unit tests for derivation + discovery   |
       |                                            |
       v                                            |
Story 2: Backend - Poller + Instance Fields         |
  Task 2.1  Add Instance + InstanceData fields      |
  Task 2.2  Create pr_status_poller.go  <-----------+  (uses DerivePRPriority)
  Task 2.3  Wire poller into BuildRuntimeDeps
  Task 2.4  Add proto fields + generate
  Task 2.5  Map proto fields in conversion
  Task 2.6  Unit tests for poller
       |
       v
Story 3: Web UI - GitHubBadge Status Display
  Task 3.1  Create GitHubBadge.css.ts (vanilla-extract)
  Task 3.2  Extend GitHubBadge.tsx with priority props
  Task 3.3  Update SessionCard.tsx to pass fields
  Task 3.4  Accessibility + hover tooltip
  Task 3.5  Manual integration test
```

---

## Story 1: Backend -- Enrich GitHub Client + PR Discovery

**Goal:** Extend `github/client.go` to fetch review decisions, CI status, and discover PRs by branch name. Add server-side priority derivation.

### Task 1.1: Extend PRInfo Struct with Review/CI Fields

**Files:** `github/client.go`

Add fields to the `PRInfo` struct:

```go
// Add to existing PRInfo struct (after line 29)
ReviewDecision        string // APPROVED / CHANGES_REQUESTED / REVIEW_REQUIRED
ApprovedCount         int    // Computed from reviews array
ChangesRequestedCount int    // Computed from reviews array
CheckConclusion       string // Rollup: SUCCESS / FAILURE / PENDING / IN_PROGRESS / NEUTRAL
CheckStatus           string // completed / in_progress / queued
```

Add corresponding fields to `ghPRResponse` for JSON deserialization. The `reviews` field from `gh pr view --json` returns an array of review objects with `{author:{login}, state, body}`. The `statusCheckRollup` field returns an array of objects with `{context, state, conclusion}`. The `reviewDecision` field returns a single string.

Update the `--json` fields string in `GetPRInfo()` (line 105):

```
number,title,body,headRefName,baseRefName,state,url,createdAt,updatedAt,isDraft,
mergeable,additions,deletions,changedFiles,author,labels,reviews,reviewDecision,
statusCheckRollup
```

Parse the `reviews` array to compute `ApprovedCount` and `ChangesRequestedCount`. Count unique reviewers by their latest review state -- if the same reviewer has both COMMENTED and APPROVED, count only APPROVED. The `reviewDecision` field gives the GitHub-computed rollup but individual counts need the array.

Parse `statusCheckRollup` to compute `CheckConclusion` (worst-case rollup: any FAILURE -> FAILURE, any IN_PROGRESS -> IN_PROGRESS, all SUCCESS -> SUCCESS, none -> empty string).

**Acceptance criteria:**
- `GetPRInfo()` returns populated review and CI fields
- Existing callers of `GetPRInfo()` are unaffected (fields are additive)
- Reviews with DISMISSED state are not counted

### Task 1.2: Add GetPRForBranch() PR Discovery

**Files:** `github/client.go`

Add a new function for branch-based PR discovery:

```go
func GetPRForBranch(owner, repo, branch string) (*PRInfo, error)
```

Implementation:
1. Call `gh pr list --head <branch> --repo <owner>/<repo> --json number,url,headRefName,updatedAt --state open`
2. If result array is empty, return `nil, nil` (no PR, not an error)
3. If multiple PRs match (rare), pick the most recently updated by `updatedAt`
4. Call `GetPRInfo(owner, repo, matchedNumber)` to fetch full enriched PR data
5. Return the enriched `PRInfo`

Handle the case where `branch` contains slashes (feature branches like `feat/auth-fix`). The `--head` flag handles this natively.

**Acceptance criteria:**
- Returns correct PR for a known branch with one open PR
- Returns `nil, nil` for a branch with no open PR
- Multiple-PR case selects most recently updated
- Does not error on branches with slashes in name

### Task 1.3: Add DerivePRPriority() Function

**Files:** `github/priority.go` (new file)

Create a pure function implementing the priority derivation from ADR-003:

```go
type PRPriority string

const (
    PRPriorityBlocking  PRPriority = "blocking"
    PRPriorityReady     PRPriority = "ready"
    PRPriorityPending   PRPriority = "pending"
    PRPriorityDraft     PRPriority = "draft"
    PRPriorityComplete  PRPriority = "complete"
    PRPriorityNoPR      PRPriority = "no_pr"
    PRPriorityAuthError PRPriority = "auth_error"
)

func DerivePRPriority(info *PRInfo) PRPriority
```

Derivation precedence (first match wins):
1. `info == nil` -> `no_pr`
2. `State == "MERGED" || State == "CLOSED"` -> `complete`
3. `IsDraft` -> `draft`
4. `ChangesRequestedCount > 0` -> `blocking`
5. `CheckConclusion == "FAILURE" || CheckConclusion == "ACTION_REQUIRED"` -> `blocking`
6. `ApprovedCount > 0 && (CheckConclusion == "SUCCESS" || CheckConclusion == "")` -> `ready`
7. `CheckConclusion == "PENDING" || CheckConclusion == "IN_PROGRESS"` -> `pending`
8. default -> `pending`

Also add `IsTerminal(priority PRPriority) bool` returning true for `complete`.

**Acceptance criteria:**
- Pure function with no side effects or I/O
- Table-driven test covers all 8 derivation paths

### Task 1.4: Add IsForkRepo() Detection

**Files:** `github/client.go`

```go
func IsForkRepo(ctx context.Context, owner, repo string) (bool, error)
```

Implementation:
1. Call `gh api repos/<owner>/<repo> --jq '.fork'` using `exec.CommandContext` with 5-second timeout
2. Parse output as `"true\n"` or `"false\n"`
3. Return boolean result

For Phase 1, the poller calls this once per session (at first discovery) and caches the result on the Instance. Fork repos get `PRPriority = "no_pr"` with a log message.

**Acceptance criteria:**
- Returns `true` for fork repos, `false` for non-fork repos
- Respects context timeout (does not hang)
- Returns error on network failure (does not panic)

### Task 1.5: Add ETag Cache + `gh api` Wrapper

**Files:** `github/etag_cache.go` (new file)

Create an in-memory ETag cache and a wrapper for conditional `gh api` requests:

```go
type ETagCache struct {
    mu     sync.RWMutex
    etags  map[string]string    // key: "owner/repo/prNumber" -> etag value
    data   map[string]*PRInfo   // key: same -> cached PRInfo
}

func NewETagCache() *ETagCache
func (c *ETagCache) Get(owner, repo string, prNumber int) (etag string, cached *PRInfo, ok bool)
func (c *ETagCache) Set(owner, repo string, prNumber int, etag string, info *PRInfo)
func (c *ETagCache) Remove(owner, repo string, prNumber int)
```

Add a conditional fetch function:

```go
// GetPRInfoConditional fetches PR info using ETag for conditional requests.
// Returns (info, changed, error). When changed=false, info is from cache (304).
func GetPRInfoConditional(ctx context.Context, owner, repo string, prNumber int, cache *ETagCache) (*PRInfo, bool, error)
```

Implementation approach using `gh api --include`:
- The `--include` flag prepends HTTP response headers to stdout
- Parse output: split on first blank line, headers above, JSON body below
- Extract `ETag:` header value and HTTP status code from headers
- On 304: return cached data, `changed=false`
- On 200: parse JSON body into enriched PRInfo, store new ETag, return `changed=true`

Create a helper `parseGHAPIIncludeOutput(output []byte) (statusCode int, headers map[string]string, body []byte, err error)` with robust parsing.

**Acceptance criteria:**
- Cache hit with matching ETag returns cached data without parsing body
- Cache miss performs full fetch and stores ETag
- Thread-safe concurrent access
- Fallback: if `--include` parsing fails, fall back to non-conditional request

### Task 1.6: Unit Tests for Priority Derivation + Discovery

**Files:** `github/priority_test.go` (new), `github/etag_cache_test.go` (new)

Priority derivation tests (table-driven):
- nil PRInfo -> `no_pr`
- merged state -> `complete`
- closed state -> `complete`
- draft -> `draft`
- changes requested with passing CI -> `blocking`
- CI failure with approvals -> `blocking`
- approved with passing CI -> `ready`
- approved with no CI (empty CheckConclusion) -> `ready`
- pending CI, no reviews -> `pending`
- no reviews, no CI -> `pending`
- changes requested AND CI failing -> `blocking` (verify first-match precedence)
- draft with approvals -> `draft` (draft takes precedence over ready)

ETag cache tests:
- Get on empty cache returns ok=false
- Set then Get returns correct ETag and data
- Remove clears entry
- Concurrent access safety (parallel goroutine test)

`parseGHAPIIncludeOutput` tests:
- Standard 200 response with ETag header
- 304 response with empty body
- Malformed output (graceful error)
- Response with multiple header blocks (redirect)

**Acceptance criteria:**
- All derivation paths have at least one test case
- `go test ./github/...` passes
- No flaky tests (concurrent cache test uses sufficient goroutines)

---

## Story 2: Backend -- Poller + Instance Fields

**Goal:** Create the PR status poller following `ReviewQueuePoller` pattern, add status fields to `Instance`, wire into server dependencies.

### Task 2.1: Add GitHub Status Fields to Instance + InstanceData + Storage

**Files:** `session/instance.go`, `session/storage.go`

Add fields to `Instance` struct (after existing GitHub block, around line 131):

```go
// GitHub PR status fields (populated by PRStatusPoller)
GitHubPRState          string    `json:"github_pr_state,omitempty"`
GitHubPRIsDraft        bool      `json:"github_pr_is_draft,omitempty"`
GitHubPRPriority       string    `json:"github_pr_priority,omitempty"`
GitHubApprovedCount    int       `json:"github_approved_count,omitempty"`
GitHubChangesReqCount  int       `json:"github_changes_req_count,omitempty"`
GitHubCheckConclusion  string    `json:"github_check_conclusion,omitempty"`
GitHubPRStatusTerminal bool      `json:"github_pr_status_terminal,omitempty"`
LastPRStatusCheck      time.Time `json:"last_pr_status_check,omitempty"`
GitHubIsFork           *bool     `json:"github_is_fork,omitempty"` // nil = not yet checked
```

Add matching fields to `InstanceData` in `storage.go`.

Add a thread-safe update method on Instance:

```go
func (i *Instance) UpdatePRStatus(state, priority, checkConclusion string,
    approvedCount, changesReqCount int, isDraft, terminal bool) {
    i.stateMutex.Lock()
    defer i.stateMutex.Unlock()
    i.GitHubPRState = state
    i.GitHubPRPriority = priority
    i.GitHubCheckConclusion = checkConclusion
    i.GitHubApprovedCount = approvedCount
    i.GitHubChangesReqCount = changesReqCount
    i.GitHubPRIsDraft = isDraft
    i.GitHubPRStatusTerminal = terminal
    i.LastPRStatusCheck = time.Now()
}
```

Add a storage partial-update method:

```go
func (s *Storage) UpdateInstancePRStatus(title string, state, priority, checkConclusion string,
    approvedCount, changesReqCount int, isDraft, terminal bool) error {
    return s.updateFieldInRepo(title, func(d *InstanceData) {
        d.GitHubPRState = state
        d.GitHubPRPriority = priority
        d.GitHubCheckConclusion = checkConclusion
        d.GitHubApprovedCount = approvedCount
        d.GitHubChangesReqCount = changesReqCount
        d.GitHubPRIsDraft = isDraft
        d.GitHubPRStatusTerminal = terminal
        d.LastPRStatusCheck = time.Now()
    })
}
```

Update `ToInstanceData()` and `FromInstanceData()` conversion methods to include the new fields.

**Acceptance criteria:**
- New fields serialize/deserialize correctly in JSON
- `UpdatePRStatus()` on Instance is thread-safe (uses `stateMutex`)
- `UpdateInstancePRStatus()` on Storage persists atomically
- Existing session load/save paths preserve new fields (backward compatible -- missing fields default to zero values)

### Task 2.2: Create pr_status_poller.go

**Files:** `session/pr_status_poller.go` (new file)

Follow `ReviewQueuePoller` pattern exactly. Key structural elements:

```go
type PRStatusPollerConfig struct {
    PollInterval    time.Duration // Default: 60s
    CallTimeout     time.Duration // Per gh call: 8s
    MaxConcurrent   int           // Semaphore: 5
    StaleThreshold  time.Duration // Badge staleness: 10min
}

type PRStatusPoller struct {
    storage    *Storage
    eventBus   *events.EventBus
    ghAuth     *ghAuthState
    etagCache  *github.ETagCache
    instances  []*Instance
    config     PRStatusPollerConfig

    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup
    mu     sync.RWMutex
}
```

Lifecycle methods (matching `ReviewQueuePoller` signatures):
- `NewPRStatusPoller(storage *Storage, eventBus *events.EventBus) *PRStatusPoller`
- `SetInstances(instances []*Instance)`
- `AddInstance(instance *Instance)`
- `RemoveInstance(instanceTitle string)`
- `Start(ctx context.Context)`
- `Stop()`
- `IsRunning() bool`
- `GetMonitoredCount() int`

Poll loop (`pollLoop`):
1. Fire on ticker (60s default)
2. Check cached auth state (re-check every 5 minutes via `ghAuth`)
3. If auth failed: log once, skip all sessions, return
4. Copy instance slice under RLock
5. For each instance:
   a. Skip if `GitHubPRStatusTerminal == true`
   b. Skip if no branch or no owner/repo set (non-GitHub session)
   c. If `GitHubPRNumber == 0` and `GitHubOwner != ""`: attempt auto-discovery
      - Call `github.GetPRForBranch(owner, repo, branch)`
      - If found: set `GitHubPRNumber`, `GitHubPRURL` on Instance, persist via storage
      - If not found: set priority `no_pr`, continue
   d. If `GitHubIsFork == nil`: check once via `IsForkRepo()`, cache result
   e. If fork: set priority `no_pr`, continue
   f. Call `github.GetPRInfoConditional()` with ETag cache
   g. If 304 (unchanged): update `LastPRStatusCheck` only, continue
   h. If 200: call `DerivePRPriority()`, compare with previous `GitHubPRPriority`
   i. If priority changed: call `inst.UpdatePRStatus()`, persist via `storage.UpdateInstancePRStatus()`
   j. Publish `events.NewSessionUpdatedEvent(inst, []string{"github_pr_status"})` via EventBus
   k. If MERGED/CLOSED: set `GitHubPRStatusTerminal = true`

Auth caching (`ghAuthState`):
```go
type ghAuthState struct {
    mu        sync.Mutex
    available bool
    lastCheck time.Time
    recheckInterval time.Duration // 5 minutes
}

func (a *ghAuthState) IsAvailable() bool  // returns cached result, re-checks if stale
```

**Acceptance criteria:**
- Structurally mirrors `ReviewQueuePoller` (same lifecycle methods, same poll loop pattern)
- Terminal sessions (merged/closed) are skipped after first detection
- Auth failure pauses polling gracefully (does not crash or retry every tick)
- Auto-discovery runs once per session (sets PRNumber or no_pr, does not retry every tick)
- Status changes emit `EventSessionUpdated` via EventBus
- Each `gh` call uses `exec.CommandContext` with 8-second timeout

### Task 2.3: Wire Poller into BuildRuntimeDeps

**Files:** `server/dependencies.go`

Add `PRStatusPoller` to the dependency chain:

1. Add `PRStatusPoller *session.PRStatusPoller` to `ServiceDeps` struct (line 298)
2. In `BuildServiceDeps()`: construct `PRStatusPoller` after `ReviewQueuePoller`:
   ```go
   prStatusPoller := session.NewPRStatusPoller(core.Storage, core.EventBus)
   ```
3. Add `PRStatusPoller` to `ServerDependencies` struct (line 19)
4. In `BuildRuntimeDeps()`:
   - After `reviewQueuePoller.SetInstances(instances)` (line 379):
     ```go
     prStatusPoller.SetInstances(instances)
     ```
   - After controllers start (around line 418), start the poller:
     ```go
     prStatusPoller.Start(context.Background())
     log.InfoLog.Printf("PRStatusPoller started")
     ```
   - In `ExternalDiscovery.OnSessionAdded` callback (around line 454):
     ```go
     prStatusPoller.AddInstance(instance)
     ```
   - In `ExternalDiscovery.OnSessionRemoved` callback (around line 458):
     ```go
     prStatusPoller.RemoveInstance(instance.Title)
     ```

**Acceptance criteria:**
- `PRStatusPoller` starts alongside existing pollers during server boot
- External session discovery adds/removes from PR poller
- Server shutdown stops poller cleanly (via context cancellation)
- Build compiles with no errors

### Task 2.4: Add Proto Fields to Session Message

**Files:** `proto/session/v1/types.proto`

Add new fields to the `Session` message after field 32 (`external_metadata`):

```protobuf
// GitHub PR status fields (populated by PRStatusPoller).
// PR lifecycle state: "open", "merged", "closed", or empty if no PR.
string github_pr_state = 33;

// Whether PR is marked as draft.
bool github_pr_is_draft = 34;

// Derived priority: "blocking", "ready", "pending", "draft", "complete", "no_pr", "auth_error".
string github_pr_priority = 35;

// Number of approving reviews.
int32 github_approved_count = 36;

// Number of changes-requested reviews.
int32 github_changes_req_count = 37;

// CI check rollup conclusion: "success", "failure", "pending", "in_progress", "neutral".
string github_check_conclusion = 38;

// When PR status was last refreshed.
google.protobuf.Timestamp last_pr_status_check = 39;
```

Run `make generate-proto` to regenerate Go and TypeScript code.

**Acceptance criteria:**
- Proto compiles without errors
- Generated Go code has new fields on `Session` message struct
- Generated TypeScript code has new fields accessible in web-app
- Field numbers 33-39 do not conflict with existing fields (32 is the last used)

### Task 2.5: Map Proto Fields in Session Conversion

**Files:** `server/services/session_service.go` (find the Instance -> proto Session conversion function)

Locate the function that converts `session.Instance` to proto `Session` message. Add mappings:

```go
protoSession.GithubPrState = inst.GitHubPRState
protoSession.GithubPrIsDraft = inst.GitHubPRIsDraft
protoSession.GithubPrPriority = inst.GitHubPRPriority
protoSession.GithubApprovedCount = int32(inst.GitHubApprovedCount)
protoSession.GithubChangesReqCount = int32(inst.GitHubChangesReqCount)
protoSession.GithubCheckConclusion = inst.GitHubCheckConclusion
if !inst.LastPRStatusCheck.IsZero() {
    protoSession.LastPrStatusCheck = timestamppb.New(inst.LastPRStatusCheck)
}
```

**Acceptance criteria:**
- New fields appear in proto responses from `ListSessions` and `GetSession` RPCs
- `LastPrStatusCheck` correctly converts `time.Time` -> `google.protobuf.Timestamp`
- Zero-value fields (empty string, 0) do not cause proto serialization issues

### Task 2.6: Unit Tests for Poller

**Files:** `session/pr_status_poller_test.go` (new)

Test cases:
- Poller starts and stops cleanly (no goroutine leak)
- `SetInstances` / `AddInstance` / `RemoveInstance` lifecycle methods
- Terminal sessions (`GitHubPRStatusTerminal == true`) are skipped in poll loop
- Sessions without owner/repo are skipped (non-GitHub sessions)
- Sessions without PRNumber trigger auto-discovery path
- Auth failure sets auth-unavailable state (does not panic or infinite-loop)
- Status change emits event via EventBus (verify with a test subscriber)
- ETag cache hit skips field update (304 path)

Use interface injection or function fields for GitHub API calls to avoid actual `gh` CLI calls in tests. Pattern: define a `type GitHubFetcher interface` or use function fields on the poller struct that can be replaced in tests.

**Acceptance criteria:**
- `go test ./session/... -run TestPRStatus` passes
- No actual GitHub API calls in tests
- No goroutine leaks (verified by `goleak` or manual check)

---

## Story 3: Web UI -- GitHubBadge Status Display

**Goal:** Extend `GitHubBadge.tsx` to show PR priority as a color-coded pill with review/CI counts and accessible tooltip.

### Task 3.1: Create GitHubBadge.css.ts (vanilla-extract)

**Files:** `web-app/src/components/sessions/GitHubBadge.css.ts` (new file)

Per ADR 009 (`docs/adr/009-vanilla-extract-type-safe-css.md`), new styles go in vanilla-extract `.css.ts` files colocated with the component. The existing `GitHubBadge.module.css` remains for the base PR/repo badge styles; the new `.css.ts` file adds status-variant styles.

Define a recipe for priority-based badge variants:

```typescript
import { recipe } from '@vanilla-extract/recipes';
import { style, globalStyle } from '@vanilla-extract/css';

export const statusBadge = recipe({
  base: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '4px',
    padding: '4px 10px',
    borderRadius: '12px',
    fontSize: '12px',
    fontWeight: 600,
    textDecoration: 'none',
    cursor: 'pointer',
    border: '1px solid transparent',
    transition: 'all 0.2s ease',
  },
  variants: {
    priority: {
      blocking:   { background: '#da3633', color: '#ffffff', borderColor: '#b62324' },
      ready:      { background: '#238636', color: '#ffffff', borderColor: '#1a7f37' },
      pending:    { background: '#d29922', color: '#ffffff', borderColor: '#bb8009' },
      draft:      { background: '#6e7681', color: '#ffffff', borderColor: '#57606a' },
      complete:   { background: '#8957e5', color: '#ffffff', borderColor: '#7c3aed', opacity: 0.7 },
      auth_error: { background: '#f85149', color: '#ffffff', borderColor: '#da3633' },
    },
    size: {
      compact: { padding: '4px 8px', fontSize: '11px' },
      normal:  { padding: '4px 10px', fontSize: '12px' },
    },
  },
  defaultVariants: { size: 'normal' },
});

export const reviewCountRow = style({
  display: 'flex',
  alignItems: 'center',
  gap: '6px',
  fontSize: '11px',
  marginTop: '2px',
});

export const tooltipContainer = style({
  position: 'relative',
});

export const tooltipContent = style({
  position: 'absolute',
  bottom: '100%',
  left: '50%',
  transform: 'translateX(-50%)',
  padding: '8px 12px',
  borderRadius: '6px',
  fontSize: '12px',
  lineHeight: 1.4,
  whiteSpace: 'pre-line',
  pointerEvents: 'none',
  zIndex: 1000,
  opacity: 0,
  transition: 'opacity 0.15s ease',
  selectors: {
    [`${tooltipContainer}:hover &`]: { opacity: 1 },
  },
});
```

Include dark mode via `@media (prefers-color-scheme: dark)` for adjusted colors.

**Acceptance criteria:**
- `.css.ts` file compiles with vanilla-extract build
- All 6 priority variants have distinct, accessible color combinations
- Dark mode support included
- Compact and normal size variants defined

### Task 3.2: Extend GitHubBadge.tsx with Priority/Status Props

**Files:** `web-app/src/components/sessions/GitHubBadge.tsx`

Add new optional props to the component interface:

```typescript
interface GitHubBadgeProps {
  // Existing props (unchanged)
  prNumber?: number;
  prUrl?: string;
  owner?: string;
  repo?: string;
  sourceRef?: string;
  compact?: boolean;

  // New status props (all optional for backward compatibility)
  priority?: string;
  approvedCount?: number;
  changesRequestedCount?: number;
  checkConclusion?: string;
  lastPrStatusCheck?: { seconds: bigint; nanos: number };
}
```

Rendering logic within the existing PR badge section:

1. If `priority` is undefined or `"no_pr"`: fall through to existing behavior (show `#prNumber` badge or repo badge)
2. If `priority` is set and `prNumber > 0`: render status-aware PR badge
   - Import and apply `statusBadge({ priority, size: compact ? 'compact' : 'normal' })` from `.css.ts`
   - Icon selection based on priority:
     - `blocking`: X-circle icon (red)
     - `ready`: checkmark-circle icon (green)
     - `pending`: clock icon (yellow)
     - `draft`: pencil icon (gray)
     - `complete`: merge icon (purple)
     - `auth_error`: warning-triangle icon
   - Text: `#<prNumber>` (keep existing text, color changes convey status)
   - Priority label after number in non-compact mode: `#123 Ready` or `#123 Blocked`

Existing callers that do not pass `priority` will render identically to current behavior. This is critical -- the prop is additive.

**Acceptance criteria:**
- Existing callers without `priority` prop render identically to before (regression-free)
- Badge shows correct color for each of the 6 priority values
- PR number remains a clickable link to GitHub
- Icon changes per priority value
- Compact mode renders smaller badge

### Task 3.3: Update SessionCard.tsx to Pass New Proto Fields

**Files:** `web-app/src/components/sessions/SessionCard.tsx`

Update the `GitHubBadge` usage (around line 324) to pass the new fields from the proto `Session` message:

```tsx
<GitHubBadge
  prNumber={session.githubPrNumber}
  prUrl={session.githubPrUrl}
  owner={session.githubOwner}
  repo={session.githubRepo}
  sourceRef={session.githubSourceRef}
  compact={true}
  // New status fields from proto
  priority={session.githubPrPriority || undefined}
  approvedCount={session.githubApprovedCount}
  changesRequestedCount={session.githubChangesReqCount}
  checkConclusion={session.githubCheckConclusion || undefined}
  lastPrStatusCheck={session.lastPrStatusCheck}
/>
```

The `|| undefined` guards ensure that empty strings from proto (default for unset string fields) are treated as "not set" by the badge component.

**Acceptance criteria:**
- Proto fields flow from server -> SessionCard -> GitHubBadge without TypeScript errors
- Sessions without PR status data render existing badge (no regression)
- Sessions with PR status data render the new status badge

### Task 3.4: Accessibility + Hover Tooltip

**Files:** `web-app/src/components/sessions/GitHubBadge.tsx`, `web-app/src/components/sessions/GitHubBadge.css.ts`

Add comprehensive `aria-label` to the status badge:

```tsx
const getAriaLabel = () => {
  const parts = [`PR #${prNumber}`];
  if (priority === 'blocking') parts.push('blocked');
  if (priority === 'ready') parts.push('ready to merge');
  // ... etc
  if (changesRequestedCount && changesRequestedCount > 0)
    parts.push(`${changesRequestedCount} changes requested`);
  if (approvedCount && approvedCount > 0)
    parts.push(`${approvedCount} approved`);
  if (checkConclusion)
    parts.push(`CI ${checkConclusion}`);
  return parts.join(', ');
};
```

Add a CSS-only hover tooltip (no tooltip library dependency):

```tsx
<div className={tooltipContainer}>
  <a className={statusBadge({...})} aria-label={getAriaLabel()}>
    {/* badge content */}
  </a>
  <div className={tooltipContent} role="tooltip">
    {tooltipText}
  </div>
</div>
```

Tooltip content:
```
PR #123
Reviews: 2 approved, 1 changes requested
CI: passing
Last checked: 2 minutes ago
```

The "Last checked" line uses `lastPrStatusCheck` timestamp, formatted as relative time using the existing `formatTimeAgo` pattern from SessionCard.

**Acceptance criteria:**
- Screen reader announces full PR state via `aria-label`
- Hover tooltip shows review count, CI status, and freshness
- Color is never the sole status indicator (icon + text always present)
- Focus state visible for keyboard navigation
- Tooltip does not overflow viewport (use `left: 50%` + `transform` centering)

### Task 3.5: Manual Integration Test

No automated E2E test for Phase 1. Manual verification checklist:

1. `make restart-web` starts server successfully
2. Session on branch with open PR: badge appears with correct priority within 60 seconds
3. PR with 2 approvals, CI passing: green "Ready" badge, tooltip shows "2 approved"
4. PR with changes requested: red "Blocked" badge
5. PR with CI in progress: yellow "Pending" badge
6. Draft PR: gray "Draft" badge
7. Merged PR: purple dimmed "Complete" badge, no further polling (check server logs)
8. Session on branch with no PR: no status badge, just `#123` or repo badge
9. Revoke `gh auth` -> no crash, badge disappears or shows error state
10. Dark mode: verify colors are readable

**Acceptance criteria:**
- All 10 scenarios verified
- No JavaScript console errors
- No Go panics in server logs
- Badge updates within 60 seconds of PR state change on GitHub

---

## Known Issues

### Race Condition on PR Number Write During Auto-Discovery [SEVERITY: Medium]

**Description:** The poller discovers a PR number for a session via `GetPRForBranch()` and writes `GitHubPRNumber` to the Instance. Concurrently, the web UI reads Instance fields for proto conversion. Without `stateMutex` protection, the read may see partially-updated state.

**Mitigation:**
- Use `inst.UpdatePRStatus()` method which takes `stateMutex.Lock()` atomically
- For PR number discovery, add a separate `inst.SetGitHubPRNumber(number, url)` method under `stateMutex`
- Storage update is separate (uses repo-level locking in `updateFieldInRepo`)

**Files Likely Affected:** `session/pr_status_poller.go`, `session/instance.go`

### `gh api --include` Output Parsing for ETag [SEVERITY: Medium]

**Description:** The `gh api --include` flag outputs HTTP headers before the JSON body, separated by a blank line. Edge cases include: redirect responses producing multiple header blocks, and truncated output on timeout.

**Mitigation:**
- Dedicated `parseGHAPIIncludeOutput()` function with unit tests
- If parsing fails, fall back to non-conditional request (treat as cache miss)
- Unit test with captured real output samples

**Files Likely Affected:** `github/etag_cache.go`

### Auth Check Redundancy -- `CheckGHAuth()` on Every `GetPRInfo` Call [SEVERITY: Medium]

**Description:** The existing `GetPRInfo()` (line 97) calls `CheckGHAuth()` on every invocation, spawning a `gh auth status` subprocess. With 60-second polling across N sessions, this means N extra subprocess spawns per tick.

**Mitigation:**
- The poller caches auth state in `ghAuthState` with 5-minute TTL
- Create a `GetPRInfoSkipAuth()` variant or pass `skipAuth bool` parameter for poller use
- Poller calls `CheckGHAuth()` once at startup and once per 5 minutes

**Files Likely Affected:** `github/client.go`, `session/pr_status_poller.go`

### `gh` CLI Hangs on Network Timeout [SEVERITY: High]

**Description:** If the network is unreachable, `gh` CLI may hang for up to 30 seconds. This blocks the poller's sequential iteration for that session's slot.

**Mitigation:**
- All `gh` calls from the poller use `exec.CommandContext(ctx, "gh", ...)` with 8-second timeout
- If timeout fires, kill process, log error, set temporary `"error"` priority, continue to next session
- Create a helper `ghCommand(ctx context.Context, args ...string) *exec.Cmd` for consistent timeout handling

**Files Likely Affected:** `github/client.go` (audit all `exec.Command` calls), `session/pr_status_poller.go`

### `gh pr list --head` Returns Empty for Fork PRs [SEVERITY: Low]

**Description:** For fork repositories, PRs are opened against the upstream repo. `gh pr list --head <branch> --repo <fork-owner>/<repo>` returns empty because the PR lives on the upstream.

**Mitigation:**
- Phase 1: `IsForkRepo()` detects forks and sets `no_pr` state
- Phase 2 (future): query upstream with `--head <fork-owner>:<branch> --repo <upstream>/<repo>`

**Files Likely Affected:** `github/client.go`, `session/pr_status_poller.go`

### Proto Field Number Conflict Risk [SEVERITY: Low]

**Description:** Fields 33-39 are allocated for PR status. If another feature branch concurrently adds Session fields in the same range, proto compilation fails or data corruption occurs.

**Mitigation:**
- Add a range reservation comment in `types.proto`: `// Fields 33-39: GitHub PR status`
- Check for conflicts before merging to main

**Files Likely Affected:** `proto/session/v1/types.proto`

### Stale ETag After Repository Transfer [SEVERITY: Low]

**Description:** If a repository is transferred or a PR is admin-deleted, the cached ETag may return 304 for a resource that effectively no longer matches the cached data semantics.

**Mitigation:**
- Clear ETag cache entry on any non-304/non-200 response (404, 403, etc.)
- Clear cache when session is deleted via `RemoveInstance()`
- Add optional cache TTL sweep (entries not accessed in 1 hour)

**Files Likely Affected:** `github/etag_cache.go`
