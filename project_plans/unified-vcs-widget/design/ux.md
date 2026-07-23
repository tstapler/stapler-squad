# UX Design: unified-vcs-widget

Companion to `requirements.md`, `research/ux.md`, and `implementation/plan.md`. This document
specifies the concrete layout, interaction flow, and acceptance criteria for `VcsWidget` and its
five sub-components (`MergeabilityPill`, `VcsWidgetHeader`, `VcsWidgetGithubRow`,
`VcsWidgetFileList`, `VcsWidgetCommitList`) across every surface, density mode, and edge case in
scope. Every wireframe below is drawn against the `VcsWidgetData` shape and `MergeabilityState`
union already fixed in `implementation/plan.md`'s Domain Glossary — this document does not
introduce new data fields, only the presentation and interaction layer on top of them.

## Design principles (carried forward from research/ux.md, restated as constraints)

1. **Two-tier disclosure.** Full mode = summary tier (always visible) + detail tier (one
   interaction away). Compact mode = summary tier only, nothing else.
2. **Mergeability-pill-first.** Every instance of the widget, in either mode, renders the
   synthesized `MergeabilityPill` as the first thing a user sees — it answers "is this ready"
   before anything else loads visual weight.
3. **Color is reinforcement, never the sole signal.** Every pill and status indicator pairs an
   icon + text label with its color. Verified against WCAG 1.4.1 (Use of Color) and 1.4.3
   (Contrast, 4.5:1 minimum) for both light and dark themes.
4. **Conditional rows, not empty-state placeholders.** A row that has nothing to show (no GitHub
   remote, zero commits ahead) is omitted entirely, not rendered as a disabled/greyed placeholder
   — matches GitLab's MR widget precedent and the existing `hasGitHub` guard.
