# ADR-003: Single-Select for the "Multiple Choice" Question Type

**Status**: Accepted (Auto Mode — no open questions)
**Date**: 2026-09-11

## Context

Requirements open question 7 asks Phase 3 to decide single-select vs. multi-select
for "multiple choice," defaulting to single-select absent a concrete reason
otherwise. `research/stack.md` §6 found `web-app/src/components/ui/RadioGroup.tsx`
— a generic, already-accessible **single-select** primitive (`role="radiogroup"`,
roving tabindex, arrow-key-cycles-and-selects) already used by
`RuleBuilderForm.tsx`, `TriggerFormModal.tsx`, `WorkspaceSwitchModal.tsx`, and
`ThemePicker.tsx`. No existing checkbox-group primitive was found for multi-select.

## Decision

Single-select. `GuidanceRequestProto`'s multiple-choice answer is one string (the
chosen option's value), not a repeated field. `question_type =
GUIDANCE_QUESTION_TYPE_MULTIPLE_CHOICE` renders via the existing `RadioGroup`
component unmodified.

## Reasoning

- No consumer in the requirements or research (triage clarification, autonomous
  driver disambiguation) names a scenario needing more than one selected option
  — every example given ("which of these three approaches," "is this the right
  repo") is inherently exclusive-choice.
- `RadioGroup.tsx` is a direct, zero-new-code fit; a multi-select checkbox-group
  equivalent does not exist anywhere in `web-app/src/components/ui/` and would be
  net-new UI work with no current consumer, contradicting the "no rich/dynamic
  form schemas beyond yes/no, multiple choice, and short text ... unless trivial
  to include" scoping in requirements.md.
- A single scalar `answer` field (string) covers yes/no, multiple-choice, and
  short-answer uniformly in the ent schema and proto — one column/field, not a
  variant-shaped answer type — which keeps `GuidanceRequest`'s storage schema
  simple (see `session/ent/schema/guidance_request.go` in Phase 1, Epic 1.1).

## Consequences

**Positive**: no new UI component needed; `answer` stays a single `string` field
in both the ent schema and `GuidanceRequestProto`, keeping `AnswerGuidanceRequest`
trivial to validate (`answer` must be one of `choices` for multiple-choice,
`"yes"`/`"no"` for yes/no, non-empty for short-answer).

**Negative**: if a genuine multi-select need surfaces later, it requires a new
`question_type` value (e.g. `GUIDANCE_QUESTION_TYPE_MULTI_SELECT`) and a
`repeated string answers` field alongside (not replacing) `string answer`, plus a
new checkbox-group UI primitive — a real but currently-hypothetical follow-on,
not a blocking gap for this iteration.

## Alternatives Considered

**Multi-select via `repeated string`.** Rejected for this iteration per the above
— no concrete driving use case, and it would require inventing a UI primitive
this codebase doesn't have yet, contradicting the explicit non-goal of "rich
form schemas ... unless trivial."
