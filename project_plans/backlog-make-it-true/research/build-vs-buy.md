# Research: Build vs. Buy — backlog-make-it-true

Scope: workstreams A (workflow engine), B (settings/quota), C (`allow_auto_merge` lookup),
E (config-surface-has-runtime-test enforcement). Workstream D is covered by the final
"Soak evidence for flag defaults" section. Library facts about `looplab/fsm`, `qmuntal/stateless`
and Temporal are carried over from the earlier plan
(`project_plans/backlog-custom-workflow-stages/research/build-vs-buy.md:17-34`) and were NOT
re-verified against the registries today (UNVERIFIED for versions/dates). In-repo facts below
were checked in this worktree.

Standing discipline applied: prefer a battle-tested library only when it removes work that is hard
and risky; prefer in-repo patterns when the library would only replace already-correct plumbing
(`.../backlog-custom-workflow-stages/research/build-vs-buy.md:189-216`, ADR-013).

---

## A. Config-driven workflow engine / state machine

Key framing (VERIFIED, `requirements.md:11`): the engine already exists. ADR-013
(`docs/adr/013-workflow-engine-replaces-valid-transitions.md`) defines the `WorkflowEngine`
interface (`CanTransition`, `ValidateGates`, `AllowedTransitions`) and names `ConfiguredWorkflowEngine`
as the drop-in. The gap is wiring (`server/dependencies.go:678`, `:1477`), not engine choice. The
question is therefore "adopt a library to replace/complement `ConfiguredWorkflowEngine`?"

### A1. In-repo `ConfiguredWorkflowEngine` (wire it in)
- Pros: already built and cache-backed; satisfies the NFR "no per-call DB round trip"
  (`requirements.md:62`); same interface as `DefaultWorkflowEngine`, so zero call-site change;
  shadow-mode comparison against `DefaultWorkflowEngine` is trivial because both implement one
  interface; reuses the `pipelineModeCache` and fail-closed-with-Warn idioms
  (`.../build-vs-buy.md:113-141`); no new dependency (VERIFIED: `go.mod` has no `looplab/fsm`,
  `qmuntal/stateless` or Temporal SDK; `grep -n "stateless\|looplab" go.mod` printed nothing).
- Cons: bespoke code that LLMs wrote, so graph-validation edge cases (self-loops, dangling statuses,
  removed stages with in-flight items) are the risk; needs adversarial table tests and the
  byte-for-byte seed-equivalence test from `requirements.md:116`.
- Verdict: **Recommended.**

### A2. `qmuntal/stateless` (or `looplab/fsm`)
- Pros: well-known, thread-safe guard/entry/exit hooks, DOT export.
- Cons: models transition legality only, which the repo already answers; has no gate taxonomy
  and no notion of stage liveness; would force either migrating the built-in 9-state graph onto it
  (churn, no benefit) or running two transition mechanisms side by side (the anti-pattern the
  earlier plan rejected, `.../build-vs-buy.md:26`). Stateless FSMs are normally built from code
  at construction, so a DB-edited graph means rebuilding the machine on every config change and
  re-deriving `AllowedTransitions` from its permitted-triggers API; that adds an adapter layer with
  no correctness gain.
- Verdict: **Not recommended.**

### A3. Temporal (embedded or Cloud)
- Pros: durable execution, timers.
- Cons: solves distributed durable execution, which a single-operator local tool does not have;
  embedded server is documented dev/test-only in the earlier research
  (`.../build-vs-buy.md:31`, UNVERIFIED today); re-platforms the autonomous driver. Cost and
  size are wildly out of proportion to "make an existing engine take effect."
- Verdict: **Not recommended.**

### A4. Small graph-validation library (e.g. `dominikbraun/graph`) for save-time cycle/reachability checks
- Pros: removes a classic LLM-bug class (DFS visited-set mistakes).
- Cons: one new dependency for ~50 lines; the earlier plan already judged "viable, lean bespoke
  with adversarial tests" (`.../build-vs-buy.md:79-87`). Note a cycle is not necessarily an
  error here (the default graph likely has rework loops, e.g. review back to in_progress), so
  "cycle detection" may be the wrong check; reachability and no-dangling-status checks matter more
  (INFERRED, check `validTransitions` in `session/backlog.go` before planning).
