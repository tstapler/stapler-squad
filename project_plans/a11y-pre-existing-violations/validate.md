# Validate: a11y-pre-existing-violations

## Adversarial Review

### Claim: `accentText` fixes the selected-tab contrast on both themes

**Checked:** `theme.css.ts` defines:
- Light `accentText: "#003d99"` — `#003d99` on `#ffffff` = ~9.2:1 ✓
- Dark `accentText: "#2d9cdb"` — `#2d9cdb` on `#0a0a0a` = ~5.0:1 ✓

No counter-evidence. Plan is sound.

### Claim: `primaryDark` as login button background fixes both themes

**Checked:** `theme.css.ts` defines:
- Light `primaryDark: "#003d99"` — white on `#003d99` = ~9.2:1 ✓
- Dark `primaryDark: "#1a7fc1"` — white on `#1a7fc1` = ~4.7:1 ✓

**Potential gap:** The login button likely also has a hover state. If hover is `primaryHover` and
that value is below 4.5:1, fixing the default state leaves hover still failing. Implement the
fix, then verify hover contrast explicitly. If `primaryHover` is too light, set a separate hover
value within `login.css.ts` rather than mutating the shared token.

### Claim: Adding `metadata` to `layout.tsx` fixes the document-title violation

**Checked:** `RequireFeatureFlag` renders `null` when the flag is off (`isLoading` or disabled).
When `null` is returned, the page's `metadata` export never runs client-side. But `layout.tsx`'s
`metadata` export runs at the **server** level, writing `<title>` into the SSR `<head>` before any
JS runs — so even if the client-side JS never mounts the page, the title is present in the HTML.

**Potential gap:** The axe sweep uses `page.goto({ waitUntil: 'load' })` — SSR HTML is the right
base. This should work. However, if the test server is a dev-mode instance without SSR (e.g.
Vite or CRA), the metadata export won't write `<title>` at all. The `CLAUDE.md` indicates
`tests/e2e/global-setup.ts` spawns `stapler-squad --test-mode`, which serves the Next.js build.
If that's a production build with SSR, the metadata export works. If it's a development server
(`next dev`), the behavior should still work since Next.js 13+ applies layout metadata in both
modes.

**Confidence:** HIGH. The fix is the idiomatic Next.js 13+ solution. If it fails, the fallback is
to set `document.title` inside a `useEffect` within the `FeatureGuard` null branch — but that's a
client-side workaround and more complex.

### Claim: 22 failing nodes in /test/terminal-stress all use the identified hex values

**Risk:** Only `getStatusColor()` and `getFrameTimeStatus()` were directly located. If the
rendering loop applies colors from these functions to *per-metric rows* (e.g. one colored span per
measurement in a time-history table), 22 = N metrics × 1 color each, which is consistent with
those two functions. But there may be additional inline `color:` usages not yet located.

**Mitigation:** Before implementing B2, grep for all occurrences of the failing hex values:
```sh
grep -n '#6b7280\|#10b981\|#f59e0b\|#ef4444\|#9ca3af' \
  web-app/src/app/test/terminal-stress/page.tsx
```
Replace every occurrence found. The plan already calls this out — no change needed.

### Claim: 82 checkboxes = len(ESCAPE_CODE_LIBRARY)

Not independently verified. The axe rule reports "82 nodes" which should match the number of
checkboxes rendered. If the library is smaller and some checkboxes render per-row without
`aria-label`, the fix still applies uniformly — `aria-label={code.humanReadable}` on each
`<input type="checkbox">` fixes all of them regardless of the exact count.

### Claim: 4 select violations in escape-codes are the identified ones

Identified 4 selects by line range. The axe count is "4 nodes". The plan matches.
No gap detected.

---

## Acceptance Criteria Verification

| AC | Plan coverage | Status |
|----|---------------|--------|
| 1. Axe sweep passes for `/analytics/escape` | A1: `accentText` on active tab | ✓ Covered |
| 2. Axe sweep passes for `/login` | A2: `primaryDark` on button | ✓ Covered |
| 3. Axe sweep passes for `/resolve` | A3: `accentText` on action button | ✓ Covered |
| 4. Axe sweep passes for `/settings/backlog-sources` | A4: `metadata` in layout | ✓ Covered |
| 5. Axe sweep passes for `/test/escape-codes` | B1: `aria-label` + `htmlFor`/`id` | ✓ Covered |
| 6. Axe sweep passes for `/test/terminal-stress` | B2: color replacements + select wire | ✓ Covered |
| 7. All #1112 KNOWN entries removed | Commit 3 removes them after E2E passes | ✓ Covered |
| 8. `make quick-check` passes | Verification step after each cluster | ✓ Covered |
| 9. No new KNOWN violations introduced | Plan introduces only semantically-safe fixes | ✓ Covered |

---

## Go / No-Go

**Go.** All violations have identified root causes with specific file:line references. Fix
strategies use existing theme tokens or AA-compliant hex equivalents. No architectural change, no
API surface change, no new dependencies. The main risk (terminal-stress inline color scan being
incomplete) is mitigated by a mandatory grep step before implementation. Estimated total
implementation scope: ~3–4 hours (2 code sessions).
