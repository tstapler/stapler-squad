# UX Design: Artifacts Tab

**Feature**: Async JSONL artifact extraction — session-history-metadata
**Component**: `ArtifactsTab` inside `SessionDetailView`
**Date**: 2026-06-22
**Status**: Design complete — ready for Phase 4 implementation

---

## Overview

The Artifacts tab surfaces structured outputs from a Claude Code session — GitHub PR links, git commit SHAs, and notable external URLs — extracted asynchronously from the session's JSONL conversation history. It lives as a new tab in the existing session detail panel, which is approximately 400–600 px wide on desktop and full-width on mobile.

**Design goals**:
1. Highest-signal artifacts (PR links) must be immediately visible without scrolling.
2. The tab must be useful at zero data (new session) — no dead end, no confusion.
3. Real-time updates must not disrupt the user's current reading flow.
4. Every interactive element must be keyboard accessible and screen-reader friendly.
5. The component must never introduce new CSS variables — all tokens come from `theme-contract.css.ts`.

---

## Tab Anatomy

The Artifacts tab contains these zones, in order from top to bottom:

```
┌─────────────────────────────────────────────┐
│ [Scan status bar]                    [timer] │  ← sticky footer alternative; see §Timestamp
├─────────────────────────────────────────────┤
│ Pull Requests  (2)                          │  ← section header with count badge
│  ○ owner/repo#42     [Open]                 │
│  ○ owner/repo#43     [Merged]               │
├ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┤  ← subtle divider (borderSubtle)
│ Commits  (1)                                │  ← section header with count badge
│  ⬡ a1b2c3d  [copy]                          │
├ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ┤
│  ▼ Show 12 external URLs                   │  ← disclosure toggle, collapsed by default
└─────────────────────────────────────────────┘
```

**Section dividers**: Use a 1 px horizontal rule styled with `vars.color.borderSubtle`. Do not use heavy separators — the section headers already provide sufficient visual grouping (Gestalt proximity principle). A divider between Pull Requests and Commits, and between Commits and the external URLs disclosure, is sufficient.

**Section count badges**: Yes — every section header includes a count badge in parentheses rendered inline, e.g. "Pull Requests (2)". The badge uses `vars.fontSize.xs`, `vars.color.textMuted`, and `vars.fontWeight.normal` to read as supplementary information, not as a primary element. Do not render the badge when the section is empty (omit the section entirely rather than showing "Pull Requests (0)").

**Scroll behavior**: The root container must set `height: "100%"` and `overflowY: "auto"` to participate in the panel's scroll model. Content overflows inside the tab; the tab strip itself does not scroll.

---

## Information Hierarchy

Render in this order:

1. **Pull Requests** — highest signal. A PR URL means the session shipped deliverable code. Users navigating between sessions in a review queue will look here first.
2. **Commits** — second highest signal. Commits confirm work was landed even if no PR exists (direct-to-main pushes, drafts not yet opened).
3. **External URLs** (collapsed by default) — lowest signal. These are often CI run links, documentation pages, or GitHub blob links that happen to appear in tool output. They are valuable for forensic auditing but not for the primary reading flow.

**Rationale for collapsing external URLs**: Nielsen's heuristic of aesthetic and minimalist design and Steve Krug's principle of removing unnecessary decisions from the user's path. The median Artifacts tab usage is "did this session open a PR?" — external URLs answer a different question. Collapsing them by default does not hide them; it reduces noise for the common case.

---

## Component States

### State 1: Not yet started (`session.artifacts === undefined`)

`artifacts` is `undefined` when the proto field is absent — meaning the extractor has never scanned this session's JSONL file (new session started since the server deployed, or no JSONL file exists yet).

**Icon**: `Clock` from lucide-react, `vars.color.textMuted`, 24 px.

**Heading**: "Extraction pending"

**Body**: "Artifact scanning starts automatically once the session writes its first history entry. Check back after running a command."

**Copy rationale**: Names the precondition (history entry) so the user understands what triggers scanning. Avoids the phrase "in the background" alone, which implies something may be wrong.

---

### State 2: Scanned, nothing found (`artifacts` defined, all arrays empty)

`lastScannedAt` is set. The extractor ran but found no PR URLs, commit SHAs, or external URLs in the session's JSONL history.

