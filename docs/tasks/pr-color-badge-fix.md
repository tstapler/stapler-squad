# PR Color Badge Fix

**Feature:** Fix GitHub PR badge to show correct status colors instead of always showing the base purple (merged-style) color.

**Status:** Ready for implementation
**Branch:** `stapler-squad-pr-colors`
**Created:** 2026-04-14
**Label required:** `patch`

---

## Root Cause Analysis

The badge always renders in the base purple (`#8250df`) because `session.githubPrPriority` is empty string when it reaches `GitHubBadge`. The `priorityClass()` switch hits the `default: return ""` branch, so no variant class is appended to the base `.prBadge` class.

The base `.prBadge` color (`#8250df`) matches GitHub's merged-PR purple, causing the perception that the badge is stuck on "merged". In reality it is showing the fallback color for missing priority data.

### Why the priority field is empty on first render

The `PRStatusPoller` polls at a 60-second interval. Sessions created from GitHub PR URLs have `GitHubPRNumber > 0` populated immediately (from session creation), but `GitHubPRPriority` starts as `""` — the first poll tick runs 60 seconds after poller start. Until the first poll completes, every badge renders purple with no label.

### Secondary issue: module.css uses hardcoded hex colors

`GitHubBadge.module.css` hardcodes hex color values (`background: #8250df`, `border-color: #6e40c9`, etc.) in violation of the vanilla-extract ADR (ADR-009). The variant classes (`.prBadgeBlocking`, `.prBadgeReady`, etc.) correctly reference `var(--error)`, `var(--success)`, `var(--warning)` from `globals.css`, but the base `.prBadge` and `.repoBadge` classes are hardcoded.

---

## Dependency Visualization

```
Task 1: Diagnose data flow - confirm prPriority is actually empty on first render
  (no code change; just confirms the hypothesis before fixing it)
        |
        v
Task 2: Initial poll on poller start
  session/pr_status_poller.go
  Add immediate first-tick check so sessions show real status within ~10s of startup
        |
        v
Task 3: Migrate GitHubBadge styles to vanilla-extract
  web-app/src/components/sessions/GitHubBadge.css.ts  (new)
  web-app/src/styles/theme.css.ts                     (new, or extend if it exists)
  web-app/src/components/sessions/GitHubBadge.tsx     (update import)
  Delete web-app/src/components/sessions/GitHubBadge.module.css
        |
        v
Task 4: Add "loading" / "unknown" badge state
  GitHubBadge.tsx: when prNumber > 0 but prPriority == "" show a neutral gray
  badge with no label (not purple, not confusing)
        |
        v
Task 5: Verification
  Manual smoke test with a live session that has a PR
  Check each badge state: open/pending, approved/ready, failing/blocking, draft, merged/complete
```

---

## Story 1: Diagnose + Fix Data Flow

### Task 1.1: Confirm priority is empty on first render

**Goal:** Verify the hypothesis that `githubPrPriority` is empty string (not "complete") when badges appear purple.

Add a `console.log` temporarily in `GitHubBadge.tsx` before the `priorityClass()` call to confirm the received value. Alternatively, inspect the network response from the ConnectRPC `ListSessions` call and check the `github_pr_priority` field.

**Files:** `web-app/src/components/sessions/GitHubBadge.tsx` (temporary debug, revert before commit)

**Acceptance criteria:**
- Confirmed: `prPriority` is `""` on sessions loaded before the first poll tick
- OR discovered: a different root cause (e.g. proto serialization, field mapping bug)

---

### Task 1.2: Trigger initial PR status check on poller start

**Goal:** Eliminate the 60-second delay before the first real priority is visible.

**File:** `session/pr_status_poller.go`

Modify `pollLoop()` to call `checkAllSessions()` once immediately on start, before the ticker fires:

```go
func (p *PRStatusPoller) pollLoop() {
    defer p.wg.Done()
    ticker := time.NewTicker(p.config.PollInterval)
    defer ticker.Stop()

    // Run an immediate check so sessions show real status within the
    // first poll timeout rather than waiting a full PollInterval.
    p.checkAllSessions()

    for {
        select {
        case <-p.ctx.Done():
            return
        case <-ticker.C:
            p.checkAllSessions()
        }
    }
}
```

