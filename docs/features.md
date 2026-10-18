# Stapler Squad — Feature Overview

## Dashboard

See all your AI agents at a glance. Status badges, diff stats, tags, and categories give you a complete picture without opening a single terminal.

![Dashboard](../assets/demo-recording/01-dashboard.png)

---

## Live Terminal Streaming

Full xterm.js terminal per session with real-time output streaming. Attach and reprompt any agent without leaving the browser.

---

## Browser Element Inspector

While watching an agent's browser in the **Browser** tab, click **Inspect**, then click any element in the live view. Stapler Squad queries Chrome over CDP and shows the element's cropped screenshot, `outerHTML` (truncated to 8 KB) and key computed CSS (display, position, width, height, color, background-color, font-size, font-family, border, margin, padding). **Send to Agent** types a `[Browser Inspector] Element at (x, y): HTML: … CSS: …` block into the session's terminal input without pressing Enter, so you can add a note before submitting. The screenshot stays in the panel; only text is sent.

- Requires a running Chrome/Chromium with CDP; **Inspect** is disabled until the stream is live.
- While Inspect is on, clicks are not forwarded to Chrome; the hover box is an approximation, and the exact element box is known after the click.
- Press `Esc` or **✕ Close** to leave Inspect mode.
- Endpoint: `POST /api/sessions/{id}/inspect-element` with `{"x": int, "y": int}`; returns 503 when CDP is not available for the session.

---

## Diff Viewer

Per-session git diff with VCS context. See exactly what each agent has changed before you approve or merge.

---

## Notifications

Real-time alerts when agents need attention — approvals pending, sessions paused, or errors detected — without polling.

---

## Instant Search

Type to filter sessions in real time across titles, paths, branches, and tags. "payment" narrows 6 agents down to 2 immediately.

![Instant Search](../assets/demo-recording/02-search-payment.png)

---

## Status Filtering

Filter by status to surface agents that need your attention — spot every `Needs Approval` gate without scrolling through the full list.

![Status Filter](../assets/demo-recording/03-needs-approval.png)

---

## Tag-Based Grouping

Switch between 8 grouping strategies — tag, category, status, branch, path, program, session type, or flat list. A single session can appear under multiple tags simultaneously.

![Group by Tag](../assets/demo-recording/04-group-by-tag.png)

---

## Workspace Switcher

Manage multiple project contexts from one UI. Each workspace gets its own isolated session state, configuration, and approval history.

---

## Bulk Actions

Select multiple sessions and act on them together — pause, resume, delete, or tag in one operation.

---

## Review Queue

Structured triage for every pending agent action. Review diffs and approve or reject before anything reaches your codebase.

![Review Queue](../assets/demo-recording/05-review-queue.png)

---

## Auto-Approval Rules

42 built-in rules block dangerous operations automatically. Read-only commands, safe git operations, and common dev tasks are pre-approved out of the box.

![Auto-Approval Rules](../assets/demo-recording/06-rules.png)

---

## Custom Rules

Teach Stapler Squad what's always safe for your project. Add a pattern like `^git log` once and never review it again.

![Custom Rule](../assets/demo-recording/07-add-rule.png)

---

## Approval Analytics

Visualise decision trends and classifier performance over time. See which rules fire most, how often agents are approved vs. blocked, and where your review time is going.

---

## History Search

Searchable, filterable record of every agent action across all sessions. Filter by session, time range, action type, or free-text search to find exactly what happened and when.

---

## Logs Viewer

Live-tail application logs with time range picker, multi-select filters, export, and density controls. No need to SSH in to debug an agent.

---

## Config Viewer

Inspect your current Stapler Squad configuration directly from the UI — no hunting for JSON files.

---

## Isolated Git Workspaces

Every agent gets its own git worktree. No branch conflicts, no dirty state bleed between sessions. Merge when you're ready.

![Final](../assets/demo-recording/08-final.png)
