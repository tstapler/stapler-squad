# UX Research: cross-host-claim-dedup

Agent 5 (UX Research), SDD Phase 2. Scope: comparable dedup/conflict UX patterns
already in this codebase, user mental models, accessibility, error/edge-case UX,
and jobs-to-be-done for surfacing "another host already claimed this."

## 0. Key finding up front

There is **no existing UI-facing "duplicate detector" to mirror** for this feature.
The requirements doc's phrase "mirroring the existing `report_duplicate` MCP flow"
describes a backend/agent tool, not a human-facing UI pattern — see §1. The closest
genuine UI precedent for the cross-host half of this problem is
`DeepLinkErrorBanner.tsx`, built for a different feature (backlog deep linking) but
solving an almost identical problem: telling a user "this thing lives on another
host, which may or may not be reachable right now." That component's design
rationale (`project_plans/backlog-deep-linking/design/ux.md`, referenced in its own
header comment) should be treated as binding prior art, not just inspiration —
reusing its `role="alert"` vs `role="status"` split and its "wrong workspace," not
"broken link," framing keeps this feature visually and behaviorally consistent with
a UI the user will already have seen.

## 1. Comparable patterns already in this codebase

### `report_duplicate` MCP flow (backend, not UI)

[`server/mcp/tools_backlog.go:2170-2450`](../../../server/mcp/tools_backlog.go) is an
MCP tool an *agent* calls, not a UI a human opens. It has two modes, gated on the
item's current status:

- **Claimed item** (`in_progress`/`pr_pending`): writes `duplicate_ref=<url>
  reason=<text>` into `VerificationNotes` and transitions the item to `review`
  ([`tools_backlog.go:2245-2321`](../../../server/mcp/tools_backlog.go)) — it does
  **not** silently resolve anything. `session/review_gate.go:150`'s
  `parseDuplicateRef` reads that marker back out so the review gate treats an
  empty-diff session ending in a duplicate claim as a legitimate FAIL exemption
  rather than "session did nothing" ([`session/backlog_review.go:271-272`](../../../session/backlog_review.go)).
- **Unclaimed item** (`idea`/`refining`/`ready`/`queued`, no session link at all —
  `unclaimedDuplicateSourceStatuses`,
  [`tools_backlog.go:266-271`](../../../server/mcp/tools_backlog.go)): archives the
  item directly, no human step, because nobody has invested claimed work in it yet.

Both paths verify `duplicate_ref` resolves to a real GitHub PR/issue/commit
*before* mutating anything (`resolveDuplicateRef`,
[`tools_backlog.go:2341`](../../../server/mcp/tools_backlog.go)) — the codebase's
consistent stance that a duplicate claim is evidence-gated, not vibes-gated.

**The human-visible surface for this today is the standard review UI** —
`GateVerdictBox.tsx`'s Approve/Reopen/Override/Skip-Gate actions
([`web-app/src/components/backlog/GateVerdictBox.tsx:29-38`](../../../web-app/src/components/backlog/GateVerdictBox.tsx)) —
not a dedicated "duplicate" badge or banner. A human reviewing a
`duplicate_ref`-flagged item sees it exactly like any other review-gate item, with
the duplicate reasoning readable in the verification notes text. There is no
today-shipped equivalent of "Omnibar duplicate-detector entry" — Omnibar's
`detector.ts`/`dispatch.ts` registry ([`web-app/src/lib/omnibar/detector.ts`](../../../web-app/src/lib/omnibar/detector.ts))
detects *input shapes* typed into the omnibar (GitHub PR/branch URLs, commands,
aliases) to drive session creation, an unrelated concept to flagging an
already-claimed backlog item.

### `BacklogItemDetail.tsx`'s "Copy Link" — where a claim-conflict banner would sit

[`web-app/src/components/backlog/BacklogItemDetail.tsx:1468-1473`](../../../web-app/src/components/backlog/BacklogItemDetail.tsx):
the sticky header renders `Copy ID` and `Copy Link` buttons side by side; Copy Link
currently builds `ssq://${window.location.host}/backlog/v1/${item.publicId ||
item.id}` and flips its own label to "✓ Copied" for 1.5s via a `copiedField` state
var (no toast). This exact affordance, and its interaction/confirmation pattern,
was already researched in depth for the deep-linking project — see
[`project_plans/backlog-deep-linking/research/ux.md:43-79`](../../../project_plans/backlog-deep-linking/research/ux.md)
for the full comparable-products table (Linear/Notion/GitHub/Jira). Its conclusions
carry over directly: keep inline-label confirmation for copy actions, don't add a
toast, and don't bury the link affordance behind an overflow menu. A claim-conflict
banner sits naturally in the same sticky header region, above or adjacent to this
row, since that's where a user's eye already goes to get a shareable reference for
the item.