- Verdict: **Viable** (lean bespoke, same as before).

LLM-bespoke vs library summary for A: the risk in this workstream is not the graph algorithm but
behavioral drift when the enforcing engine swaps. No library mitigates that; shadow mode plus the
seed-equivalence test does.

---

## B. Settings / quota surface

Facts (VERIFIED): `AssumedWindowTokenBudget int64` is a plain JSON field
(`config/types.go:759-763`, default 0 by design at `:792`). Feature flags are live-settable via
`GetFeatureFlags`/`UpdateFeatureFlag` RPCs (`proto/session/v1/session.proto:504-508`) and
`cfg.GetFeatureFlagWithDefault` (`config/config.go:555`). `go.mod` has no viper/koanf (only the
indirect `go-viper/mapstructure`, `go.mod:153`).

### B1. Extend the existing config + feature-flag plumbing, add a settings page
- Pros: matches the standing rule that rollout flags are live-settable through the flag RPC/panel,
  never env vars; no new dependency; the config reload path and `status_detail` display
  (`settings/features/page.tsx:170`) already exist; the "0 disables the proactive signal" coupling
  can be made legible in the same page.
- Cons: bespoke validation (ranges, "0 means off") and a new RPC or flag-shape for numeric values
  (the flag RPC is boolean-shaped: INFERRED from the `enabled` field in the live response, so a
  numeric budget needs a config RPC rather than a flag); must write a runtime-effect test per
  setting (see E).
- Verdict: **Recommended.**

### B2. Adopt a config library (viper/koanf) for live reload
- Pros: file watching, typed unmarshal, layered sources.
- Cons: the repo already has `config.LoadConfig()` read at the decision instant (e.g.
  `server/mcp/tools_diagnose.go:343`), so live reload is solved; a library would create a second
  config source of truth; large migration for a few fields.
- Verdict: **Not recommended.**

### B3. Derive the budget from observed usage instead of a user number (open question 4)
- Pros: removes the hidden zero-disables coupling.
- Cons: no safe derivation exists today (`config/types.go:792` says "no safe guess exists");
  needs research on available usage telemetry. Not a build-vs-buy question, listed so it is not
  lost.
- Verdict: **Viable as a later enhancement; not in this workstream's critical path.**

---

## C. `allow_auto_merge` lookup

Facts (VERIFIED): `go.mod` does not contain `google/go-github` (`grep -c "google/go-github" go.mod`
printed 0). The repo has a native `github/` client: `NewConditionalRequest` with ETag cache
(`github/http_client.go:342-362`), `NewConditionalRequestNoCache` (`:367`), a rate-limit transport
(`:78`), per-host token handling (`:276`), and an existing REST repo helper file `github/repos.go`
(`GetIssue`, `GetPR`, ...). `EnablePRAutoMerge` shells out to `gh pr merge --auto --squash`
(`session/git/worktree_git.go:1139-1148`). The `norawghrequest` analyzer
(`.claude/rules/norawghrequest.md`) forces new calls through the constructors.

### C1. Native `github/` client: `GET /repos/{owner}/{repo}` via `NewConditionalRequest` with the ETag cache
- Pros: reuses auth, GHE host support, rate-limit transport and ETag/304 savings (304 costs zero
  quota, per the rule file); satisfies the lint rule by construction; no dependency; one small
  function (~30 lines) plus decode of the `allow_auto_merge` bool. A repo's setting changes rarely,
  so ETag-conditional polling is cheap and "persist vs. render-time" staleness trade-off
  (`requirements.md:96`) mostly disappears.
- Cons: bespoke JSON struct for a single field (low risk); the ETag cache is keyed by
  `RepoRef + prNumber` for PRs (`github/etag_cache.go:52`), so a repo-level key needs a small
  cache-key addition or use of the raw-path `cache.get(path)` seen at `http_client.go:355`
  (INFERRED: check that `cache.set` accepts arbitrary path keys before planning).
- Verdict: **Recommended.**

