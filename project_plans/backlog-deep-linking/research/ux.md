# UX Research: backlog-deep-linking

Agent 5 (UX Research), SDD Phase 2. Scope: comparable copy-link patterns, user mental
models, accessibility requirements, error/edge-case UX, and job-to-be-done — plus where
this feature attaches in the existing web UI.

## 0. Existing implementation to build on (not greenfield)

A "Copy Link" button **already exists** and is the natural home for this feature —
this is an *upgrade* of existing UX, not new UI:

- [`web-app/src/components/backlog/BacklogItemDetail.tsx:1257-1277`](../../../web-app/src/components/backlog/BacklogItemDetail.tsx) —
  the sticky header's `idRow` renders the raw ID (`item.id`) plus two buttons,
  `Copy ID` and `Copy Link`, side by side.
- Current link value (line 1271): `` `${window.location.origin}/backlog?item=${item.id}` ``
  — exactly the baseline the requirements doc calls out as insufficient (host-bound,
  opaque UUID, no type info). This is the line the `ssq://`-generation logic replaces.
- Confirmation pattern (lines 122, 161-168, 1266, 1275): a `copiedField: "id" | "link" | null`
  state var, set via `handleCopy(field, value)` which calls the shared
  `copyToClipboard()` helper ([`web-app/src/lib/clipboard.ts`](../../../web-app/src/lib/clipboard.ts),
  `navigator.clipboard.writeText` with an `execCommand` fallback for plain-HTTP LAN
  access) and flips the button's own label to `"✓ Copied"` for 1.5s
  (`copyTimerRef`, cleared on unmount and re-triggered on rapid re-click — comment at
  line 151 explicitly calls out the two-button race this guards against).
- A **separate**, toast-based confirmation pattern also exists in the same file
  (`useNotifications().showActionToast`, e.g. line 498 "Session deleted.") used for
  destructive/state-changing actions elsewhere in the panel. Copy-link intentionally
  does *not* use this — see §1 for why inline-label is the better fit here.
- `BacklogItemCard.tsx` (the board/list card, as opposed to the detail panel) has no
  copy-link affordance today — only `onAction`/`onClick` handlers for status-changing
  actions. Whether to add a copy-link affordance to the card (not just the detail
  panel) is a Phase 3 scope call; the card would need a way to trigger copy without
  opening the detail panel (icon button, or a "..." overflow menu) since it's rendered
  in dense list/board layouts.

**Implication for Phase 3 planning**: the diff is small and localized. Change the
`onClick` at line 1271 to call a new link-builder (`ssq://<host>/backlog/v1/<id>`, or
its `https://` equivalent) instead of building the query-string URL inline, and reuse
`handleCopy`/`copiedField`/`copyToClipboard` as-is. No new component, no new state
shape, no toast-vs-inline-label decision to make — that's already been made and
matches recommendation (§1) independently.

## 1. Comparable UX patterns: "copy link to this item"

General knowledge of these products' current interaction design (not independently
re-verified against a live session for this research pass — flag as informed
priors, not fresh screenshots):

| Product | Placement | Trigger | Confirmation | Notes |
|---|---|---|---|---|
| **Linear** | Icon button (link icon) in the issue detail header, and in the right-click/`Cmd+K` context menu on an issue row | Click, or keyboard shortcut (`Cmd+Shift+C` copies issue ID; link-copy is in the menu) | Small toast, bottom-of-screen, auto-dismisses (~2s) | Linear's identifier (`ENG-123`) is itself copyable separately from the full URL — mirrors this repo's existing "Copy ID" vs "Copy Link" split |
| **Notion** | "Copy link" in the `•••` page menu, and on hover-triggered per-block controls | Click | Inline toast top-center, "Copied to clipboard" | Notion's link always resolves through Notion's own domain, not a raw scheme — closest analog to the `https://`-fallback requirement here |
| **GitHub Issues/PRs** | Small link/clipboard icon that appears on hover next to the issue/PR title, and "Copy permalink" on line-level code selections | Click | Icon swaps to a checkmark for ~1-2s, no toast | This is the pattern closest to what's already implemented in `BacklogItemDetail.tsx` (inline state swap, no toast) — validates the existing choice |
| **Jira** | "Copy link" in the "•••" (more actions) menu on the issue view | Click | Toast, "Link copied to clipboard" | Jira's URL always includes the human-readable key (`PROJ-123`) merely as a path segment, no separate short ID exposed |