### `DeepLinkErrorBanner.tsx` — the closest real precedent for cross-host state

[`web-app/src/components/DeepLinkErrorBanner.tsx`](../../../web-app/src/components/DeepLinkErrorBanner.tsx)
already solves "this backlog item's canonical location is a different host" for the
deep-link-resolution case, and its two cross-host reasons map almost one-to-one onto
this project's edge cases:

- `"unreachable"` — host previously seen via gossip, not currently reachable; shows
  `This item lives on "<host>"` + `"<host>" isn't reachable right now. Last seen
  <time>.`, with `role="status"`/`aria-live="polite"` (not alert — not a failure of
  *this* link) and optional "Retry" / "Copy host address" actions.
- `"not-registered"` — host never seen by this instance's gossip registry at all;
  same `role="status"` treatment, different body copy.
- Genuine dead ends (`deleted`, `archived`, `malformed`, `version-mismatch`) use
  `role="alert"`/`aria-live="assertive"` instead — the component's own header
  comment states the rule: alert for dead ends *of this link*, status for
  cross-host cases that aren't failures, just "elsewhere."

A cross-host claim-conflict banner is a **third cross-host case**, structurally
identical to `unreachable`/`not-registered`: "someone else has this, and here's
where." It should be `role="status"`, reuse the hostname-naming rule (never say "an
instance"/"elsewhere" — name the actual host), and reuse the
`advertisedAddress`/`onCopyHostAddress` pattern for offering a deep link back to the
owning host/item once one is known.

### `SystemBanner`/`SystemBannerStack` — generic reusable banner if this needs to be app-wide

[`web-app/src/components/ui/SystemBanner.tsx`](../../../web-app/src/components/ui/SystemBanner.tsx)
is the existing generic "severity-colored bar with message + actions + dismiss" used
for whole-app system-health signals (its own doc comment names
`TmuxVersionMismatchBanner` as first consumer, and explicitly excludes per-item/list
banners like `MemoryPressureCallout` from being forced through it). If the "Review
Queue banner" option in the requirements' open question means a single persistent
app-level banner ("N items may be claimed elsewhere"), `SystemBanner` with
`severity="warning"` and its built-in analytics tracking
(`system_banner_action`/`system_banner_dismissed`) is the right vehicle. If instead
it means a per-item inline flag inside `BacklogItemDetail`/`BacklogBoard`, that's a
`DeepLinkErrorBanner`-style component, not `SystemBanner` — these are not
interchangeable; picking wrong means rebuilding the a11y/analytics wiring from
scratch. This is a Phase 3 design decision, not resolved by this research.

### `WorkspacePeersPanel` — a same-host, not cross-host, precedent

[`web-app/src/components/sessions/WorkspacePeersPanel.tsx:63-71`](../../../web-app/src/components/sessions/WorkspacePeersPanel.tsx)
lists other sessions sharing the *same working directory on the same host*
(`session.activeDir`), derived live from Redux with no extra RPC. It is **not** a
cross-host precedent despite the "peers" naming — it confirms that today's only
shipped "is someone else already working on this?" signal is host-local. This
reinforces the requirements doc's framing that cross-host claim visibility is
genuinely new territory, not an existing feature being extended.

## 2. User mental models

The operator here is a single person (Tyler) running stapler-squad across several
of his own machines — not a multi-tenant SaaS product with strangers colliding.
That changes the right mental model versus, say, Slack's "wrong workspace" prompt
(the nearest analog cited in the deep-linking research): the user isn't a stranger
who opened someone else's link, they're the same person whose *other machine*
already picked something up. The expectation is closer to **multi-device sync
conflict** (e.g. opening a doc on a second laptop that a first laptop already has
open) than to a multi-user permission conflict.

Given that framing and the codebase's existing evidence-gated, human-reviewed
pattern for `report_duplicate` (§1 — never a silent auto-resolution once real work
is claimed):

