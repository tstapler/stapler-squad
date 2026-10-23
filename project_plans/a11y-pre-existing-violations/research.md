# Research: a11y-pre-existing-violations

## Summary

Six routes carry KNOWN a11y exemptions tagged `#1112` (added by Story 7.8a). All violations are
in the Next build only. Research below identifies the exact elements, color values, and fix
locations for each.

---

## 1. `/analytics/escape` — color-contrast (1 node, `#tab-per_session`)

**File:** `web-app/src/components/analytics/EscapeAnalyticsPage.css.ts:64–68`

The active tab button (`[aria-selected="true"]`) uses `color: vars.color.primary`:
```ts
'&[aria-selected="true"]': {
  color: vars.color.primary,        // light: #0070f3, dark: #2d9cdb
  borderBottomColor: vars.color.primary,
  fontWeight: vars.fontWeight.semibold,
},
```

The page background is `vars.color.background` (light: `#ffffff`). Contrast ratio:
- Light: `#0070f3` on `#ffffff` ≈ **4.15:1** — fails WCAG AA (needs ≥ 4.5:1)
- Dark: `#2d9cdb` on `#0a0a0a` ≈ **5.0:1** — passes, but worth checking

The axe rule `color-contrast` fires on the in-focus tab button in its selected state. The tab
background is transparent so the contrast is against the root background.

**Fix (component-scoped CSS only):** Change `color: vars.color.primary` to
`color: vars.color.accentText` (light: `#003d99`, contrast ~9.2:1 on white; dark unchanged because
`accentText: "#2d9cdb"` and passes there). The `borderBottomColor` can keep `vars.color.primary`
since borders are not text and need only 3:1.

The default (non-selected) tab uses `color: vars.color.textMuted` (light: `#6b6b6b`, dark:
`#8a8a8a`) — `#6b6b6b` on white is ~4.93:1 (passes); no change needed there.

---

## 2. `/login` — color-contrast (1 node)

**File:** `web-app/src/app/login/login.css.ts:53–74`

The login/register button uses:
```ts
background: vars.color.primary,    // light: #0070f3
color: vars.color.primaryText,     // #ffffff
```

White text on `#0070f3` is the same ~4.15:1 that fails in item 1. In the dark theme:
`primary: #2d9cdb` gives white text ~3.34:1 — fails more severely.

The hint text (`color: vars.color.textMuted = #6b6b6b`) is on `cardBackground: #f9f9f9` → ~4.72:1
— passes. The subtitle (`textSecondary: #4a4a4a`) also passes.

**Fix:** Change the button `background` from `vars.color.primary` to `vars.color.primaryDark`
(light: `#003d99`, dark: `#1a7fc1`). Both give white text well above 4.5:1. The hover state
(`vars.color.primaryHover`) should similarly point to a dark enough value; confirm both themes.

---

## 3. `/resolve` — color-contrast (1 node)

**File:** `web-app/src/components/DeepLinkErrorBanner.css.ts:52–63`

When visited with no `?url=` param, the page immediately shows `reason="malformed"` via
`DeepLinkErrorBanner` in the `alertContainer` state. The `primaryActionButton` uses:
```ts
color: vars.color.primary,    // light: #0070f3 on errorBg: #fee2e2
```

`#0070f3` on `#fee2e2` (light pink error bg) ≈ **3.62:1** — fails.

The `errorText: #991b1b` on `errorBg: #fee2e2` is ~6.38:1 — passes fine.

**Fix:** Change `primaryActionButton.color` from `vars.color.primary` to `vars.color.accentText`
(`#003d99` in light; in dark, `accentText: #2d9cdb` on `errorBg: #7f1d1d` needs checking but
dark errorBg is much darker so contrast should be fine). The `accentText` token was introduced
exactly for this pattern (comment in `theme.css.ts:116–120`).

---

## 4. `/settings/backlog-sources` — document-title