### C2. Add `google/go-github`
- Pros: typed `Repository.AllowAutoMerge`; maintained by the community.
- Cons: new dependency whose `*http.Client` would have to be wired through the repo's rate-limit,
  priority-admission and telemetry transports to avoid bypassing them (`http_client.go:63-135`),
  duplicating auth/host logic; its requests bypass `NewConditionalRequest`, so the repo's
  ETag and lint-rule intent is lost (the analyzer only catches `http.NewRequest*`, so it would
  silently not flag go-github, which is the exact "compiles, passes, costs quota forever" failure
  the rule exists to prevent). Buys nothing for one boolean.
- Verdict: **Not recommended.**

### C3. Shell out to `gh api repos/{owner}/{repo}` (matching `EnablePRAutoMerge`)
- Pros: trivial; consistent with the existing auto-merge call.
- Cons: bypasses ETag and in-process rate-limit accounting; subprocess cost per lookup; the repo has
  a `prefer-go-git-over-subshells` posture and a `norawexec`/`norawgitcli` lint family
  (`tools/lint/`), so new shell-outs face friction.
- Verdict: **Viable fallback only.**

Semantic caveat (from `requirements.md:87`): repo `allow_auto_merge` is necessary but not
sufficient (branch protection/required checks). Neither C1 nor C2 resolves this; whichever is
chosen, define "known" as "repo flag read successfully" and keep the existing `pr_ready_unmerged`
nudge as the backstop.

---

## E. Enforcing "every config surface has a runtime-effect test"

Facts (VERIFIED): `tools/lint/` is a separate Go module (`tools/lint/go.mod`, `golang.org/x/tools
v0.42.0`) with 14 `go/analysis` analyzers, run together by `make lint-custom` (`Makefile:844`).
Existing analyzers are lexical and per-call-shape (e.g. `silenttransition/analyzer.go:51`,
`norawghrequest`). The repo also enforces structure by test and generator (`make registry-generate`,
`vet-rpc-markers` is advisory, `Makefile:809`).

The thing to enforce is semantic: "a proto field / UI control / config key has a test whose
assertion depends on its runtime effect." That is not a lexical property.

### E1. Go test that enumerates surfaces and fails if any lacks a registered runtime-effect test (registry-driven)
- Pros: a table (surface name -> test function name) in one file; a `go test` iterates the proto
  descriptors / config struct tags (reflection over `StageConfig`/quota structs) and fails when a
  field has no entry or the named test does not exist (checked via `go/parser` over `_test.go` or
  by `go test -list`); runs in `make test`/CI with no new module; failure message can name the exact
  missing field; easy to review, easy to adjust. Matches how this repo already tracks surfaces
  (`docs/reference/feature-testing-registry.md`, omnibar registries).
- Cons: only as good as the registry; an entry can point at a test that does not truly exercise
  runtime effect (needs a convention such as the test must call a shared helper
  `AssertConfigChangesBehavior(...)` that flips the value and compares outputs, which the checker
  can grep for); registry can rot if reflection does not cover newly added surfaces.
- Verdict: **Recommended.**

### E2. `go/analysis` analyzer in `tools/lint/*`
- Pros: runs in the existing `lint-custom` single pass; has a documented escape hatch pattern
  (`//nolint:<name> reason`) and a precedent for "ratchet" rules
  (`noarchivedrevival`, `.claude/rules/noarchivedrevival.md`); catches a new field at the struct
  definition line.
