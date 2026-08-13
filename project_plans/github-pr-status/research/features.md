# Features Research: PR Status Display

Date: 2026-04-09

## Status States That Matter

### Review Decision States (GitHub canonical)
| State | Priority | Action |
|---|---|---|
| `CHANGES_REQUESTED` | 🔴 Blocking | Developer must address before merge |
| `APPROVED` | 🟢 Ready | Can merge |
| `COMMENTED` | 🔵 Info | Informational; no blocking action |
| `PENDING` | 🟡 Waiting | Review submitted but not yet decided |

### CI/Check States (GitHub Actions)
| State | Priority | Action |
|---|---|---|
| `FAILURE` | 🔴 Blocking | Investigate failing checks |
| `ACTION_REQUIRED` | 🔴 Blocking | Manual intervention needed |
| `IN_PROGRESS` / `QUEUED` | 🟡 Waiting | Automation running |
| `SUCCESS` | 🟢 Clear | All checks passed |
| `NEUTRAL` / `SKIPPED` | ⚪ Informational | Not blocking |

### PR Lifecycle States
| State | Priority | Notes |
|---|---|---|
| `MERGED` | ✅ Complete | Terminal state; stop polling |
| `CLOSED` | ✅ Complete | Terminal state; stop polling |
| `DRAFT` | 📝 Deferred | Not ready for review |
| `OPEN` (no reviews) | 🟡 Waiting | Awaiting review request |

### Derived Priority Taxonomy (for Stapler Squad)
Map compound state → single priority for session card scanning:

```
BLOCKING  = CHANGES_REQUESTED OR check FAILURE OR merge conflict
READY     = APPROVED AND checks SUCCESS (or no checks)
PENDING   = checks IN_PROGRESS OR awaiting first review
DRAFT     = isDraft = true
COMPLETE  = MERGED or CLOSED
INFO      = only COMMENTED (no blocking reviews)
NO_PR     = branch has no PR (no badge shown)
```

## How Comparable Tools Display PR Status

### GitHub.com (native)
- **Card-level:** Color-coded state badge (green Open, purple Merged, red Closed)
- **Review summary:** Count badges — "2 approvals · 1 change requested" with reviewer avatars
- **CI summary:** Single indicator: ✓ green check / ✗ red X / ⏳ yellow dot
- **Stacking:** Review decisions shown as horizontal row of avatar + badge pairs
- **Pattern:** Visual + text — never color alone (accessibility)

### GitHub Pull Requests VS Code Extension
- Shows per-PR status in a tree view sidebar
- Review decision shown as icon next to PR title (✓ approved, changes icon, comment icon)
- CI status shown as status dot next to title
- Compact: icon + PR title + branch name per line
- **Key insight:** Single-line compact format works; users scan title + 2-3 icons

### Raycast GitHub Extension
- PR listed with: title, repo, status pill (Open/Merged/Closed/Draft)
- Color-coded pill is the primary scan signal
- Secondary metadata (author, updated time) in smaller text
- No inline review/CI breakdown — relies on click-through
- **Key insight:** For rapid scanning, a single priority pill is often enough

### Linear (GitHub PR sync)
- Links issues to PRs; shows PR state as a small badge on the issue
- States: `Open`, `In Review`, `Ready to Merge`, `Merged`, `Closed`
- **Key insight:** Derived states ("In Review", "Ready to Merge") are more useful than raw API states

## Recommended Status Taxonomy for Stapler Squad

### Compact `PRPriority` enum (derives from compound state)

```typescript
type PRPriority =
  | "blocking"    // 🔴 Red  — changes requested OR CI failing
  | "ready"       // 🟢 Green — approved + CI passing
  | "pending"     // 🟡 Yellow — checks running OR awaiting review
  | "draft"       // ⚫ Gray  — isDraft
  | "complete"    // ✅ Purple/dim — MERGED or CLOSED
  | "no_pr"       // (no badge shown)
  | "error"       // ⚠️ Orange — auth failure, API error
```

