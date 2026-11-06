# Validation: Help Tooltip Coverage

## Acceptance Criteria Verification

### AC 1: `Tooltip` accepts `helpSlug?` prop

**Verifiable by:** Unit test in `Tooltip.test.tsx` (or extend existing stories):
```tsx
render(<Tooltip label="Connection lost" helpSlug="tmux-integration"><span>?</span></Tooltip>)
expect(screen.getByRole("link", { name: /learn more/i })).toHaveAttribute("href", "/help?doc=tmux-integration")
```
**Gap:** No `Tooltip.test.tsx` exists today — the component is untested. A new test file must be created alongside the implementation.

### AC 2: `/help` page supports `?doc=<slug>` deep-link

**Verifiable by:** Update `web-app/src/app/help/__tests__/page.test.tsx`:
```tsx
// mock docLoader to return [{slug:"omnibar",...}]
render(<HelpPage />, { searchParams: "?doc=omnibar" })
// after loadDocs resolves, assert "omnibar" doc is selected
expect(screen.getByRole("button", { name: /Omnibar/ })).toHaveAttribute("aria-current", "page")
```
The existing test file exists and already mocks `docLoader` — this is an additive test case.

### AC 3: Every `helpSlug` ref has a corresponding doc file + DOC_FILES entry

**Verifiable by:** The optional part of `doc_files_coverage.test.ts` — grep `web-app/src` for `helpSlug="..."` patterns and assert each is in `DOC_FILES`. This can be added after AC 4's basic consistency check is in.

### AC 4: `DOC_FILES` ↔ `public/docs/` consistency enforced by CI

**Verifiable by:** `doc_files_coverage.test.ts` failing when out of sync. Confirm by temporarily adding a dummy slug to `DOC_FILES` and running `npx jest doc_files_coverage`; the test must fail.

### AC 5: New doc files for backlog, insights, notifications, tagging-rules, workflows

**Verifiable by:** `ls web-app/public/docs/` shows all 5 new `.md` files. `doc_files_coverage.test.ts` non-empty check passes. Manual review of content quality.

### AC 6: High-traffic components migrated from `title=` to `<Tooltip>`

**Verifiable by:** `grep -n 'title=' web-app/src/components/layout/Header.tsx` returns no tooltip-style usages (only `<title>` tag or non-tooltip uses). The migrated elements render `<Tooltip>` wrapping the icon buttons — confirm with E2E screenshots or manual testing.

### AC 7: No existing Tooltip callers broken

**Verifiable by:** `cd web-app && npx jest --testPathPattern=Tooltip` and all existing tests pass. The `helpSlug` prop is optional; all 6 existing `<Tooltip ...>` call sites have no `helpSlug` and will not change.

### AC 8: Coverage test runs in `make ci`

**Verifiable by:** `make ci` exits 0 with the new test passing. Or: `cd web-app && npx jest --no-coverage doc_files_coverage` passes in < 1s (pure filesystem checks, no network).

---

## Open Questions / Risks to Re-examine Before Implementation

1. **Radix Tooltip + interactive link accessibility**: The "Learn more" link inside Tooltip content is not keyboard-reachable. Is that acceptable for phase 1, or does this block the `helpSlug` approach entirely? If blocked: consider rendering the help link as a sibling icon button instead of inside the Tooltip (e.g., `ℹ` icon next to the label that links to the doc, with its own `aria-label`).

2. **`useSearchParams` Suspense requirement**: Next.js 13 App Router requires components using `useSearchParams` to be wrapped in `<Suspense>`. The help page currently has no Suspense boundary. Determine whether the existing `loading` state (managed by `useState`) is sufficient, or whether a `<Suspense fallback="Loading...">` wrapper is needed at the page level.

3. **`DOC_FILES` as an exported constant vs. inline array**: Currently `DOC_FILES` is not exported from `docLoader.ts`. The coverage test would need to either: (a) import it (requires exporting), or (b) read it from the filesystem directly and compare against the manifest. Option (b) avoids a source change but may be brittle if the doc list is ever generated. Recommendation: export `DOC_FILES` from `docLoader.ts`.

4. **Doc file minimum viable content**: Is a 100-word stub acceptable at ship time, or must each doc reach a quality bar before merging? Define this before writing C1 doc files.

5. **Link destination inside the tooltip**: Should `helpSlug` navigate to `/help?doc=<slug>` (SPA nav) or open in a new tab? In-app navigation keeps context; new-tab is conventional for "learn more" links. Current preference: in-app navigation (`<Link href={...}>` from `next/link`), since the user likely wants to stay in Stapler Squad.
