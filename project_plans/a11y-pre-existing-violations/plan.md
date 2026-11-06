# Plan: a11y-pre-existing-violations

## Approach

Fix 8 WCAG AA violations across 6 routes in two clusters, ordered by dependency and risk.

No new dependencies. All fixes are isolated to their component's CSS or JSX; the only cross-cutting
change is adding `metadata` to `backlog-sources/layout.tsx`. No schema, API, or state changes.

---

## Cluster A — CSS token swaps (low risk, ~30 min each)

Execute in a single commit. Each change is 1–2 lines.

### A1: `/analytics/escape` — active tab contrast

**File:** `web-app/src/components/analytics/EscapeAnalyticsPage.css.ts:65`

Change:
```diff
-      color: vars.color.primary,
+      color: vars.color.accentText,
```

`accentText` is `#003d99` (light) / `#2d9cdb` (dark). Light: ~9.2:1 on white — passes. Dark:
`#2d9cdb` on `#0a0a0a` stays ~5.0:1 — still passes.

Leave `borderBottomColor: vars.color.primary` — decorative border, not text, needs only 3:1.

### A2: `/login` — button background contrast

**File:** `web-app/src/app/login/login.css.ts:56` (the `background:` line in the `.button` style)

Change:
```diff
-      background: vars.color.primary,
+      background: vars.color.primaryDark,
```

`primaryDark` is `#003d99` (light) / `#1a7fc1` (dark). White text on `#003d99` ~9.2:1; white on
`#1a7fc1` ~4.7:1. Both pass.

Confirm hover state uses `primaryHover` — if it's `primary` too, adjust it to `primaryDark` hover
equivalent. Check that `primaryDark` is defined in both themes in `theme.css.ts` (it is: line 84).

### A3: `/resolve` — DeepLinkErrorBanner action button color

**File:** `web-app/src/components/DeepLinkErrorBanner.css.ts:55` (the `color:` in `.primaryActionButton`)

Change:
```diff
-      color: vars.color.primary,
+      color: vars.color.accentText,
```

`accentText: #003d99` on `errorBg: #fee2e2` → ~5.0:1 (passes). Dark: `#2d9cdb` on dark
`errorBg: #7f1d1d` → ~4.9:1 (passes).

### A4: `/settings/backlog-sources` — document-title via layout metadata

**File:** `web-app/src/app/settings/backlog-sources/layout.tsx`

Add a `metadata` export before the component:
```ts
import type { Metadata } from "next";
export const metadata: Metadata = { title: "Backlog Sources" };
```

The title must match the value in `route-meta.ts` for `BacklogSourcesSettingsLayout`'s entry (axe
checks the `<title>` tag content). Verify the existing `route-meta.ts` row for this route; if
`guard: "backlog"` is already there, no change to that file is needed.

---

## Cluster B — Accessible label additions (~60–90 min, separate commit)

### B1: `/test/escape-codes` — checkbox labels + select names

**File:** `web-app/src/app/test/escape-codes/page.tsx`

**Checkboxes (82 nodes, lines 293–298):**
```diff
  <input
    type="checkbox"
+   aria-label={code.humanReadable}
    checked={selectedCodes.has(code.code)}
    onChange={() => toggleCodeSelection(code.code)}
    onClick={(e) => e.stopPropagation()}
  />
```

**Filter selects (lines 237–248, 250–260) — add `aria-label`:**
```diff
- <select value={filterCategory} onChange={...}>
+ <select aria-label="Filter by category" value={filterCategory} onChange={...}>
```
```diff
- <select value={filterPriority} onChange={...}>
+ <select aria-label="Filter by priority" value={filterPriority} onChange={...}>
```