**Files:**
- `web-app/src/app/settings/backlog-sources/layout.tsx` — wraps children in `<FeatureGuard flag="backlog">`
- `web-app/src/app/settings/backlog-sources/page.tsx` — exports `metadata` (title)
- `web-app/src/app/FeatureGuard.tsx` → `RequireFeatureFlag` in `FeatureFlagsContext.tsx:165–195`

`RequireFeatureFlag` renders `null` and calls `router.replace(redirectTo)` when the `backlog`
flag is disabled OR while still loading. In the E2E test environment the flag is likely off or
not yet resolved, so the component renders null immediately after hydration.

The `metadata` export in `page.tsx` is a Next.js server-side mechanism: it writes `<title>` into
the SSR HTML. But in the test environment the axe sweep may use a client-side navigation (not a
fresh SSR page load), in which case Next.js updates the `<title>` only when the page component
actually mounts — which never happens because `FeatureGuard` short-circuits it. The document
title is therefore whatever the previous page left it as, or the shell layout's default (empty or
undefined).

Confirmed: `layout.tsx` has **no** `metadata` export. Only `page.tsx` exports `metadata`. When
`FeatureGuard` renders null, the page-level metadata never applies client-side.

**Fix:** Export `metadata` from `layout.tsx` in addition to (or instead of) `page.tsx`. The
layout-level metadata runs independently of whether children render, so the title is always set.
Alternatively, ensure `RequireFeatureFlag`'s null-render state is preceded by a `<title>` set
via `document.title` or a `<Head>` component — but the layout metadata approach is simpler and
idiomatic for Next.js 13+.

---

## 5. `/test/escape-codes` — label (82 nodes) + select-name (4 nodes)

**File:** `web-app/src/app/test/escape-codes/page.tsx`

### label (82 nodes) — checkboxes

Each code item renders (lines 293–298):
```tsx
<input
  type="checkbox"
  checked={selectedCodes.has(code.code)}
  onChange={() => toggleCodeSelection(code.code)}
  onClick={(e) => e.stopPropagation()}
/>
<span className={styles.codeTitle}>{code.humanReadable}</span>
```

No `id`, no `aria-label`, not wrapped in a `<label>`. There are ~82 codes in `ESCAPE_CODE_LIBRARY`
(matches the "82 nodes" count). Axe rule: `label` (input must have an accessible label).

**Fix:** Add `aria-label={code.humanReadable}` to each checkbox. Wrapping in `<label>` is
technically cleaner but would require restructuring the click handler; `aria-label` is the minimal
change.

### select-name (4 nodes) — filter and control selects

Four `<select>` elements lack accessible names:

| Line | Element | Issue |
|------|---------|-------|
| 237–248 | Category filter `<select>` | No `id`, no `aria-label`, no associated `<label>` |
| 250–260 | Priority filter `<select>` | Same |
| 345–354 | Frame Rate `<select>` | `<label>Frame Rate (FPS)</label>` present but no `htmlFor` + `id` |
| 358–369 | Test Pattern `<select>` | Same |

**Fix:** For the two filter selects, add `aria-label="Filter by category"` / `"Filter by priority"`.
For the two control selects, add matching `htmlFor` on `<label>` and `id` on `<select>`.

---

## 6. `/test/terminal-stress` — color-contrast (22 nodes) + select-name (1 node)

**File:** `web-app/src/app/test/terminal-stress/page.tsx`

### color-contrast (22 nodes)

The page uses **inline styles with hardcoded hex values** (not CSS tokens). Problematic colors on
white background:

| Color | Usage | Contrast on white | Status |
|-------|-------|-------------------|--------|
| `#6b7280` | Description text (line 390), idle status | ~4.18:1 | **FAILS** (needs 4.5:1) |
| `#10b981` | Completed status, "Excellent" frame time (lines 372–384) | ~2.41:1 | **FAILS** |
| `#f59e0b` | "Good" frame time color | ~2.90:1 | **FAILS** |
| `#ef4444` | "Poor" frame time / stop button color | ~4.49:1 | borderline, likely fails in some contexts |
| `#9ca3af` | Disabled button background | ~2.79:1 | fails as text |