**Icon**: `SearchX` from lucide-react, `vars.color.textMuted`, 24 px.

**Heading**: "No artifacts found"

**Body**: "This session hasn't produced any tracked outputs yet — no PR links, commits, or notable URLs were detected in the conversation history."

**Note**: Do not say "extraction ran successfully" — users do not care about process success; they care about results. The message should be matter-of-fact.

---

### State 3: Mid-scan (artifacts defined, `lastScannedAt` is within the past 30 seconds)

The extractor is likely mid-run. Data may be partial.

**Behavior**: Show whatever data is available (do not withhold it), but append a subtle inline indicator below the scan status bar:

**Inline indicator** (in the scan status bar): Replace the timestamp copy with "Scanning now..." alongside a `Loader2` icon (lucide-react, 14 px, CSS animation `spin`). Apply `vars.transition.base` to the icon's opacity so it fades in only after 500 ms delay (prevents flash for fast scans).

**Threshold**: 30 seconds from `lastScannedAt`. Client-side computed: `Date.now() - lastScannedAt.toDate().getTime() < 30_000`.

---

### Loading State (before any `session.artifacts` data arrives from WatchSessions)

Show a skeleton on first mount only — specifically when the component mounts and `session.artifacts` is `undefined` AND `session.id` has changed (i.e., this is the first render for this session, not a returning visit with cached data).

Skeleton structure:
```
[skeleton block 40 px tall, full width, borderRadius: vars.radii.md]   ← simulates section header
[skeleton block 20 px tall, 80% width]                                  ← simulates PR row
[skeleton block 20 px tall, 65% width]                                  ← simulates PR row
[skeleton block 40 px tall, full width, mt: vars.space[4]]              ← simulates second section header
[skeleton block 20 px tall, 50% width]                                  ← simulates commit row
```

Skeleton background: `vars.color.surfaceMuted`, animated with a shimmer using a `@keyframes` in the `.css.ts` file. Do not use a shimmer that loops faster than 1.5 s — rapid animations increase perceived load anxiety.

**When to dismiss the skeleton**: The first time `session.artifacts` is non-undefined (even if the data is an empty object). If `session.artifacts` remains undefined for more than 3 seconds, transition to State 1 (extraction pending) — the skeleton should not persist indefinitely.

Implementation note: Track skeleton visibility in local state (`const [showSkeleton, setShowSkeleton] = useState(session.artifacts === undefined)`). Dismiss it in a `useEffect` when `session.artifacts` becomes defined.

---

## PR Link Display

### Format

Parse the PR URL using the existing `parsePRDisplay` helper from the plan. Render as: `owner/repo#42`.

For repositories with long names (e.g., `organization-name/very-long-repository-name`), truncate using CSS `text-overflow: ellipsis` on the anchor element — do not truncate in JavaScript. This preserves the full URL in the DOM (accessible to screen readers and copy-paste).

### PR Status Badge — Recommendation: Future Enhancement, Not MVP

**Decision**: Do not ship PR status badges in the MVP. Here is the reasoning:

The current proto does not include PR status. Fetching PR status requires polling the GitHub API (a network call per PR URL), which introduces latency, GitHub rate-limit exposure, and additional backend complexity outside this feature's scope. The plan already acknowledges a separate `prCallbackFn` integration for updating `GitHubPRNumber` — PR status (open/merged/draft/closed) is a separate concern.

**MVP behavior**: Render the PR link as `owner/repo#42` with no badge. The link itself opens the GitHub PR page where the user can see status directly.

**Future enhancement path**: When `SessionArtifacts` is extended with a `pr_statuses` repeated field (a map from PR URL to status string), add a status badge inline. The badge design when implemented:

| Status | Background token | Foreground token | Border token |
|---|---|---|---|
| `open` | `vars.color.successBg` | `vars.color.success` | `vars.color.success` (0.3 opacity via box-shadow) |
| `merged` | `vars.color.accentBg` | `vars.color.primary` | `vars.color.primary` (0.3 opacity) |
| `draft` | `vars.color.surfaceMuted` | `vars.color.textMuted` | `vars.color.borderMuted` |
| `closed` | `vars.color.errorBg` | `vars.color.error` | `vars.color.error` (0.3 opacity) |

