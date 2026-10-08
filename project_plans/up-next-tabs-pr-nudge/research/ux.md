# UX Research: Up Next tabs + PR nudge

Confidence labels: VERIFIED = opened in this repo; INFERRED = from general knowledge of the products/specs (no web lookup done in this pass).

## 1. Comparable patterns (INFERRED)

| Product | Pattern | Takeaway for us |
|---|---|---|
| GitHub "Pull requests" dashboard | Segmented list: Created / Assigned / Mentioned / Review requests; per-row CI icon, review state, comment count | Row-level status glyphs + counts are the expected PR vocabulary. Keep the existing CI/review chips. |
| GitHub Notifications | Inbox with filters, "Done" removes item | Dismiss = "handled", not delete. We already have dismiss/snooze on worktrees. |
| Graphite PR inbox | Sections by actionability ("Needs your attention", "Waiting on others", "Approved") | Group by "needs action" rather than by repo; a failing-CI PR is the primary signal. Supports a sort of failing-first inside the PRs tab. |
| Linear inbox | Keyboard-first triage (j/k, e), snooze, undo toast after each action | Model for the nudge confirmation: instant action, toast with Undo or at least a visible "sent" state. |
| Tab + badge UIs (Gmail, GitHub repo tabs "Pull requests 12") | Count badge on the tab label; badge reflects *attention* count, not total | Badge = PRs needing attention (CI failing / changes requested / unresolved comments); show total in the tab content header. Zero hides the badge (no "0" noise). |

## 2. Mental model

User thinks in terms of "what is blocked on me or on an agent", not "what data source is this". Today's scroll page is organized by data source (Stuck / Worktrees / Queue). Tabs by source are fine because the user chose them, but the PR tab must be organized by *actionability*: PR -> its sessions -> a fix button. The PR card is the join point between GitHub state and local agent sessions; a card with failing CI and a linked session should read as "tell the session to fix this".

Implication: the Nudge button is shown only when there is something to say (failing checks or unresolved comments). On green/approved PRs, no nudge button (avoid a disabled-everywhere button wall).

## 3. Accessibility and reuse

Existing tab implementations (VERIFIED by grep of `web-app/src`):
- `components/analytics/EscapeAnalyticsPage.tsx` (lines ~96-145): the most complete WAI-ARIA tabs example: `role="tablist"` + `aria-label`, `role="tab"` with `aria-selected`, `aria-controls`, roving `tabIndex` (0 selected, -1 others), `onKeyDown` handling ArrowLeft/ArrowRight, `role="tabpanel"`. Best template to extract.
- `components/sessions/SessionDetailView.tsx` (~877-935): tablist with Arrow key handling via DOM querySelectorAll; panels with `aria-labelledby`, but no roving tabindex noted.
- `app/logs/page.tsx` (~122): `role="tab"` + `aria-selected` only; incomplete (no keys/controls).
- `components/pane/MobilePaneTabStrip.tsx`, `components/window/WindowTabStrip.tsx`: other strips, different purpose (window/pane switching).
- No shared `Tabs` primitive exists. Recommendation: extract a small reusable `TabList`/`useRovingTabs` from EscapeAnalyticsPage rather than writing a fifth copy (jscpd threshold is 0.12%, copy-pasted key handlers would count).

WAI-ARIA Authoring Practices tabs pattern (INFERRED from the spec; confirm against APG):
- Tab = `role="tab"`, `aria-selected`, `aria-controls` -> panel id; panel = `role="tabpanel"`, `aria-labelledby` -> tab id.
- Roving tabindex; Left/Right move focus (wrap); Home/End jump to first/last. EscapeAnalyticsPage handles only Left/Right; add Home/End.
- Automatic activation (focus selects) is OK because panels are cheap to switch (data already loaded by hooks). If panel switch triggers heavy render/fetch, use manual activation (Enter/Space).
- Badge counts must be in the accessible name: `aria-label="PRs, 3 need attention"` or visually-hidden text; do not rely on color.
- Badge changes are not announced by default; do not add `aria-live` to the tablist (noisy, polling updates). Announce only the nudge result.
- Hidden panels: render only the active panel (or `hidden` on others) so Playwright `data-testid` locators and screen readers don't see stale content. Note: unmounting resets in-section state (filters, StuckItems expand); either keep filter state in `UnfinishedTab` (already is for worktree filter) or mount-all with `hidden`.
- Nudge button: native `<button>`, accessible name includes PR number and target session ("Nudge session X to fix CI on PR #123"). A session picker with multiple sessions should be a native `<select>` or a menu button using the APG menu-button pattern; split-button (primary = default session, caret = choose) keeps one-click.
- Result announcement: `role="status"` (polite) region for "Nudge sent to <session>", `role="alert"` for failure.

