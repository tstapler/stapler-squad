# Requirements: durable-guidance-requests

**Date**: 2026-09-11
**Type**: feature addition (existing project: stapler-squad)
**Backlog item**: `1c08da73-d569-47df-ab9b-676301748159` — "Durable structured Q&A / form mechanism for LLM-to-user guidance requests"

**Note on process**: This project was launched as a full SDD run in a non-interactive
worktree-agent session (no live user turn-by-turn available for the Phase 1
`AskUserQuestion` interview). The user's original request already supplied the
substance of every interview question (problem, users, success criteria, constraints,
scope, exclusions) in prose. This document restates that prose into the standard
requirements template rather than re-asking it. Anything genuinely undecided is
listed under Open Questions for the research/plan phases to resolve.

## Problem Statement

There is no durable way for an LLM-driven actor in stapler-squad — a backlog-driven
session, automated triage, or any other stapler-squad-managed agent — to ask the user
a structured question (yes/no, multiple choice, or short form) and be notified
asynchronously when it's answered.

Today the only mechanism is a live chat turn inside an interactive session. Anything
running unattended — automated triage evaluating a backlog item, a paused or idle
session, a background workflow — has no way to pause and durably request guidance.
The practical consequence (per the linked triage investigation in memory,
`instinct_code_review.md` / `project_backlog_stuck_review_investigation.md`-adjacent
work) is that unattended automation either guesses, silently proceeds on an
assumption, or gets stuck with no structured path to ask for clarification.

## Users / Consumers

- **Automated triage** (`session/backlog_triage.go`) — needs to ask a clarifying
  question about an ambiguous backlog item instead of guessing or silently
  proceeding.
- **Backlog-driven / autonomous sessions** (`session/autonomous_driver.go` and any
  session created to work a backlog item) — needs to pause unattended work and
  request guidance without staying alive to receive a live chat reply.
- **The human user** — answers the structured question through stapler-squad's own
  UI surfaces (backlog item detail, triage panel, session view), not through a chat
  reply.
- **Any other stapler-squad-managed agent** going forward — the mechanism must be
  general enough that a new call site doesn't require inventing a new dialog.

This is both a human-facing feature (the user answers) and an automated-system
feature (the LLM/session asks and later reads the answer) — both sides matter to the
design.

## Success Metrics

- Automated triage can ask a structured clarifying question on an ambiguous item and
  receive the answer on a later triage pass (or a resumed/fresh session), without a
  human needing to intervene in any way other than answering the question.
- A question created by a session that is later paused, restarted, or no longer live
  is still answerable by the user, and the answer is still durably retrievable by
  whichever session/process later picks up the same backlog item or checks for it.
- The construct renders as a structured form (not a raw chat message) in at least
  the three named UI surfaces: backlog item detail, triage panel, session view.
- No existing behavior regresses: sessions/triage that don't use this construct
  continue to work exactly as before it's added.

## Constraints

- Must integrate with existing durable/async patterns already in the codebase
  rather than inventing a parallel notification system — specifically the
  `WatchBacklogItems` streaming pattern (`proto/session/v1/backlog.proto` +
  frontend subscription) and whatever `notify()` call/service already backs
  "post a visible comment + notify()" (referenced in
  `feedback_document_ai_decisions_in_edge_cases.md`) — the research phase must
  identify and cite the actual code for both.
- Must survive process/session restarts: the question, its answer, and the
  association back to the asking party must be persisted, not held in memory.
- Must not require the asking session to remain alive to receive the answer.
- Standard repo gates apply before shipping: `make ci` / `make ready` must pass;
  this is a Go backend (ConnectRPC) + React/Next.js web-app change, so both
  backend and frontend conventions in this repo's `CLAUDE.md` apply (proto →
  `make proto-gen`, feature registry markers, e2e test conventions, etc.).
- PRs in this repo are ready-for-review by default (not draft) per repo
  `CLAUDE.md`.

## Scope

### In Scope

- A durable "Question" / "Guidance Request" construct that can be created by any
  subscribed LLM/session, scoped to one of: a backlog item, a session, or standalone
  (no owning entity).
- Support for at minimum: yes/no, multiple choice (single-select from a fixed list),
  and short free-text answer question types.
- Persistence of the question and its answer such that a restarted/resumed
  session (or a different session picking up the same backlog item) can read the
  answer.
- A durable notification path to the asking party when the question is answered,
  reusing/extending the existing notification mechanism rather than a new one, to
  the extent research confirms that's viable.
- Rendering the question as a structured form in the backlog item detail view, the
  triage panel, and the session view (exact component-level design is a Phase 3
  planning decision, informed by Phase 2 research into current view structure).
- Wiring automated triage (`session/backlog_triage.go`) to use this construct for
  at least one real clarification scenario, replacing "guess or silently proceed"
  with "ask and wait/defer."
- New proto messages/RPCs as needed (this is expected to be a genuinely new
  cross-cutting construct per the task description).

### Out of Scope (for this PR / initial delivery — may become follow-on work)

- Rebuilding the live interactive chat Q&A path — that mechanism already exists
  and is not being replaced, only supplemented for the unattended case.
- Rich/dynamic form schemas beyond yes/no, multiple choice, and short text
  (e.g. multi-field forms with conditional logic) unless Phase 2/3 research finds
  this is trivial to include; otherwise it's an explicit follow-on.
- Any specific UI visual redesign beyond what's needed to render the new
  question/answer form component.
- If Phase 2/3 planning concludes the full scope (durable storage + notification
  foundation, *and* UI integration across ≥3 views, *and* wiring triage to use it)
  is too large for one PR, the plan must say so explicitly and propose a split
  (e.g. "foundation" PR followed by a "UI integration" PR) rather than silently
  cutting scope or forcing one oversized PR. This decision is made in Phase 3 with
  reasoning shown, not unilaterally in Phase 1.

## Open Questions

(For Phase 2 research and Phase 3 planning to resolve — not blocking Phase 1.)

1. What is the actual `notify()` call/service referenced in
   `feedback_document_ai_decisions_in_edge_cases.md` ("self-heal/auto-close actions
   should post a visible comment + notify()")? Confirm the real function/service
   name and file.
2. What is the current shape of `WatchBacklogItems` (proto message, streaming RPC
   implementation, frontend subscription) and how directly can a new
   "GuidanceRequest"/"Question" entity reuse that same watch/subscribe
   infrastructure vs. needing its own watch stream?
3. Where does `session/autonomous_driver.go`'s idle-nudge prompt mechanism live
   architecturally, and is "ask a durable question and go idle/pause until
   answered" a natural extension of it, or a separate code path?
4. What does `session/backlog_triage.go`'s current control flow look like at the
   point where an item is ambiguous — what would "ask and defer" concretely
   replace, and does the triage pipeline have a natural "waiting for guidance"
   state today, or does one need to be added?
5. Standalone-scoped questions (no backlog item, no session) — what UI surface do
   these render in if not backlog item detail, triage panel, or session view? Is a
   basic global "guidance inbox" view needed, or is standalone scope lower
   priority for this iteration?
6. Should answering a question be one of the actions gated by the existing
   approval-rule system (`upsert_approval_rule`/`list_approval_rules`), or is it
   always a direct, ungated user action since the user is the one being asked?
7. Multi-select vs. single-select for "multiple choice" — the feature request says
   "multiple choice" without specifying; default assumption is single-select
   unless research surfaces a concrete need for multi-select.
