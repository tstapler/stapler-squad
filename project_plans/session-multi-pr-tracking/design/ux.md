# UX design: session-multi-pr-tracking

Inputs: `requirements.md`, `research/ux.md`, `research/artifacts.md`, `implementation/plan.md` (Epics 6, 8.2, 9.2, 10). Source paths are under `web-app/src/`. Nothing here was built or run; component names and line references come from the research docs and are INFERRED until implementation.

Design stance: the list answers "is anything waiting on me" with one badge; the detail answers "what happened and what do I act on". Every PR action names its target PR. The badge reflects the active PR, never a blend.

## 0. Surface inventory

| # | Surface | Type | Plan ref |
|---|---|---|---|
| S1 | PR chip on session card and row (+N) | interactive | T6.1.2, T6.2.1 |
| S2 | "Pull requests" list in session detail | interactive | T6.1.3, T6.2.2 |
| S3 | Per-PR actions (merge, close, ask-to-fix, pin, not-mine) and confirm dialogs | interactive | T6.1.3, T6.2.6 |
| S4 | PR switcher in VCS widget / actions panel | interactive | T6.2.3 |
| S5 | Backlog "k of N PRs merged" | interactive | T6.2.5 |
| S6 | Up Next "session: name" chip | interactive (link) | T6.2.4 |
| S7 | Artifacts tab (states and grouped view) | interactive | T7.2.1, T7.2.4, T8.2.2 |
| S8 | `/artifacts` page: browse, filter, search | interactive | T10.1.2 |
| S9 | Omnibar "Find sessions for this PR" | interactive | T9.2.2 |
| S10 | Flag-off fallback, live announcements, token/contrast registry | non-interactive (condensed) | T6.3.1, T6.1.2 |
| S11 | Offer to close the GitHub issue when a finished item is re-imported | interactive | T5.5.3 |

Shared vocabulary used below:

- Backfilled historical rows that are unverified terminal (`state=unknown` with `terminal=true`; the adapter derives the `outcome_unverified` flag, there is no stored column) use the `closed` glyph with the text "Closed or merged (unverified)" until the one-time verify poll replaces it; never the word "Merged" (plan iteration 2, backfill seed).
- State glyphs (distinct shape, never color alone): open = hollow circle-dot, draft = dashed circle, merged = merge-arrows, closed = circle-x. Every glyph is paired with visible text ("Open", "Draft", "Merged", "Closed") except in compact chip mode, where it carries `title` plus `aria-label`, and the text is visible at widths above NARROW.
- Target phrase: "#12 acme/app" is always how an action names its PR.

---

## S1. PR chip on card and row

### Wireframes

Desktop card header (1280px):

```
+-----------------------------------------------------------+
| refactor-auth                [o #12 Open] [+2]   ... menu |
| branch: feat/auth-b                                       |
+-----------------------------------------------------------+
```

Mobile (390px, compact chip):

```
+------------------------------------+
| refactor-auth                  ... |
| [o #12 Open] [+2]                  |
+------------------------------------+
  chip and +2 are each >= 44px tall hit areas
```

All PRs terminal, history exists (show latest, real state):

```
| [m #11 Merged] [+1]                |
```

No PR: existing repo badge fallback, unchanged. Flag `reader:ui` off: legacy single badge, no "+N".

### Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Clicks the badge | Opens the active PR on GitHub in a new tab (existing `GitHubBadge` behavior) |
| 2 | Clicks or taps `+2` | Navigates to the session detail with the PR section scrolled into view and its heading focused (URL hash via `replaceHash` from `@/lib/nav`, so Back returns to the list) |
| 3 | Keyboard: Tab to badge, Tab to `+2`, Enter/Space | Same as 1 and 2 |
| 4 | Hovers badge (desktop) | Tooltip with approvals/CI. Same content is in the detail row, so touch loses nothing |