**Acceptance criteria:**
- Sessions with `GitHubPRNumber > 0` have a non-empty `GitHubPRPriority` within ~10 seconds of server start
- Existing 60-second polling cadence is unchanged after the initial check

---

### Task 1.3: Verify proto field mapping is correct end-to-end

**Goal:** Confirm `github_pr_priority` (proto field 35) flows correctly from Go instance through the adapter to the TypeScript `Session` type.

**Files to check (read-only):**
- `server/adapters/instance_adapter.go` — `GithubPrPriority: inst.GitHubPRPriority` (line 50) — already correct
- `proto/session/v1/types.proto` — field 35 `github_pr_priority` — already defined
- Generated `web-app/src/gen/session/v1/types_pb.ts` — verify `githubPrPriority` camelCase field exists

No code change expected; this is a verification step. If a mismatch is found, regenerate protos with `make generate-proto`.

---

## Story 2: Fix Badge Visual State

### Task 2.1: Add "not yet loaded" state to GitHubBadge

**Goal:** When a session has a PR number but priority has not yet been polled, show a neutral gray badge instead of the misleading purple.

**File:** `web-app/src/components/sessions/GitHubBadge.tsx`

In `priorityClass()`, the `default` case already returns `""` (falls back to base purple). The label logic in `priorityLabel()` returns `""` for unknown priority, so the badge shows `#123` with no label in purple — visually indistinguishable from "merged".

Add a `"loading"` sentinel to distinguish "no priority yet" from "default purple":

```tsx
function priorityClass(priority: string | undefined): string {
  switch (priority) {
    case "blocking":   return styles.prBadgeBlocking;
    case "ready":      return styles.prBadgeReady;
    case "pending":    return styles.prBadgePending;
    case "draft":      return styles.prBadgeDraft;
    case "complete":   return styles.prBadgeComplete;
    case "no_pr":      return styles.prBadgeNoPR;      // new: gray, no label
    case "auth_error":
    case "error":      return styles.prBadgeError;     // new: gray/dim, error label
    default:           return styles.prBadgeUnknown;   // new: gray, no label (not loaded yet)
  }
}

function priorityLabel(priority: string | undefined, isDraft?: boolean): string {
  if (isDraft) return "Draft";
  switch (priority) {
    case "blocking":   return "Blocked";
    case "ready":      return "Ready";
    case "pending":    return "Pending";
    case "draft":      return "Draft";
    case "complete":   return "Merged";
    case "auth_error": return "Auth Error";
    case "error":      return "Error";
    default:           return "";  // no label for no_pr / loading
  }
}
```

**Acceptance criteria:**
- Sessions with no priority data show gray badge (not purple)
- Sessions with priority data show the correct color
- No regression on existing priority states

---

### Task 2.2: Migrate GitHubBadge styles to vanilla-extract

**Goal:** Replace `GitHubBadge.module.css` with a `GitHubBadge.css.ts` vanilla-extract file that uses theme tokens. Required by ADR-009.

**Files:**
- `web-app/src/styles/theme.css.ts` — create the shared token contract (if it does not exist yet)
- `web-app/src/components/sessions/GitHubBadge.css.ts` — new vanilla-extract styles
- `web-app/src/components/sessions/GitHubBadge.tsx` — update import from `.module.css` to `.css.ts`
- `web-app/src/components/sessions/GitHubBadge.module.css` — delete

#### Step A: Create `web-app/src/styles/theme.css.ts`

The `theme.css.ts` file does not exist yet. Create a minimal theme contract with only the tokens needed for this component. It can be extended later as more components migrate.

```ts
// web-app/src/styles/theme.css.ts
import { createThemeContract } from '@vanilla-extract/css';

// Token contract — values come from globals.css CSS custom properties.
// This provides type-safe references in .css.ts component files.
// Note: vanilla-extract is build-time only; at runtime the CSS custom properties
// from globals.css are the actual source of truth.
export const vars = createThemeContract({
  color: {
    // Status
    statusError: null,
    statusSuccess: null,
    statusWarning: null,
    // Text
    textSecondary: null,
    textMuted: null,
    // Surfaces
    prBadgePurple: null,
    prBadgePurpleDark: null,
    prBadgePurpleBorder: null,
    prBadgeGrayBg: null,
    prBadgeGrayText: null,
    prBadgeGrayBorder: null,
  },
});
```