The badge uses `vars.fontSize.xs`, `vars.radii.full`, horizontal padding `vars.space[2]`, vertical padding `vars.space[1]`, and `vars.fontWeight.medium`.

### PR Row Layout

```
[GitPullRequest icon 14px, textMuted]  [anchor: owner/repo#42]  [ExternalLink icon 12px, textMuted]
```

The `ExternalLink` icon appears on hover/focus only (`:hover, :focus-within` selector). This follows the convention used elsewhere in the panel and prevents icon clutter at rest.

Anchor color: `vars.color.primary`
Hover state: `textDecoration: "underline"`, `vars.color.primaryHover`
Visited state: `vars.color.textSecondary` (visited PRs should look settled, not identical to unvisited)

---

## Commit SHA Display

### Format

7-character short SHA in monospace: `a1b2c3d`

Font: `vars.font.mono`
Font size: `vars.fontSize.sm`
Color: `vars.color.textPrimary`
Background: `vars.color.surfaceSubtle`
Border radius: `vars.radii.sm`
Padding: `vars.space[1]` vertical, `vars.space[2]` horizontal

This renders the SHA as a distinct chip/badge, matching the visual language used for branch names elsewhere in the panel.

### Navigation — When owner/repo is Known

The session proto includes `workingDir` and `githubPrUrl` fields. If `githubPrUrl` is non-empty, extract `owner/repo` from it using the same regex as `parsePRDisplay`. Construct the commit URL: `https://github.com/{owner}/{repo}/commit/{sha}`.

When owner/repo is derivable, wrap the SHA chip in an anchor (`<a href=... target="_blank">`).

### Navigation — When owner/repo is Unknown

`githubPrUrl` may be empty if no PR was opened (commits to main, or session is still in progress). In this case:

- Render the SHA chip as non-navigable text (not an anchor).
- Show a **copy button** (`Copy` icon from lucide-react, 14 px, `textMuted`) to the right of the chip. On click, copy the full 40-character SHA to clipboard and show a brief inline confirmation: the icon transitions to a `Check` icon for 1.5 seconds, then reverts. Use `vars.transition.fast` for the icon swap.
- Do not attempt to link to a generic git host — incorrect links are worse than no links.

### Commit Metadata

The proto does not include commit messages. Do not fabricate supplementary metadata. The SHA chip alone is sufficient for MVP. When the user clicks through to GitHub they see the full commit message there.

**Future enhancement**: If `commit_messages` is added to `SessionArtifacts`, render the message in `vars.color.textSecondary` at `vars.fontSize.xs` below the SHA chip.

### Commit Row Layout

```
[GitCommit icon 14px, textMuted]  [SHA chip]  [copy icon OR external link icon]
```

---

## External URL Display

### Disclosure Toggle — Collapsed by Default

The external URLs section is hidden behind a `<details>`/`<summary>` disclosure element (or a controlled React boolean state toggle). Default state: collapsed.

**Toggle label (closed)**: `Show {n} external URLs` using a `ChevronRight` icon (14 px, `textMuted`) before the text.
**Toggle label (open)**: `Hide external URLs` using a `ChevronDown` icon (14 px, `textMuted`).

Toggle text: `vars.fontSize.sm`, `vars.color.textSecondary`, `vars.fontWeight.medium`.
Toggle uses `cursor: "pointer"` and has a `:hover` background of `vars.color.hoverBackground` applied to the entire toggle row.

The toggle state persists only within the component's lifetime (React local state). It resets when the user switches sessions. This is intentional — the default collapsed state is correct for most sessions.

### URL Rows

Each URL row:
- `Globe` icon (lucide-react, 14 px, `vars.color.textMuted`) prepended
- URL text truncated to 60 visible characters using CSS `text-overflow: ellipsis` (not JavaScript substring — the full URL must remain in the `href` attribute and be readable by screen readers via `title` or `aria-label`)
- `title` attribute on the anchor set to the full URL for tooltip on hover
- Font: `vars.font.sans`, `vars.fontSize.sm`, `vars.color.primary`
- Hover: `vars.color.primaryHover`, `textDecoration: "underline"`

### Domain Grouping — Not Recommended for MVP