### Derivation logic

```
if PR.state == MERGED or CLOSED  → "complete"
if PR.isDraft                    → "draft"
if reviews.CHANGES_REQUESTED > 0 → "blocking"
if checks.FAILURE or ACTION_REQUIRED → "blocking"
if reviews.APPROVED > 0 AND checks.SUCCESS → "ready"
if checks.IN_PROGRESS or PENDING → "pending"
if no reviews AND no checks      → "pending"
if auth error / API error        → "error"
```

### Data structure for web UI

```typescript
interface PRStatus {
  // Core PR identity
  number: number;
  url: string;

  // Lifecycle state
  state: "open" | "closed" | "merged";
  isDraft: boolean;

  // Derived priority (computed server-side)
  priority: PRPriority;

  // Review breakdown
  approvedCount: number;
  changesRequestedCount: number;
  commentedCount: number;
  pendingReviewCount: number;

  // CI/checks
  checkConclusion: "success" | "failure" | "pending" | "in_progress" | "action_required" | "skipped" | "neutral" | null;

  // Freshness
  lastRefreshedAt: string; // ISO8601
  isStale: boolean;        // true if > 10 min since refresh
}
```

## Visual Design Patterns (Compact Card Display)

### Primary badge (top-right of session card)
Single pill showing priority:
- 🔴 **BLOCKED** (red) — changes requested or CI failure
- 🟢 **READY** (green) — approved + checks passing
- 🟡 **PENDING** (yellow) — awaiting review or checks running
- ⚫ **DRAFT** (gray) — isDraft
- ✅ **MERGED** (dim purple/gray) — complete

### Secondary inline row (below branch name)
Show only when relevant:
```
✓ 2  ✗ 1  ⏳ checks
```
- `✓ N` — approval count (green, show if > 0)
- `✗ N` — changes-requested count (red, always show if > 0)
- `⏳` — checks in progress indicator (yellow, if running)
- `✗ CI` — CI failure (red, if failed)

### Hover tooltip (rich detail)
```
PR #123: Fix authentication timeout
Branch: feature/auth-fix → main

Reviews:
  ✓ Approved by @alice, @bob
  ✗ Changes requested by @carol

Checks:
  ✓ tests (2m 14s)
  ✗ lint — 3 errors
  ⏳ deploy-preview

Last updated: 3 minutes ago
```

### Accessibility notes (from GitHub Primer)
- Never use color alone — always pair with icon or text
- Use `aria-label` on badge: `aria-label="PR #123: blocked — changes requested"`
- Ensure sufficient contrast ratio for red/green badges on dark background

## Key Design Decisions for Stapler Squad

1. **Compute `priority` server-side** (in Go) — don't send raw review arrays to client; send the derived enum. Reduces frontend logic complexity.

2. **Show NO badge when `no_pr`** — absence of badge is the signal. Don't show a grayed-out "No PR" badge; it adds noise.

3. **Show stale indicator** — if last refresh > 10 min, dim the badge and show age: "READY (8m ago)".

4. **Extend existing `GitHubBadge`** — don't create a new component. Add a `priority` prop to `GitHubBadge`; it drives the color/icon.

5. **Single-line compact target** — the session card already shows branch, status, tags. PR status must fit in ≤ 40px badge + optional inline count row.

## Sources

- [GitHub REST API - Pull Request Reviews](https://docs.github.com/en/rest/pulls/reviews)
- [GitHub REST API - Checks - Runs](https://docs.github.com/en/rest/checks/runs)
- [GitHub REST API - Pulls](https://docs.github.com/en/rest/pulls/pulls)
- [GitHub Actions Features](https://github.com/features/actions)
- [GitHub Primer Design System](https://primer.style)
- [VS Code GitHub Pull Requests Extension](https://github.com/microsoft/vscode-pull-request-github)
