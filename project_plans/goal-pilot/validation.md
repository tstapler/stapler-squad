# Validation: goal-pilot

## AC Coverage Check

| AC | Plan Coverage | Risk |
|---|---|---|
| AC1: GoalMode launch option with feature flag | T1 (runner), T2 (flag) | Low — flag wiring is well-understood in this codebase |
| AC2: stream-json parsing → structured outcome fields | T1 (stream parser) | Medium — schema not independently verified against a live CC instance |
| AC3: cost ceiling → `cost_exceeded` outcome | T1 (cost watch) | Low — pattern exists in `session/headless/cost_ceiling.go` |
| AC4: 60s heartbeat log line | T1 (heartbeat) | Low |
| AC5: 10 items piloted, results recorded | T5 (operator run), T6 (results doc) | Medium — requires operator time and real items |
| AC6: verdict in `docs/explanation/goal-pilot-results.md` | T6 | Low |
| AC7: no tmux, subprocess direct launch | T1 (ExecCommand, no tmux) | Low — ExecCommand is already used for headless sessions |
| AC8: existing autonomous driver unchanged | Architecture (additive only) | Low — flag is off by default |
| AC9: hooks compatibility verified | T4 (compat test) | Medium — untested interaction |

## Go Compatibility

The new `session/goalrunner` package:
- Imports: `session/launchcmd`, `session/domain`, `log`, `config` — no upward imports into `session` or `server`.
- Depguard: needs an `isolate_goalrunner` row added to `.golangci.yml`.
- Boundaries: `!server`, `!session` (top-level), following the leaf-package rule.
- BUILD.bazel: add after `make gazelle`.

## Missing Details / Open Questions

1. **Exact `stream-json` schema per CC version** — the schema assumed in research.md is from
   docs; the actual field names need empirical verification. T4's compat test should capture
   real output from a live `claude --output-format stream-json` call and assert the fields.

2. **Baseline metrics** — the plan says "last 10 chore/docs items on the autonomous driver."
   This needs a DB query or log grep to actually surface those 10 items before T6 can
   produce a valid comparison. Add a sub-task: query `nudge_outcome` rows filtered to `chore`
   class and extract the metrics.

3. **Item criteria field** — where does `<criteria>` come from? The item's description field,
   the acceptance criteria, or a new dedicated field? The plan assumes it comes from the item's
   existing description/criteria. If a new field is needed, that's a schema migration (ent) that
   adds ~1h to T1.

4. **N turns value per item** — hardcoded 20 is the default; should it be per-item configurable
   via a field or just a global config? For the pilot, global is sufficient.

## Go ahead

The plan is scoped correctly for a pilot. It is additive (flag-off by default), has clear
metrics for the verdict, and explicitly excludes full migration. The main risks are the hook
compatibility interaction and the stream-json schema assumptions — both are addressed by T4.
The plan is ready for implementation with one clarification needed: how `<criteria>` is
sourced per item (description vs AC list vs new field). For the pilot, using the item's
`Description` field truncated to the first 500 chars is a safe default; document that choice
in the results doc.
