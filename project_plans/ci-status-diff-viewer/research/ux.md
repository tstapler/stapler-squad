# UX Research: CI Status Badge in Diff Viewer

Scope: badge visual/interaction design + "block approval when CI red" rule UX, for
`project_plans/ci-status-diff-viewer/requirements.md`.

## 1. Comparable UX patterns

| Product | Pattern |
|---|---|
| GitHub PR checks summary | Single rolled-up pill (`✓ All checks have passed` / `✗ Some checks were not successful` / `● N pending`) in the PR merge box. Click expands an inline dropdown listing each check (name, icon, duration, "Details" link to the run). The rollup is always visible without a click; per-check detail is opt-in. |
| GitLab MR pipeline widget | Similar rollup badge + expandable stage list (build/test/deploy), each stage a colored icon; clicking a stage links to its job log. |
| CircleCI/Buildkite status badges (README-embedded) | Static image badge (passing/failing/no status), click-through to the build page. No inline expansion — these are single-state, link-only, because they're embedded outside the product's own UI. |

**What transfers to stapler-squad's diff viewer:**
- **Rollup-first, detail-on-demand** is the right shape: the badge itself should show one summary state (not one badge per check), matching GitHub's own rollup — the diff viewer is a review surface, not a CI dashboard (non-goal in requirements.md confirms no in-app log tailing).
- A **click → link out to the GitHub Actions run** (same as CircleCI/Buildkite's link-only badge, and GitHub's "Details" link) satisfies acceptance criterion 2 without building an expandable per-check list — that richer interaction (GitHub/GitLab's expand-in-place) is more than this feature's non-goals call for. Keep it as a future enhancement if reviewers ask for per-check breakdown.
- Existing codebase precedent already does a rollup, not per-check: `GitHubBadge.tsx` (`web-app/src/components/sessions/GitHubBadge.tsx:116`) puts `checkConclusion` into the tooltip string (`CI: ${checkConclusion}`) rather than a separate UI element — the CI badge should follow the same "rollup value, tooltip for detail" convention already established for PR status, not invent a new interaction shape.

## 2. User mental model — glance-test states

Reviewer needs "is it safe to approve" answered in <1s. That requires each state to be
visually and textually distinct at a glance, not just on hover:

| State | When | Must convey |
|---|---|---|
| Passing | all required checks green | safe to approve |
| Failing | ≥1 required check red | do not approve without looking |
| Pending | checks running/queued | wait, not yet safe, not yet unsafe |
| No checks / no PR | session has no associated PR, or PR has no CI configured | not applicable — silently absent, not a warning (per acceptance criterion 7: "no CI badge, not an error state") |
| Error fetching | GitHub API call failed/timed out/rate-limited | **unknown**, distinct from both "no checks" and "failing" — the reviewer must not read this as either "safe" or "red" |

This 5-way split is the crux of the feature: 3 of these states (no-checks / pending / error)
are all "not green," but only one of them ("failing") should look alarming, and "no checks"
vs "error fetching" must not collapse into the same gray badge (requirements point 4 / this
doc §4).

## 3. Component reuse and accessibility

**Existing components to reuse, not duplicate:**
- `web-app/src/components/ui/Badge.tsx` + `Badge.css.ts` — generic `intent`/`size` variant badge. Thin wrapper; usable as a base but has no icon slot.
- `web-app/src/components/sessions/GitHubBadge.tsx` — closest precedent. It already has a `checkConclusion` prop (`GitHubBadge.tsx:29`) it currently only surfaces in the tooltip (`GitHubBadge.tsx:116`), and a `priorityClass`/`priorityLabel` switch pattern (`GitHubBadge.tsx:36-62`) mapping a status enum to a CSS class + human label, exactly the shape a CI-status badge needs. **Recommendation:** either (a) extend `GitHubBadge` to render `checkConclusion` as its own colored segment instead of tooltip-only, or (b) add a small sibling `CIStatusBadge` component following the identical `priorityClass`/`priorityLabel`-switch pattern — do not invent a third badge idiom.
- `web-app/src/components/sessions/StatusBadge.tsx` — the pattern to copy for **icon + text + color, never color alone**: every variant pairs an emoji icon (`⚠️`, `✅`, `❌`, `⏰`) with a text label and `role="status"` + `aria-label` (`StatusBadge.tsx:94-104`). This is the accessibility template — reuse it exactly for CI states (e.g. `✅ Passing`, `❌ Failing`, `⏳ Pending`, `⚠️ Unavailable`), not icon-only or color-only chips.
- `web-app/src/components/sessions/GitHubBadge.css.ts` already defines the color variants needed: `prBadgeReady` (green, `vars.color.success`), `prBadgeBlocking`/`prBadgeError` (red, `vars.color.error`), `prBadgePending` (amber, `vars.color.warning`), `prBadgeUnknown` (gray, `vars.color.surfaceSubtle`/`textSecondary`) — these map almost 1:1 onto passing/failing/pending/error-or-no-checks. Reuse these tokens via `vars.color.success/error/warning/textMuted` (confirmed defined in `web-app/src/styles/theme-contract.css.ts:41-50`), per `.claude/rules/css-architecture.md` — never hardcode hex.

**WCAG / color-blindness:**
- Color alone must never carry the state (WCAG 1.4.1). `StatusBadge.tsx` and `GitHubBadge.tsx` both already avoid this by pairing an icon glyph + text label with every color variant — the CI badge must do the same: e.g. a checkmark glyph for passing, an X/cross glyph for failing, a clock/hourglass for pending, and a distinct "unknown/error" glyph (not reused from any of the above) for fetch-error state.
- Red/green is the classic deuteranopia failure mode (~8% of men) — the shape difference (✓ vs ✗ vs ⏳ vs ⚠) is the actual accessibility guarantee here, not the hue.
- Contrast: `--success` (`#22c55e`) and `--warning` (`#f59e0b`) as background fill with white/dark text need a contrast check against `globals.css` foreground tokens — `GitHubBadge.css.ts`'s existing `prBadgeReady`/`prBadgePending` variants already ship in production with `vars.color.primaryText` on top, so reusing those exact class/token pairs (rather than picking new colors) inherits contrast that's already been through review, instead of re-litigating it.
- `title` tooltip + `aria-label` (both present in `GitHubBadge.tsx` and `StatusBadge.tsx` today) should be carried over verbatim — screen reader users get the full state description, not just the visual glyph.

## 4. Error states must look different, not converge on gray

Per requirements' explicit acceptance criterion 7 and the glance-test in §2, these three
"non-green" cases must be visually distinguishable, not all rendered as a generic gray chip:

| Case | Cause | Badge treatment |
|---|---|---|
| No CI configured | PR exists, repo has no workflow / no check runs reported | Neutral gray, label "No checks", low visual weight — informational, not a warning |
| API error/timeout/rate-limit | `gh`/GitHub API call failed | Distinct treatment — e.g. amber/gray with a **warning glyph** (⚠, not the pending clock) and label "CI status unavailable" — must not be silently swallowed into "no checks," since a rate-limited fetch could be masking a real failing state. Tooltip should say why (e.g. "GitHub API rate-limited, retrying") when the underlying error is known, so a reviewer isn't left guessing whether it's safe. |
| No PR / no branch (one-off, directory session) | session has no `GitHubPRNumber` | No badge at all (render nothing) — per acceptance criterion 7, this is not an error state and must not render any chip |

The "block approval when CI red" rule (acceptance criterion 5) must key off *only* the true
"failing" state — it must not fire on "error fetching" or "no checks," since acceptance
criterion 7 explicitly requires sessions without CI to be unaffected by the blocking rule,
and blocking on a transient API error (rather than an actual red run) would be a false
positive that erodes trust in the rule. When the approval action is blocked, the UI must show
*why* inline (not a silent disabled button) — e.g. a message near the Approve button:
"Approval blocked: CI is failing on this branch — [view run]," consistent with requirements'
"visibly explained in the UI (not a silent no-op)."

## 5. Jobs-to-be-done

- **Functional** — "Is this branch safe to merge right now?" answered without leaving the diff viewer or opening a new tab; the badge is the answer, the link-out is only for reviewers who want the run detail.
- **Emotional** — reduces the anxiety of approving code that turns out to be broken; a reviewer who trusts the badge doesn't have to separately tab over to GitHub Actions "just to check" before every approval, which is the behavior this feature is explicitly meant to replace (requirements' Problem section: "Reviewers can approve a diff that is about to fail CI").
- **Social** — the reviewer wants a visible, checkable record that they didn't approve red CI on purpose. The "block approval when CI red" rule (opt-in, acceptance criterion 5) plus its inline explanation serves this: it gives the reviewer a system-level reason ("the tool wouldn't let me") rather than requiring them to personally remember to check, which is also why the block message must be visible/explained rather than a silent no-op — a silent block removes the social cover the rule is meant to provide.

## Recommendations summary

1. Reuse `GitHubBadge.css.ts`'s existing color variants (`prBadgeReady`/`prBadgeBlocking`/`prBadgePending`/`prBadgeUnknown`) and `vars.color.success/error/warning/textMuted` tokens rather than introducing new colors.
2. Follow `StatusBadge.tsx`'s icon+text+`aria-label`+`title` pattern — never color-only.
3. Rollup badge, not per-check list; click-through link satisfies "expand to detail" without building an in-app checks dashboard (matches non-goals).
4. Five distinct states are required: passing / failing / pending / no-checks (silent, no badge per PR/no-PR case) / fetch-error (visually distinct from no-checks, must not silently degrade to gray).
5. The CI-red approval block must be keyed strictly to "failing," never to "error fetching" or "no checks," and must render a visible inline explanation next to the blocked Approve action.
