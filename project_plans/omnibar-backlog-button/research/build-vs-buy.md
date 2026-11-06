# Build vs Buy: Omnibar "Create Backlog Item" footer button

Scope: one extra `<button>` in the `OmnibarCreationPanel` footer row. Verified in-repo:
`web-app/src/components/sessions/Omnibar.css.ts` exports `footer` (l.238), `button`, `buttonSecondary` (l.257), `buttonPrimary` (l.269);
`OmnibarCreationPanel.tsx` imports them (l.18) and the footer (l.1023-1044) already renders Cancel (`buttonClass buttonSecondary`) and Create Session (`buttonClass buttonPrimary`). The `onCreateBacklog` prop and button (l.1024-1029) already exist.

## 1. Reuse existing in-repo styles (build)
- Pros: zero deps; identical look and a11y to sibling buttons; same vanilla-extract theme tokens; no bundle growth; no jscpd/duplication risk.
- Cons: none material; the repo has no shared `<Button>` component, so buttons are class-composed inline.
- Verdict: ADOPT. `buttonClass` + `buttonSecondary`.

## 2. OSS component library (Radix, MUI, Headless UI, etc.)
- Pros: ready-made button primitives.
- Cons: new dependency for a single element; clashes with vanilla-extract styling (CSS architecture doc); inconsistent with neighbours; bundle/lockfile churn.
- Verdict: REJECT.

## 3. SaaS / hosted UI kit
- Pros: none applicable to one button.
- Cons: external runtime dependency, licensing, network/privacy, absurd for scale.
- Verdict: REJECT.

## 4. LLM-generated new component
- Pros: fast.
- Cons: would duplicate existing styles, add a new abstraction with no second user, risks jscpd/dupl gates.
- Verdict: REJECT (the change is a few JSX lines; nothing to generate beyond them).

## 5. Fork/adapt existing pattern
- Pros: the Cancel button JSX is the exact template; copy with a new `data-testid` (`omnibar-create-backlog-button`) and label.
- Cons: minor JSX repetition (below the 20-line jscpd threshold).
- Verdict: ADOPT as the mechanism for option 1.

## Decision
Reuse `footer`, `buttonClass`, `buttonSecondary`; handler is the existing `openBacklogCompose` via `onCreateBacklog`. No new dependency or style.