**Control selects (lines 345–354, 358–369) — wire `<label>` to `<select>` via `id`/`htmlFor`:**
```diff
- <label>Frame Rate (FPS)</label>
- <select ...>
+ <label htmlFor="escape-fps-select">Frame Rate (FPS)</label>
+ <select id="escape-fps-select" ...>
```
```diff
- <label>Test Pattern</label>
- <select ...>
+ <label htmlFor="escape-pattern-select">Test Pattern</label>
+ <select id="escape-pattern-select" ...>
```

### B2: `/test/terminal-stress` — inline color fixes + select name

**File:** `web-app/src/app/test/terminal-stress/page.tsx`

**Description text (inline style, ~line 390):**
```diff
- <p style={{ color: '#6b7280', marginBottom: '20px' }}>
+ <p style={{ color: '#4b5563', marginBottom: '20px' }}>
```
`#4b5563` on white: ~7.5:1 — passes.

**`getStatusColor()` (~line 371–378) — replace all non-compliant hex values:**
```diff
- case 'idle': return '#6b7280';
+ case 'idle': return '#4b5563';       // ~7.5:1 on white
- case 'completed': return '#10b981';
+ case 'completed': return '#065f46';  // ~6.8:1 on white
- case 'running': return '#f59e0b';
+ case 'running': return '#92400e';    // ~5.0:1 on white
```

**`getFrameTimeStatus()` (~line 381–385) — same replacements:**
```diff
- if (frameTime < 100) return { label: 'Excellent', color: '#10b981' };
+ if (frameTime < 100) return { label: 'Excellent', color: '#065f46' };
- if (frameTime < 200) return { label: 'Good', color: '#f59e0b' };
+ if (frameTime < 200) return { label: 'Good', color: '#92400e' };
- return { label: 'Poor', color: '#ef4444' };
+ return { label: 'Poor', color: '#991b1b' };   // ~5.8:1 on white
```

**Scan for any additional inline `color:` usages of these values** (there may be more in the
metrics panel rendering, accounting for the 22-node count). Replace all occurrences of each
failing hex value with its compliant counterpart.

**Preset select — wire label to select (~lines 403–436):**
```diff
- <label style={{ display: 'block', marginBottom: '5px', fontWeight: 500 }}>
-   Test Preset
- </label>
- <select value={selectedPreset} data-testid="preset-selector" ...>
+ <label htmlFor="stress-preset-selector" style={{ display: 'block', marginBottom: '5px', fontWeight: 500 }}>
+   Test Preset
+ </label>
+ <select id="stress-preset-selector" value={selectedPreset} data-testid="preset-selector" ...>
```

---

## Verification Steps

After each cluster:
1. `make quick-check` (build + test + lint) — must pass
2. `cd tests/e2e && bun run test -- --grep "a11y-route-sweep"` or the equivalent pattern —
   the sweep must pass without the #1112-tagged KNOWN entries
3. Remove the corresponding KNOWN entries from `a11y-route-sweep.spec.ts` as each route passes

Remove KNOWN entries only after confirming the violation is gone — don't pre-delete them.

---

## Commit Strategy

| Commit | Contents |
|--------|----------|
| 1 | Cluster A: CSS token fixes + layout metadata (A1–A4) |
| 2 | Cluster B: escape-codes + terminal-stress label/contrast fixes (B1–B2) |
| 3 | Remove all #1112 KNOWN entries from a11y-route-sweep.spec.ts |

Three small, reviewable commits. Commit 3 depends on the sweep passing cleanly.

---

## Risk Assessment

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Darker blue (`accentText`/`primaryDark`) changes visual appearance noticeably | Low | Both tokens are existing theme values already used elsewhere in the system |
| `backlog-sources` metadata fix doesn't work via layout (client nav path) | Low | Next.js layout metadata propagates server-side; the title is in `<head>` regardless of client redirect |
| `/test/terminal-stress` has more than 22 failing color nodes than identified | Medium | Grep for all occurrences of failing hex values before changing them |
| E2E axe sweep flakiness masks a fix or false-positive | Low | Sweep is deterministic; each route loads fresh |