Then create a light-theme binding file `web-app/src/styles/theme.light.css.ts` that assigns the globals.css values:

```ts
// web-app/src/styles/theme.light.css.ts
import { createTheme } from '@vanilla-extract/css';
import { vars } from './theme.css';

export const lightTheme = createTheme(vars, {
  color: {
    statusError:            'var(--error)',
    statusSuccess:          'var(--success)',
    statusWarning:          'var(--warning)',
    textSecondary:          'var(--text-secondary)',
    textMuted:              'var(--text-muted)',
    prBadgePurple:          '#8250df',
    prBadgePurpleDark:      '#6e40c9',
    prBadgePurpleBorder:    '#6e40c9',
    prBadgeGrayBg:          'var(--text-muted, #6e7681)',
    prBadgeGrayText:        '#ffffff',
    prBadgeGrayBorder:      'var(--text-muted, #6e7681)',
  },
});
```

NOTE: If the project is not yet applying lightTheme class to `<html>`, an alternative is to use `globalStyle` in `theme.css.ts` to bridge the gap — or simply reference `var(--xxx)` globals directly from `GitHubBadge.css.ts` using the `vars()` CSS escape hatch. See the ADR-009 "bridge pattern":

```ts
// Simpler approach while theme contract is not yet applied globally:
// Use CSS custom properties directly in vanilla-extract
export const dynamicCard = style({
  background: 'var(--error)',   // OK in vanilla-extract; typed via csstype
});
```

For this initial migration, the simpler `var(--xxx)` reference approach (without `createThemeContract`) is acceptable given that `theme.css.ts` does not yet exist. Revisit full `createThemeContract` when more components migrate.

#### Step B: Create `web-app/src/components/sessions/GitHubBadge.css.ts`

```ts
// web-app/src/components/sessions/GitHubBadge.css.ts
import { style, recipe } from '@vanilla-extract/css';

// ---- Base badge ----
export const badge = style({
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
  padding: '4px 10px',
  borderRadius: 12,
  fontSize: 12,
  fontWeight: 600,
  textDecoration: 'none',
  transition: 'all 0.2s ease',
  cursor: 'pointer',
  border: '1px solid transparent',
  ':hover': {
    transform: 'translateY(-1px)',
    boxShadow: '0 2px 4px rgba(0, 0, 0, 0.1)',
  },
  ':active': {
    transform: 'translateY(0)',
  },
  ':focus': {
    outline: '2px solid var(--primary)',
    outlineOffset: 2,
  },
});

// ---- PR badge recipe (priority-driven variants) ----
export const prBadge = recipe({
  base: [
    badge,
    {
      // Default: GitHub merged-PR purple (shown when PR exists, no priority yet)
      // Only applied when priority variant is not matched.
      background: '#8250df',
      color: '#ffffff',
      borderColor: '#6e40c9',
      ':hover': {
        background: '#6e40c9',
        boxShadow: '0 2px 6px rgba(130, 80, 223, 0.3)',
      },
    },
  ],
  variants: {
    priority: {
      blocking: {
        background: 'var(--error)',
        color: '#ffffff',
        borderColor: 'var(--error)',
        ':hover': { opacity: 0.9, boxShadow: '0 2px 6px rgba(0,0,0,0.2)' },
      },
      ready: {
        background: 'var(--success)',
        color: '#ffffff',
        borderColor: 'var(--success)',
        ':hover': { opacity: 0.9, boxShadow: '0 2px 6px rgba(0,0,0,0.2)' },
      },
      pending: {
        background: 'var(--warning)',
        color: '#1a1a1a',
        borderColor: 'var(--warning)',
        ':hover': { opacity: 0.9, boxShadow: '0 2px 6px rgba(0,0,0,0.2)' },
      },
      draft: {
        background: 'var(--text-muted, #6e7681)',
        color: '#ffffff',
        borderColor: 'var(--text-muted, #6e7681)',
        ':hover': { opacity: 0.9, boxShadow: '0 2px 6px rgba(0,0,0,0.15)' },
      },
      complete: {
        background: 'var(--text-secondary, #57606a)',
        color: '#ffffff',
        borderColor: 'var(--text-secondary, #57606a)',
        opacity: 0.8,
        ':hover': { opacity: 1 },
      },
      no_pr: {
        background: 'var(--text-muted, #6e7681)',
        color: '#ffffff',
        borderColor: 'var(--text-muted, #6e7681)',
        opacity: 0.7,
      },
      error: {
        background: 'var(--text-muted, #6e7681)',
        color: '#ffffff',
        borderColor: 'var(--text-muted, #6e7681)',
        opacity: 0.7,
      },
      unknown: {
        // Not-yet-loaded: neutral gray instead of misleading purple
        background: 'var(--text-muted, #6e7681)',
        color: '#ffffff',
        borderColor: 'var(--text-muted, #6e7681)',
        opacity: 0.6,
      },
    },
    compact: {
      true: {
        padding: '4px 8px',
        fontSize: 11,
      },
    },
  },
});

// ---- Repository badge ----
export const repoBadge = recipe({
  base: [
    badge,
    {
      background: '#f6f8fa',
      color: '#24292f',
      borderColor: '#d0d7de',
      ':hover': {
        background: '#eaeef2',
        borderColor: '#afb8c1',
        boxShadow: '0 2px 6px rgba(0, 0, 0, 0.1)',
      },
      '@media': {
        '(prefers-color-scheme: dark)': {
          background: '#21262d',
          color: '#c9d1d9',
          borderColor: '#30363d',
        },
      },
    },
  ],
  variants: {
    compact: {
      true: {
        padding: '4px 8px',
        fontSize: 11,
      },
    },
  },
});

// ---- Sub-elements ----
export const icon = style({
  width: 14,
  height: 14,
  flexShrink: 0,
});

export const iconCompact = style({
  width: 12,
  height: 12,
});

export const text = style({
  whiteSpace: 'nowrap',
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  maxWidth: 200,
});

export const textCompact = style({
  maxWidth: 100,
});

export const priorityLabel = style({
  fontSize: 10,
  fontWeight: 700,
  letterSpacing: '0.03em',
  textTransform: 'uppercase',
  opacity: 0.9,
  paddingLeft: 2,
  borderLeft: '1px solid rgba(255, 255, 255, 0.4)',
  marginLeft: 2,
});
```