With 22 failing nodes, the metrics display panel (not read yet, likely offset ~400+) almost
certainly renders many per-metric colored spans using these values. `getStatusColor()` (line
371–378) and `getFrameTimeStatus()` (line 381–385) return hardcoded hex strings applied as inline
`style={{ color: ... }}`.

**Fix:** Replace hardcoded inline style colors with either:
- WCAG-compliant equivalents: `#4b5563` for gray (~7.5:1 on white), `#065f46` for green (~6.8:1),
  `#92400e` for amber (~5.0:1), `#991b1b` for red (~5.8:1)
- Or CSS token references (`vars.color.warningText`, `vars.color.successText`, etc. from theme.css.ts)

Preferably refactor to a `.css.ts` file using the existing theme tokens instead of inline styles.
The page currently has no companion `.css.ts` — the full page uses one giant inline-style `<div>`.

### select-name (1 node)

The preset selector (lines 403–436):
```tsx
<div>
  <label style={{ display: 'block', marginBottom: '5px', fontWeight: 500 }}>
    Test Preset
  </label>
  <select
    value={selectedPreset}
    ...
    data-testid="preset-selector"
  >
```

`<label>` has no `htmlFor`, `<select>` has no `id`. Axe cannot associate them.

**Fix:** Add `htmlFor="preset-selector"` to `<label>` and `id="preset-selector"` to `<select>` (it
already has `data-testid="preset-selector"` — just add the `id` alongside it).

---

## Implementation Strategy

Group into two natural clusters by effort:

**Cluster A — quick CSS token fixes (30–60 min each):**
1. `/analytics/escape`: 1-line CSS change in `EscapeAnalyticsPage.css.ts`
2. `/login`: 1-line CSS change in `login.css.ts`
3. `/resolve`: 1-line CSS change in `DeepLinkErrorBanner.css.ts`
4. `/settings/backlog-sources`: add `metadata` export to `layout.tsx`

**Cluster B — test page accessible-label additions (60–90 min each):**
5. `/test/escape-codes`: `aria-label` on 82 checkboxes + `htmlFor`/`id` or `aria-label` on 4 selects
6. `/test/terminal-stress`: `htmlFor`/`id` on preset selector + replace inline hardcoded colors with
   AA-compliant values (the larger of the two because 22 failing nodes implies many inline-style
   colored spans need updating)

The `/test/*` pages are developer debug tools only (no end-user path). They still must meet WCAG AA
per the sweep's mandate, but the bar for visual refinement is low — `aria-label` + minimum contrast
fixes are sufficient.

---

## Key File References

| Violation | File | Lines |
|-----------|------|-------|
| `/analytics/escape` active tab color | `web-app/src/components/analytics/EscapeAnalyticsPage.css.ts` | 64–68 |
| `/login` button background | `web-app/src/app/login/login.css.ts` | 53–74 |
| `/resolve` action button color | `web-app/src/components/DeepLinkErrorBanner.css.ts` | 52–63 |
| `/settings/backlog-sources` layout | `web-app/src/app/settings/backlog-sources/layout.tsx` | 1–5 |
| `/test/escape-codes` checkboxes | `web-app/src/app/test/escape-codes/page.tsx` | 293–298 |
| `/test/escape-codes` filter selects | `web-app/src/app/test/escape-codes/page.tsx` | 237–261 |
| `/test/escape-codes` control selects | `web-app/src/app/test/escape-codes/page.tsx` | 344–369 |
| `/test/terminal-stress` inline colors | `web-app/src/app/test/terminal-stress/page.tsx` | 371–390, ~400+ |
| `/test/terminal-stress` select-name | `web-app/src/app/test/terminal-stress/page.tsx` | 403–436 |
| Theme token values | `web-app/src/styles/theme.css.ts` | 64–172 |