Grouping by domain (e.g., all `github.com` URLs together) adds complexity without proportionate benefit given the 50-item cap. The user reading the external URLs section is already in "forensic audit" mode; they will scan the list. Implement grouping only if user research reveals confusion.

---

## Tab Badge

### Count Logic

The tab label should show a numeric badge when the total count of high-signal artifacts is greater than zero:

```
count = (session.artifacts?.prUrls.length ?? 0) + (session.artifacts?.commitShas.length ?? 0)
```

External URLs are **excluded** from the tab badge count. They are noisy and would inflate the badge for sessions that make many network calls without shipping deliverables.

### Badge Appearance

When `count > 0`, render: `Artifacts · {count}` in the tab label string, OR (preferred for visual clarity) render a small numeric badge chip to the right of the tab icon using the same pattern as the approval badge (`ApprovalNavBadge.tsx`).

Badge chip: `vars.color.primaryText` text on `vars.color.primary` background, `vars.radii.full`, min-width 18 px, height 18 px, `vars.fontSize.xs`, `vars.fontWeight.semibold`.

When `count === 0` (or `session.artifacts` is undefined), the tab renders as: `Package` icon + "Artifacts" label, no badge.

**Implementation note**: The tab label in `SessionDetailView` is currently a string. To support a badge chip, change the tab definition to accept `labelSuffix?: React.ReactNode` in the tab config object, or compute a badge label string (`"Artifacts (3)"`). The simpler string approach — `label: count > 0 ? \`Artifacts (${count})\` : "Artifacts"` — is sufficient for MVP and avoids adding custom tab rendering logic.

---

## Timestamp and Scan Status

### Placement

The scan status line appears as a **sticky footer** pinned to the bottom of the Artifacts tab container. This placement:
- Does not compete with artifact content at the top (the user came to see PRs and commits, not metadata about when they were found)
- Remains visible regardless of scroll position
- Mirrors the pattern used by the Logs tab's status line

```
┌─────────────────────────────────────────────┐
│  [content area, scrollable]                 │
├─────────────────────────────────────────────┤
│  Scanned 3 minutes ago                      │  ← sticky footer
└─────────────────────────────────────────────┘
```

Footer styling: `vars.color.borderSubtle` top border (1 px), `vars.color.surfaceMuted` background, `vars.space[2]` vertical padding, `vars.space[4]` horizontal padding, `vars.fontSize.xs`, `vars.color.textMuted`.

### Copy

| Condition | Copy |
|---|---|
| `lastScannedAt` null | "Not yet scanned" |
| < 60 seconds ago | "Scanned just now" |
| 1–59 minutes ago | "Scanned {N} minute(s) ago" |
| 1–23 hours ago | "Scanned {N} hour(s) ago" |
| 1+ days ago | "Scanned on {date}" (format: "Jun 22") |
| Mid-scan (< 30s and artifacts just arrived) | "Scanning now..." |

### Refresh Action — Not Recommended

Scanning is fully automatic; it triggers on every JSONL file change via the `HistoryLinker` callback. A manual refresh button would:
1. Mislead users into thinking manual action is required.
2. Create a false mental model of polling vs. push-based updates.
3. Add a button that does nothing when the extractor is already current.

Do not include a refresh button. If a user reports a scan that appears stale, the correct diagnosis path is the server logs, not a client-side retry.

---

## Real-time Update Behavior

### Recommendation: Animate In With Brief Highlight (Option b)

When WatchSessions pushes an updated `session.artifacts` with a new PR URL or commit SHA that was not in the previous render, silently insert the new row at the top of its section with a brief background highlight animation.

**Behavior**:
1. On each render, compare `session.artifacts.prUrls` to the previous render's value using a `useRef` to hold the prior list.
2. If new URLs appear, the component re-renders naturally (React state update from parent). New rows are inserted at the top of the section list.
3. New rows receive a CSS animation: `background` transitions from `vars.color.accentBg` to transparent over 2 seconds using `vars.transition.slow`. This uses a one-shot CSS animation (`@keyframes artifactHighlight`) — no JavaScript timers needed.
4. After the animation completes, the row is visually indistinguishable from existing rows.