#### Step C: Update `GitHubBadge.tsx` to use the new styles

Replace the import:
```tsx
// Before
import styles from "./GitHubBadge.module.css";

// After
import * as styles from "./GitHubBadge.css";
```

Update class application:
```tsx
// Map priority string to the recipe variant key
type PriorityVariant = "blocking" | "ready" | "pending" | "draft" | "complete" | "no_pr" | "error" | "unknown";

function toPriorityVariant(priority: string | undefined): PriorityVariant {
  switch (priority) {
    case "blocking":   return "blocking";
    case "ready":      return "ready";
    case "pending":    return "pending";
    case "draft":      return "draft";
    case "complete":   return "complete";
    case "no_pr":      return "no_pr";
    case "auth_error":
    case "error":      return "error";
    default:           return "unknown";
  }
}

// In the render:
const priorityVariant = toPriorityVariant(prPriority);

<a
  className={styles.prBadge({ priority: priorityVariant, compact })}
  ...
>
  <svg className={compact ? styles.iconCompact : styles.icon} ...>
  <span className={compact ? styles.textCompact : styles.text}>#{prNumber}</span>
  {statusLabel && <span className={styles.priorityLabel}>{statusLabel}</span>}
</a>
```

**Acceptance criteria:**
- `GitHubBadge.module.css` is deleted
- All badge colors use design tokens from `globals.css` (no hardcoded hex in component layer)
- `prBadge` base class purple is only shown when priority is genuinely unknown
- Recipe compiles without TypeScript errors

---

## Story 3: Tests

### Task 3.1: Unit tests for `priorityClass` / `toPriorityVariant`

**File:** `web-app/src/components/sessions/__tests__/GitHubBadge.test.tsx` (new)

Test cases:
- Each priority string maps to its correct variant
- Empty string `""` maps to `"unknown"` (not purple/complete)
- `"auth_error"` maps to `"error"`
- `null` / `undefined` maps to `"unknown"`

### Task 3.2: Poller initial check test

**File:** `session/pr_status_poller_test.go`

Add test: Start poller, verify `checkAllSessions` is called once immediately without waiting for first tick. Can use a short `PollInterval` (100ms) and confirm first callback fires before 50ms.