- **Hard block on import is the wrong default.** The existing `report_duplicate`
  design never silently kills claimed work without a review step, and gossip state
  can be stale (a peer host that gossiped a claim 10 minutes ago may have since
  crashed, been reset, or abandoned the item). A hard block on stale information
  would block real work with no recourse.
- **A warning the operator can override, mirroring `GateVerdictBox`'s
  Approve/Reopen/**Override**/Skip-Gate actions
  ([`GateVerdictBox.tsx:29-38`](../../../web-app/src/components/backlog/GateVerdictBox.tsx)),
  is the right default.** "Override" already exists in this codebase as the
  established UX affordance for "the system flagged a concern, a human's judgment
  wins" — reuse that verb and that action, not a new one.
- **"Go look at this instead" (redirect) is the primary value, not the block
  itself.** The operator's actual want is "don't make me discover the duplicate PR
  myself after both hosts have already burned an autonomous session on it" — a deep
  link straight to the owning host's item is the payoff, independent of whether
  import is blocked or merely flagged.
- Expect the check to be **fast and non-blocking to the eye**, matching the
  deep-linking research's point (§2 of that doc) that any network round-trip for
  link/status resolution must not spinner-gate a fast interaction. `ImportGitHubIssue`
  already does a synchronous local DB check
  ([`GetBacklogItemByExternalURL`](../../../server/services/backlog_service_sync.go)) —
  a cross-host check that adds perceptible latency to import, especially when
  running against multiple queued issues in a batch, will read as a regression.

## 3. Accessibility requirements

No repo-wide a11y doc was found beyond conventions embedded directly in existing
components' code comments (no dedicated `docs/reference/`-level a11y guide, and
`docs/reference/tag-organization.md` doesn't mention banners/badges at all) — the
binding "spec" here is the pattern already shipped in `DeepLinkErrorBanner.tsx` and
`SystemBanner.tsx`, both of which the `ui-web-design-guidelines` skill's WCAG/ARIA
checks would validate against. Reuse, don't reinvent:

- **Assertiveness matches consequence, not visual weight**: `role="status"` +
  `aria-live="polite"` for "flagged, not blocked" (mirrors `unreachable`/
  `not-registered` in `DeepLinkErrorBanner`); reserve `role="alert"` +
  `aria-live="assertive"` for a genuine hard-stop state, if the design ever adds one
  (e.g. import outright refused because the peer confirmed a PR already merged).
- **Name the host, never say "elsewhere"** — `DeepLinkErrorBanner`'s own rationale
  comment (line 41-42) calls this out as an explicit acceptance criterion from its
  design doc; carry the same rule here for whichever host claimed the item.
- **Icons are `aria-hidden="true"` with the meaning carried in text**, not
  icon-only — both `DeepLinkErrorBanner` (`✕`/`ℹ`) and `SystemBanner` (`icon` prop
  rendered alongside a text `message`) follow this.
