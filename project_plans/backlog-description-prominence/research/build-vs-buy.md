# Build vs. Buy: backlog-description-prominence

**Scope**: flip `descriptionExpanded` default from `false` to `true` in
`useSectionExpandState(itemId, "description", false)`
(`web-app/src/components/backlog/BacklogItemDetail.tsx:323`), and the
matching `defaultExpanded={false}` prop on `DescriptionSection`'s
`CollapsibleSection` call (`web-app/src/components/backlog/detail/DescriptionSection.tsx:20`,
currently dead in grouped mode per `Collapsible.tsx:139-157` but kept
consistent with the driving value to avoid the dev-mode divergence warning).
This is a boolean literal change, not new code. The 4-point framework is
applied below for completeness; the expected answer for every point is
"build" (i.e. "just change the literal").

## 1. Existing OSS library/framework

**Pros of switching**: none identified.
**Cons**: would introduce a second disclosure/accordion primitive alongside
Radix, duplicate ARIA/keyboard-nav work Radix already provides (roving
tabindex across `CollapsibleGroup` siblings, per `Collapsible.tsx:6-10`),
and break the shared-`Accordion.Root` pattern every other section in
`BacklogItemDetail.tsx` relies on.
**Verdict**: No. `@radix-ui/react-accordion`, wrapped by
`web-app/src/components/ui/Collapsible.tsx`, is already the standard
disclosure primitive for this exact component tree. Reaching for a
different library for a default-value flip would be pure scope creep with
no capability gap to justify it.

## 2. SaaS/managed API

Not applicable. This is a client-side React default-prop/initial-state
change with no external data, no network call, and no service dependency —
there is nothing a SaaS/managed API could provide here.

## 3. LLM-generated implementation vs. battle-tested library

**Custom logic present**: none. The only "logic" touched is passing `true`
instead of `false` as the third argument to the already-implemented,
already-tested `useSectionExpandState` hook
(`web-app/src/lib/hooks/useSectionExpandState.ts:15-19`), which already
handles localStorage read/write, try/catch fallback, and per-item/per-section
key scoping. No new algorithm, state machine, or persistence logic is being
authored.
**Correctness risk**: near-zero. The change can't introduce a new class of
bug because it exercises the exact same code path six sibling sections
already use (`reviewing`, `pull-request`, `sessions`, etc. at
`BacklogItemDetail.tsx:314-323`) — two of which (`pull-request`, `sessions`)
already default to `true` today, proving the `true` branch of this hook is
already live and correct in production for this component.
**Verdict**: Build (flip the literal) — there is no meaningful build-vs-buy
tension because there's no algorithm to buy a substitute for.

## 4. Fork or adapt

**Existing pattern to copy**: yes, and this is the one substantive angle
here. `PullRequestSection` and `SessionsSection` are the two existing
sibling sections that already default open:
- `BacklogItemDetail.tsx:316`: `useSectionExpandState(itemId, "pull-request", true)`
- `BacklogItemDetail.tsx:319`: `useSectionExpandState(itemId, "sessions", true)`
- `PullRequestSection.tsx:31` mirrors the state through its own
  `CollapsibleSection ... defaultExpanded={true}`.

Description should adopt the identical shape: `useSectionExpandState(itemId,
"description", true)` at `BacklogItemDetail.tsx:323`, plus
`defaultExpanded={true}` on `DescriptionSection.tsx:20` to keep the
component's own prop consistent with the value threaded from the parent
(matching `PullRequestSection`'s convention, and avoiding the dev-mode
"diverges from group state" warning in `Collapsible.tsx:150-156`).
**Verdict**: Fork/adapt the existing `pull-request`/`sessions`
default-`true` pattern verbatim — no new pattern is being invented.

## Summary

All four points converge on "build," and "build" here means changing two
boolean literals (`false` → `true`) using machinery and a sibling pattern
that already exist and are already proven in this exact component. No
library, service, or custom logic decision is actually in play.
