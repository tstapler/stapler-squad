# Backlog UX Audit

Open the backlog in Playwright, inspect each status category (Idea, Ready, In Progress, Review, Done), and report bugs in the UI with code fixes.

## When to use
- Proactively after backlog UI changes
- When the user says "audit the backlog" or "check the backlog UI"
- After adding new backlog status flows (review gate, triage, etc.)

## Steps

### 1. Open the backlog and take inventory

```bash
# Navigate to backlog (local dev server)
# URL: http://localhost:8543/backlog
```

Use Playwright to:
1. Navigate to `/backlog`
2. Screenshot the full list — note any status badges (REVIEW, IN PROGRESS, etc.)
3. For each status present, click one item to inspect the detail panel

### 2. For each status, check these things

**REVIEW items** (highest-value to check):
- Does the REVIEWING section show the active review session link?
- Can you click the review session to open the terminal?
- Does the SessionMonitor render and show terminal/history output?
- Is the Gate Verdict section up to date?

**IN PROGRESS items**:
- Does the SessionMonitor show terminal output (not "No output yet")?
- Does the History tab show conversation messages?
- Does "↗ Open" link navigate to the live session?

**All items**:
- Sessions list: are clickable sessions showing as links (not plain text)?
- Headless review sessions (`headless-review-*`) should be links, not plain text
- Headless triage sessions (`headless-triage-*`) and `review-blocked-*` should remain plain text

### 3. Known bug patterns to look for

| Bug | Location | Symptom |
|-----|----------|---------|
| `headless-review-*` sessions unclickable | `BacklogItemDetail.tsx` sessions list | Review sessions show as plain text, no terminal link |
| SessionMonitor missing for review | `BacklogItemDetail.tsx` monitor finder | Items in REVIEW show no live monitor |
| REVIEWING section missing review session | `BacklogItemDetail.tsx` REVIEWING block | Only shows work session, not the active review agent |
| Session monitor shows "No output yet" | `SessionMonitor.tsx` | Terminal not fetched; verify both fetchTerminal + fetchConversation run |
| "No conversation history yet" with no fallback | `SessionMonitor.tsx` | Was fixed: History tab now separate from Terminal tab |

### 4. The fix pattern for review session wiring

In `BacklogItemDetail.tsx`, the REVIEWING section, sessions list, and SessionMonitor all need to treat `headless-review-*` like a work session, not a triage session:

```tsx
// Sessions list — allow headless-review to be a link
s.role === "triage" || s.sessionId.startsWith("headless-triage-") || s.sessionId.startsWith("review-blocked-")
// NOT: s.sessionId.startsWith("headless-")  ← too broad, catches review sessions

// SessionMonitor finder — include review sessions even if headless
.find((s) => !s.endedAt && s.role === expectedRole && !s.sessionId.startsWith("review-blocked-") &&
  (expectedRole !== "review" || !s.sessionId.startsWith("headless-triage-")))
// NOT: !s.sessionId.startsWith("headless-")  ← excludes headless review sessions

// REVIEWING section — find + show active review session
const activeReviewSession = [...item.linkedSessions].reverse().find((s) => s.role === "review" && !s.endedAt);
// Then render a link: <a href={`/?session=${activeReviewSession.sessionId}`}>
```

### 5. After fixing, verify

```bash
make quick-check
make install-service
```

Then re-run this audit in the browser to confirm:
- Review items show a SessionMonitor with Terminal/History tabs
- Clicking the review session ID opens the session terminal
- The REVIEWING section shows both work session and active review session

## Files to inspect

| File | What to check |
|------|--------------|
| `web-app/src/components/backlog/BacklogItemDetail.tsx` | Sessions list, REVIEWING section, SessionMonitor finder |
| `web-app/src/components/backlog/SessionMonitor.tsx` | Terminal/History toggle, both fetches fire |
| `web-app/src/components/backlog/BacklogItemPanel.tsx` | Side panel on session view (separate from detail) |
