# UX research: attention inbox consolidation

Confidence labels: VERIFIED = opened/ran it this session; INFERRED = reasoning; UNVERIFIED = recalled, not opened. The live app at :8543 was NOT inspected (grounding is from source only).

## 1. Comparable patterns

| Product | Pattern | Takeaway | Source |
|---|---|---|---|
| Cursor Agents window | Sidebar grouped by Status gives a "Needs Attention" section: agents blocked on the user plus finished-but-unreviewed work, across workspaces | One bucket mixes "blocked" and "finished, unreviewed" - same as the operator's "needs me / has information for me" | https://forum.cursor.com/t/agents-approval-window/170388 (VERIFIED via search snippet) |
| Codex agents dashboard | Three states: Need input / Working (leave alone) / Ready (review) | Three-state model is the common denominator | https://hackernoon.com/the-terminal-tab-problem-codex-finally-solved-for-multi-agent-work (snippet) |
| agent-deck, marcnuri dashboard | Color-coded waiting (yellow) / idle (gray) / error (red); waiting sessions in tmux status bar | Color must be paired with text/icon (WCAG 1.4.1) | https://pkg.go.dev/github.com/asheshgoplani/agent-deck , https://blog.marcnuri.com/ai-coding-agent-dashboard |
| GitHub notifications | Inbox shows everything not Done/unsubscribed; Done removes; Save keeps; `is:done`/`is:saved`/`is:unread` queries; up to 15 saved custom filters; bulk triage | Durable until acted on, recoverable via Done view. Directly answers "missed notification is unrecoverable" | https://docs.github.com/en/enterprise-server@3.8/account-and-profile/managing-subscriptions-and-notifications-on-github/viewing-and-triaging-notifications/managing-notifications-from-your-inbox |
| Timestamps | Hybrid: relative for recent, absolute after ~7 days; absolute when users need the exact moment | Relative text + absolute in tooltip/`title` | https://making.close.com/posts/ultimate-guide-to-timestamps/ , https://zds.zurich.com/0.5.5/atoms/time__guidelines.html , https://uxmovement.com/content/absolute-vs-relative-timestamps-when-to-use-which/ |
| WCAG 2.5.8 | 24x24 CSS px target minimum or spacing | Row actions need 24px floor; 44px for mobile as project practice | https://dequeuniversity.com/rules/axe/4.8/target-size |

Not found by search (UNVERIFIED, do not cite as fact): Nielsen Norman guidance on timestamps (search returned none), Linear inbox snooze, PagerDuty incident inbox, Superhuman, Devin session list, Claude Code web/desktop session lists. From memory only: PagerDuty separates Triggered/Acknowledged/Resolved and re-pages on un-acked; Linear/Superhuman support snooze and inbox-zero. If the ADR cites these, fetch primary sources first.

Pattern synthesis:
- Every comparable collapses to a single ordered queue with 3 states: blocked-on-me, finished-for-review, working/quiet.
- Items persist until a human action (Done/ack) or the state changes. Notifications are a transient alert layer on top of that durable list, not a separate list.
- "Gone quiet" is a derived filter (no meaningful change for N minutes), not an event.

## 2. Operator mental model and jobs-to-be-done

Single operator, tens of parallel sessions, one question: "what do I look at next?" (INFERRED from requirements.md).

JTBD statements:
1. When I come back from other work, show me everything blocked on me, ranked, so I can clear it in order.
2. When an agent finishes, tell me once, and keep it findable until I look at it.
3. When a session has said nothing for a while, flag it distinctly from "waiting on me" (it may be hung or fine).
4. When I open a session, tell me what just happened and when, so I can resume context without scrolling the terminal.
5. When I step away (mobile), show only the actionable top few.

Proposed ordering (needs-me first): approval/question (tool or prose) > error/tests failing > finished-awaiting-direction > uncommitted/idle > quiet/stale. "Working" is excluded from the inbox (leave alone).

## 3. Current UI grounding (VERIFIED by grep/read of source)

- `ReviewQueuePanel.tsx` already has a visually-hidden `aria-live="polite" aria-atomic` region (l.1361), `role="status"` staleness and oldest-callout (l.1418, 1428), rows with `role="button"` + `onKeyDown` (l.1051-1057), and distinct empty states: "all done", hidden-by-filter, calm empty (l.1724-1778). Reusing this panel preserves a lot of a11y work.
- `NotificationsPage.tsx` has filter pills as `role="group"`, an empty state with emoji `aria-hidden`; no aria-live found by grep. Tray is transient/list-of-events, not state-of-sessions.
- `UnfinishedTab.tsx` ticks `setInterval(1s)` for "last scanned N seconds ago" (l.81) - a 1 Hz client tick. Fine for one label, bad if copied to every row (and a screen-reader hazard if inside a live region).
- Three different relative-time formatters exist: `lib/utils/timestamp.ts` `formatTimeAgo` ("5m ago", falls to `toLocaleDateString` after 7 d, reads `Date.now()` at render, never re-renders on its own), SessionCard's local copy (l.486: "Ns ago", "Never"), and `lib/unfinished/relativeTime.ts` `formatRelativeTime` ("5 min ago", pure with injected `now`, no absolute fallback). Inconsistent strings for the same concept.

