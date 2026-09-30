# UX Design: Cross-Host Claim/Dedup System

SDD Phase 3, UX design pass. Scope is bounded by `implementation/plan.md`'s Phase 2
(claim-check integration) and Phase 4 (UI surfaces) stories — this document designs
exactly those surfaces, not a broader "duplicate detection UI" than what's being built.
Binding prior art, per `research/ux.md` §0 and `plan.md`'s Pattern Decisions table:
`DeepLinkErrorBanner.tsx`'s `role="alert"`/`role="status"` split, and
`GateVerdictBox.tsx`'s **Override** verb. No new UI vocabulary is introduced.

## Resolving the three open questions from `research/ux.md`

`plan.md` (lines 133-140) already made two of these calls; this section states all
three explicitly, with the concrete component-level consequence for each, since the
prior document stopped at "which precedent" without saying "which file, which
element."

### 1. Banner placement: inline **and** board-level, not global `SystemBanner`

**Decision**: per-item inline banner in `BacklogItemDetail.tsx` (Surface 1) **plus** a
compact per-card indicator in `BacklogItemCard.tsx` (Surface 2). No `SystemBanner`
instance for this feature.

**Why**: the functional JTBD from `research/ux.md` §5 is "catch this before spawning a
session, not after" — an operator triaging the board needs the signal while scanning
cards, not only after opening one. A global `SystemBanner` ("N items may be claimed
elsewhere") would be a *third*, redundant surface: it can't name which card to look at
without the operator drilling in anyway, and `SystemBanner`'s own doc comment (per
`research/ux.md` §1) scopes it to whole-app health signals, not per-item state —
reusing it here would mean rebuilding its analytics/dismissal semantics for a job it
wasn't designed for.

### 2. "Copy Link" third mode: not added

**Decision**: `BacklogItemDetail.tsx`'s existing "Copy ID" / "Copy Link" pair
(`BacklogItemDetail.tsx:1454-1474`) is untouched. The claim-conflict banner (Surface 1)
carries its own "Copy link to \<host\>'s item" action, scoped to exactly the one
banner instance where it's relevant.

**Why**: a third mode on the always-visible header buttons would ask every operator,
on every item, to hold two similar-looking "copy" affordances in mind even when no
conflict exists. Putting the action on the banner means it exists exactly when — and
only when — there's a second host's link worth copying, and its label ("Copy link to
\"hostA\"'s item") can name the actual host, which a generic third header button
couldn't do without also knowing the conflict state.

### 3. Board-card-level visibility: yes, needed — as a footer chip, not inside `BacklogItemBadge`