- **Action buttons are real `<button type="button">` elements** with visible text
  labels (`Retry`, `Copy host address`, `Go to backlog board` in
  `DeepLinkErrorBanner`; arbitrary `label`s in `SystemBanner`'s `actions` prop) —
  never a bare `<div onClick>`.
- **Dynamic state changes need an accessible-name update or a live region** — the
  deep-linking research (§3 of that doc,
  [`project_plans/backlog-deep-linking/research/ux.md:125-150`](../../../project_plans/backlog-deep-linking/research/ux.md))
  flagged a real, still-open gap in the existing "Copy Link" button: a static
  `aria-label` means the "✓ Copied" visual swap is silently dropped for
  screen-reader users. Do not repeat that mistake in whatever "Copy deep link back
  to owning host" affordance this feature adds to a conflict banner — either make
  the label dynamic or add a visually-hidden `aria-live="polite"` announcement.
- **Keyboard**: standard tab order, no custom `tabindex`, `Enter`/`Space` activation
  — free by construction if built from real `<button>`/`<a>` elements as above.
- **Dismiss controls need `aria-label="Dismiss"`** — `SystemBanner`'s dismiss button
  already establishes this exact pattern
  ([`SystemBanner.tsx:129-142`](../../../web-app/src/components/ui/SystemBanner.tsx));
  reuse verbatim if the conflict surface is dismissible per-item.

## 4. Error states and edge cases

| Case | Recommendation | Precedent |
|---|---|---|
| Claim check times out / peer unreachable | **Proceed optimistically, but say so.** Import completes; the item shows a `role="status"` banner: "Couldn't confirm this isn't claimed elsewhere — `<peer-host>` didn't respond." Never silently swallow the failure as "no conflict found," since that's indistinguishable from a real clean check and defeats the feature's purpose. | `DeepLinkErrorBanner`'s `"unreachable"` case already has this exact copy shape (`"<host>" isn't reachable right now`) and its non-alert severity. |
| Deep link in the claim record points to a now-offline host | Same component, same case — `DeepLinkErrorBanner`'s `unreachable`/`not-registered` reasons were built for exactly this. If `#475`'s per-instance identifier is known but the host was never gossiped to this instance, that's `not-registered`; if it was seen before but the gossip entry expired/host went dark, that's `unreachable` (with `lastSeenAt`). | Same file. |
| Claim conflict resolved by a human | Reuse `GateVerdictBox`'s existing action vocabulary: **Override** (proceed anyway, log why) vs. an action to defer to the other host (close/archive this side, redirect). Do not invent a fourth verb — operators already know Approve/Reopen/Override/Skip-Gate from every other review-gated flow in this product; a new "Resolve Conflict" button family would be inconsistent for no benefit. | `GateVerdictBox.tsx:29-38`. |
| Both hosts claim within a race window (gossip lag) | Out of scope for this research pass (a protocol question, not a UX one — flagged for Phase 3 architecture, not resolved here), but whatever protocol answer emerges, the UI response is identical to the "resolved by a human" row above: surface it as a flagged item needing Override/redirect, not a new state. | — |
| PR opened by the "losing" host after a conflict is later discovered | Needs a way to trace the PR back to its originating host/item (Goal 3) independent of UI — but once traced, the *display* of that provenance is the same deep-link-back affordance as everywhere else in this doc, not a new pattern. | `BacklogOriginBadge.tsx` shows the existing pattern for "badge that links a thing back to the backlog item that spawned it" — same shape, cross-host is the only novel part. |

## 5. Jobs-to-be-done

- **Functional job**: avoid two hosts independently spawning work sessions and
  opening competing PRs for the same GitHub issue — wasted compute, wasted agent
  turns, and a merge-conflict cleanup burden on the operator that this system is
  explicitly supposed to prevent (per the `backlog-pr-conflict-detection` project's
  own problem statement, this operator has already had to manually rebase/resolve
  conflicts from uncoordinated autonomous work before —
  [`project_plans/backlog-pr-conflict-detection/requirements.md:9`](../../../project_plans/backlog-pr-conflict-detection/requirements.md)).
- **Emotional job**: confidence that running stapler-squad on multiple machines
  simultaneously is *safe* rather than a foot-gun — the single-operator,
  multi-device framing from §2 means this is really "trust my own fleet not to trip
  over itself," closer to the peace-of-mind job a sync-conflict warning does for a
  single user across two laptops than a trust-a-stranger job.
- **Social job**: even with one human operator, "social" here means cross-host
  legibility — each host's automation needs to behave as if it's aware of its
  siblings' state, so the operator never has to manually correlate what's running
  where. The deep link back to the owning host/item is the artifact that makes that
  legibility concrete and inspectable, not just claimed.

## Open questions for Phase 3

1. Is the human-facing surface a per-item inline banner (`DeepLinkErrorBanner`
   shape), a global `SystemBanner`, or both (inline detail always; global banner
   only when the operator is actively mid-import/triage)? Not resolved here — needs
   a product decision on how noisy multi-host conflicts are expected to be in
   practice.
2. Does the "Copy Link" affordance in `BacklogItemDetail.tsx` need a third mode
   (beyond Copy ID / Copy Link) for "Copy link to the *other* host's item," or does
   the conflict banner's own action button cover that need standalone?
3. Should a claim conflict be visible on `BacklogBoard`'s card view (a badge, akin
   to `BacklogItemBadge.tsx`) or only inside the detail panel — i.e., does an
   operator need to see this while scanning the board, or only after opening an
   item? Given the functional JTBD (catch this *before* spawning a session, not
   after), board-level visibility is likely necessary, not optional — flag for
   Phase 3 rather than deciding here.
