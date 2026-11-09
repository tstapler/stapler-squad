# Cutover gate results

Shared log for Epic 8 (Soak and cutover) stories. Each story appends its own dated section rather than overwriting another's; do not clobber existing entries.

## Story 8.4: Soak runbook and exit criteria

PR 8 has not merged yet, so no soak window has started and none of the dated fields below are filled in. This entry exists to carry the mechanism and defaults into place now, per the plan's AC, and will be appended to (not replaced) once PR 8 merges.

**Defaults (plan, Story 8.4; owner may lengthen, never shorten — record any change below with a reason):**

- First Soak window (PR 8 -> PR 8b gate): >= 14 calendar days and >= 3 distinct dev sessions after PR 8 merges, with zero reverts and no open P1 `web-build` issue.
- Second Soak window (PR 8b -> PR 9 gate): >= 7 further days after PR 8b merges.
- T3 (ADR-007 Vite 8 fallback trigger) is counted from CI across the whole Soak window: a Bun bundler crash or miscompile in >= 2 CI runs fires it, regardless of which PR's CI run it appears in.

**PR 9 deadline reminders (pre-mortem failure mode 9):** once PR 8 merges on date D, two dated reminders are created — D+21 (earliest PR 9) and D+45 (stop-loss decision point) — via `mcp__stapler-squad__schedule_wake` or a scheduled routine (mechanism: `schedule_wake` or a scheduled routine). Their links will be recorded here as a new dated entry once PR 8 actually merges; no date is fabricated in advance of that merge.

**Stop-loss principle:** the dual-build tax (two CI cells, aliases, conditionals in tests) must not become permanent. If PR 9 has not merged by D+45, the owner decides in writing — recorded as a new entry in this file — between finishing PR 9 and reverting to Next outright. Epics 1, 2c, 2d, and 7a remain valuable under Next either way, so a stop-loss revert at D+45 does not waste that work.

<!-- Next entry: owner's lengthen/shorten decisions, actual Soak dates, T3 occurrences, and the dated-reminder links, appended here once PR 8 merges. -->

## Story 8.5: Restart-safety precondition (Task 8.5a)

**Date**: 2026-10-22
**Command (read-only; never restarted, stopped, or otherwise touched the live service)**:

```
$ systemctl --user cat stapler-squad | grep -- --tmux-keep-server
ExecStart=/home/tstapler/Programming/stapler-squad/stapler-squad --remote-access --tmux-keep-server --profile --profile-port 6060
```

**Result**: present. The deployed systemd user unit's `ExecStart` already
includes `--tmux-keep-server` (shipped by #180 /
`docs/explanation/tmux-keep-server-on-restart.md`), so a restart via `make
install-service` does not tear down the tmux server or any live session.

Per the acceptance criteria: since the flag is present, no explicit decision
to accept session loss is required and Story 8.2 is not blocked on this
precondition. Owner: Tyler Stapler (tystapler@gmail.com). No change made to
the live deployment; this is a read-only observation.

The `install-service` Makefile preflight (Task 8.5d) now also enforces this
check at build time, independent of this manual record — see
`docs/how-to/rollback-web-build.md`.

**Tasks 8.5c and 8.5e** (required-check removal command rehearsal; rollback
flip rehearsal on a ManualInstance) both ran successfully with real command
output — see `docs/how-to/rollback-web-build.md` for the full transcripts
and timings rather than duplicating them here. Neither touched `:8543` or
any live tmux session.

## Story 8.2: Default flip (PR 8), gate status at PR time

**Date**: 2026-10-27. Recorded by the agent that prepared the PR; the owner decides whether to merge.

| Gate | Status | Evidence |
|---|---|---|
| 5x build-time ratio (Story 8.3) | MET | `docs/explanation/bun-migration-results.md` M2: 33.90 s / 1.88 s = 18.03x |
| `--tmux-keep-server` precondition (Story 8.5a) | MET | Entry above (2026-10-22) |
| Rollback runbook + rehearsal (8.5b-e) | MET | `docs/how-to/rollback-web-build.md` exists with transcripts |
| `e2e-bun` comparator green on `main` (Story 7.10 / 8.1 baseline compare) | **NOT MET** | Latest `main` Build runs (40539331462, 40392784895, 40366134530) fail `e2e (Bun build)`: comparator reports new Bun-only failures (e.g. backlog-copy-link.spec.ts, backlog-detail-live-updates.spec.ts) plus flaky-to-fail on enter-detection and history-behaviour. `e2e (Next build)` was green in the two newest runs |
| Story 8.1 ManualInstance 3-run gate, passkey, keyboard, screen-reader, iPhone checklists | **NOT RECORDED** | No section in this file; owner-run |
| `WEB_BUILD_GATE_MEMBERS` at the flip | Variable unset (`gh variable get` returns "not found"), so the script default `detect-changes,web-build-smoke-next,e2e-next` applies; Bun jobs are not gate members | `gh variable get WEB_BUILD_GATE_MEMBERS --repo tstapler/stapler-squad` |
| Green `workflow_dispatch` of the edited workflow (Story 8.2 precondition c) | see PR body | |

No required-check name changed (`web-build-gate` is unchanged; the only workflow edit pins `web-build-smoke-next` to `WEB_BUILD: next` so it stays the Next canary).

## Story 9.1-9.3: Owner decision to shorten the soak and remove Next (PR 9)

**Date**: 2026-11-01. **Decided by**: the owner (Tyler Stapler), in the session that prepared PR 9.

**Change to the Story 8.4 soak defaults**: the 14-day / 3-session first window and the 7-day second window are shortened to the time Bun has already been the default (PR #1206, 2026-10-28, about 4 days), and Next is removed now. **Reason**: the owner wants Bun speed and to drop the dual-build complexity. The plan lets the owner lengthen but not shorten these defaults without a recorded reason; this entry is that record. The D+21 and D+45 reminders (Story 8.4b) are moot.

**State at PR 9**:

| Item | Status |
|---|---|
| Soak window 1 (14 days, 3 sessions) | Not completed; shortened by the owner decision above |
| PR 8b (minification on) | **Not done.** `WEB_MINIFY ?= 0` stays in the `Makefile`; PR 9 does not turn minification on. Soak window 2 therefore never started |
| `e2e-bun` green on `main` | **Not met** when PR 9 was prepared: separate fixes (an a11y fix and an e2e flake triage) are in flight. PR 9 must not merge before that job is green on `main` |
| `web-build-gate` / `WEB_BUILD_GATE_MEMBERS` | Neither exists in this repo's workflows, so there is no variable to set before merging |
| Rollback | None to Next once PR 9 merges; forward fix only. Tag `web-dual-build-last` marks the last dual-build commit |

**Comparator without a Next control** (Story 7.10 follow-through): `e2e-bun` no longer has a same-run Next report to tell Bun-only failures from shared ones. `scripts/e2e-baseline-compare.ts` now treats Next as clean when no `--next-report` is given, so the committed per-test classes in `tests/e2e/baseline/next-e2e-baseline.json` (recorded from the last Next build, kept as is) are the gate: a baseline-`pass` test that hard-fails after the one-worker confirmation rerun fails the job, as does a failing spec absent from the baseline. Baseline entries whose test was deleted with the Next-only specs are reported, not failed.