**Decision**: needed (confirms `research/ux.md`'s own leaning). But `plan.md`'s Story
4.1.3 names `BacklogItemBadge.tsx` as a candidate file — that component is the
*non-board* `/backlog` **list row** (confirmed by its own test file's header comment),
not the board card. The board card is `BacklogItemCard.tsx`
(`web-app/src/components/backlog/BacklogItemCard.tsx`, rendered from
`BacklogBoard.tsx:176-184`). Its footer row already holds `AcSummary`, `VerdictBadge`,
an external-URL provenance badge, an action button, and — when relevant — a compact
`BlockerChip` (`BacklogItemCard.tsx:240`). A claim conflict is a fifth footer-row
citizen, added the same way `BlockerChip` was: an inline compact chip in that flex
row, not an absolute-positioned overlay (no such overlay convention exists anywhere in
`components/backlog/*.css.ts`, confirmed by grep). See Surface 2 below for the
resulting layout, and note for the implementing story: **update Story 4.1.3's file
list to `BacklogItemCard.tsx`/`BacklogItemCard.css.ts` — `BacklogItemBadge.tsx` is the
wrong component.**

---

## Surfaces designed

| # | Surface | Component(s) | Treatment |
|---|---|---|---|
| 1 | Claim-conflict banner, item detail | `ClaimConflictBanner.tsx` (new) in `BacklogItemDetail.tsx` | Full |
| 2 | Claim indicator, board card | `ClaimChip.tsx` (new) in `BacklogItemCard.tsx` | Full |
| 3 | "Already claimed" state, GitHub issue import | `GitHubIssuePicker.tsx` / `app/backlog/page.tsx` | Full |
| 4 | `check_cross_host_claim` MCP tool result | `server/mcp/tools_claim.go` | Condensed (agent-facing, not a human UI) |
| 5 | PR provenance stamp (GitHub comment) | `github/pr_comments.go` | Condensed (visible on GitHub itself, not inside stapler-squad) |

---

## Surface 1: Claim-conflict banner (`BacklogItemDetail.tsx`)

Covers Stories 4.1.1 and 4.1.2. Renders in the sticky header region, above/adjacent to
the existing Copy ID / Copy Link row (`BacklogItemDetail.tsx:1453-1489`), whenever this
item's `ExternalURL` has a `ClaimVerdict` of `ClaimHeldByOther` or
`ClaimCheckIndeterminate` from the `check_cross_host_claim` check.

### Wireframe — `ClaimHeldByOther`

```
┌──────────────────────────────────────────────────────────────────────┐
│ Fix the flaky retry test                              P2  Created 2d │
│ bl_01J7QK...   [Copy ID]  [Copy Link]                                 │
│                                                                        │
│ ┌──────────────────────────────────────────────────────────────────┐ │
│ │ ℹ  This issue is already claimed by "hostA"                      │ │
│ │    "hostA" claimed this 12m ago. Working on it here risks a      │ │
│ │    duplicate PR.                                                  │ │
│ │                                                                    │ │
│ │    [ Copy link to "hostA"'s item ]  [ Override — work on it here ]│ │
│ └──────────────────────────────────────────────────────────────────┘ │
│                                                                        │
│ Status: ready   Priority: P2   ...                                    │
```

### Wireframe — `ClaimCheckIndeterminate`

```
│ ┌──────────────────────────────────────────────────────────────────┐ │
│ │ ℹ  Couldn't confirm this isn't claimed elsewhere                 │ │
│ │    None of the known hosts responded to the claim check.         │ │
│ │    Proceeding is allowed, but check back if this seems off.      │ │
│ │                                                                    │ │
│ │    [ Retry check ]                                                │ │
│ └──────────────────────────────────────────────────────────────────┘ │
```

### Wireframe — `Disputed` (new: resolves cross-artifact-consistency BLOCKER CB-2)

`requirements.md`'s Out of Scope section is explicit that a genuine simultaneous-claim
race must be flagged for a human, not auto-arbitrated — the original version of
this document had no visible state for that case at all, so the backend's
last-write-wins pick silently read as an ordinary, undisputed `ClaimHeldByOther`. This
state renders instead whenever `ClaimRecord.Disputed == true` (plan.md Story 1.1.2's
new AC, Task 1.1.2e):

```
┌──────────────────────────────────────────────────────────────────────┐
│ ┌──────────────────────────────────────────────────────────────────┐ │
│ │ ⚠  Claim disputed — two hosts both claim this issue              │ │
│ │    "hostA" and "hostB" have each claimed this. Nothing was        │ │
│ │    auto-resolved; pick which claim is correct.                    │ │
│ │                                                                    │ │
│ │    [ Copy link to "hostA"'s item ]  [ Resolve — work on it here ] │ │
│ └──────────────────────────────────────────────────────────────────┘ │
```

`role="status"`/`aria-live="polite"` still applies (this is not a dead end for the
*current* link, same reasoning as the other two states) — never `role="alert"`. The
"Resolve" action reuses `ClaimHeldByOther`'s Override interaction shape exactly (same
reason-capture form, same audit log convention) but is relabeled "Resolve" rather than
"Override," since there's no single already-settled claim being overridden here — the
operator is the human `RecordClaim` deferred to. Confirming calls
`ClaimIndex.ResolveDispute` (plan.md Task 1.1.2e) in addition to the existing
override/proceed effect, clearing the disputed flag for future checks.

### Interaction flow

1. `BacklogItemDetail` mounts for an item with a non-empty `externalUrl`; it fires the
   `check_cross_host_claim` RPC (Story 2.2.3) in the background — no loading spinner
   gates the rest of the panel (per `research/ux.md` §2: a network round trip must not
   block a fast interaction the operator is already mid-way through, e.g. reading the
   item body while the check is still in flight).
2. **Unclaimed** result: banner never renders. No visible state change — this is the
   overwhelming common case (single-host setups, AC3) and must be invisible.
3. **`ClaimHeldByOther`** result: banner renders with `role="status"`, names the
   claiming host, states the claim's age (`ClaimedAt` formatted the same way
   `item.updatedAt` already is at `BacklogItemDetail.tsx:1447-1451`), and offers:
   - **Copy link to "\<host\>"'s item** — copies `ItemDeepLink` to the clipboard;
     button label swaps to "✓ Copied" for the same 1.5s window `handleCopy` already
     uses (`BacklogItemDetail.tsx`'s existing pattern), **and** additionally updates a
     visually-hidden `aria-live="polite"` announcement region — closing the exact gap
     `research/ux.md` §3 flags in the existing Copy Link button (a static `aria-label`
     silently drops the state change for screen readers). This is a **fix carried
     forward into the new component**, not a re-introduction of the known bug.
   - **Override — work on it here** — reuses `GateVerdictBox`'s Override interaction
     shape (`GateVerdictBox.tsx:410-465`): a toggle reveals a short-text reason field
     (same `MIN_OVERRIDE_REASON_LENGTH = 5` minimum, same Cancel/confirm button pair,
     same `role="form"` treatment), submitting logs the override with its reason
     (mirrors `onOverride(reason)`'s existing call shape) and dismisses the banner for
     this session. This is the exit path for an operator who has verified the other
     host's claim is stale (e.g., that host is gone) and wants to proceed anyway — it
     does **not** delete or contact the other host's item.
4. **`ClaimCheckIndeterminate`** result: banner renders with `role="status"`, states
   that the check couldn't be completed (never phrased as "unclaimed" — collapsing
   "unknown" into "clear" is the one thing `plan.md`'s `ClaimVerdict` sum type exists
   to prevent), and offers **Retry check** only — no Override, since there is nothing
   confirmed to override.
5. Dismissing the banner (browser back-navigation, closing the item) does not persist
   any dismissal state — the check re-runs next time the item is opened. This is
   intentional: gossip state changes over the item's lifetime (`ClaimIndex` is
   eventually consistent), and a stale "I already saw this, don't show it again" flag
   would contradict the "proceed optimistically, but say so" design`research/ux.md`
   settled on.