**What works well, and why it matters here:**

1. **Icon-swap/inline-label confirmation for a single, low-stakes, immediately
   reversible action (copy) reads faster than a toast** — the user's eyes are
   already on the button they just clicked; a toast forces a saccade to wherever it
   renders. Toasts earn their cost for actions with a *side effect* worth confirming
   away from the trigger point (item deleted, moved, sent) — GitHub's and this
   repo's existing choice to skip a toast for copy is well-supported by comparables.
   **Recommendation: keep the existing inline-label pattern for backlog item
   copy-link; do not add a toast.**
2. **The short/typed identifier and the full link are treated as two different
   affordances** in every comparable (Linear ID vs URL, Jira key vs URL) — this
   repo's existing "Copy ID" / "Copy Link" split already matches that model. The
   type-prefixed ULID (`bl_01J...`) actually strengthens this: "Copy ID" now copies
   something that's self-describing on its own (an unprefixed UUID pasted into Slack
   with no context reads as noise; `bl_01J...` reads as "a backlog item").
3. **No comparable product buries copy-link inside a rarely-opened settings/overflow
   surface** — it's always reachable in ≤1 click from the item's primary view. The
   existing placement (visible unconditionally in the sticky header, not inside the
   "•••"-style overflow if one exists) is correct and should stay above the fold.
4. **None of the comparables need a cross-host fallback** — this is genuinely novel
   territory (see §4); Slack/Notion/Linear links always resolve on a server the
   product itself controls. The nearest analog is a *dead link* / *404* experience,
   not a *host mismatch* one — see the message pattern recommended in §4.

## 2. User mental models and expectations

**On clicking "Copy Link":**
- Expect literally nothing to change on screen except the button/icon flipping to a
  confirmation for ~1-2 seconds — no modal, no page navigation, no loading state.
  Copy is expected to be synchronous and instant; if link generation ever needs a
  network round-trip (e.g. resolving current hostname or peer registry state) that
  round-trip must be invisible — resolve it locally/from already-loaded state, don't
  spinner-gate the copy button.
- Expect the copied value to be a URL, not raw JSON or an internal ID with no scheme
  — pasting into Slack/a browser address bar should "just work" without the user
  editing it first.
- Expect the action to be silently repeatable — clicking twice in a row is not an
  error state, and should not produce two different values or a second toast/modal.

**On later opening that link (this is the more novel half — no existing feature to
anchor on):**
- **Same-host case**: expect the exact behavior of clicking a same-origin link
  anywhere else on the web — the app opens/focuses (if `ssq://` triggers an
  already-open tab or launches the desktop-registered handler) and scrolls/navigates
  directly to the item, no extra clicks, no "are you sure" interstitial. This is the
  bar the requirements doc sets (§Success Metrics) and it's the right bar — any added
  friction (a "resolving..." spinner, a confirmation click) will read as broken
  compared to every other link-opening experience the user has.
- **Cross-host case**: this is where the mental model needs deliberate design, because
  there's no widely-shared precedent (most SaaS products don't have this problem —
  everything lives on one server). The two nearest available mental models a user
  will unconsciously reach for:
  - *"Broken link" model* (404 page) — implies nothing more can be done, dead end.
    **Wrong model to evoke** — the item isn't gone, just elsewhere.
  - *"Wrong account/workspace" model* (e.g. a Slack link opened while signed into the
    wrong workspace, which prompts "open in workspace X instead?") — **this is the
    right model to evoke.** The fallback message should name the actual host
    (`bl_01J... lives on "myhost" — this instance is "otherhost"`) and, if a
    peer URL is knowable, offer it as a clickable link/button, mirroring how Slack
    handles the cross-workspace case rather than how a browser handles a 404.
- **Old-format (UUID) link opened after the ID scheme changes**: expect it to work
  identically to a new-format link — the requirements doc is explicit that backward
  compatibility is permanent, not deprecated-with-a-warning. Any "this is an old-style
  link" messaging would violate the mental model that a link, once shared, keeps
  working (this is exactly the promise a permalink makes — Jira/GitHub/Linear links
  are permanent by design, and breaking that expectation even with a soft warning
  erodes trust in every link the user has already shared).

## 3. Accessibility requirements

