# Build vs. Buy: Omnibar Modal Scroll Fix

## Question
Should the fix for the Omnibar modal growing past the viewport (unreachable footer button on short screens) reach for a library/primitive, or copy the plain CSS pattern already proven in `ResumeSessionModal.css.ts` and `WorkspaceSwitchModal.css.ts`?

## What's already in the repo

- **`@radix-ui/react-dialog@^1.1.15` is already a dependency** (`web-app/package.json`), and Radix's `Dialog.Content` does not solve viewport-overflow scrolling by itself — it still needs the same `maxHeight` + `flex` CSS on the content wrapper. Adopting it would not remove the need for this CSS fix; it only changes focus-trap/accessibility plumbing, which is a separate concern from the bug being fixed.

- **A shared `Modal` primitive already exists**: `web-app/src/components/ui/Modal.tsx` + `web-app/src/components/ui/Modal.css.ts`. It already implements the exact fix pattern needed here — `maxHeight: "85vh"` + `overflowY: "auto"` on the scrollable region (`Modal.css.ts` lines ~33–34), plus `display: flex` for the footer layout (line ~55).
  - Already adopted by four other components: `ApprovalRulesPanel.tsx`, `SessionList.tsx`, `SessionDetailView.tsx`, and covered in tests (`SessionList.mobile.test.tsx`, `SessionCard.click.test.tsx`, `SessionDetail.embedded.test.tsx`).
  - `Omnibar.tsx` does **not** use it — it hand-rolls its own `role="dialog"` markup and CSS in `Omnibar.css.ts` (two `role="dialog"` instances at lines 875 and 1038), independently of `ui/Modal.tsx`.

- **`ResumeSessionModal.css.ts`** and **`WorkspaceSwitchModal.css.ts`** *also* hand-roll their own modal CSS rather than using `ui/Modal.tsx`, and both already carry the `maxHeight: "80vh"` / `display:flex; flexDirection:column` (container) + `flex:1; overflowY:"auto"` (body) pattern this task is asked to replicate in `Omnibar.css.ts`.

So there are now three (about to be four, with `Omnibar.css.ts`) independent hand-rolled implementations of the same scroll-clamp pattern, alongside one shared primitive (`ui/Modal.tsx`) that already does this correctly and is used elsewhere in the codebase.

## Verdict

**Plain CSS copy of the existing `ResumeSessionModal`/`WorkspaceSwitchModal` pattern into `Omnibar.css.ts` — Recommended for this task.**

Rationale: NFR1 scopes this fix to CSS-only changes on `.modal`/`.body` in `Omnibar.css.ts`; a library (Radix Dialog primitive swap) wouldn't even remove the need for this CSS, and migrating Omnibar onto the shared `ui/Modal.tsx` primitive is a structural component refactor (markup changes, prop wiring, portal/focus-trap behavior) that is out of scope for a targeted scroll-clamp bug fix. The three-line CSS addition is lower risk, matches an established in-repo convention exactly, and ships the fix without touching component structure.

**Follow-up worth filing (not this task):** `Omnibar.tsx`'s modal markup is a third hand-rolled dialog implementation coexisting with the shared `ui/Modal.tsx` primitive already used by `ApprovalRulesPanel`, `SessionList`, and `SessionDetailView`. A future refactor should evaluate migrating `Omnibar`, `ResumeSessionModal`, and `WorkspaceSwitchModal` onto `ui/Modal.tsx` (or onto `ui/Modal.tsx` built on top of the already-installed `@radix-ui/react-dialog`) to eliminate this duplication and get focus-trap/`aria` correctness for free — but that refactor should be scoped and reviewed separately from this scroll-clamp bug fix.
