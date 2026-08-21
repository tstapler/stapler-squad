# Backlog Management — Engineering-Ready UX Component Specifications

**Document type**: Component specification (implementation-ready)
**Extends**: `project_plans/backlog-management/ux-design.md`
**Date**: 2026-05-12
**Status**: All blocking gaps resolved — ready for engineering

---

## Table of Contents

1. [User Flow Maps](#1-user-flow-maps)
2. [GateVerdictBox](#2-gateVerdictBox)
3. [TriageLoadingIndicator](#3-triageloadingindicator)
4. [InlineError](#4-inlineerror)
5. [Empty State Redesign](#5-empty-state-redesign)
6. [Filter-Zero State](#6-filter-zero-state)
7. [Accessibility Checklist](#7-accessibility-checklist)
8. [UX Readiness Gate](#8-ux-readiness-gate)

---

## 1. User Flow Maps

### 1.1 Item Lifecycle — Where Components Appear

```
[/backlog page loads]
        │
        ├── Zero items → EmptyState (§5)
        │       │
        │       └── Click "+ Create First Item"
        │               └── Inline form expands (§5.3)
        │
        └── 1+ items → Table view
                │
       [User opens BacklogItemDetail]
                │
        ┌───────┴──────────────────────────────────────┐
        │                                              │
   item.status = "idea" / "ready" / "in_progress"    item.status = "review" / "done"
        │                                              │
   Normal action buttons                       GateVerdictBox (§2)
   (existing)                                         │
        │                                    ┌─────────┴──────────┐
   "Help me flesh this out" →                │                    │
   TriageLoadingIndicator (§3)         verdict = PENDING    verdict = PASS
        │                                    │                    │
   success / error                     Spinner shown       [Approve] primary
        │                                    │             [Reopen] secondary
   InlineError on failure (§4)        verdict = PARTIAL/FAIL      │
                                            │              [Skip gate] link
                                     [Reopen] primary
                                     Override form (§2.4)
```

### 1.2 Triage Sub-Flow (Item Level)

```
User clicks "Help me flesh this out"
        │
        ▼
service.triggerTriage(itemId) called
        │
        ▼
TriageLoadingIndicator renders in AC section
 ◌ Thinking about acceptance criteria...  0s  [Stop]
        │
    ┌───┴────────────────────────────────────────┐
    │                                            │
   ≤ 60s                                     > 60s
    │                                            │
 (label unchanged)              label → "Still thinking — up to 3 min"
    │                                            │
    └────────────────────────────────────────────┘
        │
    ┌───┴───────────────────────────┐
    │               │               │
 success        timeout          error
    │            > 180s             │
 Suggested AC   InlineError       InlineError
 badges appear  type=timeout      type=transient|permanent
```

### 1.3 Override Sub-Flow

```
User clicks "Override: Mark done anyway" toggle
        │
        ▼
Override form expands (animated ~200ms ease-out)
Focus moves to textarea
        │
        ▼
User types reason (≥ 5 characters) → "Mark Done — Override" button enables
        │
        ▼
User submits → service.overrideVerdict(itemId, "done") called
        │
    ┌───┴──────────┐
    │              │
 success         error
 item → done    InlineError inside override form
 panel refreshes
```

### 1.4 Skip Gate Sub-Flow

```
User clicks "Skip gate and mark done without review ↗"
        │
        ▼
Inline confirmation expands immediately below the link
Focus moves to [Cancel] button (first focus trap target)
        │
        ▼
Escape / Cancel → form collapses, focus → skip link
        │
        ▼
[Confirm — Skip Gate] → service.overrideVerdict(itemId, "done")
  with reason = "__skip_gate__"
item → done, panel refreshes
```

---

## 2. GateVerdictBox

### 2.1 Props Interface

```typescript
interface GateVerdictBoxProps {
  verdict: "PASS" | "PARTIAL" | "FAIL" | "PENDING";
  summary: string; // max 200 chars
  criteria?: Array<{ label: string; passed: boolean }>; // PARTIAL/FAIL only
  elapsedSeconds?: number; // PENDING only
  onApprove: () => Promise<void>;
  onReopen: () => Promise<void>;
  onOverride: (reason: string) => Promise<void>;
  onSkipGate: () => Promise<void>;
  actionPending?: boolean;
}
```

### 2.2 Visual Layout — All States

**PASS:**
```
┌── role="status" aria-live="polite" aria-label="Gate verdict" ───────────────┐
│  Gate Verdict                                                                │
│  ┌── border-left: 4px solid var(--success) ─────────────────────────────┐  │
│  │  ✓  PASSED                                                            │  │
│  │     All criteria met. Session output aligns with spec.                │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│  [  Approve — Mark Done  ]    [ Reopen for Revision ]                       │
│   ← primary                    ← secondary                                  │
│  Skip gate and mark done without review   ← sm, color: var(--text-muted)   │
└──────────────────────────────────────────────────────────────────────────────┘
```

**PARTIAL:**
```
┌── role="status" aria-live="polite" aria-label="Gate verdict" ───────────────┐
│  Gate Verdict                                                                │
│  ┌── border-left: 4px solid var(--warning) ─────────────────────────────┐  │
│  │  ◑  PARTIAL  ·  2 of 3 acceptance criteria met                        │  │
│  │     "AC-3: Error handling spec was not addressed."                     │  │
│  │  Criteria:                                                             │  │
│  │    ✓ Auth flow handles expired tokens                                  │  │
│  │    ✗ Error handling spec addressed                                     │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│  [  Reopen for Revision  ]   ← primary                                      │
│  ┌── id="override-section" (collapsed) ─────────────────────────────────┐  │
│  │  Override: Mark done anyway  [▸ expand]                               │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│  Skip gate and mark done without review                                      │
└──────────────────────────────────────────────────────────────────────────────┘
```

**FAIL:** Identical to PARTIAL but `border-left: 4px solid var(--error)`, icon `✗`.

**PENDING:**
```
┌── role="status" aria-live="polite" aria-label="Gate verdict" ───────────────┐
│  Gate Verdict                                                                │
│  ┌── border-left: 4px solid var(--text-muted) ──────────────────────────┐  │
│  │  ◌  PENDING  ·  Review in progress  (12s)                             │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│  [  Approve — Mark Done  ]    [ Reopen for Revision ]                       │
│   ← aria-disabled="true"       ← aria-disabled="true"                       │
│     title="Wait for gate result or use Skip Gate below"                      │
│  Skip gate and mark done without review   ← always enabled in PENDING       │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 Verdict Badge Tokens

| Verdict | Border | Icon | Text color | Background |
|---|---|---|---|---|
| PASS | `var(--success)` | ✓ | `var(--success)` | `var(--success-bg)` |
| PARTIAL | `var(--warning)` | ◑ | `var(--warning)` | `var(--warning-bg)` |
| FAIL | `var(--error)` | ✗ | `var(--error-text)` | `var(--error-bg)` |
| PENDING | `var(--text-muted)` | ◌ (CSS spin) | `var(--text-muted)` | `var(--card-background)` |

Status is icon + text + color — never color alone (WCAG 1.4.1).

### 2.4 Override Form

Disclosure widget (`aria-expanded`), not a modal.

```
┌── id="override-form" aria-label="Override gate verdict" role="form" ─────┐
│  Override: Mark done anyway  [▾ collapse]                                 │
│  <label for="override-reason">Reason for override (required)</label>      │
│  <textarea id="override-reason" rows="3"                                  │
│    placeholder="Explain why this item should be marked done despite..."   │
│    aria-describedby="override-hint" />                                    │
│  <span id="override-hint">Enter at least 5 characters to continue.</span>│
│  [ Cancel ]   [ Mark Done — Override ]                                    │
│               ← intent: danger                                            │
│               ← aria-disabled="true" until textarea.length >= 5          │
└───────────────────────────────────────────────────────────────────────────┘
```

- **On expand**: focus → textarea (after `transitionend`)
- **Cancel / Escape**: collapses form, clears textarea, focus → override toggle

### 2.5 Skip Gate Inline Confirmation

```
┌── role="alertdialog" aria-labelledby="skip-gate-warning" ────────────────┐
│  <span id="skip-gate-warning">                                           │
│    Skip gate and mark done without review                                │
│  </span>                                                                 │
│  The acceptance criteria will not be evaluated. This cannot be undone.   │
│  [ Cancel ]   [ Confirm — Skip Gate ]   ← intent: danger                │
└──────────────────────────────────────────────────────────────────────────┘
```

Focus trap: Tab/Shift+Tab cycles between Cancel and Confirm only.
On open: focus → Cancel. Escape / Cancel: close, focus → skip link.

### 2.6 Keyboard Shortcuts

| Key | Context | Action |
|---|---|---|
| `Ctrl+Enter` | GateVerdictBox, PASS verdict | Approve |
| `Ctrl+Enter` | GateVerdictBox, PARTIAL/FAIL | Reopen for Revision |
| `Escape` | Override form open | Close form, focus → toggle |
| `Escape` | Skip gate confirmation open | Close, focus → skip link |
| `Tab`/`Shift+Tab` | Skip gate confirmation | Cycle between Cancel and Confirm |

---

## 3. TriageLoadingIndicator

### 3.1 Props Interface

```typescript
interface TriageLoadingIndicatorProps {
  elapsedSeconds: number; // updated externally via setInterval
  context: "item" | "list";
  onCancel: () => void;
  compact?: boolean; // true = pill form (list context)
}
```

### 3.2 Visual Layout

**Item context (block form):**
```
┌── role="status" aria-live="polite" aria-label="Triage in progress" ──────┐
│  ◌  Thinking about acceptance criteria...  23s              [Stop]        │
└───────────────────────────────────────────────────────────────────────────┘
```

**After 60s:**
```
┌── role="status" aria-live="polite" ──────────────────────────────────────┐
│  ◌  Still thinking — up to 3 min  87s                       [Stop]       │
└───────────────────────────────────────────────────────────────────────────┘
```

**List context (compact pill):**
```
[ ◌  Thinking...  41s  × ]       →  [ ◌  Still working — up to 3 min  87s  × ]
```

### 3.3 States

| Elapsed | Label |
|---|---|
| 0–59s | "Thinking about acceptance criteria..." (item) / "Thinking..." (list) |
| 60–179s | "Still thinking — up to 3 min" (item) / "Still working — up to 3 min" (list) |
| 180s+ | Replace with `InlineError type="timeout"` |

### 3.4 Interaction

- **Stop (item)**: `aria-label="Cancel triage"` — calls `onCancel()`, component unmounts, link re-appears
- **× (list)**: `aria-label="Cancel triage"` — calls `onCancel()`, pill unmounts

### 3.5 Accessibility — Announcement Throttling

Update `aria-label` every **30 seconds only** (not every second — avoids screen reader spam):

```typescript
const ariaElapsed = Math.floor(elapsedSeconds / 30) * 30;
const ariaLabel = `Triage in progress, ${ariaElapsed} seconds elapsed`;
```

Visible counter can tick every second. Only `aria-label` is throttled.

### 3.6 Navigate-Away Persistence

Triage session is server-side — continues when component unmounts. On `BacklogItemDetail` mount, check:

```typescript
const activeTriageSession = item.triageSessionId && item.triageStatus === "running";
// If true: render TriageLoadingIndicator with elapsed = (now - item.triageStartedAt)
```

Requires new fields on `BacklogItem` response (see §8 Data Requirements).

---

## 4. InlineError

### 4.1 Props Interface

```typescript
interface InlineErrorProps {
  type: "transient" | "timeout" | "permanent";
  onRetry: () => void;
  logsSessionId?: string; // permanent only — shows "View session logs"
  customMessage?: string;
}
```

### 4.2 Copy per Type

| Type | Headline | Body | Actions |
|---|---|---|---|
| `transient` | "Triage failed" | "Network error. The request could not be completed." | [Retry ↺] + × |
| `timeout` | "Triage timed out" | "The triage session did not complete within 3 minutes." | [Retry ↺] + × |
| `permanent` | "Triage failed" | "The triage session exited unexpectedly (exit code 1). Check the session logs for details." | [View session logs] + [Retry ↺] + × |

### 4.3 Visual Layout

**Transient / Timeout (single line pill):**
```
[ ✕  Triage failed — network error.  [Retry ↺]   × ]
← role="alert" aria-live="assertive"
← border: 1px solid var(--error), border-radius: pill
← background: var(--error-bg), color: var(--error-text)
```

**Permanent (expanded block):**
```
┌── role="alert" aria-live="assertive" ─────────────────────────────────────┐
│  ✕  Triage failed                                                  [  ×  ] │
│     The triage session exited unexpectedly (exit code 1).                  │
│     Check the session logs for details.                                    │
│     [ View session logs ]   [ Retry ↺ ]                                   │
└────────────────────────────────────────────────────────────────────────────┘
```

### 4.4 Accessibility

- Container: `role="alert"`, `aria-live="assertive"` — errors require immediate announcement
- Retry: `aria-label="Retry triage"`
- Dismiss ×: `aria-label="Dismiss error"`
- ✕ icon: `aria-hidden="true"`
- External logs link: `aria-label="View session logs (opens in new tab)"` if `target="_blank"`

---

## 5. Empty State Redesign

### 5.1 State Machine

| Condition | State | Rendered |
|---|---|---|
| `items.length === 0`, form closed | first-run | Lifecycle diagram + CTA |
| `items.length === 0`, form open | first-run-form | Lifecycle diagram + inline form |
| `items.length > 0`, filters → 0 results | filter-zero | See §6 |
| `items.length > 0`, none in_progress | partially-populated | List + footer nudge |
| `items.length > 0`, 1+ in_progress | active | Normal list |

### 5.2 First-Run Layout

```
┌── role="region" aria-label="Backlog — empty" ────────────────────────────────┐
│                                                                              │
│         Your backlog is empty.                                               │
│         Create a work item, define what "done" looks like,                  │
│         spawn an agent — the system reviews output automatically.            │
│                                                                              │
│         ┌── aria-hidden="true" ───────────────────────────────────────┐    │
│         │   idea  ──►  ready  ──►  in progress  ──►  review  ──►  done│    │
│         │    ◉           ○              ○               ○          ○  │    │
│         │  (you start here)                                            │    │
│         └────────────────────────────────────────────────────────────┘    │
│                                                                              │
│                      [ + Create First Item ]                                 │
│                        ← autoFocus, only focusable element                   │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Lifecycle diagram tokens:**
- Active node ◉: `color: var(--primary)`, `font-size: lg`
- Inactive nodes ○: `color: var(--text-muted)`
- Labels: `font-size: sm`, `font-weight: 500`, `color: var(--text-secondary)`
- Diagram: `aria-hidden="true"` — meaning carried by prose above it
- Below 480px: stack vertically with downward arrows

### 5.3 Inline Form Expansion

No modal overlay. CTA is replaced; lifecycle diagram stays visible.

```
┌── role="region" aria-label="Backlog — empty" ────────────────────────────────┐
│  Your backlog is empty.                                                      │
│  idea  ──►  ready  ──►  in progress  ──►  review  ──►  done  (diagram)      │
│                                                                              │
│  ┌── role="form" aria-label="Create new backlog item" ─────────────────┐   │
│  │  <label for="item-title">Title</label>                               │   │
│  │  <input id="item-title" autoFocus required                          │   │
│  │    placeholder="What do you want to build or fix?" />                │   │
│  │  Priority  [ Low ▾ ]                                                 │   │
│  │  [ Cancel ]                          [ Create Item ]                 │   │
│  │  ← focus → CTA button on cancel      ← disabled until title valid   │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Focus management:**
1. Form renders → `titleInput.focus()` after `transitionend`
2. Cancel / Escape → form collapses, CTA re-appears, CTA receives focus
3. Submit success → form unmounts, list renders with new item highlighted

**Validation:**
- `<span role="alert">Title is required.</span>` inline below title input
- Create Item button: `aria-disabled="true"` + `disabled` when title empty

**Fields**: Title (required), Priority (optional, default Low), Labels (optional).  
AC is omitted — items start as `idea`; the detail view surfaces it as next step.

### 5.4 Partially-Populated Footer Nudge

```
┌── role="status" aria-live="polite" ──────────────────────────────────────────┐
│  No items are currently in progress.                                         │
│  Mark an item ready and spawn a session to start working.                    │
└──────────────────────────────────────────────────────────────────────────────┘
```

Rendered below the last table row. Disappears when any item → `in_progress`.

---

## 6. Filter-Zero State

When `items.length > 0` AND active filters produce 0 matches:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  [ search: "auth" ]  [ Status: Ready × ]  [ Priority: P1 × ]  (filter bar) │
│─────────────────────────────────────────────────────────────────────────────│
│                                                                             │
│  ┌── role="status" aria-live="polite" aria-label="No results" ──────────┐  │
│  │  No items match your filters.                                         │  │
│  │  [ Clear filters ]                                                    │  │
│  └────────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

**"Clear filters"**: `<button>` (action, not link). Clears all filter state in one click. Focus stays on the button.

**Must NOT show**: lifecycle diagram, "+ Create Item" CTA, onboarding copy.

| Visual element | First-run | Filter-zero |
|---|---|---|
| Lifecycle diagram | ✓ | ✗ |
| "+ Create First Item" CTA | ✓ | ✗ |
| Onboarding subline | ✓ | ✗ |
| "No items match" copy | ✗ | ✓ |
| "Clear filters" button | ✗ | ✓ |

---

## 7. Accessibility Checklist

### GateVerdictBox

| Requirement | Spec |
|---|---|
| Tab order | verdict region → primary CTA → secondary CTA → override toggle → textarea → Cancel → submit → skip link |
| Color independence | Icon + text + color (3 signals) |
| Contrast | `var(--error-text)` on `var(--error-bg)` ≥ 4.5:1 |
| Focus indicator | 2px `:focus-visible` outline, `var(--primary)`, 2px offset |
| ARIA — verdict box | `role="status"`, `aria-live="polite"`, `aria-label="Gate verdict"` |
| ARIA — override form | `role="form"`, `aria-label="Override gate verdict"` |
| ARIA — skip gate confirmation | `role="alertdialog"`, `aria-labelledby="skip-gate-warning"`, `aria-modal="true"` |
| Focus trap | Skip gate: Tab/Shift+Tab → Cancel ↔ Confirm only |
| Escape | Override form: close → toggle. Skip gate: close → skip link |
| Disabled buttons | `aria-disabled="true"` + `disabled` + `title` with reason |
| Touch targets | All buttons ≥ 44×44px |
| Spinner | `aria-label="Loading"` on animated span |

### TriageLoadingIndicator

| Requirement | Spec |
|---|---|
| Live region | `role="status"`, `aria-live="polite"` |
| Announcement cadence | `aria-label` updates every 30s only |
| Cancel label | `aria-label="Cancel triage"` |
| Spinner | `aria-hidden="true"` |
| Touch target | Stop/× ≥ 44×44px |

### InlineError

| Requirement | Spec |
|---|---|
| Live region | `role="alert"`, `aria-live="assertive"` |
| Retry | `aria-label="Retry triage"` |
| Dismiss | `aria-label="Dismiss error"` |
| Error icon | `aria-hidden="true"` |
| External link | `aria-label="View session logs (opens in new tab)"` |

### Empty State

| Requirement | Spec |
|---|---|
| Region | `role="region"`, `aria-label="Backlog — empty"` |
| Autofocus | `autoFocus` on "+ Create First Item" |
| Lifecycle diagram | `aria-hidden="true"` |
| Form region | `role="form"`, `aria-label="Create new backlog item"` |
| Focus on open | `titleInput.focus()` after animation |
| Focus on cancel | Returns to CTA button |
| Validation errors | `<span role="alert">` inline below field |
| Footer nudge | `role="status"`, `aria-live="polite"` |

### Filter-Zero State

| Requirement | Spec |
|---|---|
| Live region | `role="status"`, `aria-live="polite"`, `aria-label="No results"` |
| "Clear filters" | `<button>` not `<a>` |
| Focus after clear | Stays on button; list re-renders without forced jump |

---

## 8. UX Readiness Gate

- [x] **User flow mapped** — lifecycle, triage, override, skip gate, empty state expansion, navigate-away persistence
- [x] **Key states identified** — PENDING/PASS/PARTIAL/FAIL, loading 0–60s/60–180s/timeout, transient/permanent error, cancel, filter-zero, first-run, first-run-form, partially-populated, active
- [x] **Accessibility requirements noted** — ARIA roles, live region strategy (polite vs assertive), focus management, focus traps, color independence, keyboard shortcuts, touch targets, screen reader throttling
- [x] **Component specs ready for engineering** — TypeScript props, ASCII layouts, exact copy, interaction per input method, ARIA attribute lists, CSS token dependencies

---

## Implementation Notes

### New Files Required

```
web-app/src/components/backlog/
  GateVerdictBox.tsx + GateVerdictBox.css.ts
  TriageLoadingIndicator.tsx + TriageLoadingIndicator.css.ts
  InlineError.tsx + InlineError.css.ts
  BacklogEmptyState.tsx + BacklogEmptyState.css.ts  (extracted from page.tsx)
```

### Backend Fields Required on BacklogItem

```typescript
triageSessionId: string | null     // navigate-away persistence
triageStatus: "idle" | "running" | "done" | "failed"
triageStartedAt: string | null     // ISO 8601
gateVerdict: "PASS" | "PARTIAL" | "FAIL" | "PENDING" | null
gateVerdictSummary: string | null  // one-sentence summary
gateCriteria: Array<{ label: string; passed: boolean }> | null
```

### CSS Tokens Already Available

All tokens referenced by new components exist in `globals.css`:
`--success`, `--success-bg`, `--warning`, `--warning-bg`, `--error`, `--error-bg`, `--error-text`, `--text-muted`, `--primary`, `--text-secondary`, `--card-background`