**Why not a toast (Option c)**: Toasts interrupt the user's current task — if they are reading the Artifacts tab, an overlapping toast notification about the Artifacts tab is redundant noise. If they are on a different tab, a toast is appropriate via the existing `NotificationContext` (this is separate from the ArtifactsTab component's responsibility).

**Why not a banner (Option d)**: The "N new artifacts" banner requires a second dismiss interaction. The inline highlight provides the same discoverability signal with zero cognitive overhead.

**Why not silent insert (Option a)**: A purely silent insert may go unnoticed if the user is not actively looking at the list. The highlight anchors their attention without demanding it.

**Implementation**: Add a `data-new` attribute to newly inserted rows for a single render cycle, and apply the highlight animation via a CSS selector on `[data-new="true"]`. Remove the `data-new` attribute after the animation duration using a `useEffect` with a `setTimeout` (2100 ms — animation plus a small buffer).

```ts
// ArtifactsTab.css.ts
export const artifactRow = style({});

// highlight applied once via data attribute
globalStyle(`${artifactRow}[data-new="true"]`, {
  animation: `${highlightKeyframes} 2s ease-out forwards`,
});
```

---

## Accessibility

### Keyboard Navigation

- All anchor links and the copy button are natively keyboard focusable (no `tabIndex` manipulation needed).
- The external URLs disclosure toggle must be keyboard activatable. Prefer a `<button>` element (not a `<div onClick>`) to get Enter/Space activation for free.
- Tab order within a section follows DOM order: section header → artifact rows, top to bottom.
- The copy button receives focus after the SHA chip it copies. Use `aria-label="Copy full SHA {sha}"` so screen readers describe what will be copied.

### ARIA Landmarks and Labels

- Each section (`<section>`) has an implicit ARIA landmark role. Add `aria-labelledby` pointing to the `<h3>` section title id.
- The disclosure toggle uses `aria-expanded="true|false"` on the `<button>` element.
- External URL anchors that open in a new tab include `aria-label="{truncated display text} (opens in new tab)"` — or at minimum, a visually hidden `<span className="sr-only">(opens in new tab)</span>` inside the anchor.
- The empty state container uses `role="status"` so screen readers announce it when it appears without requiring the user to navigate to it.
- The scan status footer uses `role="status"` and `aria-live="polite"` so timestamp updates are announced without interrupting current screen reader flow.

### Focus Management

When a new PR or commit appears via real-time update (highlighted row), do not auto-focus it — the user may be typing or reading elsewhere. The highlight animation serves as the visual cue without forcing focus disruption.

### Color Contrast

All text on `vars.color.surfaceSubtle` or `vars.color.surfaceMuted` backgrounds must meet WCAG AA minimum 4.5:1 for normal text. The SHA chip (`textPrimary` on `surfaceSubtle`) satisfies this in both light and dark themes — verify with the theme values in `theme.css.ts` before finalizing.

The external URL disclosure toggle text (`textSecondary`) must also meet 4.5:1. If `textSecondary` in either theme falls below this ratio against `background`, use `textPrimary` instead.

---

## CSS Token Mapping

Explicit mapping of every design decision to `theme-contract.css.ts` tokens:

