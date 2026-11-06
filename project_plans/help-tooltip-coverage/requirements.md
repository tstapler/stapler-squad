# Requirements: Help Tooltip Coverage

## Item Summary

**Title:** Improve help coverage and help links in Stapler Squad tooltips  
**Item ID:** 18148d93-6c9c-4f5a-8d32-0092dc9e6fe9  
**Category:** Feature  

## Problem Statement

Stapler Squad has a `/help` page backed by 6 markdown docs in `web-app/public/docs/`, and a `Tooltip` component built on Radix UI. However:

1. **Most tooltips use native `title=`** — 135 production `.tsx` files use bare HTML `title=` attributes (which have poor accessibility and browser-inconsistent rendering) vs. only 6 files using the accessible `<Tooltip>` component.
2. **The `Tooltip` component has no help-link support** — it only accepts a `label: string`; there is no way to include a "Learn more → /help#slug" link inside a tooltip.
3. **Help doc coverage is narrow** — only 6 topics are registered in `docLoader.ts`'s `DOC_FILES` list: `what-is-stapler-squad`, `session-types`, `omnibar`, `keyboard-shortcuts`, `configuration`, `tmux-integration`. Major surfaces (backlog, insights, workflows, settings stages/remotes/LLM backends, tagging rules, notifications) have no in-app help doc.
4. **No regression check** — there is no test that verifies the `DOC_FILES` list stays consistent with files in `public/docs/`, or that wires tooltip help-link slugs to valid docs (compare: `notification_docs_exist.test.ts` does this for notification docs).

## Scope

The three parts of work are:
1. **Audit** — enumerate which surfaces lack help text or valid help links.
2. **Fix** — add/correct help content using the existing mechanism (extend `Tooltip` with an optional `helpSlug` prop; add missing `public/docs/*.md` files; replace key native `title=` usages with `<Tooltip>`).
3. **Gate** — add a coverage check that fails CI when the `DOC_FILES` list diverges from `public/docs/` contents, and optionally when a component opts into help-link coverage but the slug it references doesn't exist.

## Acceptance Criteria

1. The `Tooltip` component accepts an optional `helpSlug` prop; when provided, the tooltip renders a "Learn more" link that navigates to `/help` with the slug pre-selected (e.g., `?doc=<slug>` query param or anchor).
2. The `/help` page supports deep-linking: when `?doc=<slug>` is present in the URL on page load, the page pre-selects that doc in the sidebar and scrolls to it.
3. Every slug referenced by any `helpSlug` prop in the codebase has a corresponding file in `web-app/public/docs/<slug>.md` and an entry in `docLoader.ts`'s `DOC_FILES`.
4. `docLoader.ts`'s `DOC_FILES` list is consistent with the actual files present in `web-app/public/docs/`: a CI test (following the `notification_docs_exist.test.ts` pattern) fails if a slug is listed but the file is missing, or a file exists but is not listed.
5. New `public/docs/*.md` files exist for at least the following topics: `backlog`, `insights`, `tagging-rules`, `notifications`, `workflows`.
6. At least the following high-traffic components replace their native `title=` attribute with `<Tooltip>` (with or without `helpSlug`): `Header.tsx` (keyboard shortcuts, notifications, create-session buttons), `ConnectionIndicator.tsx`, `DrawerNav.tsx` (collapsed-state icon labels), `StatusBadge.tsx`, `SessionCard.tsx` (status chips).
7. No existing Tooltip story or test is broken by the `helpSlug` addition (the prop is optional; existing callers need zero changes).
8. The coverage CI test runs as part of `make ci` (or `cd web-app && npx jest`).

## Out of Scope

- Replacing every `title=` in the codebase — only prioritized high-traffic surfaces listed in AC 6.
- Rewriting the help page UI (sidebar/search stays as-is; only deep-link support is added).
- Adding `helpSlug` to every existing `<Tooltip>` call — only new/updated ones need it.