### Error and edge cases

| Case | Behavior | Exit path |
|---|---|---|
| Claim check RPC itself fails (network/5xx, distinct from a peer-query timeout inside the check) | Banner does not render; failure logged client-side only (matches `research/ux.md`'s "must not spinner-gate" — a broken check silently degrades to "no banner," same visible behavior as a clean unclaimed result, not a scary error box for a non-critical-path feature) | N/A — page usable as normal |
| `ItemDeepLink` present but points at a host with no `HostRegistry` entry (never gossiped to this instance) | Same banner, but "Copy link" is still offered (the raw `ssq://` URL is copyable even if this instance can't resolve it) — resolving that link is `DeepLinkErrorBanner`'s job (the `not-registered` reason), a different, already-shipped surface, not re-implemented here | Copy the link; open it elsewhere |
| Operator clicks Override, then the underlying claim is resolved (other host abandons/closes) before this session ends | No re-check while the banner is dismissed for the session (see step 5) — operator continues working; the next detail-page load re-evaluates from scratch | Reload the page |
| Two conflicting hosts both show up (rare: `ClaimIndex` conflict, `research/ux.md`'s "race window" row) | Banner shows whichever host `ClaimIndex`'s last-write-wins comparison resolved to (per `plan.md`'s Story 1.1.2 tie-break rule) — from the UI's perspective this is indistinguishable from a normal single-claimant `ClaimHeldByOther`; no separate "there's a 3-way dispute" UI state exists or is needed, since the backend already collapses it to one authoritative record | Same Override/Copy-link actions apply |

---

## Surface 2: Board-card claim indicator (`ClaimChip`, `BacklogItemCard.tsx`)

Covers the corrected Story 4.1.3 (see "Resolving the three open questions" §3 above).

### Wireframe

```
┌ BacklogItemCard (280px column width) ───────────────┐
│ Fix the flaky retry test              P2   ready    │  ← cardHeader
│                                                       │
│ ✓ 3/5    PASS    ⛒ #142    [ Claim… ]                │  ← cardFooter
└───────────────────────────────────────────────────────┘
        AcSummary  Verdict  provenance   ClaimChip
                              badge      (compact)
```

`ClaimChip` (compact variant) sits immediately after the existing provenance badge
(`item.externalUrl` link, `BacklogItemCard.tsx:202-215`) — the two are related (both
concern this item's external-URL identity) and reading order puts "this links to
GitHub" next to "and someone else already claimed it." Icon-plus-short-label, matching
`BlockerChip`'s compact-variant shape exactly (`BacklogItemCard.tsx:240`) rather than
inventing a new chip visual language:

```
[ 🔗 Claimed: hostA ]        ← ClaimHeldByOther
[ ? Claim unknown ]          ← ClaimCheckIndeterminate (shown only if a peer-query
                                already ran for this item in this page load; the board
                                does not fire one on-demand check per card — see below)
[ ⚠ Disputed ]               ← Record.Disputed == true (CB-2); click-through opens
                                Surface 1's full "Disputed" banner for resolution
```

### Interaction flow

1. `BacklogBoard` does **not** fire a `check_cross_host_claim` RPC per visible card —
   doing so for every card on every board render would be exactly the "spinner-gates
   a fast interaction" and "adds perceptible latency… especially running against
   multiple queued issues in a batch" regression `research/ux.md` §2 warns against,
   multiplied by the card count. Instead, `ClaimChip` renders from whatever claim
   status is **already known locally** (`ClaimIndex.CheckClaim`, the fast local-only
   read Story 1.1.2c exposes) — i.e., claims that have already propagated via gossip
   or a prior on-demand check elsewhere in the app. A card for an item whose conflict
   hasn't gossiped here yet simply shows no chip, same as the single-host case — this
   is the same best-effort/eventually-consistent tradeoff `ADR-002`/`ADR-001` already
   accept for the rest of this feature, not a new compromise invented for the UI.
2. Hover/focus on the chip shows the full claim age via `title`
   (`title="Claimed by hostA 12m ago"`), matching `BacklogItemCard`'s existing
   `title`-attribute convention for the item title itself.
3. Clicking the chip navigates into the item detail (same target as clicking the card
   itself) where Surface 1's full banner — with Copy-link/Override — takes over. The
   chip itself carries no independent action; it is a heads-up, not a control surface,
   consistent with `BlockerChip`'s own compact variant (detail, not action, at this
   density — full actions live one click away in `LifecycleSummary`, per that
   component's own precedent cited in `BacklogItemBadge.tsx`'s comment).

### Error and edge cases

| Case | Behavior | Exit path |
|---|---|---|
| Card footer already at capacity (AcSummary + VerdictBadge + provenance + action button + BlockerChip, all present at once) | `ClaimChip` wraps to a second footer line rather than being dropped silently — a claim conflict is higher-priority information than a card fitting on one visual line, unlike `BacklogItemBadge`'s deferred `BlockerChip` case (where the reason was that the *list badge* already had zero users depending on it for stuck-reason visibility, since `/unfinished` was named the primary surface for that). This item has no equivalent secondary surface for board-level claim visibility, so it may not be silently dropped. | Chip visible on line 2; card height grows by one line for that card only |
| Item has no `externalUrl` at all | No provenance badge, no `ClaimChip` — nothing to check (mirrors the backend's own `item.ExternalURL == ""` skip, Story 2.2.2a) | N/A |
| Claim later resolves (other host abandons) while the board is open | No live re-poll; chip clears on next board refresh/reload, same staleness tolerance as every other gossip-derived signal in this feature | Refresh the board |

---

## Surface 3: "Already claimed" state during GitHub issue import

Covers Story 2.2.1, surfaced through the **existing** import flow — `GitHubIssuePicker`
modal opened from the "New Item" modal's "Import from GitHub Issue" tab
(`app/backlog/page.tsx:857-895`), which is the only reachable import path today (the
manual paste-a-URL form at `page.tsx:897` is dead code, gated `{false && (...)}` — not
a surface this feature revives).

**Important existing-behavior note carried into this design**: today's *same-host*
duplicate check does **not** error — `ImportGitHubIssue` returns 200 with
`alreadyExisted: true` and the existing item (`backlog_service_sync.go:274-283`), and
`handlePickerSelect` (`page.tsx:545-601`) tallies this into a `duplicates` counter shown
as inline text below the picker, no link. The **cross-host** case is different by
design (`plan.md` Story 2.2.1a): a confirmed `ClaimHeldByOther` returns a distinct
*structured outcome* and creates no item **by default** — but, unlike a same-host
duplicate, it is not a dead end: point 4 below documents the "Import anyway" override
that resubmits and creates the item, with a captured reason (per CC-2). Read this
section's block-by-default framing together with point 4's override, not as a plain
hard error with no recourse. These two "already have this" outcomes must **not**
collapse into the same "already imported" message the way they would if cross-host
outcomes fell into the existing generic `failures` bucket (`page.tsx:570-580`) — that
bucket's copy ("Import failed... try again") is actively wrong for this case (retrying
without override will get the same block) and, worse, drops the deep link the whole
feature exists to surface (Goal 2). This is the one place in this design where behavior
must change in the existing handler, not just add a new component.

### Wireframe — batch import result banner (inline, below the picker's issue list)

```
┌ Import from GitHub Issue ───────────────────────────────────────────┐
│ Repository: acme/widgets                                             │
│ ☑ #101  Fix flaky retry test                                         │
│ ☑ #104  Add dark mode toggle                                         │
│ ☑ #108  Handle empty state in picker                                 │
│                                              [ Cancel ]  [ Import 3 ] │
│                                                                        │
│ Imported 1 new item.                                                  │
│ #104 already imported.                                               │
│ #108 is already claimed by "hostA" —                                 │
│   [ Copy link to hostA's item ]  [ Import anyway ]                   │
└────────────────────────────────────────────────────────────────────┘
```
Clicking "Import anyway" expands a reason field inline (same shape as Surface 1's
Override form) before resubmitting — see point 4 below.

### Interaction flow

1. Operator selects one or more issues and confirms import (`handlePickerSelect`).
2. For each issue, `importGitHubIssue` now needs to distinguish three outcomes instead
   of two — the RPC-error case must carry structured data (claiming host + deep link)
   through to the caller rather than collapsing to a bare `Error` string, since
   `getErrorMessage(err, "Failed to import GitHub issue.")`
   (`useBacklogService.ts:1303-1306`) currently discards everything but a message:
   - success, new item created → existing "Imported N new item(s)" copy, unchanged.
   - success, `alreadyExisted: true` (same-host duplicate) → existing "already
     imported" copy, unchanged.
   - **error, `connect.CodeAlreadyExists` with claim metadata** (new) → per-issue line
     naming the host and offering **Copy link to "\<host\>"'s item** — the redirect
     value the requirements doc (Goal 2) is built around, available at the exact
     moment the operator would otherwise have gone hunting for it.
   - error, anything else (genuine failure, e.g. issue is actually a PR) → existing
     generic "Import failed" copy, unchanged.
3. The picker **stays open** after a partial batch result, exactly as it does today for
   the duplicates/failures cases (`page.tsx:571-573`'s existing comment already states
   the reasoning: closing first would unmount the message before it renders) — so an
   operator importing a batch of 5 with one cross-host conflict still gets the other 4
   imported and sees exactly which one was blocked and why.
4. **Superseded by plan.md's post-adversarial-review revision**: Story 2.2.1 no longer
   blocks creation outright. `ClaimHeldByOther` now returns a structured
   `already_claimed_elsewhere` outcome (no item created by default, same picker/copy-link
   behavior described above) **plus** an `override: true` resubmit path that creates the
   item anyway and logs `import_github_issue.claim_override` — the backend call this
   surface needs now exists. This surface's Offer-Override affordance should therefore
   mirror Surface 1's Override verb (reusing `GateVerdictBox`'s pattern) rather than
   omitting it: add an "Import anyway" action next to Copy Link that resubmits with
   `override: true`.
   **Reason capture (resolves CC-2 — audit parity with Surface 1)**: clicking "Import
   anyway" opens a short reason field (≥5 characters, same
   `MIN_OVERRIDE_REASON_LENGTH`/Cancel-confirm shape as `GateVerdictBox.tsx:421-462` and
   Surface 1's own Override form above) rather than resubmitting immediately — this is
   equally a security/audit-relevant action here as it is in Surface 1, so it gets the
   same reason-capture treatment, not a one-click shortcut just because it's in a batch
   context. The reason is sent as `override_reason` alongside `override: true` and appears
   in the `import_github_issue.claim_override` log line (plan.md Task 2.2.1a). The action
   is surfaced only after the operator has seen the conflict deep link.

### Error and edge cases

| Case | Behavior | Exit path |
|---|---|---|
| All selected issues are cross-host-claimed | Every line in the result list reads "already claimed by \<host\>" with its own Copy-link action; `[Import N]` re-enables so the operator can immediately try a different selection | Pick different issues, or copy links and go check the other host |
| Claim check indeterminate for one issue in the batch | That issue **imports normally** (per `plan.md` Story 2.2.1's AC: indeterminate proceeds optimistically) but logs a note; the result line reads "Imported — couldn't confirm this isn't claimed elsewhere" rather than a plain "Imported" so the operator isn't given false confidence for that one item specifically | None needed — import succeeded; operator can open the item to see Surface 1's indeterminate banner if they want to investigate |
| Mixed batch: some new, some same-host duplicate, some cross-host-claimed, some genuine failures | All four outcome types can appear as separate lines in the same result block — this is an accumulation of the existing duplicates/failures rendering plus the two new lines above, not a new "pick one summary" model | Every line's info is visible simultaneously; nothing is hidden behind a "see more" |
| Operator has JavaScript-disabled clipboard access blocked (copy fails) | Button falls back to the same failure handling `handleCopy` already has elsewhere in this codebase — button label does not falsely claim "✓ Copied" | Manually select/copy the visible ssq:// URL text, which is rendered as plain text alongside the button, not hidden behind it |

---

## Surface 4 (condensed): `check_cross_host_claim` MCP tool result

Agent-facing, not a human UI (per `research/ux.md` §1's finding that `report_duplicate`
and its siblings are backend/agent tools). Representative result shape (Story 2.2.3):

```json
{
  "claimed": true,
  "checked": true,
  "claiming_host_id": "host_01K3F...",
  "item_deep_link": "ssq://hostA/backlog/v1/bl_01J7QK..."
}
```
`checked: false` (with `claimed: false`) distinguishes "confirmed clear" from
"couldn't confirm" — the same three-way split as `ClaimVerdict`, never collapsed to a
bare boolean.

**Acceptance criteria**:
- An agent calling this tool with a claimed URL receives `claiming_host_id` and
  `item_deep_link` populated, not just a boolean.
- An indeterminate check returns `checked: false`, never `claimed: false` alone (which
  would read as "confirmed unclaimed").
- Tool description text (surfaced to the agent, not a human) explicitly states the
  `checked`/`claimed` distinction, so an agent doesn't have to infer it from the schema.

---

## Surface 5 (condensed): PR provenance stamp (GitHub comment)

Visible on GitHub itself, not inside stapler-squad's own UI (Story 3.1.2). Representative
comment body:

```
🔗 Opened by stapler-squad host `host_01K3F...`
Backlog item: ssq://hostA/backlog/v1/bl_01J7QK...
```

**Acceptance criteria**:
- The comment never contains a raw LAN IP, bare hostname:port, or any
  `AdvertisedAddress` value — only the opaque `HostID` string and the `ssq://` deep
  link (per `research/pitfalls.md`'s leak warning, restated in `plan.md`'s
  `PRProvenanceStamp` glossary entry).
- A human reading the PR on GitHub, with no access to stapler-squad at all, can still
  see which host/item is responsible for it, satisfying Goal 3 independent of any
  in-app UI.
- Posting this comment never blocks or fails the PR-created flow itself if the GitHub
  API call errors — logged, not surfaced as a PR-creation failure (best-effort, per
  `plan.md` Task 3.1.2a).

---

## UX Acceptance Criteria (testable)

**Task completion**
1. An operator can copy a deep link to the claiming host's item in ≤ 1 click from the
   detail-view banner (Surface 1), and in ≤ 1 click per flagged issue from the import
   result (Surface 3).
2. An operator can identify which cards on the board have a cross-host claim conflict
   without opening any of them (Surface 2) — the chip is visible in the card's default,
   unexpanded state.
3. An operator can override a stale claim and continue working on an already-open item
   in ≤ 2 steps (open Override form, enter reason ≥ 5 characters, confirm) — matching
   `GateVerdictBox`'s existing override step count exactly, so this isn't a new pattern
   to learn.

**Error and edge-case states**
4. `ClaimCheckIndeterminate` is never displayed or logged as "unclaimed" anywhere in the
   UI, RPC response, or MCP tool result — verified by the `ClaimVerdict`/`checked` sum
   types remaining structurally distinct end to end (no boolean coercion at any
   boundary).
5. A cross-host-claimed issue in an import batch shows the claiming host's name and a
   working Copy-link action, distinct from the existing "already imported" (same-host)
   message — the two must not read as the same event to the operator.
6. No dead ends: every state introduced by this feature (banner, chip, import-result
   line) has at least one action that either resolves the state (Override, Retry) or
   gets the operator to more information (Copy link, click through to detail) — no
   surface leaves the operator with only a description and no recourse.
7. A claim check that fails outright (not indeterminate — the RPC call itself errors)
   degrades to "no banner shown," never to a visible error state, on the (non-critical,
   per Observability Plan) detail-page load — consistent with `research/ux.md`'s
   "must not spinner-gate a fast interaction."

**Accessibility**
8. `ClaimConflictBanner` (Surface 1) uses `role="status"`/`aria-live="polite"` for both
   its `ClaimHeldByOther` and `ClaimCheckIndeterminate` states — never `role="alert"`,
   since neither is a dead end for *this* link (mirrors `DeepLinkErrorBanner`'s
   documented rule).
9. Every action in Surfaces 1-3 (Copy link, Override, Retry, Cancel) is a real
   `<button type="button">` (or `<a>` for genuine navigation), reachable via Tab in
   document order, activatable with Enter/Space, with no custom `tabindex` — verified
   by keyboard-only manual pass and by each component's test asserting `getByRole`
   queries resolve (not `getByText`/class selectors).
10. The Copy-link action's state change ("Copied") is announced to screen readers via
    either a dynamic `aria-label` or a paired `aria-live="polite"` region — not a
    static label — in every one of Surfaces 1, 2, and 3's copy affordances. This is a
    regression test, not just a new-feature check: it is the exact gap
    `research/ux.md` §3 found in the pre-existing Copy Link button, and this feature
    must not reintroduce it in any of its three new copy actions.
11. The claiming host's name is always rendered as literal text in the accessible name
    or body of the banner/chip/import-line — never only conveyed via icon, color, or a
    generic "elsewhere"/"another host" phrase (mirrors `DeepLinkErrorBanner`'s explicit
    acceptance criterion, carried over verbatim).
12. Icons (`ℹ`, `🔗`, `?`) in all three surfaces are `aria-hidden="true"`, with the
    meaning fully carried in adjacent visible text — never icon-only.
13. Text/background color pairs used for the banner (status blue/gray, matching
    `DeepLinkErrorBanner`'s existing `statusContainer` token values) and the chip
    (matching `BlockerChip`'s existing compact-variant tokens) meet WCAG AA contrast
    ≥ 4.5:1 for body text and ≥ 3:1 for large text/icons — verified by reusing the
    already-audited token values rather than introducing new colors that would need a
    fresh contrast check.
14. `ClaimConflictBanner`'s Override form, when open, does not trap focus outside
    itself unexpectedly and restores focus to the toggle button on Cancel/Escape —
    mirroring `GateVerdictBox`'s existing `handleOverrideFormKeyDown` behavior exactly
    (`GateVerdictBox.tsx:227-233`), since this design reuses that component's
    interaction contract, not just its visual style.

**No-behavior-change guarantee (AC3 from requirements.md)**
15. With `cross_host_claim_dedup` off (default at merge) or no peers ever registered,
    none of Surfaces 1-3 render any new element, text, or state — single-host behavior
    is pixel-identical to today, verified by each new component's test suite including
    an explicit "renders nothing when unclaimed/checker unimplemented" case.

---

## Summary

- **3** interactive surfaces given full wireframe + interaction-flow + error-table
  treatment (item-detail banner, board-card chip, GitHub-import result).
- **2** non-interactive surfaces given condensed treatment (MCP tool result shape, PR
  provenance comment).
- **15** testable UX acceptance criteria, spanning task completion, error/edge-case
  handling (including an explicit no-dead-ends criterion), accessibility (keyboard,
  screen-reader labels, ≥4.5:1 contrast), and the single-host no-behavior-change
  guarantee.
- All three of `research/ux.md`'s open design questions resolved with a concrete,
  file-level recommendation (banner placement, Copy Link mode, board visibility) —
  including one correction to `plan.md` Story 4.1.3's file list (`BacklogItemBadge.tsx`
  is the wrong component; the board card is `BacklogItemCard.tsx`) and one required
  behavior change to the existing import-result handler (`handlePickerSelect` needs a
  third outcome bucket, not just failures/duplicates) that Story 2.2.1's current task
  breakdown doesn't yet call out.