| Design element | Token | Notes |
|---|---|---|
| Container scroll | `height: "100%"`, `overflowY: "auto"` | Per page scroll convention |
| Container padding | `vars.space[4]` | Standard panel content padding |
| Section gap | `vars.space[6]` | Between PR / Commits / URL sections |
| Section divider | `vars.color.borderSubtle` | 1 px hr between sections |
| Section title color | `vars.color.textSecondary` | All-caps, `vars.fontSize.sm` |
| Section title weight | `vars.fontWeight.semibold` | Distinct from body without shouting |
| Count badge color | `vars.color.textMuted` | De-emphasized inline count |
| Row gap | `vars.space[2]` | Between rows within a section |
| Anchor color (default) | `vars.color.primary` | PR links, external URLs |
| Anchor color (hover) | `vars.color.primaryHover` | Consistent with other link patterns |
| Anchor color (visited) | `vars.color.textSecondary` | Settled appearance for visited PRs |
| Icon color (decorative) | `vars.color.textMuted` | GitPullRequest, Globe, GitCommit icons |
| SHA chip background | `vars.color.surfaceSubtle` | Subtle code chip |
| SHA chip text | `vars.color.textPrimary` | Readable monospace |
| SHA font family | `vars.font.mono` | Code content |
| SHA chip border radius | `vars.radii.sm` | Consistent with other code chips |
| SHA chip padding | `vars.space[1]` / `vars.space[2]` | v / h |
| Copy button color | `vars.color.textMuted` | Matches icon-only button pattern |
| Copy success color | `vars.color.success` | Checkmark confirmation |
| Disclosure toggle bg (hover) | `vars.color.hoverBackground` | Row hover affordance |
| Disclosure toggle text | `vars.color.textSecondary` | "Show N external URLs" |
| Empty state text | `vars.color.textSecondary` | Muted informational message |
| Empty state icon | `vars.color.textMuted` | De-emphasized state icon |
| New artifact highlight | `vars.color.accentBg` → transparent | One-shot CSS keyframe animation |
| Scan status footer bg | `vars.color.surfaceMuted` | Recessed footer |
| Scan status footer border | `vars.color.borderSubtle` | Separator from content |
| Scan status text | `vars.color.textMuted` | Metadata, lowest priority |
| Scan status font size | `vars.fontSize.xs` | Footer annotation scale |
| Skeleton background | `vars.color.surfaceMuted` | Pulse shimmer base |
| Tab badge bg | `vars.color.primary` | Matches approval badge pattern |
| Tab badge text | `vars.color.primaryText` | Contrast on primary |
| Tab badge border radius | `vars.radii.full` | Pill shape |
| Tab badge font | `vars.fontSize.xs`, `vars.fontWeight.semibold` | Compact but legible |
| PR status open (future) | `vars.color.successBg` / `vars.color.success` | Green = open |
| PR status merged (future) | `vars.color.accentBg` / `vars.color.primary` | Purple = merged |
| PR status draft (future) | `vars.color.surfaceMuted` / `vars.color.textMuted` | Gray = draft |
| PR status closed (future) | `vars.color.errorBg` / `vars.color.error` | Red = closed |

---

## Implementation Notes

### Bug 1: Wrong Token Name in CSS Draft

The plan's `ArtifactsTab.css.ts` draft uses `vars.color.actionPrimary`:

```ts
export const link = style({
  color: vars.color.actionPrimary,  // BUG: this token does not exist
```

This token is not defined in `theme-contract.css.ts`. The correct token is `vars.color.primary`. The engineer implementing Task 4.1.1b must replace `vars.color.actionPrimary` with `vars.color.primary` in the `link` style.

### Bug 2: Wrong Import Path in CSS Draft

The plan's `ArtifactsTab.css.ts` draft imports from:

```ts
import { vars } from "../../styles/theme.css";
```

The actual vanilla-extract theme contract file is `theme-contract.css.ts`, not `theme.css`. The correct import is:

```ts
import { vars } from "../../styles/theme-contract.css";
```

`theme.css.ts` exports the concrete theme values assigned at build time; `theme-contract.css.ts` exports the `vars` contract object that `.css.ts` files must reference. Importing from `theme.css` would not provide `vars` and would likely produce a runtime error or type error.

### Proto Changes Required (not client-side)

The following design elements require changes to the proto layer before the frontend can implement them:

- **PR status badges**: Requires a `map<string, string> pr_statuses = 5` field (or a repeated `PRStatus` message) in `SessionArtifacts`. This is a future enhancement.
- **Commit messages**: Requires a `map<string, string> commit_messages = 6` field in `SessionArtifacts`. Future enhancement.

The following design elements are fully client-side and require no proto changes:

- External URL disclosure toggle
- Copy-to-clipboard for SHAs
- Short SHA display (7-char slice of the 40-char value already in the proto)
- Real-time highlight animation (triggered by React re-render from WatchSessions push)
- Skeleton loading state (driven by `session.artifacts === undefined`)
- Scan status footer with timestamp formatting
- Tab badge count (computed from existing proto array lengths)
- Owner/repo extraction for commit links (derived from `session.githubPrUrl` already in the Session proto)

### Commit Link Owner/Repo Derivation

The `Session` proto has a `githubPrUrl` field (field 41 based on existing usage). Extract owner/repo using:

```ts
function extractOwnerRepo(session: Session): string | null {
  const m = session.githubPrUrl?.match(/github\.com\/([\w.-]+\/[\w.-]+)\/pull\/\d+/);
  return m ? m[1] : null;
}
```

Pass this to `ArtifactsTab` as a derived prop or compute it inside the component. Do not add a new proto field for owner/repo — it can always be derived from an existing PR URL.

### Tab Always Visible

Per the plan's acceptance criteria, the Artifacts tab is always present in the tab strip — it does not conditionally appear only when artifacts exist. The empty state handles the no-data case. This is the correct decision: hiding a tab until data exists violates Nielsen's heuristic of predictability and consistency. Users learn the tab bar layout once; dynamically appearing tabs are disorienting.

---

## UX Readiness Gate

Use this checklist to validate the implementation before marking Phase 4 complete:

### Visual

- [ ] PR links render as `owner/repo#N` (not raw URLs)
- [ ] Commit SHAs display as 7-char monospace chips with correct background
- [ ] External URLs are collapsed by default with disclosure toggle
- [ ] Section count badges appear when sections are non-empty
- [ ] Section count badges are absent when sections are empty
- [ ] Empty state (state 1: pending) shows correct icon and copy
- [ ] Empty state (state 2: scanned, empty) shows correct icon and copy
- [ ] No hardcoded hex colors in `ArtifactsTab.css.ts`
- [ ] No `vars.color.actionPrimary` reference (non-existent token) in `ArtifactsTab.css.ts`
- [ ] Import path is `../../styles/theme-contract.css` (not `theme.css`)

### Behavior

- [ ] Copy button appears for commit SHAs when owner/repo is unknown
- [ ] Copy button shows a `Check` icon for 1.5 s on success
- [ ] External URL `title` attribute contains the full URL (not the truncated display)
- [ ] Disclosure toggle state is `aria-expanded` correct (true/false)
- [ ] Real-time new artifacts receive `data-new="true"` attribute and highlight animation
- [ ] `data-new` attribute removed after animation completes (no permanent attribute pollution)
- [ ] Skeleton visible only on initial mount when `session.artifacts` is undefined
- [ ] Skeleton dismissed within 3 s even if `session.artifacts` remains undefined (transitions to empty state)
- [ ] Scan status footer shows correct copy per timestamp age
- [ ] "Scanning now..." indicator appears when `lastScannedAt` is within 30 s

### Accessibility

- [ ] All anchors have visible `:focus` styles (browser default or custom via `outline`)
- [ ] External URL anchors have "(opens in new tab)" text for screen readers
- [ ] Copy button has `aria-label="Copy full SHA {sha}"`
- [ ] Section elements have `aria-labelledby` pointing to their `<h3>` id
- [ ] Disclosure toggle is a `<button>` element, not a `<div>`
- [ ] Disclosure toggle has `aria-expanded` set correctly
- [ ] Scan status footer has `aria-live="polite"` and `role="status"`
- [ ] Empty state container has `role="status"`

### Layout

- [ ] Root container has `height: "100%"` and `overflowY: "auto"`
- [ ] Component renders correctly at 400 px width (narrow panel)
- [ ] Component renders correctly at full mobile width (375 px)
- [ ] Sticky footer does not overlap content when panel is short

### Tests

- [ ] `ArtifactsTab_should_showPendingState_When_artifactsIsUndefined`
- [ ] `ArtifactsTab_should_showEmptyState_When_artifactsIsDefinedButEmpty`
- [ ] `ArtifactsTab_should_renderPRLinks_When_artifactsHasPRURLs`
- [ ] `ArtifactsTab_should_renderShortSHA_When_artifactsHasCommitSHAs`
- [ ] `ArtifactsTab_should_collapseExternalURLs_By_default`
- [ ] `ArtifactsTab_should_showDisclosureToggle_When_externalUrlsExist`
- [ ] `ArtifactsTab_should_truncateExternalURLDisplay_When_URLExceeds60Chars` (CSS-only; verify `title` attr has full URL)
- [ ] `ArtifactsTab_should_showCopyButton_When_ownerRepoIsUnknown`
- [ ] `ArtifactsTab_should_showTabBadge_When_prOrCommitCountIsPositive`
- [ ] `ArtifactsTab_should_excludeExternalUrls_From_tabBadgeCount`