- Cons: the property is cross-package and semantic (proto field -> handler -> test). The repo's own
  rule files state analyzers here are lexical and name-matched, "does not follow calls across
  packages" (`.claude/rules/noarchivedrevival.md`, Limits). The proto structs are generated and
  gitignored, so the analyzer would need to load generated code or parse `.proto`;
  heavier to write and test; high false-positive/false-negative risk, and LLM-written analyzers
  of this kind are the hardest to trust. Reserve it for a narrow lexical sub-rule (for example
  "any new field added to the stage-config or quota struct must carry a `runtime_test:"..."`
  struct tag"), which is lexical and cheap.
- Verdict: **Viable** as a narrow companion to E1 (tag-presence check), **Not recommended** as the
  sole mechanism.

### E3. CI script / grep over proto diff
- Pros: simplest.
- Cons: diff-based checks cannot know whether a test exists or is meaningful; gameable.
- Verdict: **Not recommended.**

---

## Summary table

| Workstream | Option | Verdict |
|---|---|---|
| A | Wire in existing `ConfiguredWorkflowEngine` + shadow mode | Recommended |
| A | `qmuntal/stateless` / `looplab/fsm` | Not recommended |
| A | Temporal | Not recommended |
| A | Small graph library for save-time validation | Viable (lean bespoke + tests) |
| B | Extend existing config + flag RPC + settings page | Recommended |
| B | viper/koanf | Not recommended |
| B | Derive budget from usage | Viable later |
| C | Native `github/` client via `NewConditionalRequest` + ETag | Recommended |
| C | `google/go-github` | Not recommended |
| C | `gh api` subprocess | Viable fallback |
| E | Registry-driven Go test with reflection | Recommended |
| E | `go/analysis` analyzer (narrow tag-presence rule only) | Viable companion |
| E | Diff/grep CI script | Not recommended |

---

## Soak evidence for flag defaults (workstream D, open question 3)

### Headline finding: the premise "all three flags are off" is false on the live instance

`requirements.md:14` and `:109` describe `backlog:sdd-default-pipeline`, `diagnose_nudge_enabled`
and `autonomous_stall_nudge_enabled` as default-off. Code defaults are indeed false
(`config/config.go:555-556`; flag description in the registry says "Default: off"), but on the
live instance (workspace `d685c4b1a423cca3`) all three are currently enabled:

- Read-only RPC: `curl -s -X POST localhost:8543/api/session.v1.SessionService/GetFeatureFlags -H
  'Content-Type: application/json' -H 'Connect-Protocol-Version: 1' -d '{}'` (the `/api` prefix is
  required; without it the server returns the SPA HTML). Parsed result: 32 flags returned;
  `backlog:sdd-default-pipeline` enabled=true, `diagnose_nudge_enabled` enabled=true,
  `autonomous_stall_nudge_enabled` enabled=true. VERIFIED (2026-10-17).
- Cross-check, workspace config file: `~/.stapler-squad/workspaces/d685c4b1a423cca3/config.json`
  `feature_flags` = `{'autonomous_stall_nudge_enabled': True, 'backlog:sdd-default-pipeline': True,
  'diagnose_nudge_enabled': True}`. VERIFIED.
- Corroborated by `docs/explanation/autonomous-stall-causes.md:164`: "`diagnose_nudge_enabled` is
  already `true` in the owner's workspace config".

So workstream D is not "decide whether to flip" but "record that the operator already runs these on,
decide whether the CODE default should change, and document evidence/rollback". Rollback for all
three is already a live flag flip (UpdateFeatureFlag), not attempted here (read-only).
I could not determine WHEN each flag was enabled or who enabled it (no audit trail found; UNVERIFIED).
That date is the start of the soak window and should be established (check `git log` of any
checked-in config, the flag-service audit, or ask the owner).

### Where soak evidence lives (and how thin it is)

| Source | Finding | Command / citation |
|---|---|---|
| Live structured log `~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad.log` (the active log for this workspace; the `~/.stapler-squad/logs/staplersquad.log` named in CLAUDE.md is stale, last written 2026-09-05) | Window covers only 2026-10-17T19:08:38 to 19:31:27 (about 23 minutes; 15.1 MB file, rotated/restarted). Far too short to be soak evidence. | `ls -la` of both paths; `grep -o '"time":"[^"]*'` head/tail |
| Same log, in-process stall-nudge delivery | 0 occurrences of `stall nudge`, 0 of `stall nudge skipped`, 0 of `autonomous_stall_nudge`. The only stall-nudge log string in code is the Warn at `server/services/autonomous_orchestration_service.go:727`; successful delivery has no dedicated Info line I could find, so absence is weak evidence (UNVERIFIED whether a success is logged elsewhere, see gap below). | `grep -c 'stall nudge' <log>` -> 0 |
| Same log, `diagnose_nudge_session` | `grep -c -i 'diagnose_nudge' <log>` -> 2, both are "creating session" / "tmux session created" lines for a `diagnose:6291672f:...` session (a diagnose agent was dispatched at 19:20:32); no `[diagnose_nudge_session]` handler lines (the handler's Warn at `server/mcp/tools_diagnose.go:361` fires only on a record failure). So: 1 dispatch observed, 0 nudge deliveries observable. | `grep -i 'diagnose_nudge' <log> \| cut -c1-400` |
| Same log, generic `nudge` | `grep -c -i nudge <log>` -> 327, but mostly noise: 165 "migrating category to tags" and 142 "DoesSessionExistNoCache" lines that merely contain a session name with "nudge" in it. Real signals: 14 x `AutonomousDriver: suppressed nudge` (`session/autonomous_driver.go:479`, the older driver path) and 1 x `SessionDriver: sent backlog nudge` (idle 300s, at 19:24:18; this is the older `attemptBacklogNudge`, O6 in the stall doc, not the new in-process nudge). | msg histogram via python over `grep -i nudge <log>` |
| Same log, `sdd-default-pipeline` | 0 matches; the flag is read, not logged. | `grep -c 'sdd-default-pipeline\|sdd_default_pipeline' <log>` -> 0 |
| Notification history RPC | `GetNotificationHistory {"limit":500}` returned 500 of total 500 (the response `totalCount` equals the page, so UNVERIFIED whether more exist beyond the cap), oldest `createdAt` 2026-09-05T23:12:04Z. 18 records contain "nudge", but every one is about a backlog item titled "Inspect and nudge before restarting from the stale-work detector" (sessions `...-inspect-nudge-before-restart-aea335bb`, r1-r4), not a nudge delivery event. There is no notification type for a nudge. Result: no queryable nudge-outcome record. | `curl ... GetNotificationHistory -d '{"limit":500}'`, python filter on `'nudge' in json.dumps(record).lower()` |
| Item data | `DiagnoseNudgeCount` exists on `BacklogStuckState` (`config/config.go:323-328` cap) but `grep -n "nudge_count" proto/session/v1/backlog.proto` found no field, so the count is not exposed over RPC. | grep -> no match |
| Docs | `docs/explanation/autonomous-stall-causes.md:151-166` "Known gaps" and the nudge-vs-respawn table (`:185-193`) describe design and caps, not outcomes. No document records an observed soak result. `docs/how-to/` has no soak guide for these flags (the only soak doc is `flip-git-backend-cohort.md`, a different feature). | grep in docs |
| Plans | `project_plans/autonomous-stall-diagnosis/` has requirements, ADR-001/002, research, plan, validation, pre-mortem (no soak results file). `project_plans/backlog-sdd-default-pipeline/` has requirements and plan; its flag was designed off-by-default (`requirements.md:87`). Neither contains runtime outcome data (not opened in full; UNVERIFIED beyond file listing and flag grep). | `ls project_plans/autonomous-stall-diagnosis/*` |

### What the evidence currently shows

- sdd-default-pipeline: enabled; no queryable evidence of outcomes in logs or notifications (the
  flag only changes pipeline mode for new items with no explicit mode). Evidence would have to come
  from items created since enabling: compare completion/stuck rates of `sdd` vs flat-pipeline items
  via `ListBacklogItems` (not run here).
- diagnose_nudge_enabled: enabled; one diagnose dispatch observed in a 23-minute window;
  zero observable deliveries. Not evidence of safety or of usefulness.
- autonomous_stall_nudge_enabled: enabled; zero observable in-process nudges in the same window.
  Not evidence either way.
- Net: there is **no soak evidence in a queryable form**. This confirms the risk stated at
  `requirements.md:101`. Any "flip default on" decision today would rest on design review
  (`autonomous-stall-causes.md`, ADR-001) rather than measured outcomes.

### Gaps to close before recording a default decision

1. Establish the enablement date for each flag on the live instance (start of the soak window).
2. Add (or locate) a durable record of nudge outcomes: a structured log line or counter for
   delivered / uncertain / slot-unavailable / fallback-to-respawn in the stall path. Without it the
   exit criteria cannot be measured. This is a small observability addition, within D's remit.
3. Use rotated logs: only the current 23-minute log exists in the active workspace; check whether
   older workspace logs were rotated away (`ls ~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/`,
   not run) before concluding data is gone.
4. Owner decision (open question 5) on exit criteria, for example: N nudges delivered over M days
   with zero `ErrNudgeSubmitUncertain`, zero nudges followed by a respawn within X minutes, and no
   increase in `autonomous_stuck` rows.

Recommended framing for the D decision record: "currently ON in the operator's workspace; code
default OFF; keep code default OFF until criteria 4 are met against data from gap 2; rollback is a
live flag flip."