**Copy-link button:**
- Real `<button type="button">`, not a `<div onClick>` — already satisfied by the
  existing implementation (`BacklogItemDetail.tsx:1268-1276`).
- `aria-label` describing the action's outcome, not just its icon — existing
  `aria-label="Copy shareable link"` is correct; if the button is ever reduced to an
  icon-only affordance (e.g. added to `BacklogItemCard`'s denser layout per §0), the
  `aria-label` becomes mandatory rather than a decoration, since there's no visible
  text fallback.
- **State-change announcement**: the "✓ Copied" label swap is a visual-only signal
  today. A screen-reader user tabbed to the button and activating it via keyboard
  gets no equivalent audible confirmation unless the label change is inside (or
  adjacent to) an `aria-live="polite"` region, or the button's accessible name itself
  updates (which it does, since `aria-label` is static but the visible text inside
  changes — screen readers announce a button's *accessible name*, and if that's
  driven by the static `aria-label` rather than the dynamic inner text, the "✓
  Copied" change is silently dropped for AT users). **Concrete fix needed in Phase
  3**: either make the `aria-label` itself dynamic (`"Copy shareable link"` →
  `"Link copied"`) for the ~1.5s confirmation window, or add a visually-hidden
  `aria-live="polite"` span announcing "Link copied to clipboard" — the latter is
  more robust since it doesn't depend on re-reading the whole button.
- Focus must remain on the button after click (no focus loss/reset) — already true
  by construction since no DOM removal/re-mount happens on click.
- Keyboard: reachable via standard `Tab` order (no custom `tabindex` needed, it's a
  real `<button>`), activatable via `Enter`/`Space` for free.

**"Item lives on another host" fallback UI:**
- Must be announced to AT on appearance without requiring focus to already be there
  — this is the WCAG 4.1.3 (Status Messages) case: if rendered as an inline banner
  replacing content, wrap it in `role="status"` (polite) rather than `role="alert"`
  (assertive/interrupting) — it's informational, not a failure the user caused, and
  an assertive interrupt would be jarring during passive navigation.
- If offering a "open in host X" action, that must be a real, keyboard-operable
  `<button>` or `<a href>`, with the target hostname in the accessible name itself
  (`aria-label="Open this item on myhost"`) rather than relying on adjacent visual
  text a screen reader user might not associate with the control.
- Color must not be the only signal distinguishing this from a normal loaded state —
  pair any warning color with an icon + text, consistent with existing patterns in
  this codebase (`InlineError.tsx`, `TriageErrorBanner.tsx` already establish this
  convention for other non-fatal-but-notable states — reuse one of these components'
  structure rather than inventing a third banner pattern).
- Contrast: WCAG AA (4.5:1 for body text, 3:1 for large text/icons) for whatever
  banner styling is chosen — check against this repo's existing design tokens rather
  than picking new colors (see `.claude/docs/css-architecture.md` for the
  vanilla-extract token setup).

## 4. Error states and edge cases

| Case | Recommended UX | Rationale |
|---|---|---|
| Link points to a **deleted/archived** item on the current host | Distinct message from "wrong host" — e.g. "This backlog item no longer exists" (or "...has been archived," if archived items are soft-deleted and could theoretically be restored) — do not reuse the cross-host message, which would misleadingly suggest the item is just elsewhere | Conflating "gone" with "elsewhere" sends the user on a fruitless hunt across other hosts |
| Link's hostname is **unreachable** (peer not registered, or registered but offline) | The requirements doc's own fallback: "this item lives on host X, which isn't reachable right now" — name the host explicitly, and if the peer registry has *any* last-known info (URL, last-seen time), surface it ("last seen 2 hours ago") rather than a bare unreachable state | Matches the "wrong workspace" mental model from §2; a bare "unreachable" with no host name is a dead end indistinguishable from a 404 |
| Link is **malformed** (bad scheme, missing segments, garbage ID) | Generic "This link isn't valid" state, logged per the Observability Requirements section of requirements.md, but *not* surfaced as a stack trace or raw parse error | A malformed link is most often a paste/truncation accident (e.g. only half a Slack-hyperlinked URL got copied) — the recovery action for the user is "go back and re-copy the link," not to interpret an error code |
| **Old-format (UUID) link** opened after the ID scheme change | Must resolve exactly like a native new-format link — no warning banner, no "legacy" labeling | Permanence promise (§2); the requirements doc is explicit that old IDs keep working "indefinitely," and any special-casing in the UI beyond invisible routing logic would visibly break that promise |
| **New-format (ULID) link** pasted into a build/instance that predates the ID-format change (i.e. an older deployed binary that doesn't know the `bl_` prefix) | Out of this research's direct scope, but worth flagging to Phase 3: if `version` in `ssq://host/type/version/id` is meant to let a resolver reject/handle unknown formats gracefully, this is the concrete case that justifies it — an old binary getting a not-yet-invented `v2` should fail with a version-mismatch message, not a raw parse error | Ties directly to the open "what does `version` actually version" question flagged in requirements.md's Rabbit Holes |
| User has **JS clipboard permission denied** (rare, some locked-down browser configs) | Already handled by the existing `copyToClipboard` fallback chain (`execCommand`) — if *both* fail, `handleCopy`'s `ok` check already no-ops rather than showing a false "✓ Copied" (confirmed in `clipboard.ts` and the `if (!ok || !mountedRef.current) return;` guard at `BacklogItemDetail.tsx:163`) | No new work needed, but worth stating as already-covered so Phase 3 doesn't reinvent it — however it does mean a *true* failure is currently silent (button just doesn't change) with no error state at all; consider whether a failed-copy state deserves at least a console log or the existing InlineError pattern, since a silent no-op looks identical to "I forgot to click" |

## 5. Job-to-be-done

**Functional job**: "When I find something worth pointing at later — a backlog item
mid-triage, one blocked on a decision, one I want a teammate/my-other-machine's
Claude session to pick up — I want to hand off a single, resilient pointer to it,
without either party needing to already have the right browser tab open or
remember which of my machines the item actually lives on."

**Emotional job**: relief from the low-grade anxiety of *fragile* references — the
current `?item=<uuid>` link is silently wrong the moment it's opened on the wrong
host, and a wrong-but-silent result (wrong item, or nothing) is worse than an
outright error because it erodes trust that *any* copied link will work next time.
A link that either resolves correctly or fails loudly and specifically ("it's on
host X") converts "I hope this still works" into "I know exactly what will happen
when I click this."

**Social job** (for Tyler specifically, solo/small-team across multiple machines):
the link functions as a **coordination artifact between his own past/future
selves and machines**, not between distinct people in the common SaaS sense — the
"share" is often "message this to myself in Slack from my laptop, open it later on
my desktop." That reframes some of the comparable-product patterns: enterprise
tools (Jira, Linear) optimize sharing for *other humans* reading it cold, but here
the primary consumer of a pasted link is often a **future automated agent session**
(a Claude session on another machine resuming work) as much as a human — which
argues for the link being maximally self-describing and machine-parseable
(type-prefixed ID, explicit host, explicit version) even more than a typical
human-facing product would need, since an agent resolving the link has no
surrounding conversational context to disambiguate an opaque UUID the way a human
teammate might.

## Summary of concrete recommendations for Phase 3

1. Attach "generate `ssq://` link" to the existing `handleCopy("link", ...)` call at
   [`BacklogItemDetail.tsx:1271`](../../../web-app/src/components/backlog/BacklogItemDetail.tsx#L1271) — replace the URL-builder, keep the state/confirmation machinery as-is.
2. Keep the inline-label confirmation (no toast) — matches the strongest comparable
   (GitHub) and is already implemented.
3. Fix the accessibility gap in the *existing* copy-confirmation (static `aria-label`
   not reflecting the "Copied" state to screen readers) as part of this work, not as
   a separate follow-up — it's a pre-existing bug this project's UI change touches
   directly.
4. Model the cross-host fallback on "wrong workspace," not "404" — name the host,
   offer a direct link/action when a peer URL is known, use `role="status"` not
   `role="alert"`, and reuse the existing `InlineError`/`TriageErrorBanner` visual
   pattern rather than inventing a new banner style.
5. Never let old-format (UUID) links look or behave differently from new-format
   (ULID) links in the UI — the backward-compatibility promise is a UX requirement,
   not just a routing requirement.
6. Treat the link's self-describing format (type prefix, explicit host, explicit
   version) as serving an agent-resolving-a-link use case as much as a human one —
   this should raise, not lower, the bar for how unambiguous the format needs to be.
