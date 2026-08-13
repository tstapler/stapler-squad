# Architecture Review: launchd-shell-sourcing
**Date**: 2026-07-25
**Verdict**: CONCERNS

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this
repository (`docs/adr/` contains `ADR-001-gemini-exit-code-contract.md` and
numbered ADRs 003-017, but no `ADR-000-architecture-constitution.md`). No
constitution to check the plan against — this section is N/A.

## Constitution Violations
- none (no constitution file present)

## Context

This is confirmed as a Complexity-1, documentation-only task: zero new code,
one Markdown insertion (`BUG-006`) into an existing doc following an
established five-entry format, plus two `/backlog/fail-N` reports for
tool-capability gaps. Lenses 1-2 (SOLID, layer coupling, DDD boundaries,
primitive obsession, illegal states, parse-at-boundary, PoEAA/GoF fit, API
contract design) are legitimately N/A — there is no code, no types, no
service/persistence layer in scope. Findings below are limited to what
actually applies: testability of the verification story, and — per the
explicit instruction — consistency of the BUG-006 content plan with
`research/build-vs-buy.md`'s recommendation.

## Blockers

(none)

## Concerns

- [ ] **Task 2.1 (BUG-006 content plan) / Pattern Decisions / Risk Control
  table** — the plan instructs the writer to list, as a "Possible Future
  Mitigation" for macOS, **"install-time plist inlining or a minimal
  `. env-file` wrapper under `/bin/sh`"** — presenting the wrapper as a
  co-equal alternative to plist inlining. This is sourced from
  `research/stack.md` §2 (which calls the wrapper "safe" because POSIX `.`
  isn't interactive-shell sourcing), but it is **not** what
  `research/build-vs-buy.md` — the doc this session's own Unresolved
  Questions section and this review's charter both point to as the
  authoritative build-vs-buy recommendation — actually recommends.
  `build-vs-buy.md` §1 and §4 describe exactly one macOS mechanism:
  "generation-time inlining into `EnvironmentVariables`" performed by the
  installer script itself, and explicitly frames a wrapper-process-at-start
  approach as the disfavored shape ("reintroduces a wrapper process at
  service-start time, similar shape to the shell-wrapper anti-pattern this
  item just removed" — said of `op run`, but the same reasoning applies to
  any `/bin/sh` wrapper invoked by `ProgramArguments` at service start).
  Concretely: a `/bin/sh -c '. "$file" && exec "$bin_path" ...'`-style
  `ProgramArguments` entry is structurally a shell wrapper around the
  binary — the same category of thing acceptance criterion 1 was written to
  rule out (`ProgramArguments` invokes the binary directly, no shell
  wrapper), even though POSIX `.` of a flat `KEY=VALUE` file has none of
  `.zshrc`'s interactive hazards (no prompts/`stty`/hangs). Two research
  docs disagree (`stack.md` endorses the wrapper as safe; `build-vs-buy.md`
  recommends only inlining and is wary of wrapper processes generally) and
  the plan resolves the disagreement by keeping both options in the
  doc-entry draft rather than deferring to `build-vs-buy.md`'s narrower,
  more-considered recommendation.
  **Remediation**: In Task 2.1, drop the `. env-file` wrapper bullet (or
  demote it to a single clearly-secondary clause, e.g. "plist inlining at
  install time — the wrapper-script alternative sketched in early research
  was set aside because it reintroduces a shell process at service start,
  the same class of thing this fix removed") so the doc entry matches
  `build-vs-buy.md`'s actual recommendation: `EnvironmentFile=` (Linux) +
  install-time plist inlining (macOS) only. This keeps the deferred-doc
  entry from planting a wrapper-script idea that a future maintainer could
  implement verbatim and unknowingly reintroduce a shell-wrapper pattern.

## Nitpicks

- Story 1's verification (Tasks 1.1-1.3) re-confirms criteria 0-2 by manual
  `grep`/`Read` rather than adding an automated regression check (e.g. a
  lightweight shell/Go test asserting `install-service.sh` never emits a
  shell-wrapper `ProgramArguments`/`ExecStart`). Reasonable to leave out of
  scope for a Complexity-1 doc task, but worth noting since it's the kind of
  invariant that's easy to silently regress later; BUG-006's own
  `**Prevention:**` field (Task 2.1) already gestures at a future regression
  test for the *env-file* mechanism, which would be a natural place to also
  cover the "no shell wrapper" invariant if/when that future work happens.
