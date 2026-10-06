# UX Research: Does threatscan need any user-facing surface?

## Existing pattern: `RunPreGateSecurityCheck` block (session/review_gate.go:277-312)

This is the closest precedent — a pre-flight guardrail that blocks review-gate prompt
construction before it happens, using the exact same "scan, error, block" shape the
requirements doc proposes for strict-scope threatscan. Tracing what a user/operator
actually sees when it fires:

1. **Terminal FAIL verdict recorded** via `recordTerminalReviewVerdict` (session/backlog_review.go:574)
   with `summary := fmt.Sprintf("Review blocked by security check: %v. Override required to proceed.", secErr)`.
   `secErr` is always `fmt.Errorf("secret pattern detected: %s", p.name)` — pattern
   *name* only, never the matched substring (session/backlog_review.go:48).
2. **That summary renders directly in the existing UI**, no new component needed:
   `web-app/src/components/backlog/BacklogItemPanel.tsx:193-194` renders
   `item.gateVerdictSummary` as plain text, and `GateVerdictBox.tsx:280` (`<p className={styles.verdictSummary}>{summary}</p>`)
   does the same in the item detail view. Both are pre-existing, generic "show whatever
   string is in the FAIL verdict" surfaces — they don't know or care that the FAIL came
   from a security check vs. a normal failed review.
3. **A generic notification fires** via `r.getNotifier().Notify(itemID, "Review blocked by security check", fmt.Sprintf("%s — override required to proceed.", item.Title), NOTIFICATION_TYPE_ERROR, PRIORITY_HIGH)`
   (session/review_gate.go:288-297) → `server/services/backlog_notifier.go`'s
   `EventBusNotifier.Notify` → the existing notification bus consumed by
   `web-app/src/app/notifications/NotificationsPage.tsx` and
   `web-app/src/components/ui/NotificationToast.tsx`. Same generic (title, message,
   type, priority) shape used for every other notification in the app.
4. **Feeds the standard auto-reopen/rework loop** (`AutoReopenAfterFailedReview`) —
   identical to any other FAIL verdict, capped by `maxAutoReworkIterations` with the
   existing cap-hit notification path.

**Conclusion: zero new UI was built for this precedent, and none would be needed for a
strict-scope threatscan block that follows the same shape.** The FAIL-verdict-summary
and generic-notification paths are both string-in/string-out already; a threatscan
block just needs to produce a good string.

## Where strict-scope threatscan would actually plug in — this differs from the precedent

`RunPreGateSecurityCheck` scans a computed diff at a point in `review_gate.go` that
already returns/handles errors procedurally. The requirements doc's actual strict-scope
targets are different: `BuildSessionInitialPrompt`, `BuildHeadlessReviewPrompt`,
`BuildHeadlessTriagePrompt`, `BuildHeadlessRetriagePrompt` — all four are **pure string
builders today** (`func BuildHeadlessReviewPrompt(...) string`, confirmed at
session/backlog_review.go:307; same shape for the other three). None of them return
`error`. `PipelineEngine.ReviewPromptFor` / `TriagePromptFor` (session/pipeline_engine.go:378,402)
wrap them and also return plain `string`.

This means wiring a "return an error, block the gate" behavior (AC #3 in requirements.md)
is not just "call Scan() inside the builder" — it requires threading an `error` return
through the builder signatures and every call site (`review_gate.go`, `backlog_commands.go:181`'s
`WriteBacklogContextFile`, triage call sites in `pipeline_engine.go`, any retriage path).
That's a plan-phase/architecture concern, not a UX concern — but it matters for UX
because **where** the error surfaces depends on which call site catches it:
- If caught at `review_gate.go`'s existing block (review/triage spawn time), it can reuse
  the exact FAIL-verdict + notification path above with zero new UI.
- If caught earlier, e.g. at session-creation time when `BuildSessionInitialPrompt` is
  first invoked for `WriteBacklogContextFile`, the error would instead propagate as a
  synchronous RPC error back through `server/services/*.go` to whatever UI action
  triggered creation (Omnibar/session creation flow) — also an existing generic
  error-propagation path (toast/inline form error), not new UI, but a **different**
  existing path than the FAIL-verdict one. This decision (which call site owns the
  block) is a plan-phase question, not identified as resolved by any code found so far.

## Recommendation for the error message content (no new UI, just string content)

Since the block manifests only as a message string riding an existing generic channel,
the design work is entirely in what that string says. Two constraints must both hold:

1. **Legitimate user must be able to self-fix.** Unlike the secret-scanner precedent
   (`"secret pattern detected: %s"`, pattern name only), a threatscan block on backlog
   title/description/AC text needs to point at *which field* to edit, since the user
   didn't attach a diff — they wrote prose. E.g.:
   `"Content blocked by security scan (pattern: role_play_hijack) — review your item's title/description/AC text for phrasing that could be mistaken for AI instructions, then retry."`
   Mirroring `RunPreGateSecurityCheck`'s existing convention of surfacing the pattern
   *name* (`p.re` never appears in the error) is directly reusable here — same
   discipline, same reason (don't teach an attacker the exact regex via the block
   message).
2. **Never reveal exploitable pattern internals** — per the requirements doc's own
   constraint ("Pattern IDs for logging — never log the matched substring, only the
   pattern name"), the same rule should apply to the user-facing string: pattern ID,
   not matched text or regex source.

## Bottom line

No new UI component, page, or frontend code is needed. This is purely a backend
error-string content question riding two pre-existing generic error-propagation
surfaces (FAIL-verdict summary text, and the notification bus) — plus, depending on
plan-phase call-site choice, possibly a third pre-existing generic surface (synchronous
RPC error at session-creation time). The only UX-relevant deliverable is the wording of
the blocked-content error message itself (see recommendation above), which should be
decided in the plan phase alongside the call-site decision, since the two are coupled.
