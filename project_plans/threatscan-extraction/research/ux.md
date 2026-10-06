# UX research: threatscan extraction

## Summary

This refactor touches no frontend files (confirmed: requirements.md's scope is
`pkg/threatscan`, `session/backlog_review.go`, `session/review_gate.go` only).
The one user-facing surface identified — `BlockedNotice.tsx` — is a **generic
string-passthrough component**, not a parser, so it has no format dependency
on the refactor. Two points are worth a one-line note for future work (AC 6 /
companion issue #115), not action items for this item.

## 1. Does BlockedNotice.tsx parse or tolerate the "secret pattern detected: <name>" string?

Generic passthrough — confirmed by reading
[`BlockedNotice.tsx:72-84`](../../../web-app/src/components/backlog/detail/BlockedNotice.tsx#L72-L84):

```tsx
const summary = session.reviewVerdict?.summary;
...
<p className={styles.summaryText}>{summary || config.fallbackText}</p>
```

No regex, no substring extraction, no format assumption — it renders whatever
string lands in `session.reviewVerdict.summary` or falls back to a fixed
`"No summary recorded."` string. It cannot break from a format change in the
underlying error string.

The byte-for-byte preservation requirement in requirements.md (AC 10, "Current
state") is a **backend contract**, enforced by
`TestRunPreGateSecurityCheck_should_NeverEmbedRawSecretSubstringInErrorString_When_SecretDetectedInDiff`
in `session/backlog_review_test.go` and by the string construction at
[`session/review_gate.go:281`](../../../session/review_gate.go#L281):
`fmt.Sprintf("Review blocked by security check: %v. Override required to proceed.", secErr)`.
`BlockedNotice.test.tsx` (lines 13-14, 21-22) hardcodes a full example summary
string as test *fixture data* it constructs itself — it is not asserting that
the component parses or validates that format, so it also doesn't constrain
the migration.

**Conclusion**: the plan does not need to preserve the error string
byte-for-byte for frontend correctness. It should still preserve it, because
the existing Go test and the "no raw secret value" security contract both key
off the exact `"secret pattern detected: %s"` template (AC 2, AC 10) — that's
a backend-security reason, not a UX one.

## 2. Would reusing "secret pattern detected" for non-secret pattern categories mislead users?

Yes, if it happened — but AC 6 explicitly does not wire the new prompt-injection
/ HTML-injection / exfiltration categories into `RunPreGateSecurityCheck`'s
existing call site (diff scan, strict scope, secrets-only), so this item does
not introduce that problem. Flagging for whoever picks up companion issue
#115 (backlog-field scanning before LLM injection): when a non-secret
`ThreatMatch` category is ever surfaced through this same `BlockedNotice`
path, the error-string template must not literally say "secret pattern
detected" for a prompt-injection or HTML-injection match — that would
misrepresent the block reason to the user reading the notice. The fix is
cheap (branch the message template on `ThreatMatch.Category` or similar) but
is out of scope here per requirements.md's "Explicitly out of scope" section.

## 3. Accessibility / error-state handling already present — must not regress

Not at risk from this item since no frontend files are touched, but recorded
for the record (`BlockedNotice.tsx:72-84`, `BlockedNotice.test.tsx`):

- `role="status"` (polite live region) on the outer notice container.
- Icon `<span aria-hidden="true">` — decorative icon excluded from the
  accessibility tree, label text carries the meaning.
- No interactive affordance (no `link`/`button`) rendered for
  `blocked_guardrail` — asserted explicitly in the test
  (`queryByRole("link")`/`queryByRole("button")` both expect nothing), since
  a blocked-before-start row never had a live session to open.
- Fixed, non-blank fallback text (`"No summary recorded."` /
  `"No diagnostic data recorded."`) rather than an empty box when
  `reviewVerdict.summary` is undefined.

None of these are exercised by the Go-side refactor; they're listed only so a
future PR touching `pkg/threatscan` → `backlog_context.go` wiring (issue #115)
knows what invariant it must preserve if it starts affecting this UI path.
