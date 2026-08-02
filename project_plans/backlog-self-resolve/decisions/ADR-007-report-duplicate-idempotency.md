# ADR-007: `report_duplicate` Idempotency — Reject, Don't Merge, a Differing Second Ref

## Status
Proposed

## Context

`report_pr_created` has a precedent idempotency guard: a retry with the *same* PR number
after the item already reached `pr_pending` is a no-op success (`tools_backlog.go:682-686`).
`report_duplicate` needs the analogous guard, but faces an extra wrinkle `report_pr_created`
doesn't: what happens if a *second* call arrives with a **different** `duplicate_ref` after
the first call already transitioned the item to `review`?

`UpdateItemSessionVerificationNotes` (`session/storage_backlog.go:397`) is a plain overwrite
(`SetVerificationNotes`), not an append/merge — confirmed directly, not inferred. Two
concrete triggers for this scenario: (a) the agent is genuinely unsure which of two possible
duplicates is correct and tries both refs in sequence; (b) an unrelated bug causes a second,
spurious call. Either way, once the item has left the whitelist (`in_progress`/`pr_pending`),
a second `TransitionBacklogItemStatus` call will fail on the CAS precondition regardless —
the only design question is what the *error message* says, and whether to silently drop the
second ref or surface it somewhere.

## Decision

**Reject the second call outright.** It falls through the same "item is at status %q —
report_duplicate only allowed from in_progress or pr_pending" `ErrInvalidArgument` branch
that any other disallowed-source-status call hits (the item is now at `review`, not in the
whitelist). No merge, no append, no special-cased "this looks like a second duplicate report"
detection. The second `duplicate_ref` is **not** persisted anywhere.

Rejected alternative: merge/append the second ref into `VerificationNotes` (e.g.
`existing + "\n" + new`). This requires a read-modify-write of `VerificationNotes` with its
own unaddressed race (two `UpdateItemSessionVerificationNotes` calls could still clobber each
other), and the routing decision ("this item goes to review, a human/reviewer figures out
which duplicate is real") already the single reviewer must resolve anyway makes the merge
add complexity without adding real value — the reviewer looking at a `review`-status item
with a `duplicate_ref` note has all the context needed to ask the reporting session (or check
GitHub directly) if a second candidate ref matters.

## Consequences

### Positive
- Zero new code beyond the existing whitelist check (Pattern Decisions, Epic 2.1/3.1) — the
  rejection is a side effect of the same check that already exists for FR1's CAS-trap fix,
  not a separate idempotency-specific code path.
- No new race window: `UpdateItemSessionVerificationNotes` is only ever called once per
  successful `report_duplicate` invocation (the one that actually transitions the item).

### Negative
- A genuinely-better second duplicate reference (e.g. the agent found a more precise PR after
  an initial imprecise one) is silently discarded rather than surfaced — the agent's second
  call just gets a generic "wrong status" rejection with no hint that it's *specifically*
  because a first duplicate report already succeeded. Acceptable for v1: the item is already
  routed to a human-visible `review` state; a reviewer noticing the discrepancy can correct it
  by hand, and the reject message plus `get_backlog_item` gives the agent enough information
  to understand why (item status is `review`, not `in_progress`/`pr_pending`).

### Neutral
- If this proves insufficient in practice (repeated confusion from agents), a follow-up could
  add a specific "duplicate report already recorded — see item status" message distinct from
  the generic whitelist-rejection text, without changing the underlying reject-not-merge
  decision.