---

## Story 4: Manual Verification Checklist

Before closing the PR, verify each badge state visually:

| Scenario | Expected badge color | Expected label |
|----------|---------------------|----------------|
| PR with CI failing | Red | Blocked |
| PR with changes requested | Red | Blocked |
| PR with approval + CI passing | Green | Ready |
| PR open, CI in progress | Yellow | Pending |
| PR open, no reviews, no CI | Yellow | Pending |
| PR in draft | Gray | Draft |
| PR merged or closed | Dark gray, 80% opacity | Merged |
| Session with no PR yet | No badge | N/A |
| Session with PR number but priority not yet polled | Neutral gray | (no label) |
| GitHub auth error | Neutral gray | Auth Error |

---

## Known Issues

### Bug 1: 60-second window where all PR badges show purple [SEVERITY: High]

**Description:** Between server start and first poll tick, all PR badges render purple with no label because `GitHubPRPriority` is empty string. This is the primary reported bug.

**Fix:** Task 1.2 — immediate `checkAllSessions()` call on poller start.

**Files affected:**
- `session/pr_status_poller.go`

### Bug 2: No visual distinction between "not loaded" and "open PR" [SEVERITY: Medium]

**Description:** Even after the fix in Task 1.2, there is a ~10-second window where the badge shows purple (empty priority). Without a distinct "loading" color, users cannot tell whether the badge is still initializing or genuinely has no priority data.

**Fix:** Task 2.1 — add `"unknown"` priority variant mapped to neutral gray.

**Files affected:**
- `web-app/src/components/sessions/GitHubBadge.tsx`
- `web-app/src/components/sessions/GitHubBadge.css.ts`

### Bug 3: GitHubBadge.module.css violates ADR-009 [SEVERITY: Low]

**Description:** The base `.prBadge` class uses hardcoded hex values `#8250df` and `#6e40c9` that should reference design tokens. The variant classes partially comply (they use `var(--error)`, `var(--success)`, `var(--warning)`) but the file is still a `.module.css`, not a `.css.ts`.

**Fix:** Task 2.2 — full vanilla-extract migration.

**Files affected:**
- `web-app/src/components/sessions/GitHubBadge.module.css` (delete)
- `web-app/src/components/sessions/GitHubBadge.css.ts` (create)
- `web-app/src/components/sessions/GitHubBadge.tsx` (update import)

### Potential Bug 4: Priority not persisted across server restart [SEVERITY: Low]

**Description:** `session/storage.go` has `UpdateInstancePRStatus()` which persists priority to disk. The poller calls it on each update. On server restart, `GitHubPRPriority` is loaded from the stored session data, so this should work. Verify during testing that restarting the server with a session that has a known priority still shows the correct badge color immediately (before the first poll tick).

**Mitigation:** The immediate initial poll in Task 1.2 provides a safety net even if persistence fails. No separate fix required unless testing reveals the stored value is not loaded.

---

## Files Changed Summary

| File | Change type | Notes |
|------|-------------|-------|
| `session/pr_status_poller.go` | Edit | Add immediate first check in `pollLoop()` |
| `web-app/src/components/sessions/GitHubBadge.tsx` | Edit | Replace module.css import; add `toPriorityVariant()`; add unknown/error/no_pr labels |
| `web-app/src/components/sessions/GitHubBadge.css.ts` | Create | vanilla-extract styles with `recipe()` for priority variants |
| `web-app/src/components/sessions/GitHubBadge.module.css` | Delete | Replaced by .css.ts |
| `web-app/src/styles/theme.css.ts` | Create | Minimal theme contract (extend as more components migrate) |
| `session/pr_status_poller_test.go` | Edit | Add test for initial check |
| `web-app/src/components/sessions/__tests__/GitHubBadge.test.tsx` | Create | Unit tests for priority variant mapping |

---

## Implementation Order

1. Task 1.2 (poller initial check) — backend-only, lowest risk, highest impact
2. Task 1.3 (verify proto field mapping) — read-only validation
3. Task 2.1 (unknown state in GitHubBadge.tsx) — UI logic change only, no CSS
4. Task 2.2 (migrate to vanilla-extract) — CSS migration
5. Task 3.1 + 3.2 (tests)
6. Story 4 (manual verification)
