# UX Research: session-list-density (deep pass)

Feeds the existing ux-expert design spec (row layout, column demotion, container-query
breakpoint, touch targets). This pass stress-tests the open questions — mainly the
"smart middle-truncation" heuristic and the accessibility of tooltip-only supplementary
data — against external prior art. Not redoing the layout spec.

## 1. Comparable patterns: how others truncate long paths

No tool truncates fixed-N-characters-from-the-end blindly; the mature pattern is
**segment-aware, not character-aware**, and every serious implementation keeps the
*first* and *last* segment intact and sacrifices the middle:

- **macOS Finder path bar**: keeps the root and current-folder segments visible and
  *compacts* (shrinks icon+label width) the middle ancestors first, only resorting to
  illegible-but-present labels under real pressure — it does **not** use an ellipsis
  glyph for this (that idiom is a Windows `BreadcrumbBar` convention, where an ellipsis
  literally replaces the leftmost nodes). Hovering/dragging over a compacted segment
  temporarily expands it. ([Apple discussion](https://discussions.apple.com/thread/253266889), [path-bar reimplementation notes](https://github.com/russellgordon/plantoir/issues/148))
- **VS Code**: two separate mechanisms, neither of which does true middle-ellipsis on
  the tab label itself. Tabs truncate on the right (left-aligned, right-truncated),
  which is called out as a live usability bug when multiple open files share a long
  prefix — they become indistinguishable. The actual fix VS Code and Zed converged on
  is **not** truncation at all: append the immediate *parent folder name* as a suffix
  disambiguator once two tab titles collide, rather than trying to compress the shared
  prefix. Breadcrumbs (separate feature, `breadcrumbs.filePath: on|off|last`) is the
  tool for compressing a path to just its last segment when the full chain isn't
  needed. ([tab-truncation issue](https://github.com/stablyai/orca/issues/20422), [older VS Code smart-truncation proposal](https://github.com/Microsoft/vscode/issues/12040))
- **Shell prompts (fish, starship, powerlevel10k, zsh `%~`)**: the standard algorithm
  is **per-component elision, not middle-ellipsis of the whole string** — collapse every
  path *component* except the last N to a short fixed-length abbreviation (fish's
  `prompt_pwd`: default 1 char per component, last component always full), rather than
  collapsing whole segments to "…". This is the closest existing convention to what the
  requirements doc's "collapse repetitive/opaque middle segments" is reaching for, and
  it generalizes cleanly: for a session path like
  `backlog/triage-dc2c9bba-4734-475e-9a12-.../src`, the opaque UUID segment is exactly
  the kind of "component" a `prompt_pwd`-style rule would abbreviate to a fixed-length
  stub, while `backlog/triage` (semantic) and `src` (last component) stay full. Fish
  also does a *second*, unrelated truncation for when the whole rendered line overflows
  the terminal — trims from the **left** with an ellipsis prefix, on the theory that
  parent-most directories are least interesting. That's the deepest-nesting-first
  bias worth borrowing if a path has no single obviously-opaque segment. ([fish prompt_pwd docs](https://fishshell.com/docs/current/cmds/prompt_pwd.html), [line-truncation rationale](https://github.com/fish-shell/fish-shell/issues/13007))
- **Breadcrumb components generally** (ServiceNow Horizon, shadcn/ui, GitHub's own file
  browser): the converged pattern is "collapse the **middle crumbs** into a single `…`
  that expands to a dropdown/menu on click, never truncate the root or the current
  page." ServiceNow's Horizon design system literally exposes this as a config: three
  named strategies — `collapse` (whole middle segments → dropdown), `truncate-collapse`
  (truncate each visible label's *text* first, then collapse), and `wrap`. This is a
  useful naming vocabulary to reuse in the implementation plan instead of inventing new
  terms. ([ServiceNow Horizon breadcrumbs](https://horizon.servicenow.com/workspace/components/now-breadcrumbs), [breadcrumb pattern library](https://uxpatterns.dev/patterns/navigation/breadcrumb))
- **CSS middle-ellipsis mechanics** (if implementing the visual truncation directly
  rather than pre-computing the string): the standard trick is two `<span>`s (head/tail)
  with the container set `direction: rtl` so the browser's native `text-overflow:
  ellipsis` (which always clips at the logical "end") lands visually in the middle. This
  has real bidi edge cases with mixed-direction text and is generally considered a hack
  — **doing the truncation in JS on the string itself (as the requirements doc already
  proposes) is more robust** than the CSS trick, and is what VS Code's own smart-
  truncation proposal recommended for exactly this reason. ([CSS-Tricks](https://css-tricks.com/snippets/css/truncate-string-with-ellipsis/), [CSSWG mixed-direction truncation issue](https://github.com/w3c/csswg-drafts/issues/2125))

**Synthesis for the open question ("exact rule for opaque segments")**: there is no
single industry-standard regex for "opaque," but the shell-prompt precedent suggests a
simpler framing than pattern-matching UUIDs specifically: treat any path *segment*
above a length threshold (shells default to abbreviating anything beyond 1 char) or
matching a **high-entropy heuristic** (long run of hex/base36 chars, optionally hyphen-
delimited — i.e., what a UUID/short-hash *looks like* structurally, not a UUID-specific
regex) as elidable, and always preserve: the leading semantic segment(s) up to the
opaque one, and the trailing segment(s) after it (branch name, filename). This also
future-proofs against non-UUID opaque IDs (git short-hashes, workspace hash
directories) without needing a UUID-specific regex, and mirrors what `nf.cluster`-style
identifiers or Netflix's own workspace-hash directories (`~/.stapler-squad/workspaces/<hash>/worktrees/<uuid>`)
already look like in this very session's cwd.

## 2. Mental models: what actually disambiguates two similar rows

Working from the screenshot examples already known (`backlog/triage-dc2c9bba-...`,
`pr-424-compute-nop-18c993e1e9402c...`) — these are **machine-generated slugs**, not
names a person chose to be memorable. That reframes the disambiguation question: the
UUID/hash suffix is exactly the kind of low-information "opaque" segment section 1
identifies for elision, and it is almost never what the user's eye is scanning *for*.

External precedent (git-branch UI truncation debates) converges on the same finding
independently: users overwhelmingly report that truncating away the **prefix**
(category/ticket-tag) is what breaks their scanning, because the categorical prefix
(`feature/JIRA-12345/...`, or here `backlog/`, `pr-424-`) is the human-meaningful part,
while the trailing hash is the machine-meaningful part they only need on demand (to
correlate with a URL or worktree path). One GitHub Desktop issue explicitly requested
switching truncation direction from left to right *because* left-truncation was hiding
the ticket ID users needed to recognize their own branch. ([GitHub Desktop truncation-direction issue](https://github.com/starship/starship/issues/6664))

For this app specifically, the disambiguating hierarchy a user needs at a glance is
likely, in priority order:
1. **Category/type prefix** (`backlog/`, `pr-424-`) — tells them *what kind* of session
   this is without reading further.
2. **The human-chosen slug fragment** if one exists (`triage`, `compute-nop`) — the
   actual semantic content.
3. **Status/recency signal** (already covered by the existing design spec's row
   layout — status dot, timestamp) — this is what answers "resume or ignore," which is
   the actual job (see §5), and it's orthogonal to the path-truncation problem, not
   solved by it.
4. **The opaque ID suffix** — lowest priority for scanning, but must remain
   *available* (via tooltip/full accessible name) because it's the correlation key back
   to a PR number, worktree directory, or backlog item UUID the user may need to paste
   into a `gh` command or file path.

This means the smart-truncation rule in §1 should be tuned to *never* elide segment 1
(category prefix) or a human-readable slug word if one is present between the prefix
and the UUID — only the UUID/hash run itself.

## 3. Accessibility: is `title`-only sufficient?

Direct answer to the open question: **`title` is not sufficient**, but not for the
reason the requirements doc's SC 1.4.13 framing implies. Checked against
[W3C's own Understanding page for SC 1.4.13](https://www.w3.org/WAI/WCAG22/Understanding/content-on-hover-or-focus.html)
and corroborating summaries ([WCAG.com](https://www.wcag.com/authors/1-4-13-content-on-hover-or-focus/), [Deque](https://dequeuniversity.com/resources/wcag2.1/1.4.13-content-on-hover-or-focus)):

- The native browser tooltip produced by the HTML `title` attribute is **explicitly
  exempted** from SC 1.4.13 itself — the spec text calls out "browser tooltips created
  through use of the HTML title attribute" as user-agent-controlled content the
  criterion does not apply to. So a pure `title` attribute is not a 1.4.13 *violation*.
- It fails on **other, more basic criteria** instead: `title` tooltips are not
  keyboard-focus-triggered in most browsers (fails SC 2.1.1 Keyboard / 1.4.13's own
  "should work on focus too" spirit for anything that also needs focus-visibility),
  are invisible to touch users entirely (this app is explicitly used on mobile per the
  requirements doc's constraints — `title` provides **zero** accessible path to the
  agent-program/memory data on a touch device, not just a degraded one), and are not
  dismissible/hoverable/persistent by design (can't be scrolled into, vanish on any
  pointer move).
- **The real fix, and the thing to flag against the existing spec**: since the current
  approach is `title`-attribute-only and the app is explicitly used on mobile, that data
  (agent program, memory RSS) is likely **already unreachable on the touch surface
  today**, independent of anything the density redesign changes. If the redesign's
  column-demotion plan moves *more* data behind `title`-only tooltips, it compounds an
  existing mobile-accessibility gap rather than introducing a new one. Recommend the
  demoted-column data use a real disclosure pattern (e.g. `aria-describedby` pointing at
  a `role="tooltip"` element shown on tap/focus, exactly the pattern Deque and
  WCAG.com's guidance recommend as the `title` replacement) rather than perpetuating
  `title`.
- For the **middle-truncated name/path text** specifically (not the tooltip question):
  the applicable rule is simpler and not about hover content at all — the *element's
  accessible name* must be the full, untruncated string (e.g. via `aria-label` on the
  row, with the visually-truncated head/tail spans marked `aria-hidden="true"`, or by
  putting the full string in `title` purely as a supplementary — not sole — channel).
  This is a basic "don't let visual truncation lie to assistive tech" requirement
  (WCAG 1.3.1/4.1.2-adjacent, not 1.4.13), independent of the mobile-tooltip finding
  above, and should be called out explicitly in the implementation plan as its own
  acceptance criterion: **screen reader announces the full name, not the truncated
  visual string.**

## 4. Error/edge case: the no-break-point token

For a single 80-character token with no natural break (a raw UUID or hash run with no
hyphens), the researched consensus ([Zander Martineau's overview](https://zander.wtf/blog/css-text-wrapping/), [LogRocket guide](https://blog.logrocket.com/guide-css-word-wrap-overflow-wrap-word-break/)):

- `word-break: break-all` is broadly panned — it breaks *any* text at any character
  whenever a line is full, not just the unbreakable token, so if it's applied container-
  wide it will also mangle ordinary prose elsewhere in the row. Multiple sources found
  "nothing good to say about" it outside CJK text.
- `overflow-wrap: anywhere` (not the deprecated `overflow-wrap: break-word` — that's a
  compatibility shim with different `min-content` sizing behavior) is the correct tool:
  it's a **last resort** — normal words wrap at spaces/hyphens as usual, and only a
  token that's *actually* longer than the available width gets broken, character-by-
  character, as a fallback. Critically for a CSS Grid/flex row layout (which this
  redesign already uses per the requirements doc), `anywhere` also fixes `min-content`
  sizing so the unbreakable token can't blow out the grid track width — `break-word`
  can, because its min-content is still the full unbroken word.
- **Practical recommendation for this feature**: given the smart-truncation approach
  in §1 already collapses opaque high-entropy segments before render, hitting the
  "single 80-char token with zero natural break" case in practice mainly happens if
  truncation itself fails (JS error, unexpected data shape) — so `overflow-wrap:
  anywhere` should be the CSS **safety net** underneath the truncation logic, not the
  primary truncation mechanism. Don't rely on CSS wrapping to solve a problem the JS
  truncation should already have solved.

## 5. Jobs-to-be-done: what re-prioritizes line 1 vs. line 2/tooltip

The job a session row does is **"decide in under a second: resume this, or skip it"** —
not "read the full identity of this session." That reframes what belongs on the always-
visible line vs. what can be demoted:

- **Line 1 (always visible, never truncated away)**: status indicator (running/idle/
  needs-attention), the category prefix + human slug fragment (§2's priority 1–2), and
  recency/activity signal. These are the *decision* inputs.
  This is already what the prior ux-expert spec's row layout targets — this research
  pass corroborates it rather than changing it.
- **Line 2 / demoted-but-reachable (tooltip or secondary line, per §3's accessible
  pattern, not `title`-only)**: agent program glyph, memory/RSS — these answer "is
  something wrong with this session's *process*," a different, lower-frequency job
  (debugging a stuck session) than the resume/skip glance. Correctly demoted per the
  existing spec.
- **Opaque ID suffix (§1/§2)**: belongs *only* in the accessible name / on-demand
  disclosure, never on line 1 — it answers a third, even rarer job ("I need to correlate
  this row with a PR number or file path to paste somewhere"), which is exactly why
  users complained about branch-name truncation hiding the *category prefix* rather
  than the hash in the git-branch research (§2) — the hash was never the thing being
  scanned for.

This suggests one refinement worth feeding back into the implementation plan: the
smart-truncation rule and the line1/line2 demotion should use the **same underlying
"what's the job" priority list** (status/recency > category+slug > opaque ID), rather
than being designed as two independent mechanisms — a session row that already knows
which path segments are "opaque" (for truncation) has, for free, the same signal for
which text is safe to drop off line 1 entirely under container-query narrow width,
rather than just shrinking font size or wrapping.

## Sources
- [Apple Support: Finder path bar truncation](https://discussions.apple.com/thread/253266889)
- [Finder path-bar behavior reimplementation notes](https://github.com/russellgordon/plantoir/issues/148)
- [VS Code: long shared-prefix tab titles indistinguishable](https://github.com/stablyai/orca/issues/20422)
- [VS Code: smart middle-truncation proposal](https://github.com/Microsoft/vscode/issues/12040)
- [fish shell prompt_pwd docs](https://fishshell.com/docs/current/cmds/prompt_pwd.html)
- [fish shell prompt-line truncation rationale](https://github.com/fish-shell/fish-shell/issues/13007)
- [ServiceNow Horizon breadcrumbs (collapse/truncate-collapse/wrap)](https://horizon.servicenow.com/workspace/components/now-breadcrumbs)
- [UX Patterns for Developers: breadcrumb pattern](https://uxpatterns.dev/patterns/navigation/breadcrumb)
- [CSS-Tricks: truncate string with ellipsis](https://css-tricks.com/snippets/css/truncate-string-with-ellipsis/)
- [CSSWG: mixed-direction truncation inconsistency](https://github.com/w3c/csswg-drafts/issues/2125)
- [GitHub Desktop: branch-name truncation direction issue](https://github.com/starship/starship/issues/6664)
- [W3C Understanding SC 1.4.13](https://www.w3.org/WAI/WCAG22/Understanding/content-on-hover-or-focus.html)
- [WCAG.com SC 1.4.13 summary](https://www.wcag.com/authors/1-4-13-content-on-hover-or-focus/)
- [Deque University SC 1.4.13](https://dequeuniversity.com/resources/wcag2.1/1.4.13-content-on-hover-or-focus)
- [Zander Martineau: text-wrap/overflow-wrap/word-break overview](https://zander.wtf/blog/css-text-wrapping/)
- [LogRocket: CSS word-wrap/overflow-wrap/word-break guide](https://blog.logrocket.com/guide-css-word-wrap-overflow-wrap-word-break/)