State/URL (requirements rabbit hole): precedence should be `?item=` (forces Stuck) > `?tab=` > localStorage > default PRs. Write `?tab=` with `router.replace` (not push) so Back doesn't walk through tab changes. Read localStorage in an effect to avoid SSR hydration mismatch (Next.js app router) and render the default until hydrated; accept a one-frame flash or render a skeleton panel.

## 4. Error and edge states

| State | Recommended behavior |
|---|---|
| No GitHub connection | PRs tab shows the existing `DeviceAuthBanner` ("Connect GitHub") as the panel's empty state, not a spinner. Tab badge hidden. Other tabs unaffected. Because PRs is the default tab, first-run users hit this; make it the clear call to action. |
| GitHub loading ("Connecting to GitHub...") | Skeleton in panel; no badge until loaded; do not block tab switching. |
| GitHub error / rate limited | Inline error with Retry; keep last-known PR list (stale-while-error) with "Last updated N min ago". |
| No open PRs | Empty state "No open PRs" (distinct from "not connected"). |
| PR with no linked session | Show "+ Session" seeded with the fix prompt when there is something to fix; otherwise plain "+ Session". |
| PR with multiple sessions | List all as chips/links; nudge target defaults to the most recently active; the split-button caret chooses another. |
| Session paused / not running | Disable nudge for that target with a reason tooltip ("Session paused. Resume first") or offer "Resume and nudge". Reuse `diagnose_nudge_session` logic (VERIFIED it exists as an MCP tool in this repo; server-side safeguards not yet read here). |
| Session busy (mid-turn) | Prefer queue/steer semantics rather than refusal; show "Session is working; nudge will be delivered when it is idle" if the steering primitive queues, otherwise warn. Needs backend answer (see open questions). |
| Nudge sent | Button transitions to "Nudged (just now)" for a short period, then returns to normal; show toast with link "Open session". Prevent double-send: disable for N seconds / until PR state changes (nudging twice burns agent tokens). |
| Undo | A sent keystroke cannot be recalled once delivered to a tmux session. Offer a short send-delay with Undo (e.g. 5s toast) only if cheap; otherwise state in docs that nudge is not undoable and rely on the double-send guard plus a visible record in the session transcript. Recommendation: no undo, confirmation + guard (matches the requirements' "one click" choice). |
| Nudge failure | `role="alert"` toast with reason from server; button re-enabled. |
| Stale data | If CI went green between poll and click, server should rebuild the prompt from fresh data or return "nothing to fix"; UI shows that rather than sending an empty nudge. |
| Badge vs data mismatch | Badge computed from the same hook data as the list (single source) to avoid count drift. Keep `UnfinishedNavBadge` semantics unchanged unless decided otherwise (open question in requirements). |
| Deep link `?item=` for a stuck item that no longer exists | Stuck tab opens with no match; show "Item not found" instead of an empty tab. |

## 5. Jobs to be done

1. "When I open Up Next, I want to see which of my PRs need attention right now, so I can unblock them before doing anything else." -> PRs default tab, failing-first, badge.
2. "When a PR's CI fails or reviewers comment, I want the agent that owns it to deal with it without me retyping context." -> one-click nudge with generated prompt.
3. "When several sessions touch one PR, I want to choose which one acts, with a sensible default." -> default most recently active, picker.
4. "When I return to the page, I want to land where I left off." -> persisted tab, URL param.
5. "When I have no session for a failing PR, I want to start one already knowing what to fix." -> "+ Session" seeded.
6. (Secondary) "I want to trust that automation did not send duplicate or stale instructions." -> sent state, double-send guard, fresh prompt.

## Recommendations summary
- Extract a shared accessible Tabs component from `EscapeAnalyticsPage.tsx`; add Home/End and `aria-controls`/ids; render with `data-testid` per tab and panel.
- Badge = attention count, hidden at zero, included in accessible name.
- Nudge = split button, shown only when actionable, status toast + double-send guard; no undo.
- Define the URL/localStorage precedence explicitly and test the `?item=` interaction.

## Open questions for other research tracks
- Does the steering primitive queue input for a busy session or inject immediately? (affects "session busy" copy)
- What does `diagnose_nudge_session` check, and can the nudge RPC reuse it?
- Does `useGitHubPRs` expose last-updated/stale state for the stale-while-error behavior?