5. **Never a dead end.** Every error, stale, and empty state names what happened in plain language
   and offers exactly one clear next action (retry, open session, browse files, or "nothing to do
   here" when that's the honest answer).
6. **Historical data degrades gracefully, never silently.** A widget rendering `kind === "historical"`
   is always visually distinguishable from one rendering `kind === "live"` data (via the "as of"
   timestamp), and a widget with no snapshot at all says so explicitly rather than rendering a
   sparse, unexplained widget.

---

## Surface inventory

| # | Surface | Density | Data source | Desktop | Mobile |
|---|---|---|---|---|---|
| 1 | Session detail, VCS tab | Full | `fromSessionVcs` (live) | §2.1 | §2.1 |
| 2 | Backlog item detail, live worktree | Full | `fromSessionVcs` (live) | §2.2 | §2.2 |
| 3 | Backlog item detail, done/shipped, snapshot present | Full | `fromShipStatus` (historical) | §2.3 | §2.3 |
| 4 | Backlog item detail, done, no snapshot (pre-feature item) | Full | `fromShipStatus` (historical, `loadError` set) | §2.4 | §2.4 |
| 5 | Unfinished dashboard card | Compact | `fromUnfinishedWorktree` (live) | §3.1 | §3.1 |
| 6 | `MergeabilityPill` state set (10 states) | Both | `deriveMergeabilityState` | §1 | §1 |
| 7 | File list expand/click-to-navigate | Full only | n/a | §4 | §4 |
| 8 | Commit list click-to-expand | Both | n/a | §5 | §5 |
| 9 | Loading skeleton | Both | n/a | §6.1 | §6.1 |
| 10 | GitHub rate-limited / stale data | Both | n/a | §6.2 | §6.2 |
| 11 | Closed-not-merged PR vs. durable shipped status | Both | n/a | §6.3 | §6.3 |
| 12 | 50+ changed files | Full | n/a | §6.4 | §6.4 |
| 13 | No GitHub remote | Both | n/a | §6.5 | §6.5 |
| 14 | Zero linked sessions | Full (Backlog) | n/a | §6.6 | §6.6 |
| 15 | Concurrent active sessions on one backlog item | Full (Backlog) | n/a | §6.7 | §6.7 |

15 distinct surfaces/states designed below.

---

## §1 — `MergeabilityPill`: the mergeability-state synthesis

This is the single highest-value new UX element per `research/ux.md` §2 — none of the three
existing surfaces synthesizes CI + review + conflict + shipped signals into one token today.

### Precedence and visual spec

Precedence order (first match wins), from `deriveMergeabilityState` (plan.md Task 1.1.5c):

```
shipped → snapshot_unavailable → no_pr → draft → conflicted → changes_requested → ci_failing
        → closed_unshipped → ci_pending → ready_to_merge (fallback)
```

| State | Label | Icon (lucide) | Color token | Rationale for precedence position |
|---|---|---|---|---|
| `shipped` | "Shipped" | `CheckCircle2` | `vars.color.success` | Durable ground truth always wins — see §6.3 |
| `snapshot_unavailable` | "Status unavailable" | `AlertTriangle` | `vars.color.warning` | Checked immediately after `shipped`, before every GitHub-signal branch — a durable-snapshot capture failure must never silently render as "CI still pending" or "no PR ever existed" (architecture-review BLOCKER fix, plan.md Task 1.1.5c) |
| `no_pr` | "No pull request" | `Circle` (outline, muted) | `vars.color.textSecondary` | Neutral, not an error |
| `draft` | "Draft" | `GitPullRequestDraft` | `vars.color.textSecondary` | Author explicitly signaled "not ready" |
| `conflicted` | "Conflicts — resolve to merge" | `AlertTriangle` | `vars.color.errorText` | Blocking, actionable, outranks review/CI noise |
| `changes_requested` | "Changes requested" | `MessageSquareWarning` | `vars.color.warning` | Human blocker outranks automated CI |
| `ci_failing` | "CI failing" | `XCircle` | `vars.color.errorText` | Automated blocker |
| `closed_unshipped` | "Closed — not merged" | `XSquare` | `vars.color.errorText` | ADR-003 bug-fix case — see §6.3 |
| `ci_pending` | "CI running" | `Loader2` (spin) | `vars.color.textSecondary` | Transient, not yet actionable |
| `ready_to_merge` | "Ready to merge" | `CheckCircle` | `vars.color.success` | Fallback — everything checked, nothing blocking |

### Wireframe (pill, both densities — same element, no size variant needed at this scale)

```
┌──────────────────────────────┐   ┌──────────────────────────┐
│ ● ✗ CI failing                │   │ ● ✓ Shipped               │
└──────────────────────────────┘   └──────────────────────────┘
   ^-- icon is aria-hidden;          ^-- always renders even when
       text carries the meaning          github.prState is stale/wrong
```

### Interaction flow

The pill is **not interactive** — no click target, no expand. It is a pure status readout. This is
deliberate: research (§2, Linear precedent) shows the compact/summary tier should stay a single
glance, and the detail tier (file list, GitHub row, commit list) already provides the "why" one
scroll below it. Adding a click target to the pill would create a second, redundant way to reach
information already reachable below, adding a decision cost with no new capability.

### Accessibility

- Icon: `aria-hidden="true"` (decorative — adjacent text carries the meaning).
- Pill wrapped in `role="status" aria-live="polite"` at the `VcsWidget` level (Task 2.2.1e) so a
  poll-driven state change (e.g. `ci_pending` → `ci_failing`) is announced without stealing focus.
- Contrast: every color/background pairing (`vars.color.errorText` on `vars.color.errorBg`,
  `vars.color.success` on `vars.color.successBg`, etc.) must be verified ≥ 4.5:1 in **both**
  light and dark theme — see Acceptance Criteria §A6.

---

## §2 — Full-detail mode

### §2.1 — Session detail, VCS tab (live)

**Desktop** (session detail right pane, ~640px content width):

```
┌ VCS ─────────────────────────────────────────────────────────────┐
│ ┌──────────────────────┐                          [⟳ Refresh]    │
│ │ ● ✗ CI failing         │  ←── role="status" aria-live="polite"  │
│ └──────────────────────┘                                          │
│                                                                    │
│ ⎇ feat/vcs-widget          Uncommitted changes   ↑3 ahead ↓1 behind│
│ 📁 ~/.stapler-squad/worktrees/feat-vcs-widget                     │
│    [Copy path]  [Browse files]                                    │
│                                                                    │
│ ⑂ tstapler/stapler-squad  #42  Open                                │
│   ✓ 1 approved   ✗ 1 changes requested   CI: failing              │
│                                                                    │
│ ── Conflicts (1) ──────────────────────────────────────────────── │
│  ⚠ src/foo.ts                                          +3 −1  →   │
│                                                                    │
│ ── Unstaged (4) ───────────────────────────────────────────────── │
│  M  src/bar.ts                                          +5 −2  →  │
│  M  src/baz.ts                                          +1 −0  →  │
│  A  src/qux.ts                                         +12 −0  →  │
│  D  src/old.ts                                           +0 −8  →  │
│                                                                    │
│ ── Commits ahead of main (3) ─────────────────────────────────────│
│  a1b2c3d  fix: widget bug                    Tyler · 2 days ago   │
│  e4f5g6h  feat: add mergeability pill        Tyler · 2 days ago   │
│  i7j8k9l  wip                                Tyler · 1 day ago    │
└────────────────────────────────────────────────────────────────────┘
```

**Mobile** (390px, dark theme — same vertical stacking order as desktop; the whole VCS tab is
already a single-column view on both platforms, so full mode does not need a distinct mobile
reflow beyond width-driven wrapping):

```
┌ VCS ───────────────────────┐
│ ┌───────────────┐  [⟳]     │
│ │ ● ✗ CI failing  │         │
│ └───────────────┘          │
│                             │
│ ⎇ feat/vcs-widget           │
│ Uncommitted changes         │
│ ↑3 ahead  ↓1 behind         │
│                             │
│ 📁 …/feat-vcs-widget        │
│ [Copy]  [Browse]            │
│                             │
│ ⑂ tstapler/stapler-squad    │
│ #42 Open                    │
│ ✓1 approved  ✗1 chg req     │
│ CI: failing                 │
│                             │
│ Conflicts (1)                │
│  ⚠ src/foo.ts    +3 −1  →   │
│                             │
│ Unstaged (4)                 │
│  M src/bar.ts    +5 −2  →   │
│  M src/baz.ts    +1 −0  →   │
│  A src/qux.ts   +12 −0  →   │
│  D src/old.ts    +0 −8  →   │
│                             │
│ Commits ahead (3)            │
│  a1b2c3d fix: widget bug    │
│  Tyler · 2d ago              │
│  …                          │
└─────────────────────────────┘
```

**Interaction flow**:
1. User opens session detail → clicks VCS tab.
2. Widget shows loading skeleton (§6.1) while `useSessionVcsContext` resolves.
3. On resolve: `fromSessionVcs(status, session)` → `VcsWidget mode="full"`. All sections render;
   `MergeabilityPill` is computed client-side from the same data, no extra RPC.
4. User clicks a file row (e.g. `src/bar.ts`) → `onNavigateToFile("src/bar.ts")` fires → session
   detail switches to the Files tab, scrolled/highlighted to that file. This is the one navigation
   exception in the widget (session context has a Files tab; Backlog context does not — see §2.2).
5. User clicks `[⟳ Refresh]` → re-fetch, pill/GitHub row update in place inside the `aria-live`
   region; no full-page reload, no layout shift in already-rendered sections.
6. Background poll (existing 60s fallback, paused on `document.hidden`) updates the same region
   silently; if the poll changes `MergeabilityState`, the `aria-live="polite"` announcement fires.

### §2.2 — Backlog item detail, live worktree (full mode)

Same visual structure as §2.1, with two differences:
- No Files tab to navigate to → file rows render as **plain text**, not buttons (per Story 2.1.3's
  "when `onNavigateToFile` is omitted, render `<span>` not `<button>`" rule). Clicking a file row
  does nothing; there is nothing to click.
- `onViewDiff` is wired instead → a **"View Diff"** action (a single button below the file-list
  section header, not per-row) opens `ReviewChangesModal` in place.

```
┌ Version Control ───────────────────────────────────────────────────┐
│ ┌──────────────────────┐                                          │
│ │ ● ✗ CI failing         │                                          │
│ └──────────────────────┘                                          │
│ ⎇ feat/vcs-widget   Uncommitted changes   ↑3 ahead ↓1 behind       │
│ 📁 …/feat-vcs-widget   [Copy path]  [Browse files]                 │
│ ⑂ tstapler/stapler-squad #42 Open  ✓1 approved ✗1 chg req  CI:fail │
│                                                                     │
│ ── Conflicts (1) ──────────────────────────────  [View Diff]  ──── │
│  ⚠ src/foo.ts                                          +3 −1      │
│ ── Unstaged (4) ─────────────────────────────────────────────────  │
│  M  src/bar.ts    +5 −2      M  src/baz.ts    +1 −0                │
│  A  src/qux.ts   +12 −0      D  src/old.ts     +0 −8               │
│                                                                     │
│ ── Commits ahead of main (3) ─────────────────────────────────────  │
│  a1b2c3d  fix: widget bug          Tyler · 2 days ago              │
└─────────────────────────────────────────────────────────────────────┘
```

Mobile: identical vertical stacking to §2.1's mobile wireframe, `[View Diff]` button placed
directly under the file-list section heading (full-width tap target, ≥ 44×44px per touch-target
convention).

**Interaction flow**:
1. User opens Backlog item detail, item has an active session with a live worktree.
2. `vcsStatus` resolves non-null → `fromSessionVcs(vcsStatus)` wins over `fromShipStatus` (the
   existing fallback-by-presence rule, preserved exactly — plan.md Story 2.2.3).
3. User taps **View Diff** → `ReviewChangesModal` opens in place (modal overlay via
   `createPortal`, per `.claude/rules/css-architecture.md`'s overlay convention) — no route
   navigation, no loss of scroll position on the underlying detail page.
4. User closes the modal (Escape key, backdrop click, or close button) → returns to the identical
   scroll position on the widget.

### §2.3 — Backlog item detail, done/shipped, snapshot present (historical, full mode)

Once the worktree is cleaned up, `vcsStatus` is `null` and `fromShipStatus(shipStatus)` takes
over. With Phase 3/4 backend work landed, the snapshot carries GitHub state and per-file stats, so
the widget still renders full richness — just historical, never live.

```
┌ Version Control ───────────────────────────────────────────────────┐
│ ┌──────────────────────┐                     As of 3 days ago      │
│ │ ● ✓ Shipped            │   ←── no [⟳ Refresh] button; kind === "historical" │
│ └──────────────────────┘                                          │
│ ⎇ feat/vcs-widget  (branch deleted — already merged)                │
│ ⑂ tstapler/stapler-squad #42 Merged  ✓2 approved  CI: success      │
│                                                                     │
│ ── Files changed (7) ──────────────────────────────  [View Diff]   │
│  M  src/bar.ts    +5 −2      M  src/baz.ts    +1 −0                │
│  A  src/qux.ts   +12 −0      D  src/old.ts     +0 −8   … +3 more   │
│                                                                     │
│ ── Commits (5) ────────────────────────────────────────────────── │
│  a1b2c3d  fix: widget bug          Tyler · 3 days ago              │
│  e4f5g6h  feat: mergeability pill  Tyler · 3 days ago              │
│  …                                                                 │
└─────────────────────────────────────────────────────────────────────┘
```

Note: no worktree-path row (nothing to browse/copy — the worktree no longer exists), no clickable
file rows (same plain-text rule as §2.2, doubly true since there's no live filesystem to browse
into), no `[⟳ Refresh]` (nothing to refresh — it's a fixed historical record).

**Mobile**: same stacking, "As of 3 days ago" right-aligns under the pill on desktop but stacks
directly below it on mobile (single column, no room for a right-aligned pair at 390px):

```
┌ Version Control ────────────┐
│ ┌───────────────┐           │
│ │ ● ✓ Shipped     │           │
│ └───────────────┘           │
│ As of 3 days ago             │
│                             │
│ ⎇ feat/vcs-widget            │
│ (branch deleted — merged)   │
│                             │
│ ⑂ tstapler/stapler-squad     │
│ #42 Merged                  │
│ ✓2 approved   CI: success   │
│                             │
│ Files changed (7)            │
│  [View Diff]                │
│  M src/bar.ts    +5 −2      │
│  …  +3 more                 │
│                             │
│ Commits (5)                  │
│  a1b2c3d fix: widget bug    │
│  Tyler · 3 days ago          │
└──────────────────────────────┘
```

**Interaction flow**: identical to §2.2 minus the refresh/browse affordances. `onViewDiff` still
opens `ReviewChangesModal`, backed by the durable file-stat snapshot rather than a live diff
computation.

### §2.4 — Backlog item detail, done, no snapshot (pre-feature item)

Items shipped before this feature's backend work landed have `snapshotAt: null` and a populated
`loadError`. This is a **benign, expected** state, not an error — must never render red/alarm
styling (research §4).

```
┌ Version Control ───────────────────────────────────────────────────┐
│ ┌──────────────────────┐                                          │
│ │ ● ✓ Shipped            │                                          │
│ └──────────────────────┘                                          │
│ ⎇ feat/old-work  (branch deleted — already merged)                  │
│                                                                     │
│ ℹ️ No history captured for this item — it shipped before detailed  │
│    tracking was added.                                             │
└─────────────────────────────────────────────────────────────────────┘
```

Only what's genuinely known (`shipped` state, derived from the pre-existing `shipped`/`shippedVia`
boolean fields, and branch name if still recorded) renders above the notice; everything requiring
the snapshot (GitHub row, file list, full commit list) is simply absent — no broken/empty
sub-sections, no "0 files changed" false-precision.

**Styling**: neutral/muted background (`vars.color.textSecondary` text, no `errorBg`/`warning`
tint), an info icon (`Info` from lucide, not `AlertTriangle`), matching research §4's explicit
"generic red error styling would misrepresent it."

**Interaction flow**: no action offered beyond what's already on the page (this state has no
"retry" — there is nothing to retry, the data was never captured). This is the one legitimate
exception to "every state offers an action" (§A5) — the state itself communicates that no action
exists, which is the honest and complete answer.

---

## §3 — Compact mode

### §3.1 — Unfinished dashboard card

Per plan.md Task 2.2.1b: compact mode omits `VcsWidgetFileList` and `VcsWidgetGithubRow` entirely
(a `GitHubBadge` rendered by the caller alongside covers the PR-badge need without duplicating it).

**Desktop** (dashboard grid card, ~340px card width):

```
┌ feat/vcs-widget ──────────────────────┐
│ ┌────────────────┐                    │
│ │ ● ⚙ CI running   │                    │
│ └────────────────┘                    │
│ ⎇ feat/vcs-widget   Uncommitted        │
│ 5 files changed   +42 −8               │
│                                        │
│ ▸ fix: typo                            │
│ ▸ feat: add widget                     │
│                                        │
│ [Reattach Session] [Commit & Push]     │
│ [View Diff] [Summarize]                │
└────────────────────────────────────────┘
```

**Mobile** (390px — this is the density the original Backlog-widget reference screenshot targets:
worktree path / branch+status / sessions list stacked vertically):

```
┌ Unfinished: feat/vcs-widget ┐
│ ┌────────────────┐          │
│ │ ● ⚙ CI running   │          │
│ └────────────────┘          │
│ ⎇ feat/vcs-widget            │
│ Uncommitted changes         │
│ 5 files changed  +42 −8     │
│                             │
│ ▸ fix: typo                 │
│ ▸ feat: add widget          │
│                             │
│ [Reattach Session]          │
│ [Commit & Push]             │
│ [View Diff]                 │
│ [Summarize]                 │
└──────────────────────────────┘
```

**Interaction flow**:
1. Dashboard lists N unfinished-worktree cards; each renders `VcsWidget mode="compact"` for its
   read-only summary, with the action-button row as surrounding chrome the widget does not own
   (plan.md Story 2.2.4 — separation of concerns between display and mutation actions).
2. Tapping a commit row (`▸ fix: typo`) expands it in place to show the full message if truncated
   — same click-to-expand behavior as full mode's commit list (§5), just capped at 5 rows instead
   of 20.
3. The 4 action buttons are unchanged by this feature — `VcsWidget` only replaces the read-only
   stats/commit-list block, never the mutating actions.
4. Zero-changes case: `aggregateFilesChanged: 0` → the aggregate stat line and commit list both
   render nothing (natural zero-guard, no explicit "no changes" branch needed in `VcsWidget`
   itself) — the card still shows the pill (`no_pr` or whatever state applies) so the card isn't
   visually empty.

---

## §4 — File list interaction (full mode only)

```
Collapsed (default) state, section with 60 unstaged files:

 ── Unstaged (60) ──────────────────────────────────────
  M  src/a.ts   +5 −2        M  src/b.ts   +1 −0
  M  src/c.ts   +3 −1        M  src/d.ts   +0 −4
  … (16 more of the first 20) …
  [ Show all 60 files ]   ←── real <button>, not a link


Expanded state (after clicking "Show all 60 files"):

 ── Unstaged (60) ──────────────────────────────────────
  M  src/a.ts   +5 −2
  … all 60 rows …
  [ Show fewer ]   ←── toggles back to collapsed
```

Conflicts are **never** capped — a section with 2 conflicts and 60 unstaged files always shows
both conflict rows in full, regardless of the unstaged section's collapse state, and conflicts
render as the first section (before staged/unstaged/untracked), matching VS Code's precedent.

**Click-to-navigate flow (Session detail only — Backlog detail has no target to navigate to)**:
1. User tabs to (or clicks) a file row — it's a real `<button>`, so `Tab`/`Shift+Tab` reaches it
   naturally and `Enter`/`Space` activates it, with the browser's default focus ring visible.
2. `onNavigateToFile(path)` fires → parent switches to the Files tab, scrolled to and highlighting
   `path`.
3. No page navigation, no loss of the VCS tab's own scroll position if the user returns to it.

**When `onNavigateToFile` is absent (Backlog detail)**: the same row renders as plain `<span>`
text — visually near-identical (same glyph, same `+N −M` stat, same left-border color accent) but
with no hover/focus affordance and no button semantics, so a screen reader correctly reports it as
static text, not a broken/inert control.

---

## §5 — Commit list interaction (both modes)

```
Collapsed row (default):
  a1b2c3d  feat: add a very long commit message that def…   Tyler · 2d ago
           ^-- <button>, ellipsis-truncated via CSS, tap/click anywhere on row

After tap/click:
  a1b2c3d  feat: add a very long commit message that definitely exceeds
           one line of available width in the commit list row
           Tyler · 2d ago
           [data-testid="commit-row-expanded"]
```

- Expansion is **click/tap**, never hover-only — fixes the mobile-inaccessible `title`-tooltip
  pattern in both `UnfinishedItemDetail.tsx:118` and `ShipStatusDisplay.tsx:88` (research §3
  finding #5, §4).
- Compact mode caps at 5 rows (no "show more" — matches `UnfinishedWorktree.aheadCommitMessages`'
  existing server-side cap). Full mode caps at 20 with a "Show all N commits" button identical in
  pattern to the file list's cap (§4), for consistency of the "N items, capped, expand" idiom
  across both list types in the widget.
- Each row is independently expandable/collapsible — expanding one row does not affect others.

---

## §6 — Loading, error, stale, and empty states

### §6.1 — Loading skeleton

```
┌ VCS ─────────────────────────────────────┐
│ ┌────────────┐                           │
│ │ ▓▓▓▓▓▓▓▓▓▓  │  (pulsing skeleton pill)   │
│ └────────────┘                           │
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓                        │
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓                            │
│ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓                │
└────────────────────────────────────────────┘
```
- Skeleton mirrors the shape of full-mode's header + pill (not a generic spinner) so there's no
  layout shift once real data arrives — matches Nielsen's "visibility of system status" heuristic.
- `role="status" aria-label="Loading VCS status"` on the skeleton container so screen reader users
  get an announcement rather than silence.
- `data-testid="vcs-widget-loaded"` is intentionally absent until real data renders (per plan.md
  Task 2.2.1g) — this is the e2e test's wait condition, replacing any `waitForTimeout`.
- Implementation should reuse the existing `Skeleton` primitive at
  `web-app/src/components/ui/Skeleton.tsx` (confirmed present in the codebase) rather than building
  a second, redundant pulsing-skeleton implementation for this widget.

### §6.2 — GitHub rate-limited / fetch failed (stale-but-labeled, not blank)

This is the most important error-state fix relative to today's behavior — `VcsPanel.tsx` currently
replaces the **entire panel** with an error+retry on any GitHub fetch failure, discarding VCS data
that loaded successfully. The unified widget must not regress this.

**Plan.md cross-reference**: this section describes a *live*-poll GitHub-fetch failure (a session's
`useSessionVcsContext` poll intermittently failing while the widget keeps rendering). Plan.md's
Story 4.2.1 ("Snapshot-fetch-failure copy") covers a related but distinct case — a *historical*,
done-item snapshot whose durable capture failed at ship time (`snapshotCaptureFailed`). The
live-poll rate-limited/stale scenario described below is **not yet an explicit plan.md task** —
it should be folded into the relevant `VcsWidgetGithubRow`/`VcsPanel` wiring story during
implementation (Epic 2.1/2.2), not assumed to already be covered by Story 4.2.1.

```
┌ VCS ─────────────────────────────────────────────────────┐
│ ┌──────────────────────┐          ⚠ GitHub data stale     │
│ │ ● ✓ Ready to merge     │          (as of 14m ago)        │
│ └──────────────────────┘          [⟳ Retry]                │
│ ⎇ feat/vcs-widget   Uncommitted changes                    │
│ ⑂ tstapler/stapler-squad #42 Open   ✓1 approved  ← stale   │
│                                                             │
│ ── Unstaged (2) ── (this section is git-local, unaffected  │
│    by the GitHub fetch failure — still fresh)  ─────────── │
│  M  src/bar.ts    +5 −2                                    │
└───────────────────────────────────────────────────────────┘
```

- Only the GitHub-sourced row (`VcsWidgetGithubRow`, and the GitHub-derived inputs to
  `MergeabilityPill`) shows the stale badge — git-local sections (header, file list) that fetched
  successfully keep rendering normally, since they come from a different data source with an
  independent failure mode.
- `MergeabilityPill` still renders using the **last-known-good** GitHub values (never blanks to
  "unknown") with a small stale-indicator dot on the pill itself when its inputs are stale.
- `[⟳ Retry]` re-fetches only the failed portion; a second consecutive failure extends the "as of"
  timestamp rather than escalating to a different error UI (no error-state ratcheting).
- Only a signal that has **never** successfully loaded (e.g. first-ever load fails) falls back to
  an explicit "Couldn't load GitHub status" message with `[Retry]` and no stale data to show,
  since there's nothing to fall back to.

### §6.3 — Closed-not-merged PR vs. durable shipped status (precedence bug fix, ADR-003)

This is the concrete case §1's precedence table encodes: a backlog item can be durably `shipped`
(git history confirms code reached main) while its GitHub PR record independently shows
`prState: "closed"` (e.g. squash-merged via a different PR, or GitHub's closed/merged distinction
lagging a webhook). Naively rendering "Closed" for a genuinely shipped item is actively misleading
— it looks like the work was abandoned.

```
WRONG (today's implicit bug — closed-looking styling wins):
┌──────────────────────┐
│ ● ✕ Closed             │   ← misleading: this work is actually merged/shipped
└──────────────────────┘

RIGHT (this widget — durable shipped status wins):
┌──────────────────────┐
│ ● ✓ Shipped            │   ← deriveMergeabilityState: shipped is checked FIRST
└──────────────────────┘
```

`deriveMergeabilityState`'s precedence (§1) checks `shipped` before ever inspecting
`github.prState`, so this is structurally unrepresentable as a bug once implemented — not just a
copy fix. `closed_unshipped` only fires when `shipped === false` **and** `prState === "closed"`
(§6 acceptance criteria below make this an explicit test case, not just a design note).

### §6.4 — 50+ changed files

Already specified in §4's collapse/expand mechanics. Summary of the rule: full mode caps
non-conflict sections at 20 rows with a "Show all N" expand; conflicts are never capped; compact
mode never lists individual files regardless of count (aggregate stat line only, per §3).

### §6.5 — No GitHub remote (local-only repo)

```
┌ VCS ─────────────────────────────────────┐
│ ┌────────────────┐                       │
│ │ ● ⚪ No pull request│                     │
│ └────────────────┘                       │
│ ⎇ local-only-branch   Uncommitted         │
│ ── Unstaged (2) ── ...                    │
└─────────────────────────────────────────────┘
```
No GitHub row renders at all (`VcsWidgetGithubRow` returns `null`, per plan.md Story 2.1.5) — not
a disabled/greyed placeholder, not a "connect GitHub" upsell nag. The `no_pr` mergeability state
covers this without any dedicated "no remote" copy, since from the user's perspective a
locally-only repo and a repo with a PR that hasn't been opened yet are the same actionable state:
"nothing to review yet."

### §6.6 — Zero linked sessions

```
┌ Version Control ──────────────────────────┐
│ This backlog item has no linked work       │
│ sessions yet.                              │
│                                            │
│ [ Open Session ]                           │
└─────────────────────────────────────────────┘
```
**Citation correction**: `UnfinishedItemDetail.tsx:58-66` contains only the `handleOpenSession`
routing callback (picks between opening the sole session, toggling the session picker for 2+
sessions, or navigating to new-session-from-worktree for zero sessions) — it does not contain this
empty-state copy, and no exact "This backlog item has no linked work sessions yet." string exists
anywhere else in `web-app/src` either (verified by search). The 0/1/N-session *routing* logic in
`handleOpenSession` is a reasonable pattern to mirror for the `[Open Session]` action button, but
the copy itself is net-new and should be finalized/reviewed during implementation, not treated as a
verbatim port of existing text. This is chrome the Backlog detail page owns **outside** `VcsWidget`
(the widget itself is never rendered when there is no session/snapshot data to adapt at all; the
parent shows this empty state and skips rendering `VcsWidget` entirely). One click opens a new
session — no dead end.

### §6.7 — Concurrent active sessions on one backlog item

`VcsWidget` itself is single-source-scoped — it renders one `VcsWidgetData` at a time, sourced from
whichever session `BacklogItemDetail.tsx:186`'s existing `.reverse().find(s => s.role === "work")`
heuristic ("most recent work session wins") selects. **That heuristic is unchanged by this
feature** — per requirements.md's Out of Scope, redesigning session-selection logic is explicitly
excluded, and this widget does not add interactive session-switching.

What this feature *does* add: today, when 2+ sessions are concurrently active on one backlog item,
the heuristic's choice is invisible — nothing on the page hints that a second, unselected session
with its own VCS state even exists. `VcsWidgetHeader` (full mode only) gains a small **passive,
non-interactive** "N active sessions" indicator so that ambiguity is visible instead of silently
hidden (adversarial-review Concern, plan.md Task 2.1.2f/Task 2.2.3f). This is deliberately scoped
down from full session-switching — it makes the existing single-session-selection behavior
observable, nothing more.

```
┌ Version Control ──────────────────────────┐
│ ┌──────────────────────┐                  │
│ │ ● ✗ CI failing         │                  │
│ └──────────────────────┘                  │
│ ⎇ feat/vcs-widget-a   Uncommitted          │
│ 📁 …/feat-vcs-widget-a  [Copy] [Browse]    │
│ 👥 3 active sessions    ←── plain text,     │
│    aria-hidden Users icon; NOT a button,    │
│    NOT a listbox — no click/tap target      │
│ ...                                        │
└──────────────────────────────────────────────┘
```

**Interaction flow**:
1. `BacklogItemDetail.tsx` computes `activeWorkSessionCount = linkedSessions.filter(s => s.role
   === "work" && s.status === "active").length` alongside its existing `latestWorkSession`
   selection, and passes it as `VcsWidget`'s `activeSessionCount` prop.
2. `VcsWidgetHeader` renders "N active sessions" (with a `lucide-react` `Users` icon,
   `aria-hidden="true"`) near the worktree-path row when `mode === "full" && activeSessionCount >
   1`. When `activeSessionCount` is `1` or omitted, nothing renders — no indicator, no layout
   change.
3. There is **no click/tap target** on the indicator. It does not open a picker, does not switch
   which session's data `VcsWidget` displays, and is not part of the widget's keyboard tab order.
   Choosing a different session's VCS state to view is out of scope for this feature.
4. `MergeabilityPill` and every other section continue to render only the single session's data
   selected by the pre-existing, unchanged heuristic — there is no merged/combined pill across
   multiple sessions.

---

## §7 — Responsive behavior summary

| Breakpoint | Behavior |
|---|---|
| ≥ 768px (desktop/tablet) | Right-aligned metadata (timestamps, refresh button) sits inline with the pill row; file-list rows may render 2-up where width allows (§2.3's "Files changed" example). |
| < 768px (mobile) | Single column throughout, matching the reference screenshot's existing mobile pattern (worktree path / branch+status / commits stacked vertically). Right-aligned metadata drops to its own line below the element it annotates. All tap targets ≥ 44×44px (buttons, expand toggles). The "N active sessions" indicator (§6.7) is plain text, not a tap target, so it is exempt from this minimum. |

No new breakpoint logic is introduced beyond what `VcsWidgetHeader.css.ts`'s `mode` variant and
the top-level `VcsWidget.css.ts` `@media` rule already specify in plan.md Task 2.2.1f — this
section documents the resulting behavior for design review, not new implementation.

---

## §8 — UX Acceptance Criteria

Each criterion is written to be verified by a human tester interacting with the running app (or a
Storybook/isolated render), not just by unit-test assertions — though many map directly to the
Jest/RTL tests already specified in `implementation/plan.md`.

### A. Task efficiency

- **A1.** From Session detail, a user can identify whether a session's PR is ready to merge in
  **1 glance** (the `MergeabilityPill`, above the fold, no scroll/click required) and can reach the
  full file-level diff in **≤ 2 clicks** (click a file row → Files tab opens scrolled to it).
- **A2.** From Backlog item detail, a user can view the full diff for a shipped item (worktree
  cleaned up) in **≤ 1 click** ("View Diff" → `ReviewChangesModal` opens in place, no page
  navigation).
- **A3.** From the Unfinished dashboard, a user can identify which of N cards needs attention
  (CI failing / conflicted / changes requested) without opening any card, by pill color+label alone
  — **0 clicks** to triage across a grid of cards.
- **A4.** From Backlog item detail, a user can tell **at a glance, with 0 clicks**, whether more
  than one session is concurrently active on the item, via `VcsWidgetHeader`'s passive "N active
  sessions" indicator — surfacing the pre-existing single-session-selection ambiguity instead of
  hiding it. Switching which session's data `VcsWidget` displays is out of scope for this feature
  (requirements.md Out of Scope); session selection continues to be governed by the existing
  "most recent work session" heuristic, unchanged.
- **A5.** Expanding a truncated commit message or a capped file-list section (50+ files) takes
  **exactly 1 tap/click**, with no hover-dependent step on any device.

### B. Error and edge-case handling

- **B1.** GitHub rate-limited/fetch-failed state shows the specific text "GitHub data stale (as of
  `<relative time>`)" plus a `[Retry]` action, and **never** blanks previously-loaded git-local
  sections (header, file list) — verified by: trigger a GitHub fetch failure while git-local data
  has already rendered, confirm the file list and branch/clean-dirty row remain visible and
  unchanged.
- **B2.** A durably-shipped item whose GitHub PR record shows `closed` renders `MergeabilityPill`
  as "✓ Shipped", never "✕ Closed" — verified by constructing `VcsWidgetData{shipped: true,
  github: {prState: "closed"}}` and confirming the rendered pill text.
- **B3.** A closed-but-never-shipped PR renders "✕ Closed — not merged" — verified by
  `VcsWidgetData{shipped: false, github: {prState: "closed"}}`.
- **B4.** A file list with 50+ entries in one category shows exactly the first 20 plus a "Show all
  N files" button; conflicts in the same widget instance are never truncated regardless of count.
- **B5.** A repo with no GitHub remote renders no GitHub row and no "connect GitHub" upsell —
  verified by confirming `VcsWidgetGithubRow`'s DOM subtree is entirely absent, not
  present-but-empty.
- **B6.** A backlog item with zero linked sessions shows "This backlog item has no linked work
  sessions yet." and an `[Open Session]` button that successfully creates and links a new session.
- **B7.** A done item with no captured snapshot (`snapshotAt: null`, pre-feature) shows the exact
  copy "No history captured for this item — it shipped before detailed tracking was added." in
  neutral (non-error) styling — verified by confirming the message does not use
  `vars.color.errorText`/`errorBg`.
- **B8.** Two or more active work sessions on one backlog item render a passive "N active sessions"
  indicator in `VcsWidgetHeader` (full mode, near the worktree-path row) instead of a silent,
  invisible single-session pick — verified by constructing `linkedSessions` with 2 entries having
  `role: "work"` and `status: "active"`, confirming `activeSessionCount={2}` is passed to
  `VcsWidget` and the text "2 active sessions" appears. The indicator has no click/tap target and
  does not switch which session's data the widget displays — session selection remains governed by
  the existing `.reverse().find()` heuristic, unchanged by this feature.

### C. No dead ends

- **C1.** Every error/stale state (B1, B7) offers either a retry action or, when retry is not
  applicable (B7's benign no-snapshot case), explicitly communicates that no action exists rather
  than presenting a broken-looking widget with silently missing sections.
- **C2.** Every empty state (B5, B6) offers a next step (B6: create a session) or explicitly
  communicates "nothing to show here" without an implied broken state (B5: no GitHub row is a
  valid, complete state, not a loading/error placeholder).
- **C3.** Closing any modal opened from `VcsWidget` (`ReviewChangesModal` via View Diff) returns
  the user to the exact scroll position and expand/collapse state they had before opening it — no
  full-page reload occurs anywhere in the widget's interaction surface.

### D. Accessibility

- **D1 — Keyboard navigation.** Every interactive element in `VcsWidget` (file rows when
  `onNavigateToFile` is present, commit-row expand toggles, "Show all N" buttons, refresh button,
  copy/browse buttons) is reachable via `Tab`/`Shift+Tab` in visual order and
  activatable via `Enter` or `Space`, using native `<button>` elements — never a `<span>`/`<div>`
  with a bare `onClick` (fixes `VcsPanel.tsx:84-90`'s WCAG 2.1.1/4.1.2 violation). Verified by
  tabbing through the full-mode widget with no mouse and confirming every documented interactive
  element receives visible focus and responds to keyboard activation.
- **D2 — Accessible names on icon-only controls.** Every icon-only button (`[⟳ Refresh]`, copy
  path, browse files) has an explicit `aria-label` (not `title`-only) — verified with a screen
  reader (VoiceOver/NVDA) or by asserting `getByRole("button", { name: "..." })` resolves in RTL
  tests, per `VcsPanel.tsx:214-216`'s fix.
- **D3 — Glyph classification.** Every icon in the widget is classified as either informational
  (`aria-label` present — e.g. `✓ 2 approved`, `✗ 1 changes requested`) or decorative
  (`aria-hidden="true"` — e.g. the branch icon, mergeability-pill icon, file-status glyphs whose
  meaning is already in adjacent text/label). No icon in the widget is unclassified.
- **D4 — Theme-token colors, verified contrast.** No hardcoded hex color appears in any
  `VcsWidget` sub-component's `.css.ts` file (fixes `VcsPanel.tsx:113-116`'s `#7ee787`/`#f97583`).
  Every text/background color pairing used for a status signal (pill states, CI conclusion,
  file-status accents) resolves to ≥ 4.5:1 contrast ratio in both light and dark theme, verified
  with a contrast-checking tool (e.g. axe DevTools or the project's Axe Core CI gate) against both
  `prefers-color-scheme` states.
- **D5 — No hover-only content.** No information in the widget is revealed only on `:hover` with
  no click/tap equivalent (fixes the `title`-tooltip truncation pattern in
  `UnfinishedItemDetail.tsx:118` and `VcsPanel.tsx:79,87`) — verified by testing the widget on a
  touch-only device/emulation (no hover capability) and confirming all truncated content remains
  reachable via tap.
- **D6 — Live region for async updates.** The mergeability pill and GitHub row are wrapped in
  `role="status" aria-live="polite"`; a background poll or manual refresh that changes
  `MergeabilityState` triggers a screen-reader announcement without moving keyboard focus —
  verified with a screen reader active during a simulated poll-triggered state change.
- **D7 — Color is never the sole signal.** Every status indicator (mergeability pill, CI
  conclusion, approved/changes-requested counts, file-status glyphs) carries a text label or
  `aria-label` in addition to its color — verified by disabling color (grayscale filter or a
  colorblindness simulator) and confirming every state remains distinguishable from its neighbors.
- **D8 — Axe Core CI gate.** The widget introduces zero new WCAG AA violations as measured by the
  project's existing Axe Core CI check on `web-app/src/` PRs (per CLAUDE.md's "UX analysis CI").

---

## Summary of what changed vs. today, at a glance

| Today | This design |
|---|---|
| 3 separate components, 3 different feature sets | 1 component, 2 density modes, full feature parity across all 3 call sites |
| No synthesized "is this ready" signal anywhere | `MergeabilityPill`, above the fold, everywhere |
| GitHub fetch failure blanks the whole panel | Only the GitHub-sourced row shows stale/error; git-local data stays visible |
| Closed PR can visually read as "abandoned" even when shipped | `shipped` always wins in the precedence order — structurally, not just cosmetically |
| Clickable `<span>` file rows, keyboard-inaccessible | Real `<button>`/`<span>` per interactivity, full keyboard support |
| Hardcoded hex CI colors, contrast unverified | Theme tokens, contrast-verified in both themes |
| `title`-only icon buttons, ambiguous screen-reader names | `aria-label` on every icon-only control |
| Hover-only truncation tooltips, broken on mobile | Click/tap-to-expand everywhere |
| No live region for async status changes | `aria-live="polite"` around the pill/GitHub row |
| Done items lose all GitHub/diff richness after worktree cleanup | Durable snapshot (Phase 3/4) + explicit "no history captured" copy for pre-feature items |
