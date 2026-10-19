# Implementation Plan: Help Tooltip Coverage

## Overview

Three sequential work streams, each independently shippable:

1. **Stream A — Infrastructure** (Tooltip + deep-link): extend `Tooltip` with `helpSlug`; add `?doc=` deep-link to the help page.
2. **Stream B — Coverage gate** (CI test): add a Jest test that fails when `DOC_FILES` ≠ `public/docs/` contents, and fails when any `helpSlug` in the codebase references a missing doc.
3. **Stream C — Content + migration** (docs + `title=` sweep): add missing doc files; migrate high-traffic native `title=` usages to `<Tooltip>`.

Streams A and C can proceed in parallel once requirements are agreed; Stream B depends on the `DOC_FILES`/slug structure stabilized by A.

---

## Stream A — Infrastructure

### Task A1: Extend `Tooltip` with optional `helpSlug` prop

**File:** `web-app/src/components/ui/Tooltip.tsx`

- Add `helpSlug?: string` to the props type.
- When `helpSlug` is set, render a two-line tooltip content: the label text, then a small "Learn more →" link (`<a href={/help?doc=${helpSlug}}>`) styled as a muted underlined link.
- Keep `label` as `string` (not React node) to avoid breaking existing string-only callers.
- Add a `tooltipLink` style to `Tooltip.css.ts` (small font, muted color, underline).
- Update `Tooltip.stories.tsx` with a story showing `helpSlug`.

**Estimate:** 2h

### Task A2: Deep-link support in the help page

**File:** `web-app/src/app/help/page.tsx`

- Import `useSearchParams` from `next/navigation`.
- On load, after docs are fetched, check `searchParams.get("doc")`. If the slug is in the loaded docs list, `setSelectedSlug` to it.
- If the slug is not in the list, show the first doc (current behavior).
- Update `web-app/src/app/help/__tests__/page.test.tsx` to cover the deep-link path.

**Estimate:** 2h

### Task A3: Update `docLoader.ts` to add missing slugs

**File:** `web-app/src/lib/docs/docLoader.ts`

- Add the new slugs to `DOC_FILES`: `backlog`, `insights`, `notifications`, `tagging-rules`, `workflows`.
- (The actual `.md` files are Stream C's job; this task makes the loader aware of them so Stream B's test can verify consistency.)

**Estimate:** 30m

---

## Stream B — Coverage Gate

### Task B1: Add `doc_files_coverage.test.ts`

**File:** `web-app/src/lib/__tests__/doc_files_coverage.test.ts`

Following the pattern of `notification_docs_exist.test.ts`:

1. Read all `*.md` files from `web-app/public/docs/` via `fs.readdirSync`.
2. Assert every filename stem (slug) is in `DOC_FILES` (`docLoader.ts`'s exported constant, or a re-export of it).
3. Assert every entry in `DOC_FILES` has a corresponding file in `public/docs/`.
4. Assert every `DOC_FILES` entry file is non-empty (> 50 bytes; catches stub commits).

Optionally (lower priority): grep the `web-app/src` tree for `helpSlug="<slug>"` patterns and assert each slug is in `DOC_FILES`.

**Estimate:** 2h

---

## Stream C — Content + Migration

### Task C1: Write missing doc files

**Files:** `web-app/public/docs/{backlog,insights,notifications,tagging-rules,workflows}.md`

Each file needs:
- H1 title matching the slug
- 2–4 paragraphs explaining the feature
- Links to relevant settings pages (use relative `/settings/...` paths so they work in-app)
- Minimum viable; can be enriched later

**Estimate:** 3h (all 5 files together)

### Task C2: Migrate `Header.tsx` and `CockpitShell.tsx` to `<Tooltip>`

**Files:** `web-app/src/components/layout/Header.tsx`, `CockpitShell.tsx`

Replace native `title=` on icon buttons (create-session, notifications, keyboard-shortcuts) with `<Tooltip label="..." side="bottom">`. Keep `aria-label=` as-is (it serves screen readers; `<Tooltip>` serves sighted keyboard users). Remove `title=` (it creates a double tooltip when both are present).

**Estimate:** 1h

### Task C3: Migrate `DrawerNav.tsx` and `ConnectionIndicator.tsx` to `<Tooltip>`

**Files:** `web-app/src/components/layout/DrawerNav.tsx`, `ConnectionIndicator.tsx`

- `DrawerNav.tsx`: collapsed-state `title=` → `<Tooltip label={page.label} side="right">` wrapping the icon.
- `ConnectionIndicator.tsx`: `title={titleText}` → `<Tooltip label={titleText} side="bottom">`.

**Estimate:** 1h

### Task C4: Add `helpSlug` to key new `<Tooltip>` usages

After C1 docs are written, go back to the Tooltip calls added in C2/C3 and add `helpSlug` where a doc exists:
- Keyboard shortcuts button → `helpSlug="keyboard-shortcuts"`
- Omnibar/create-session button → `helpSlug="omnibar"`
- Connection indicator → `helpSlug="what-is-stapler-squad"` (or a new `tmux-integration` link)

**Estimate:** 30m

---

## Adversarial Review

**Q: Is the `helpSlug` approach fragile — a typo in a component creates a dead link?**  
A: Yes — that is exactly what Task B1's optional slug-existence check addresses. Even without that check, dead links are low-severity (they navigate to `/help` with an unrecognized slug and fall back to the first doc; they don't 404).

**Q: Does adding a link inside a Tooltip break Radix UI's keyboard behavior?**  
A: Radix Tooltip content is rendered in a Portal and is not keyboard-focusable by default. The "Learn more" link inside the tooltip content will not be reachable by keyboard alone. This is a known Radix limitation for interactive tooltip content. Mitigation: the link can also be exposed via the adjacent label or through a dedicated help button. For phase 1, accept this limitation; file a follow-up for a keyboard-accessible help affordance (e.g., a `?` icon next to settings labels that opens the help page).

**Q: Does the `DOC_FILES`-filesystem sync test work in CI?**  
A: Yes — the test runs in Jest with Node's `fs` module. The pattern is already proven by `notification_docs_exist.test.ts`. The working directory is `web-app/` so `path.join(process.cwd(), "../web-app/public/docs")` correctly points at the docs directory (or `path.join(__dirname, "../../public/docs")` relative to the test file).

**Q: Does `useSearchParams` work in the help page given Next.js App Router static generation?**  
A: The help page already uses `"use client"` and dynamic data fetching (`fetch` in `useEffect`). Adding `useSearchParams` is safe in a Client Component and does not trigger static-generation issues. The `Suspense` boundary requirement (Next.js 13+) may need a wrapper — check during implementation.

---

## Sequencing

```
Week 1:
  A1 (Tooltip helpSlug)  ──────────────────────┐
  C1 (write doc files)   ──────────────────────┤
                                                ├─ A3 (update DOC_FILES) ─ B1 (coverage test)
  A2 (help deep-link)    ─────────────────────-┘

Week 2:
  C2 (Header/CockpitShell migration)
  C3 (DrawerNav/ConnectionIndicator)
  C4 (add helpSlug to new usages)
```

Total estimated effort: ~12h frontend.