`+N` is a real `<button>` with `aria-label="2 more pull requests, open pull request list"` (plan T6.1.2 uses this exact string). It is not an expand-in-place control, so no `aria-expanded`. Nested interactive controls: the badge and `+N` sit inside a card/row that is itself clickable, so each stops propagation of its own click and tap; tapping `+N` opens the detail list only (it neither opens the card's default target nor the GitHub tab), and tapping the badge opens GitHub only (check in the UX-S1-4 mobile test and in `PullRequestChip.test.tsx`).

### States

| State | What the user sees | Exit path |
|---|---|---|
| Loading / unpolled | Gray "Checking" badge with a text label, not blank | Self-resolves on next poll; clicking `+N` or the card still opens detail |
| Error (poll failed for active PR) | Badge shows last known state plus a small warning glyph and `title` "Status may be out of date"; no red blend with other PRs | Click opens detail where the row shows the specific error and a Refresh action (S2) |
| Empty | Repo badge fallback; no "+N" | Card click opens detail, which offers Create PR |
| N = 1 | Badge only, no "+N" | n/a |
| Active PR merged, another open | Active is by rule the open PR, so the badge shows the open one | n/a |

### Acceptance criteria

- UX-S1-1: A session with 3 PRs shows exactly one PR badge and one `+2` button on card and row.
- UX-S1-2: `+2` reaches the detail PR list in one click/tap and moves keyboard focus to the "Pull requests" heading.
- UX-S1-3: The compact badge exposes state through glyph shape and a text label or `title`; contrast of each new state pair is >= 4.5:1 and listed in `CHIP_TOKEN_PAIRS`.
- UX-S1-4: At 390px the card has no horizontal scroll and both controls are >= 44px, and a touch `tap()` on `+N` does not also open the card target or a GitHub tab (and a tap on the badge does not also open the detail).
- UX-S1-5: Badge `aria-label` reads "View GitHub Pull Request #12, acme/app, status: open, opens in new tab".

---

## S2. "Pull requests" list in session detail

### Wireframes

Desktop:

```
Pull requests (5)                                       [Refresh]
Actions apply to #12 acme/app unless you pick another.
+-------------------------------------------------------------+
| [o] #12 Add token refresh      stacked  CI passing  1 appr  |
|     (ACTIVE)                   [Merge] [Ask to fix]  [...]  |
| [m] #11 Extract auth client             merged 2d            |
| [m] #10 Rename config keys              merged 4d            |
| [x] #9  Try cache                       closed 5d            |
|  > 2 earlier PRs (button, aria-expanded=false)              |
+-------------------------------------------------------------+
```

Mobile (390px), two-line stacked rows, actions in overflow only:

```
Pull requests (5)             [Refresh]
Actions apply to #12 unless you
pick another.
+--------------------------------+
| [o] #12  Open         (Active) |
| Add token refresh      [ ... ] |
| stacked | CI passing           |
+--------------------------------+
| [m] #11  Merged 2d     [ ... ] |
| Extract auth client            |
+--------------------------------+
 > Show all (5)
```

Rules:

- Order: active, then open by recency, then merged/closed by recency.
- Disclosure: more than 3 terminal rows collapse under "N earlier PRs" (desktop). Cap 5 visible rows on mobile with "Show all". Disclosure is `<button aria-expanded aria-controls>`.
- Role chip only for `followup`, `stacked` (the role set is `primary`, `followup`, `stacked`, `unknown`; Phase 4 C11). Never `primary`, `unknown`.
- CI/review summary is shown only for non-terminal rows.
- Merged/closed rows are visually quieter but meet 4.5:1.
- Semantics: `<section aria-labelledby>`, `<ul>`/`<li>`, active `<li aria-current="true">`. Link names unique: "Pull request #12 acme/app, Add token refresh, opens in new tab".
- Migrated legacy session: one row, no role chip, no disclosure, header hint omitted (nothing to disambiguate).

### Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Opens detail (or arrives from `+N`) | List renders from `pull_requests`; active row first |
| 2 | Activates "N earlier PRs" | Rows reveal in place; focus stays on the toggle; `aria-expanded="true"`; label becomes "Hide earlier PRs" |
| 3 | Clicks a PR title link | GitHub PR in new tab |
| 4 | Clicks Refresh | Header button shows "Refreshing" (`aria-busy`), calls `RefreshSessionPullRequests` (plan T4.2.3, which awaits `InvalidateAndRefresh` for open, `unknown` and unverified-terminal rows); verified merged or closed rows keep cached state and never spin |
| 5 | Polled state changes | Debounced (>= 2s coalesce) `role="status"` message: "PR #12 is now merged" |

### States

| State | What the user sees | Exit path |
|---|---|---|
| Empty (no PRs) | "No pull requests yet." plus **Create pull request** button (opens `CreatePullRequestModal`) | Create PR; or leave via normal tab navigation |
| Loading (flag on, list not yet received) | Skeleton rows with `aria-busy="true"` and visually hidden "Loading pull requests" | Self-resolves; if the stream never delivers, after 10s the skeleton is replaced by the error state below |
| Unpolled row | State text "Checking..." | Row Refresh in its overflow |
| Unverified terminal row (seeded from a legacy field, not yet confirmed by a poll) | State text "Closed or merged (unverified)" plus a second line. While polls are still being counted: "Checking... (attempt 2 of 5)" with `aria-busy` only on that line; once exhausted (5 counted failures over at least 30 minutes): "Couldn't verify after 5 attempts. Treated as closed." The two lines are different strings and never shown together | Header **Refresh** and the row **Refresh** both re-poll this row (plan T4.2.3: `RefreshSessionPullRequests` accepts open, `unknown` and unverified-terminal rows); on exhaustion, **Stop tracking** or Open on GitHub |
| Held (worktree or item kept because of this PR) | One line under the row, see "Held states" below | The release named in that line |
| Row poll error (auth, rate limit, network; enum mapping below) | Row keeps last known state plus inline chip "Can't refresh: GitHub auth failed" or "Rate limited until 14:05"; other rows unaffected | Row action "Retry"; for auth, link "Check GitHub token settings"; rate limit shows retry time and the VCS rate-limit banner (`lib/vcs/githubRateLimit.ts`) |
| PR deleted or inaccessible | Row retained, last known state, note "Can't reach this PR on GitHub (last seen 2d ago)" | Overflow: "Open on GitHub" (may 404), "Not mine" (detach) to remove it from this session |
| Whole list fails to load (session fetch error) | Section-level alert `role="alert"`: "Couldn't load pull requests" | **Retry** button; the legacy single-PR link stays rendered from `githubPrUrl` as a fallback so the user is never without a link |
| More than 5 rows | "Show all (N)" button | Button toggles; "Show fewer" returns |

### Held states (S2 session detail and S5 backlog item)

A worktree, an archived session or a backlog item can be kept because of a PR. User-facing copy never says "worktree" (plain wording: "local files"; the technical term stays in docs, code and tests). Every hold says why, and names how to release it; the line sits under the holding row (S2) or under the "Done when..." rule line (S5), as plain text (`role` none). Announcement rule (testable, A-10): the first appearance of each distinct hold is announced once through the surface's `role="status"` region; a hold that persists across polls or re-renders is not announced again; a hold that disappears and later returns is announced again; nothing is announced on initial render. Layout rule (A-9): every held line, including the longest (the maximum-hold copy), wraps at 390px with `overflow-wrap: anywhere`, is never truncated or hover-only, and causes no horizontal scroll. Release verbs are always one of: wait, refresh (verify), end the session, or Stop tracking (detach).

| Why held | Copy | How to release |
|---|---|---|
| Open PR | "Local files kept: #23 is still open. They are removed after #23 is merged or closed." | Wait; or **Stop tracking** #23 (it stays open on GitHub) |
| Unverified terminal PR | "Local files kept until #9 is confirmed closed or merged. Checking... (attempt 2 of 5)" | Refresh; after exhaustion the hold ends by itself ("Couldn't verify after 5 attempts. Treated as closed.") or **Stop tracking** |
| `unknown` PR (GitHub unreachable) | "Local files kept: GitHub can't be reached for #12 (last seen 2d ago)." | Retry; **Stop tracking** |
| Superseded round | "Kept for #22 from an earlier round; it is still open." (on that round's session, S2, and in the S5 holder list) | Merge or close #22; or **Stop tracking** #22 on that session |
| Merged, session still active | S5 rule line: "#21 merged. Waiting for the session to go idle (last activity 3 min ago; the item completes after 10 minutes idle or when the session ends)." S2 row: "Merged. The session is still active, so the item isn't done yet." | Wait; **End session**; Refresh does not apply. The idle window is the plan's `liveWorkIdleWindow` (default 10 minutes) and is read from the server response, never hard-coded in the copy |
| Maximum hold reached | "The session was still active 1 hour after #21 merged, so the item was marked done. The session keeps running, and its local files are kept, until it goes idle or you end it." (warning style, not an error; plan RULE L: the pane is not killed at the maximum hold) | **End session** |

Row poll-error mapping (plan T4.1.1 enum `poll_error`, Phase 4): `AUTH` shows "Can't refresh: GitHub auth failed"; `RATE_LIMITED` shows "Rate limited until HH:MM" from `rate_limit_reset_at`; `NOT_FOUND` and `REPO_INACCESSIBLE` show "Can't reach this PR on GitHub (last seen 2d ago)" (the poller cannot tell a deleted PR from an invisible repository, so the row stays held and the exit is "Not mine"); `NETWORK` shows "Can't refresh"; `NONE` shows nothing. Non-counted failures write this enum only, so a network error or a host-level auth failure is visible on the row.

Detach label: "Not mine" on a session-owned row (S2, S3), "Stop tracking" on an item-owned row (S5); both are the same detach operation on different owners (plan Glossary). The word "waive" is reserved for the backlog acceptance-criterion action (`WaiveAcceptanceCriterion`) and is never used for detach, in copy, test ids or RPC names.

Detach reversal (UX-S3-8): detaching writes a tombstone, so branch discovery, the backfill and Jules will NOT re-attach that PR on their own. The confirm dialog says so ("Remove #12 from this session? The PR stays on GitHub. It won't be added back automatically; you can track it again from Track a PR by URL."). On success an 8 s toast "Stopped tracking #12. Undo" (same keyboard behavior as the pin toast) calls `AttachSessionPullRequest` for that PR (`source=manual`, clears the tombstone). After the toast is gone the persistent route is a **Track a PR by URL** item in the S2 header overflow (and in the S5 section header): a one-field dialog that calls the same RPC and shows its errors inline (invalid URL, "not found on GitHub", "already tracked"). No tombstone list is shown; that is the smallest option that needs no new RPC.

### Acceptance criteria

- UX-S2-1: Given 3 PRs, the list shows 3 rows with visible state text and the default-target hint.
- UX-S2-2: Given 5 PRs (1 open, 4 merged), desktop shows the open PR and 2 merged rows plus a button reading "2 earlier PRs" with `aria-expanded="false"`.
- UX-S2-3: One failing row shows its own error chip and Retry; the other rows still render and stay actionable.
- UX-S2-4: A legacy single-PR session shows one row, no role chip, no disclosure, no hint line.
- UX-S2-5: At 390px there is no horizontal scroll; long titles wrap (`overflow-wrap: anywhere`); no information is available only on hover.
- UX-S2-6: The list can be fully operated by keyboard in DOM order: links, overflow buttons, disclosure.
- UX-S2-7: A PR that holds a worktree or an item shows a held line with the reason and the release (table "Held states"); a session with no holding PR shows none.
- UX-S2-8: Refresh (header and row) re-polls a seeded "Closed or merged (unverified)" row, shows "Checking... (attempt N of 5)" while polls are pending and "Couldn't verify after 5 attempts. Treated as closed." after exhaustion; the two strings differ.

---

## S3. Per-PR actions and confirmations

Actions per row (overflow `[...]`, Radix menu pattern; inline buttons on desktop for the active open row only):

| Action | Shown when | Confirm |
|---|---|---|
| Merge | PR open, not draft | Dialog: "Merge #12 acme/app into main? Method: merge (select: merge, squash, rebase; request field `MergePRRequest.method`, plan T4.1.1 and T6.1.3)" |
| Close | PR open | Focus-trapped dialog: "Close #12 acme/app without merging?" |
| Ask to fix | PR open | None; toast names target |
| Pin as current | not already pinned | None; toast with Undo |
| Unpin | pinned | None |
| Not mine (detach) | any | Dialog: "Remove #12 from this session? The PR stays on GitHub." |
| Open on GitHub / Copy link | always | None |

Every request passes the row's `PullRequestRef`; the dialog title always contains `#N owner/repo`.

### Flow: merge (representative)

```
Row overflow > Merge
  -> dialog [Merge #12 acme/app] [method select] [Cancel] [Merge]
     Esc / Cancel / backdrop -> dialog closes, focus returns to the row's overflow button
     Merge -> button shows spinner, dialog stays open, controls disabled
        success -> dialog closes, row updates to Merged, status announcement "PR #12 merged"
        failure -> dialog stays open with inline role="alert" error, buttons re-enabled
```

### Error table

| Failure | Message | Exit path |
|---|---|---|
| Not mergeable (conflicts, checks, branch protection) | "Can't merge #12: <GitHub reason>" | Retry; Cancel; link "Open on GitHub" |
| Target not attached to session (server `FailedPrecondition`) | "This pull request is no longer attached to this session." | Dialog closes on Dismiss; list refreshes |
| No PR (server "no pull request") | Merge/Close/Ask-to-fix are disabled with visible helper "No pull request to act on" | Create pull request button beside it |
| Auth / rate limit | "GitHub rejected the request (auth)" or "Rate limited until HH:MM" | Retry later; settings link |
| Network | "Couldn't reach the server" | Retry; Cancel |
| Pin fails | Toast `role="alert"` "Couldn't pin #12" with Retry | Toast dismiss |
| Pin succeeds | Toast "Pinned #12 as current. Undo" (Undo is keyboard reachable, 8s, pauses on focus/hover) | Undo restores previous derived active PR |
| Detach fails | Inline in dialog | Retry; Cancel |
| Detach succeeds | Toast "Stopped tracking #12. Undo" (8 s, Undo keyboard reachable, pauses on focus and hover) | Undo re-attaches; later, Track a PR by URL |
| Track a PR by URL fails | Inline `role="alert"` in the dialog: invalid URL, "Not found on GitHub", "Already tracked" | Edit; Cancel |
| Concurrent change (PR merged elsewhere while dialog open) | "#12 was already merged" and the dialog switches to Close-only | Dismiss |

### Undo toast (pin and detach): keyboard path and timing (WCAG 2.2.1)

The toast Undo is a convenience; the guaranteed, untimed path for every user is persistent and needs no new machinery: after a pin the row overflow shows **Unpin** (that is the undo), and after a detach **Track a PR by URL** in the S2 header overflow re-attaches. Both are reachable by keyboard at any time. The toast text says so for assistive technology ("Pinned #12 as current. Undo, or choose Unpin in the row menu."; "Stopped tracking #12. Undo, or use Track a PR by URL.").

- Keyboard reach of the toast Undo: a native `<button>` inside the toast region. Because reaching a last-in-DOM region by Tab is slow, a KEYBOARD-initiated pin or detach (the menu item or dialog confirm was activated by Enter or Space) moves focus to the toast's Undo button when the toast appears; Esc, activating Undo, or the toast timing out after focus leaves returns focus to the destination defined in UX-S3-3 (the invoking control, or the next row when the row is gone). A pointer- or touch-initiated action does not move focus. No new shortcut and no new machinery (focus on appear is a `ref.focus()` in the toast action); the persistent Unpin and Track a PR by URL paths below stay as the untimed fallback. If T6.2.6 finds the deck is not last in DOM order it moves the region, not adds a shortcut.
- Timing: auto-dismiss is 8 s, pauses while the pointer is over the toast or focus is inside it (the existing `ToastStack` hover and focus timers, `ToastStack.tsx:125-127`), and a toast holding focus is not dismissed until focus leaves, so a keyboard or switch user who has reached it can take as long as needed. Dismissing never loses the message for a screen reader: it was announced through the status region.
- Tests: Jest fake timers cover the 8 s and the pause (UX-S3-6); a real-browser row (A-11) installs `page.clock`, pins a PR by keyboard, asserts focus is already on the toast Undo, fast-forwards 9 s and asserts the toast and Undo are still present while focus is inside, then moves focus out, fast-forwards 8 s and asserts it is gone and that the persistent Unpin item still works.

### Acceptance criteria

- UX-S3-1: With #11 merged and #12 open, Merge with no explicit pick targets #12 and the dialog says so.
- UX-S3-2: Choosing another row's Merge targets that row, regardless of which row is active.
- UX-S3-3: Every dialog traps focus, closes on Esc, and returns focus to the invoking control. This holds for the 390px bottom-sheet variants of Merge, Close, Not mine / Stop tracking and Track a PR by URL under a touch project (A-16); in the Track a PR by URL sheet the URL field and its confirm button stay inside `window.visualViewport` while the soft keyboard is open. Focus destination when the invoking row is removed or reordered by the action (detach, Merge/Close that moves the row, pin that reorders): focus moves to the same-position row that now occupies the slot (the next row), else the previous row if it was the last, else the list's "Pull requests" heading; it never falls to `document.body`. Asserted for detach of a middle row, the last row and the only row (A-16 on touch, `PullRequestList.test.tsx` in Jest).
- UX-S3-4: Every failure keeps the dialog open with an inline alert and a Retry; Cancel is always available.
- UX-S3-5: Close and Detach copy states what does and does not happen on GitHub.
- UX-S3-6: Pin is undoable for 8s without leaving the keyboard, the toast pauses on hover and focus, and an untimed keyboard path (row menu Unpin; for detach, Track a PR by URL) always exists (section "Undo toast").
- UX-S3-7: Touch targets of overflow items are >= 44px at 390px.
- UX-S3-8: The Stop tracking / Not mine dialog says the PR will not be added back automatically; success shows an Undo toast that re-attaches the PR, and Track a PR by URL re-attaches one after the toast is gone. At 390px the toast does not cover the bottom sheet's buttons or the bottom navigation (it sits above both).

---

## S4. PR switcher (VCS widget and actions panel)

Shown only when more than one **open** PR exists. Defaults to the active PR. It selects the target for the widget's actions and status display. It does not change the pin.

Desktop (segmented control, max 4 segments; beyond 4, a select):

```
Pull request:  [ #12 Active ][ #14 ]      CI passing  1 approval
Actions below apply to #12 acme/app.      [Merge] [Ask to fix]
```

Mobile (390px): native `<select aria-label="Pull request">`, options "#12 Add token refresh (active)", "#14 Docs fixes". Changing the select updates the "Actions apply to #N" line, which sits in the same `aria-live="polite"` region as the fallback message ("#14 is now merged; showing #12"), so a native-select change and a fallback are both announced; asserted under `mobile-chromium` (A-16).

| Step | User does | System responds |
|---|---|---|
| 1 | Arrow keys / Tab to segment | `role="radiogroup"` semantics; arrow keys move selection; selection updates status area and the "Actions apply to" line, announced via `aria-live="polite"` |
| 2 | Chooses another PR | Local selection only; resets to the active PR when the widget remounts |
| 3 | The selected PR merges or closes while selected | Selection falls back to the active PR with message "#14 is now merged; showing #12" |

States:

| State | Behavior | Exit path |
|---|---|---|
| One open PR | No switcher; the target line is shown | n/a |
| Zero open PRs | No switcher; widget shows latest PR with real state; actions Merge/Close disabled with reason text | Create pull request |
| PR data failed for selected PR | Status area shows "Couldn't capture PR status" for that PR only (existing copy pattern); other segments still selectable | Retry in widget; pick another segment |
| Loading | Segment labels render immediately from the list; status area shows "Checking..." | Self-resolves |

Acceptance criteria:

- UX-S4-1: Two open PRs show a segmented control defaulting to the active PR on desktop and a native select at 390px.
- UX-S4-2: The target PR is stated in text next to the action buttons at all times.
- UX-S4-3: The control is operable by keyboard alone and announces selection changes.
- UX-S4-4: With one open PR no switcher renders.

---

## S5. Backlog item: "k of N PRs merged"

Wireframe (`PullRequestSection`, `LifecycleSummary`):

```
Pull requests            2 of 3 merged  [=======..]  (progress bar, role=progressbar)
Done when at least one PR is merged and none are open.
  [m] #21 Schema           merged
  [m] #22 Backfill         merged
  [o] #23 UI               open    <- blocking
```

Mobile: summary line stacks above the same two-line rows as S2; progress bar full width.

Rules:

- Text "2 of 3 merged" is the primary signal; the bar is decorative plus `aria-valuenow/valuemax/valuetext="2 of 3 pull requests merged"`.
- Closed-unmerged counts as resolved but not merged; summary reads "2 merged, 1 closed". The rule line explains why the item is or is not done: "Waiting on #23 (open)".
- Section renders whenever the item has any PR, not only in `pr_pending`.

| State | What the user sees | Exit path |
|---|---|---|
| One PR | "0 of 1 merged"/"1 of 1 merged"; no bar | n/a |
| All closed unmerged | "0 merged, 3 closed. No merged PR, so the item can't complete." | Link "Attach another PR" (existing report path) or open the item's linked session |
| Item has PRs but none reachable | Rows with last-known state plus "Can't reach" | Retry; Stop tracking on the row |
| Load error | `role="alert"` "Couldn't load pull requests for this item" | Retry; the legacy single PR link remains |
| Stuck-item list (`StuckItem*`) | Shows "k of N merged, waiting on #23" instead of one number | Click opens the item detail |
| Merged, session still active | See "Held states" (S2): rule line "Waiting for the session to go idle..." | Wait; End session |
| Cleanup held by a PR (done or archived item) | Held line naming the PR and owner ("Local files kept: #23 is still open") | Wait; Stop tracking on that PR |

Acceptance criteria:

- UX-S5-1: An item with 2 of 3 PRs merged shows "2 of 3 PRs merged" and three rows.
- UX-S5-2: The reason the item isn't done is a sentence naming the blocking PR.
- UX-S5-3: Progress is conveyed by text and `role="progressbar"` semantics, not color only (real-browser check A-18).
- UX-S5-4: Closed-unmerged-only items explain why they cannot complete and offer an exit.
- UX-S5-5: An item whose last PR merged while its session is still active reads "#21 merged. Waiting for the session to go idle..." with the idle window from the server, the held-state release verbs, and after the maximum hold the warning in "Held states"; it never reads "stuck" or an error.

---

## S6. Up Next "session: name" chip

Muted chip on `PRCard` when the PR's `PRRef` matches a tracked session: `[session: refactor-auth]`, a link to the session detail. Not shown when no match. Does not regroup the list.

- Loading of the sessions store not finished: no chip (never a placeholder), so no layout shift beyond chip insertion; reserve no space.
- Multiple sessions track the same PR: one session reads "session: name"; two or more read "N sessions" and open a small menu of links to each (Esc closes, focus returns). This is the plan's wording (T6.2.4); the earlier "most recent session plus +1" draft is dropped.
- Lookup error: silently omit the chip (it is an enhancement); no dead end because the PR card's own actions remain.
- Mobile: chip wraps to its own line below the title; 44px target (the chip and each link in the "N sessions" menu are `tap()`-checked at >= 44px under `mobile-chromium`, A-19).
- Acceptance: UX-S6-1 chip name link is "Open session refactor-auth"; UX-S6-2 absence of a match produces no chip and no empty gap.

---

## S7. Artifacts tab (session detail)

### Four reliability states

The tab header always shows a status line: "Last scanned 3 min ago" when known, plus a **Rescan** button (except in `not_linked`).

| `scan_state` | Body | Role / announcement | Exit path |
|---|---|---|---|
| `not_linked` | "This session has no linked transcript yet, so there is nothing to scan. Non-Claude programs never get one." | static text | For a Claude session: "Transcript links automatically once the session writes its first message"; no Rescan (it cannot help). Link "Learn more" to the doc. Tab can be left normally |
| `scanning` | Skeleton plus "Scanning transcript..." | `role="status"`, `aria-busy` | Rescan hidden; after 60s without completion (a client timer, no new field; plan T7.2.5), shows "Taking longer than usual" with **Rescan** and **Cancel waiting** (returns to previous data if any) |
| `none_found` | "No artifacts found in N messages scanned." | static | **Rescan**; hint "Artifacts are PRs, commits, branches, issues, plan docs and CI links the agent produced" |
| `has_data` | Grouped view below | | Rescan |
| scan failed (RPC error or scan error) | `role="alert"` "Scan failed: <reason>" with last good data still shown beneath | | **Retry**; Dismiss |

Rescan flow: click Rescan -> button disabled with spinner and `aria-busy`, state becomes `scanning` -> on completion the state updates via the existing `SessionUpdatedEvent(["artifacts"])` and the status line announces "Scan finished: 4 artifacts" (or "No artifacts found"). Rescan RPC error -> inline alert plus Retry, button re-enabled. Rescan is idempotent; double clicks are ignored while disabled.

### Grouped view

Desktop:

```
Artifacts  (Last scanned 3 min ago)                 [Rescan]
v #11 [m Merged] Extract auth client            3 items
    commit 1a2b3c4  fix: token refresh      feat-b
    branch feat-b
v #12 [o Open] Add token refresh                2 items
    commit 9f8e7d6  add retry
    plan   project_plans/x/plan.md
> Unassigned (4)
> Mentioned pull requests (2)  (collapsed; not authored by this session)
```

Mobile (390px): each group is a full-width disclosure button (44px, `aria-expanded`, `aria-controls`), items stack as two lines (kind + title, then meta), URLs `overflow-wrap: anywhere`, no horizontal scroll.

Rules:

- PR group header reuses `PullRequestChip`; mentioned PRs are collapsed by default and labeled "Mentioned, not created by this session".
- Item grouping is by temporal window; the tab says so once in a help line to avoid false certainty: "Grouped by when each item appeared relative to PR creation".
- Each item is a link or a copy button with a name that includes its kind ("Copy commit SHA 1a2b3c4").
- Redacted values display as `[redacted]` and are not copyable as secrets.
- Groups default expanded for the active PR and Unassigned if no PR groups; others collapsed beyond 3 groups.
- v1 blobs (no records) render the legacy flat lists (PR URLs, commits, URLs) with no grouping.

| Error / edge | Behavior | Exit path |
|---|---|---|
| Records present but proto older/missing fields | Fall back to the flat v1 list | n/a |
| A group fails to render (bad record) | Skip the record, show "1 item couldn't be shown" in the group | Rescan |
| Very large group (>50) | First 50 newest, "Show all (N)" | Toggle |
| Item link is a stale URL | Opens in new tab; no in-app handling | n/a |

### Acceptance criteria

- UX-S7-1: Each of `not_linked`, `scanning`, `none_found`, `has_data` and scan-failed has distinct text; "Extraction pending" never appears for a completed empty scan.
- UX-S7-2: Rescan is reachable in <= 1 click in `none_found`, `has_data` and failed; the button is disabled with a spinner during the scan; completion is announced.
- UX-S7-3: `not_linked` explains why and what to do, and does not offer a useless Rescan.
- UX-S7-4: `scanning` for more than 60s offers a way out.
- UX-S7-5: Commit at 10:30 sits under #11 and a plan doc at 11:15 under #12, unrelated links under "Unassigned".
- UX-S7-6: At 390px groups collapse, targets >= 44px, no horizontal scroll.
- UX-S7-7: Mentioned PRs are collapsed and labeled as not created by this session.
- UX-S7-8: Secrets never display; `[redacted]` shows in place of values.

---

## S8. `/artifacts` page (cross-session browse and search)

Route registration: `route-meta.ts` row title "Artifacts" equals the page metadata; no guard; navigation through `@/lib/nav`; filters live in the URL via `useReplaceSearch` so a filtered view is shareable and Back works. The nav entry is plan T10.1.2.

### Wireframe, desktop

```
Artifacts
[ Search artifacts: PR, commit, branch, issue, text...   ] [Search]
Type: [All v]  Repo: [All v]  State: [All v]     [Clear filters]
Showing 12 of 12                                  (role=status)
+--------------------------------------------------------------------+
| Kind | Artifact                       | Session        | When      |
| PR   | [m #11 Extract auth client]    | refactor-auth  | 2d ago    |
| PR   | [o #12 Add token refresh]      | refactor-auth  | 1h ago    |
| commit 1a2b3c4 fix: token refresh     | refactor-auth  | 1h ago    |
+--------------------------------------------------------------------+
[Load more]
```

### Wireframe, mobile (390px)

```
Artifacts
[ Search...                    ][Go]
[ Filters (2) v ]        <- button, aria-expanded; panel is a sheet
12 results
+--------------------------------+
| PR  [o #12 Add token refresh]  |
| refactor-auth - 1h ago         |
+--------------------------------+
| commit 1a2b3c4                 |
| fix: token refresh             |
| refactor-auth - 1h ago         |
+--------------------------------+
[ Load more ]
```

Filter sheet on mobile: trap focus, Esc or "Done" closes, "Clear" resets, focus returns to the Filters button.

### Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Opens `/artifacts` | Browse mode: `ListSessionArtifacts` newest first, bounded page; heading focused for screen readers |
| 2 | Changes a filter | URL updates (`?type=pr&repo=acme/app`), list reloads, count announced |
| 3 | Types a query and presses Enter/Search | Search mode: list becomes the `SearchSessionArtifacts` result, each hit labelled with the matched artifact type; "Clear search" returns to browse |
| 4 | Activates a row's session link | Opens the session detail at the Artifacts tab; Back returns with filters and scroll intact |
| 5 | Load more | Appends the next page; focus stays on the Load more button and new row count is announced; on the last page the button is removed |

Search is submit-driven (not per keystroke) to avoid latency churn; Esc in the search box clears it. Soft keyboard (touch devices): the focused search box and its Go button stay inside `window.visualViewport` when the keyboard opens (the same pattern the Files viewer uses), asserted by emulating the viewport shrink (A-16); the real keyboard is a manual check.

### States

| State | What the user sees | Exit path |
|---|---|---|
| Initial loading | Skeleton rows, `aria-busy`, status "Loading artifacts" | Self-resolves; timeout 15s becomes the error state |
| Empty browse (no sessions have artifacts) | "No artifacts yet. Artifacts appear as sessions open PRs, make commits and write plans." | Link "Go to sessions" |
| Empty because of filters | "No artifacts match these filters." | **Clear filters** button |
| Empty search | `No sessions found for "1a2b3c4".` plus tip "Try the PR number (acme/app#12), a branch name, or a SHA prefix of at least 7 characters" | Clear search; Clear filters |
| Search error | `role="alert"` "Search failed. Browse still works." | **Retry** and **Clear search** |
| Browse error | `role="alert"` "Couldn't load artifacts" | **Retry**; the nav stays usable |
| Load more error | Inline row at list end "Couldn't load more" | **Retry** (keeps the loaded rows) |
| Partial scan coverage | Info line "N sessions aren't scanned yet, so results may be incomplete" (when the server reports unscanned sessions) | Link to per-session Rescan |
| Slow search (> 1s) | Inline spinner next to Search, previous results stay visible, dimmed | Cancel by editing or Esc |
| Query too short (< 2 characters) | Inline validation under the box: "Enter at least 2 characters" | Edit query |
| SHA-like query shorter than 7 hex characters (2 to 6 characters, all hex) | The search runs (2+ characters is the only gate) and a hint appears under the box: "Looks like a commit SHA. Use at least 7 characters to match a commit." | Edit query |
| Session archived or hidden | Row shows an "Archived" tag and the link still works if the session exists; if it was deleted: "Session no longer exists" (no link) | Row stays; user can filter it out |
| Unauthorized/stale route | n/a (page is read-only, no flag guard) | n/a |

Desktop layout is a table with `<th scope>` headers; mobile is a list of cards. The same data renders in both; no hover-only content.

### Acceptance criteria

- UX-S8-1: Opening `?type=pr&repo=acme/app` lists only PR records for that repo, each with a session link and a `PullRequestChip`.
- UX-S8-2: Submitting `1a2b3c4` shows the search result with the matched type per row; clearing returns to browse in one action.
- UX-S8-3: Each of loading, empty-browse, empty-filter, empty-search, search error, browse error and load-more error has distinct copy and a named exit action.
- UX-S8-4: At 390px: single column, filters behind a button, no horizontal scroll, all targets >= 44px; at 1280px: tabular rows.
- UX-S8-5: Filters and query are in the URL; reload and Back restore the same view.
- UX-S8-6: The result count and loading transitions are announced through one `role="status"` region.
- UX-S8-7: Axe reports no WCAG AA violation on both widths.
- UX-S8-8: The page can be reached from the main navigation in <= 2 clicks.

---

## S9. Omnibar: "Find sessions for this PR"

Flow: user pastes `https://github.com/acme/app/pull/12` -> detector offers action "Find sessions for acme/app#12" alongside existing actions -> Enter dispatches `FindSessionsByPullRequest`.

Result presentation (decision): navigate to `/artifacts?pr=acme/app%2312` showing the exact-lookup result, each row labeled `source=session_pr` ("Tracked") or `source=artifact` ("Seen in transcript"). This reuses S8's states instead of inventing a new surface.

| State | Behavior | Exit path |
|---|---|---|
| One match | Navigate straight to that session detail (skip the list) with a toast "Opened the session tracking #12 (Tracked). 1 match. View all" (label is "Tracked" for `source=session_pr` and "Seen in transcript" for `source=artifact`; the toast stays 8 s and, at 390px, sits above the bottom navigation without covering it or the detail header's controls (A-19); its **View all** action opens `/artifacts?pr=...`, and the detail view's Pull requests header keeps a **View all matches** link while the match came from the omnibar) | Back returns to the omnibar origin; View all |
| Several matches | `/artifacts` list, ordered: every `source=session_pr` ("Tracked") row above every `source=artifact` ("Seen in transcript") row, and within each group by session recency (`last_activity` descending, ties broken by session title ascending so the order is stable); a session matched by both sources appears once, as Tracked | S8 exits |
| No match | `/artifacts` empty-search state with copy "No session has touched acme/app#12" | Clear search; Escape back |
| RPC error | Omnibar inline error "Couldn't look up sessions for this PR" | **Retry**; Esc closes the omnibar; other omnibar actions on the same input still work |
| GHE URL | Detector accepts host-aware URLs; unknown host falls back to generic URL handling | n/a |

Acceptance (ranking is asserted by check A-15, not counted as a criterion): UX-S9-1 the action appears for a pasted PR URL and is keyboard selectable (arrows plus Enter); UX-S9-2 error leaves the omnibar open with the other actions available; UX-S9-3 a single match still offers a visible View all escape (toast action and header link) and every result row and the toast show the Tracked or Seen in transcript label; UX-S9-4 a 2 to 6 character hex query runs and shows the SHA hint, a 1 character query shows the inline validation and sends no request (the same rule in the omnibar and S8).

---

## S10. Condensed non-interactive surfaces

**Flag-off fallback (`reader:ui` off).** Card, row, detail and widget show the legacy single badge from `githubPrNumber`; no `+N`, no list, no switcher. Artifacts tab and `/artifacts` are unaffected by the flag (read-only). Acceptance: UX-S10-1 toggling the flag changes UI on next render without reload; UX-S10-2 no console error when `pull_requests` is empty but the flag is on (falls back to the legacy badge).

**Live announcements.** One polite `role="status"` region per surface (list, switcher, `/artifacts`), debounced >= 2s, text names the PR ("PR #12 is now merged"). Never announces initial render. Acceptance: UX-S10-3.

**Token and contrast registry.** New chip states (open, draft, merged, closed, checking, stale/error) are added to `CHIP_TOKEN_PAIRS` and covered by `chipContrast.test.ts`, light and dark, >= 4.5:1 text and >= 3:1 for glyph boundaries. Acceptance: UX-S10-4.

**Test hooks.** All interactive elements expose `data-testid` or ARIA roles (`pr-chip`, `pr-more-button`, `pr-list`, `pr-row-<number>`, `pr-switcher`, `artifacts-scan-state`, `artifacts-rescan`, `artifacts-search`). No CSS class selectors in e2e. Acceptance: UX-S10-5.

---

## S11. Offer to close the GitHub issue when a finished item is re-imported

Owner request 2026-11-03. Trigger: the user imports a GitHub issue (the single-URL form or the bulk picker on `/backlog`) whose backlog item already exists, is FINISHED (status `done`; for items with PRs at least one merged and none open; archived and duplicate items never) and whose issue is still open on GitHub. Today the single-URL form silently opens the existing item and the bulk picker only says "Already imported"; both keep their message and gain the prompt. Nothing is closed until the user confirms. Scope: github.com issues with a token; behind scope `reader:backlog` (off: no prompt, today's behavior).

Desktop dialog (1280px):

```
+----------------------------------------------------------+
| Close GitHub issue #123?                                 |
| "Add token refresh" is already done in stapler-squad.    |
| PRs: #21 merged, #22 merged. The issue is still open.    |
| Closing posts a comment that links the merged PR. To     |
| undo, reopen the issue on GitHub; this app can't.        |
|                         [Not now]  [Close issue #123]    |
+----------------------------------------------------------+
```

Mobile (390px): a bottom sheet, buttons full width and stacked with **Close issue #123** first and **Not now** second, each >= 44px; the PR list wraps (`overflow-wrap: anywhere`); no horizontal scroll.

| Step | User does | System responds |
|---|---|---|
| 1 | Imports an issue whose item is finished | The existing "already imported" behavior, plus the prompt (`role="dialog"`, labelled by its title, focus trapped, focus on **Not now** so Enter never closes by accident) |
| 2 | Activates **Close issue #123** | Button shows a spinner, controls disabled, `aria-busy`; `CloseFinishedItemIssue` runs; the server re-checks that the item is finished and the issue open |
| 3 | Closed | Prompt closes, status toast "Closed issue #123" (`role="status"`), focus returns to the import form's trigger |
| 4 | **Not now**, Esc or backdrop | Prompt closes with no RPC; focus returns to the trigger; the user can re-import later to see it again |
| Bulk | Imports several issues | One prompt lists every offer, each row with its own **Close issue** button and a **Dismiss all**; never a bulk close |

| State | What the user sees | Exit path |
|---|---|---|
| Offer | Title, item title, PR list with states, the consequence line (exact text: "Closing posts a comment that links the merged PR. To undo, reopen the issue on GitHub; this app can't."), asserted verbatim by a test (A-12) because the close cannot be undone from here | Close issue; Not now |
| Closing | Spinner on the button, controls disabled | Self-resolves; the dialog stays open |
| Already closed (race) | "Issue #123 was already closed." | Dismiss |
| No longer finished (race) | "This item is no longer done, so the issue was left open." | Dismiss |
| GitHub Enterprise issue | "This issue is on ghe.acme.io. stapler-squad doesn't close GitHub Enterprise issues." No close button | Open on GitHub link; Dismiss |
| No token | "No GitHub token for github.com, so the issue can't be closed from here." No close button | Link to settings; Dismiss |
| Source-synced issue (the item came from a configured item source) | "This issue is synced from a source. stapler-squad closes it only when forward-sync is enabled for that source." No close button | Open on GitHub link; Dismiss |
| Close failed | `role="alert"` "Couldn't close issue #123: <reason>" inline, buttons re-enabled | Retry; Not now |
| Not finished, issue closed, or flag off | No prompt; today's behavior | n/a |

Acceptance criteria:

- UX-S11-1: Re-importing a finished item whose issue is open shows the prompt naming the issue and the item's PRs, and nothing is closed until **Close issue** is activated.
- UX-S11-2: At 390px the prompt is a bottom sheet with stacked >= 44px targets and no horizontal scroll; focus is trapped, Esc and **Not now** dismiss without any request, and focus returns to the trigger.
- UX-S11-3: Each of closed, already closed, no longer finished, GHE issue, no token, source-synced and close failed has distinct copy and a named exit action (Retry, Dismiss, Open on GitHub, or the settings link).
- UX-S11-4: An unfinished item, an already closed issue, and the flag off show no prompt and the existing "already imported" message is unchanged.
- UX-S11-5: In the bulk prompt each row has its own busy state (that row's button shows a spinner and is disabled with `aria-busy`, other rows stay operable), its own result (the row is replaced by "Closed issue #123" with an **Open on GitHub** link to the issue, or by the inline failure with Retry), and the polite status region announces each result once ("Closed issue #123"). The single prompt also shows **Open on GitHub** after a successful close. The success toast stays 8 s and pauses on hover and focus (it is also announced through the status region, so dismissal does not lose the message for a screen reader); **Dismiss all** is disabled while any row is busy.

## Cross-cutting UX acceptance criteria

- UX-X-1: Keyboard: every control in S1 to S9 and S11 is reachable by Tab in visual order, operable with Enter/Space (arrow keys for the radiogroup), with a visible focus ring >= 3:1 contrast.
- UX-X-2: Screen reader: links, buttons and status regions have unique names including `#N owner/repo` where a PR is involved; state is never color-only (WCAG 1.4.1). The automated checks are accessible NAMES (Jest and Axe) and a real-browser non-color check (A-18: the S1/S2 chips, the S5 progress bar and the S7 chips under emulated achromatopsia); announcement WORDING on a real screen reader is MANUAL-only: owner Tyler, a PR-checklist line on the T6.3.2 PR, plus a copy-review checkpoint in the same PR for the "Not mine" (session-owned row) versus "Stop tracking" (item-owned row) split, the held-state and maximum-hold lines (read for plain wording: no "worktree" in user-facing strings; the checklist line says so) and the S11 consequence line (plan Owner confirmations pending #13).
- UX-X-3: Touch targets >= 44px at 390px, >= 24px elsewhere (WCAG 2.5.8). Verified under a real touch project (`mobile-chromium`), not a resized desktop window.
- UX-X-4: At 390px no surface scrolls horizontally; content reflows at 200% zoom. Method (A-17): Playwright has no browser page-zoom API (INFERRED, not re-verified in this pass), and CSS `zoom` is not what a browser zoom does, so the automated check emulates the layout effect of browser zoom the way browsers apply it, by shrinking the CSS viewport (1280 px at 200% is a 640 px layout viewport; 320 px is the WCAG 1.4.10 reflow case) with `assertReflow`. Text-only zoom (browser minimum font size, 200% text size with the layout viewport unchanged) is MANUAL: owner Tyler, a PR-checklist line on the T6.3.2 PR, gating the `reader:ui` flip.
- UX-X-5: Every error state has at least one exit action (Retry, Cancel, Dismiss, Clear, or a link to an alternative); see table below.
- UX-X-6: Motion: spinners and skeletons respect `prefers-reduced-motion`.
- UX-X-7: Task budgets: reach another PR's detail from the card in <= 2 clicks; merge a specific non-active PR in <= 4 clicks (open detail, row overflow, Merge, confirm); find the session for a pasted PR URL in <= 3 steps; start a manual rescan in <= 2 clicks from the session detail.

## Exit-path matrix

| Flow | Cancel / back | Error exit | Empty exit | Loading exit |
|---|---|---|---|---|
| S1 chip | click away; no modal | open detail for the detailed error | repo badge, card still opens detail | self-resolves |
| S2 list | tab away; disclosure toggles | row Retry, section Retry, legacy link fallback | Create PR | 10s timeout becomes error |
| S3 actions | Cancel, Esc, backdrop | Retry in dialog | disabled with reason plus Create PR | spinner then dialog stays |
| S4 switcher | n/a (local) | per-segment fallback | no switcher; Create PR | labels render first |
| S5 backlog | n/a | Retry, legacy link | explanation plus Attach another PR | self-resolves |
| S6 chip | Esc on popover | omit silently | omit | omit |
| S7 tab | tab away; Cancel waiting | Retry/Rescan, last good data kept | Rescan or explanation | 60s escape |
| S8 page | Back, Clear, Done, Esc | Retry, Clear search | Clear filters / Go to sessions | 15s timeout becomes error |
| S9 omnibar | Esc | Retry | empty-search page | n/a |
| S11 close-issue prompt | Not now, Esc, backdrop | Retry, Dismiss, Open on GitHub | no prompt | spinner, dialog stays |

## Plan gaps the design depends on (status after the Phase 4 patch pass)

| # | Gap | Closed by | Status |
|---|---|---|---|
| 1 | Proto had no "unreachable PR" field | T4.1.1: `poll_error` enum (`none|auth|rate_limited|not_found|repo_inaccessible|network`), `last_polled_at`, `rate_limit_reset_at`; non-counted failures write the enum (T1.3.2 (d), T3.2.4) | closed |
| 2 | No `unscanned_session_count` on search/browse | T9.2.1, T10.1.1 | closed |
| 3 | No nav entry task for `/artifacts` | T10.1.2 | closed |
| 4 | `FindSessionsByPullRequest` presentation undefined | `pr` filter: T4.1.1, T4.2.6; page: T10.1.2 (routes to `/artifacts?pr=`, with the single-match shortcut) | closed |
| 5 | `scanning` had no stuck signal | T7.2.5 (split from T7.2.1): a 60 s client timer (no new field) with Rescan and Cancel waiting, the `failed` state and the "Last scanned" line | closed |
| 6 | Program-aware `not_linked` reason | T7.2.1 `transcript_supported` (shown by T7.2.5) (a different design from adding `program` to the tab props) | closed |
| 7 | Pin undo needs a previous-state snapshot | T4.2.3: client-side undo, no server RPC | closed |
| 8 | Multi-session match on the Up Next chip | T6.2.4 list lookup | closed |
| 9 | `#pull-requests` anchor | T6.2.1 `replaceHash` | closed |
| 10 | Merge-method select (S3) and S2 header Refresh had no RPC field | T4.1.1 `MergePRRequest.method`, T4.2.3 `RefreshSessionPullRequests`; UI in T6.1.3 / T6.2.3 | closed |
| 11 | S4 select beyond 4 segments; S9 single-match shortcut | T6.2.3; T10.1.2 | closed |
| 12 | S11 (issue-close prompt) needs an offer field and a close RPC | T5.5.1, T5.5.2, T5.5.3 | closed |

## Summary counts

- Surfaces designed: 11 (10 interactive with wireframe, flow and error tables; 1 condensed group).
- UX acceptance criteria: 69 (S1: 5, S2: 8, S3: 8, S4: 4, S5: 5, S6: 2, S7: 8, S8: 8, S9: 4, S10: 5, S11: 5, cross-cutting: 7).