## 4. Timestamp display (the "resets on reload" bug)

Findings:
- SessionCard shows `Updated <time>{formatTimeAgo(session.updatedAt)}` (l.1267-1272) with a tooltip of created/updated via `toLocaleString`. It also shows a separate "Active" row from `getLastActivityTimestamp` = max(`lastMeaningfulOutput`, `lastTerminalUpdate`) (`lib/session-staleness.ts:10`; `SessionCard.tsx:1030`).
- Backend `UpdatedAt` is set by `touchUpdatedAt()` on state transitions (`session/instance_state.go:36,53,103,538,571`), hibernate, actor setters (`instance_actor_setters.go:499,616`), `instance.go:2700`, rename (`instance_terminal.go:116`). So it measures "server touched the record", not "something meaningful happened". Whether reload/reconnect/hydration triggers a touch is NOT confirmed here (that belongs to the backend research); the 7 non-semantic writers make the hypothesis plausible (INFERRED).
- Client side: `formatTimeAgo` renders once with `Date.now()`; the value is derived from the server timestamp, so the client itself is not the reset source unless the server value changes. A "mount-relative" bug would only occur if some path passes a client-created timestamp (not found in these files).

Recommendation for a stable display:
1. Show two clearly named values, not one "Updated": **"Last activity"** (= last meaningful change: output, state transition into an attention state, user input) and **"Age"/"Created"** (static). Retire "Updated" from the card, keep it for sorting only if other consumers need it.
2. Source of truth is a server timestamp that is only advanced by meaningful events, ideally the latest event-log entry timestamp, so card, inbox and timeline agree and survive restart.
3. Format: relative for < 7 d ("just now", "5 min ago", "3 h ago", "2 d ago"), absolute date after (Close/Zurich hybrid). Absolute full timestamp in `title` AND, for touch (no hover), in the timeline drawer header (tooltips are not available on mobile).
4. Ticking: one shared `useNow(30_000)` hook (single interval, paused when tab hidden, re-render on `visibilitychange`) feeding the pure `formatRelativeTime(then, now)`; never per-row intervals, never inside an aria-live region. Consolidate the three formatters into the pure one (add the >7 d absolute branch). Keep `<time dateTime>` (already used).
5. Be honest about uncertainty: if the server value is missing, show "No activity recorded", not "Never" or "N/A".

## 5. Accessibility (WCAG 2.2 AA)

- New items: announce via a single polite live region ("2 sessions need you, 1 new") on count change, debounced (e.g. 2-3 s) so a burst does not flood; never `assertive` except for loss of connection. The existing visually-hidden `aria-live` in ReviewQueuePanel is the model. 4.1.3 Status Messages (AA): UNVERIFIED here, the search did not return the W3C page; confirm at https://www.w3.org/WAI/WCAG22/Understanding/status-messages.html before citing.
- Do not move focus on arrival of new items; do not reorder rows under the cursor/focus (insert a "N new - show" affordance if the list is being interacted with).
- Keyboard: roving tabindex list (`j/k` or arrows, Enter opens, `e`/Done to acknowledge, `?` help), visible focus (2.4.7, 2.4.11 focus not obscured, bottom sheet must not cover focus), no keyboard trap in the timeline drawer (Esc closes, focus returns to the opener). Rows currently use `role="button"` on a container; a nested action inside a `role="button"` is an a11y smell - prefer a link/`listitem` with a separate primary link and action buttons (axe `nested-interactive`).
- Status is never color only: icon + text label per reason (1.4.1); contrast 4.5:1 for text, 3:1 for icon/boundary (1.4.3/1.4.11).
- Target size >= 24px (2.5.8), use 44px on touch.
- Gate in repo: Axe Core on PRs touching `web-app/src/` blocks on WCAG AA violations (CLAUDE.md "UX analysis CI"). New surface needs an e2e spec with an axe pass in empty, populated and drawer-open states, and `// @feature` header, `data-testid`/ARIA locators only.
- Reduced motion: no animated pulse for new items under `prefers-reduced-motion`.
- Timeline as a semantic ordered list (`<ol>`) with `<time>` elements; reason text in words ("Finished, waiting for direction"), not icon-only.

## 6. Error and edge states

| State | Recommended behavior |
|---|---|
| Empty inbox | Positive, specific: "Nothing needs you. N sessions working." plus "last checked <time>" so empty is distinguishable from broken (existing "all done" vs calm empty vs filtered-empty copy is good; keep the three) |
| Missed notification | Item stays in inbox until ack or state change; ack is explicit (Done) and undoable ("Show done" view, GitHub `is:done` analogue); toast copy never the only record |
| Huge backlog (50+ items) | Group by reason with counts, cap rendering per group with "show N more", bulk "Mark all finished as seen", sticky "oldest waiting" callout (exists, l.1428). Sort within group by time-waiting desc |
| Intentionally paused/suspended sessions | Excluded by default (requirements: never revive; `noarchivedrevival`). Provide a collapsed "Paused (N)" group or a filter chip, no "needs you" styling, no resume-on-view side effect |
| Hidden/system sessions | Keep suppressed in the default view; a "include hidden" toggle rather than notifications |
| Offline / stream disconnect | Persistent banner "Live updates paused, last update 14:02:11 (absolute)", items dimmed with stale marker rather than silently frozen; on reconnect, refetch snapshot (do not replay as new notifications, no duplicate announcement); the existing staleness `role="status"` indicator is the base |
| Prose-question false positive | One-click "Not a question" (suppress this item) feeding observability counters; show the matched excerpt so the operator can judge |
| Event log retention | Show "older events trimmed" marker at the end of the timeline so the operator knows the log is bounded |
| Secrets | Event excerpts truncated and redacted; no raw terminal tails in the inbox row |

## 7. Mobile and desktop

- Mobile: top-of-list "Needs you (N)" only above the fold; rows >= 44px with single primary tap target (open session); secondary actions (Done, snooze, view timeline) behind a trailing overflow or swipe with a button equivalent (swipe-only fails 2.5.1/2.5.7). Timeline opens as a bottom sheet (focus trap, Esc/close button, drag handle with button alternative, `env(safe-area-inset-bottom)`, does not cover the virtual keyboard); on desktop the same component is a right-side drawer. Tooltips are unavailable, so absolute time must be in the sheet.
- Desktop: two-pane (list + detail timeline) works at >= 1024px; keyboard shortcuts; filters in a toolbar; count in the nav badge and document title ("(3) Stapler Squad").
- Up Next page on mobile already hosts PR and backlog tabs; adding session attention as a tab plus a global badge fits (tabs scroll horizontally), but it would make "Attention" one of at least three equal tabs when it should be first/default.

## 8. Option comparison (UX standpoint)

| Option | For | Against |
|---|---|---|
| A. Keep `/review-queue`, fix pipeline + log + timer | Zero migration, keeps tested a11y panel, deep links/shortcuts/e2e intact | Still three overlapping surfaces; the operator's "one place" trust problem is partly IA, not just detection; tray and queue keep diverging |
| B. Tray becomes the surface | Durable + alerting in one | Tray is an event stream (per-event rows, dedup, hidden-gate) not a state view; "item per session in current state" and "per-event" are different models, so mixing yields duplicates and no ranking; weak at grouping/filtering at scale |
| C. Merge into Up Next | One "what next" page | Up Next is worktree/PR/backlog oriented (async scan, different data lifetime); session attention is the highest-urgency and would be one tab of several on mobile; page already has its own 1 Hz scan ticker and filters; larger regression surface |
| D. Hybrid: Up Next home for "needs me", tray for toasts, event log drawer from both | Matches the pattern all comparables share (durable queue + transient alert + per-item timeline) | Most coordination; risk of 3 places still showing the same item unless one is explicitly the source of truth |

## 9. Recommendation

Choose a staged D whose first stage is A: keep one durable, state-based attention list as the single source of truth (the existing Review Queue panel, fixed and extended), define the tray as the transient alert/history layer that links into it, and ship the event timeline as a shared drawer/bottom sheet reachable from the list row and the SessionCard. Treat moving that list into Up Next as a later, flag-gated step (`attention_inbox_consolidation`) taken only after the soak shows the list is trusted.

Rationale:
1. The stated failure is trust (missed sessions), which is a detection and durability problem; consolidating pages before coverage is proven would move an untrusted list rather than fix it. A first (the flag-off baseline already in requirements) lets coverage metrics be measured on a stable surface.
2. State view (per session, ranked, ack-able) and event stream (tray) are different data models; B conflates them. Every comparable keeps both, with the stream deep-linking to the state.
3. Up Next (C) is the right long-term home only if the operator wants one page for PRs, backlog and sessions; that is a product decision for the owner, and mobile tab real estate argues for making attention the default first tab and a global nav badge either way.
4. Lowest regression risk for deep links, `/review-queue` shortcuts, onboarding, e2e and the registry; the redirect can be added last behind the flag.

Open items for the ADR/owner: whether the operator wants Up Next as "home"; whether "Done" (ack) should be explicit or implicit on opening the session (recommend implicit "seen" on open, explicit "Done" to remove finished items, so a drive-by view does not lose an unanswered question).
