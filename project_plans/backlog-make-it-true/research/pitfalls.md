# Research: Pitfalls — backlog-make-it-true

Date: 2026-10-17. Scope: what goes wrong when (a) swapping the static transition map for the
config-driven engine under live items, (b) adding GitHub repo-setting lookups, (c) flipping
autonomy flags by default. Line numbers were read from this worktree (HEAD `b416f5576`).
Labels: VERIFIED = I opened the file / ran grep this session. INFERRED = reasoned from verified
facts. UNVERIFIED = not checked; do not rely on it without a check.

Sources mined: `docs/tasks/backlog-feature-improvement.md` (recurring-shape sections, ~1364-1432,
2438-2462), `project_plans/backlog-custom-workflow-stages/{research/pitfalls.md,implementation/pre-mortem.md}`,
`project_plans/quota-aware-backlog-gating/research/pitfalls.md`,
`project_plans/backlog-stuck-item-visibility/research/pitfalls.md` + `implementation/plan.md`,
`docs/explanation/autonomous-stall-causes.md` (Known gaps, :151-166), `.claude/rules/*.md`, and the code.

## Top 5 (read these first)

1. **Swapping the engine drops every built-in business rule.** VERIFIED. `DefaultWorkflowEngine`
   delegates gates to `TransitionGuard` (`session/workflow_engine.go:88,140`;
   `session/domain/backlog.go:652`: AC required, plan approved, plan artifacts present, unresolved
   blockers, verdict-clear, done guards). `ConfiguredWorkflowEngine` never calls `TransitionGuard`
   (grep: only `workflow_engine.go` references it outside tests) and the stage seed writes zero gates
   (`grep -c -i gate session/workflow_stage_seed.go` = 0). `PendingGates` returns `nil, nil` for an
   edge with no gates (`configured_workflow_engine.go:174-176`). So the "byte-for-byte equivalent to
   `validTransitions`" test (`configured_workflow_engine_test.go:30`) proves edge parity only. After
   the swap, `TransitionBacklogItemStatus` (`backlog_service_lifecycle.go:892,933`) would let an item
   with no AC go `idea -> ready`, and `ready -> in_progress` with no approved plan. This is the
   "silent no-op that should have blocked" shape (workflow-stages pitfalls §4.1) at its worst.
2. **A failed cache load denies every transition; it does not fall back to the default graph.**
   VERIFIED. `NewConfiguredWorkflowEngine` logs a Warn and keeps an empty cache on `Load` failure
   (`configured_workflow_engine.go:80-83`); `CanTransition` then returns false (`:102-118`, empty
   cache -> `HasStage` false -> nil fallback -> false). Requirement A's "safe fallback to the default
   graph" does not exist today and must be built, with a loud log and a metric.
3. **At least three engines already coexist; "one engine" is not true yet.** VERIFIED.
   `BacklogService` gets the default engine (`server/dependencies.go:678,1477`); the lifecycle
   listener gets `stageConfigEngine` (`:1544`, for automated-review gate resolution only,
   `session/backlog_lifecycle.go:1078-1092`); `SyncLoop` builds its own
   `NewDefaultWorkflowEngine()` (`session/backlog_sync.go:57,69`, used at `:449`) and no setter wires
   it in production (grep: no `SetWorkflowEngine` for SyncLoop). Swapping only `dependencies.go:678`
   leaves GitHub-driven sync writes on the static graph.
4. **Default flips are not neutral for the owner, and "default" lives in more than one place.**
   VERIFIED for the facts below, INFERRED for the consequence. `diagnose_nudge_enabled` is already
   `true` in the owner's workspace config (`autonomous-stall-causes.md:164-166`), so flipping the code
   default changes behavior only for installs that never set it, and the thing that is actually unsoaked
   is `autonomous_stall_nudge_enabled`. The default is declared in `config/config.go:556,571`
   (`GetFeatureFlagWithDefault(..., false)`) and again in the registry
   (`server/services/feature_flag_service.go:287-292`); a flip must change both or the UI and the
   runtime disagree (the `HiddenSessionGateDefault` comment, `config/config.go:515-518`, exists
   because of exactly this).
5. **`NewConditionalRequest` is github.com-only.** VERIFIED. It hardcodes `RestBaseURLForHost("")`
   (`github/http_client.go:343`) and uses the default-host token (`getGHToken`); the host-aware
   builder `newGHRequestForHostWithToken` is unexported (`:276`). A GHE repo lookup must live inside
   package `github`, or the host-aware builder must be exported. Also `ETagCache` is keyed
   `host/owner/repo/prNumber` and stores `*PRInfo` (`github/etag_cache.go:33-58`); it cannot hold a
   repo-settings payload without a new cache.

## A. Swapping static map for config-driven engine under live items

### A1. Gate loss (see Top 5 #1)
Mitigation: before any swap, either (i) seed `TransitionGate` rows that reproduce each
`builtInGuardGate` case (`workflow_engine.go:93+`), or (ii) have `ConfiguredWorkflowEngine` wrap
`TransitionGuard` for built-in edges and add configured gates on top. Add a **gate-equivalence** test
over `(from,to,item-fixture)` including each `Err*` sentinel, not only edge parity. Note the handlers
branch on error type (`isFailedPreconditionGuardError`, `backlog_service_lifecycle.go:934-937`), so
error identity (FailedPrecondition vs InvalidArgument) must also match or clients/MCP callers change
behavior. INFERRED: gate rows are `GateKind`-typed structural data and may not be able to express
`TransitionGuard`'s inputs (`HasUnresolvedBlockers`, `PlanArtifactsPathMissing`, `OverallOutcome`);
UNVERIFIED whether `evaluateGate` (`configured_workflow_engine.go:242`) covers them.

### A2. Seed is create-if-missing, so parity decays
`workflow_stage_seed.go` writes built-in rows once and never touches them again by design (header
comment, `:3-9`). Consequence (INFERRED): if a later code change adds/removes an edge in
`domain.validTransitions`, a database seeded earlier keeps the old graph, so the shadow-divergence
alert ("any divergence on a default-seeded config is a bug") will fire on a legitimate operator edit
and stay silent on code drift. The shadow comparison must be against the live DB graph vs. the code
map, and distinguish "operator edited" from "code moved". Needs a per-edge `source=builtin|custom` or
`edited_at` marker; UNVERIFIED whether one exists (`session/ent/schema/`).

### A3. Items stranded on a removed/disabled stage or edge
- The fallback is `BuildStageConfigSnapshotFallback` (`session/ent_repository_backlog.go:145-170`),
  which reads the latest `BacklogStatusEvent` for the item's current status. It returns **nil** when
  no event matches, so items created before snapshots existed, or imported straight into a status,
  get no fallback and `CanTransition` is false (`configured_workflow_engine.go:109-111`). INFERRED:
  legacy rows are the likely victims; UNVERIFIED how many live items lack `AllowedTransitionsSnapshot`.
- Disabling the last outgoing edge from a stage that has live items: pre-mortem #2 (P1) says the
  validator must run on `UpdateStageTransition(enabled=false)` against the live item distribution
  (`project_plans/backlog-custom-workflow-stages/implementation/pre-mortem.md`, Failure #2, marked
  resolved in plan). UNVERIFIED that it is implemented; check `session/graph_validator.go`.
- Config cache vs. gate evaluation mid-flight: `GateSatisfactionRecord` has no config-version stamp
  (pre-mortem #3, P2, left open as of the doc). Editing a gate while an approval is recorded
  reinterprets the approval.
- Three hand-copied copy-on-write caches invalidate independently (pre-mortem #4, P3): a multi-RPC
  edit can yield a torn read. `BacklogService` only invalidates through
  `SetStageConfigEngine` (`backlog_service.go:417-461`); if `stageConfigEngine` is nil
  (`dependencies.go:773-778` logs Warn and continues), CRUD writes succeed but no cache refresh
  happens: **UI says saved, runtime unchanged**, the exact failure this change exists to remove.
  Add a startup log and a CRUD-response signal when the invalidator is nil.

### A4. Coverage: paths that do not go through the engine
- Historical bypass: `session.CanTransitionBacklog` direct calls at `backlog_service_lifecycle.go:801`
  and `backlog_service_sync.go:121` (`docs/tasks/backlog-feature-improvement.md:1413-1418`). Current
  grep shows no non-test callers outside `session/backlog.go:238` (alias) and a comment at
  `backlog_service_lifecycle.go:879`; so these appear fixed. VERIFIED by grep. Do not trust it
  permanently: `CanTransitionBacklog` is still an exported alias, so a new caller can reintroduce
  the bypass. Recommend a lint (same shape as `norawghrequest`) that flags non-test uses outside
  the engine package.
- Remaining non-service engines: `SyncLoop` (A.3 above). Also `server/mcp/tools_backlog.go:180` uses
  `engine.ListEnabledStageSlugs()` while `validBacklogStatuses` whitelists exist
  (pre-mortem #1, P1). UNVERIFIED that every `BacklogStatus` literal switch was audited; run an
  ast-grep sweep (`BacklogStatus` literal comparisons) rather than trusting the two named sites.
- Lifecycle auto-transitions go through `TransitionBacklogItemStatus` (service) or direct storage
  writes; any direct-ent status write skips both engine and audit event (workflow-stages pitfalls §3,
  "§1f"). UNVERIFIED current count; grep `UpdateBacklogItemStatus`-style storage calls.

### A5. Recurring bug shapes this change can re-introduce
From `docs/tasks/backlog-feature-improvement.md:1364-1432` (10+ instances): "status-transition write
fails after side effects, error only logged, RPC reports success, no sweep." A stricter configured
engine **increases the denial rate**, and callers that log-and-continue on a denied transition
(`backlog_service_triage.go:801-810` fresh-spawn path; `:2313-2321` TriggerReReview;
`session/backlog_lifecycle.go:971-984,1971-1975`) will now strand items with live sessions whenever
the operator's graph denies an edge the code assumes is legal. This is not hypothetical once edges are
editable. Mitigations:
- A custom graph must **never be allowed to disable system-driven edges** the lifecycle relies on
  (e.g. `ready->in_progress`, `in_progress->review`, `review->done`, `pr_pending->done`); mark these
  `system_required` and reject disabling them in the CRUD validator. INFERRED design requirement.
- Audit every swallowed-`TransitionBacklogItemStatus` caller; the doc's own recommendation is a shared
  `transitionOrNotify` helper or lint rather than another instance patch (`:1425-1432`).
- Fail-closed direction differs per config type (workflow-stages pitfalls §4): a missing **liveness**
  value falls back to a safe default; an unresolvable **gate** blocks. One generic "fallback to
  default" rule gets one of them wrong. Requirement A's wording ("safe fallback to the default graph")
  is only correct for edges, not for gates, and not for the "gates were never seeded" case in A1.
- The `bouncing`/orphaned-triage "no escalation/different-approach retry" shape
  (`:271`, `:2369-2371`, `:2548`): extra denials push items into remediation retries that repeat an
  identical failing approach (5 attempts, parked). Do not add a new denial path without a distinct
  stuck reason (it will otherwise surface as `orphaned_triage`/`bouncing`).
- BUG-083 shape ("write side fixed, recovery side missing", 6 prior instances): a row parked while the
  graph denied its edge must become due again after the operator fixes the graph. Add an explicit test
  that a parked item recovers via `RemediationDue` after a config correction (workflow-stages
  pitfalls §2).
- "Fix not deployed yet" confusion (`:2446-2460`): the live service is rebuilt manually. Shadow-soak
  counters must record the running binary's build/start time so a divergence is attributed to the
  right code, and "no divergence" is not read as "fixed" if the binary is old.

### A6. Shadow mode and the live-flag rollout
- Divergence counter must compare **decisions**, not only booleans: `CanTransition`, `AllowedTransitions`
  (sorted slices, `configured_workflow_engine.go:128-141`), and `PendingGates` satisfaction + gate IDs.
  The git-backend cohort precedent (`docs/how-to/flip-git-backend-cohort.md`) is byte-diff oriented
  (memory: oracle harnesses need byte-diff); UNVERIFIED that its counter shape applies.
- Shadow evaluation runs `ConfiguredWorkflowEngine.PendingGates`, which for `automated_review` /
  `custom_check` gates may touch persisted `GateSatisfactionRecord`s and, by name, "kicks off" work
  (`ResolveAutomatedReviewGateContext`, `:418`). INFERRED: shadow must be strictly read-only; verify
  `PendingGates` has no side effects (it logs at Info on every call, `:188`, which at hot-path rates
  will flood logs; downgrade to Debug or sample in shadow mode).
- The enforcement flag must be read at decision time (`config.LoadConfig()` per call is a disk read +
  unmarshal; `github/http_client.go:114-123` caches a flag with a TTL for exactly this reason on a hot
  path). `TransitionBacklogItemStatus` is a top hotspot; use a TTL-cached read or `FlagCache`, not a raw
  `LoadConfig()`.
- Flag flip is not atomic across the three engine holders in A4; a mid-transition flip could evaluate
  `CanTransition` on one engine and `ValidateGates` on the other within a single request
  (`backlog_service_lifecycle.go:892` then `:933`). Resolve the engine once per request and pass it
  down.
- Do NOT use `make install-service` to roll or test (CLAUDE.md WARNING; kills live tmux sessions).
  Use the manual instance recipe (ports 62871/62872, `STAPLER_SQUAD_TMUX_SOCKET`), and remember a
  manual instance has its own DB under `instances/<name>/`, so it has no real live items: **shadow
  data against production rows (pre-mortem #5) cannot be produced on a manual instance**; snapshot a
  read-only copy of the live DB or run the comparison as an RPC/actuator on the live service behind
  the flag.

## B. GitHub repo-setting lookups (`allow_auto_merge`)

### B1. Request construction and the lint
- `.claude/rules/norawghrequest.md`: raw `http.NewRequest*` against `GhBaseURL()`/`RestBaseURLForHost()`
  is flagged. Approved: `NewConditionalRequest`, `NewConditionalRequestNoCache`,
  `newGHRequestForHostWithToken` (+ `...AndBody`, GraphQL variant). VERIFIED
  (`github/http_client.go:249,276,292,342,367`).
- GHE: `NewConditionalRequest*` use `RestBaseURLForHost("")` and the default token (VERIFIED
  `:343,368`). For a repo whose remote is a GHE host, use `newGHRequestForHost`/`...WithToken`
  with `getGHTokenForAccount(AccountRef{Host:...})`; this is unexported, so put the repo-settings
  fetch in package `github`. If it is written outside the package with `NewConditionalRequest`, it will
  silently query github.com for a GHE repo: 404 or the wrong repo's settings (a same-named
  `owner/repo` on github.com). INFERRED consequence; make host an explicit parameter and test against
  an `httptest` GHE override (`SetEnterpriseBaseURLOverride`, `github/hosts.go:44-58`).
- Host derivation: how an item's `RepoPath` becomes `(host, owner, repo)` is not traced here.
  UNVERIFIED; reuse the existing `RepoRef` (`etag_cache.go:52-58` shows `ref.Host()/Owner()/Repo()`)
  rather than parsing remotes again. A fork/upstream mismatch (item's PR targets `upstream`, memory:
  dual remotes) can read the wrong repo's setting. INFERRED.

### B2. Rate limits and call origin
- Every call passes `rateLimitTransport`, which fails fast when `DefaultRateLimiter.IsLimited()`
  (`http_client.go:86-88`), and, when `github:priority-admission-control` is on, rejects by
  `GitHubCallOriginFrom(ctx)` (`:95-102`). Origins are an enum (`github/call_origin.go:16-25`:
  interactive, pr_status_poller, worktree_pr_poller, backlog_sync, webhook_reconcile). A new
  stuck-panel lookup with no origin tag is classified however the default is; UNVERIFIED which. Tag it
  deliberately (a render-triggered lookup is `interactive`; a prefetch is background and may be
  dropped under pressure, which must surface as "unknown: rate limited", not as `false`).
- **Do not do a per-item or per-render fetch.** Stuck-visibility plan already decided: one per-repo,
  TTL-cached fetch gated on `DefaultRateLimiter` (`project_plans/backlog-stuck-item-visibility/implementation/plan.md:96`,
  Story 4.1.4); its pitfalls doc §4 warns against any new poller that duplicates `GetPRStatus` volume.
  Key the cache by `(host, owner, repo)`, share one in-flight fetch (singleflight), and cap negative
  caching so a transient 403/429 is not cached as "false" for the full TTL.
- Conditional requests: GitHub charges nothing for a 304, but only if the cached ETag and body are kept.
  `ETagCache` TTL is 24h and sweeps every 64 writes (`etag_cache.go:20-28`); a settings value that
  changes rarely suits ETag well, but the value cached must be the **parsed setting plus the ETag**, and
  a 304 must return the cached value, not zero value (the classic "304 treated as false" bug).
- Failure must be three-valued: `true`, `false`, `unknown(reason)`. The proto field is
  `optional bool allow_auto_merge = 11` (`backlog.proto:2144`) and the UI renders "unknown" for unset
  (`StuckItemDetail.tsx:54-59`); there is no field to carry *why* it is unknown. Requirement 3 says
  "says so" when the lookup fails. INFERRED: add an `auto_merge_lookup_status` enum
  (ok / rate_limited / no_permission / not_found / error), otherwise a failure and "not looked up yet"
  render identically, which is today's bug.
- Same silent-failure family: `stuckBacklogItemToProto` leaves the field unset by design
  (`backlog_service_stuck.go:142-145`); a lookup error that is only logged keeps the panel at "unknown"
  forever with no operator signal (the doc's recurring "silently" shape).

### B3. Semantics: what "known" means
- Repo `allow_auto_merge` is necessary, not sufficient. `EnablePRAutoMerge` runs
  `gh pr merge N --auto --squash` (`session/git/worktree_git.go:1139-1152`): it also needs squash merges
  enabled, and (for auto-merge to wait on checks) branch protection with required checks. A repo with
  `allow_auto_merge=true` and no required checks merges immediately or errors depending on state;
  `allow_auto_merge=true` with squash disabled fails. INFERRED from `gh` behavior; UNVERIFIED against
  GitHub docs. Fetch `allow_squash_merge` too, and say "enabled, may still fail" in the UI.
- Field visibility depends on token permission (GitHub returns some merge-setting fields only to
  collaborators with sufficient access). UNVERIFIED; if true, a read-scope token yields an absent field
  that must map to `unknown(no_permission)`, never to `false`. Test with a token lacking push access.
- Multi-account tokens: `getGHTokenForAccount` resolves per host/user (`http_client.go:260-274`); pick
  the account that owns the PR, otherwise a different account's token can read a different permission set.
- Warn-before-PR-create (scope item C): `EnablePRAutoMerge` is already best-effort and its failure is
  swallowed into a log/notification path (`backlog_lifecycle_pr.go:718`); `backlog_lifecycle.go:1925`
  documents manually merged PRs when `allow_auto_merge` is off. A pre-flight warning must not block PR
  creation (requirements: C is additive) and must not add a synchronous GitHub call to the PR-create
  hot path; read the cached value.
- Stale TTL vs. operator action: operator enables the repo setting, the panel still says false until TTL.
  Offer a refresh (interactive origin) rather than a long TTL.
- Counter requirement (requirements.md Observability): success/failure/cached/304 as separate labels so
  the "unknown rate" is measurable; without the `reason` dimension the counter cannot distinguish rate
  limiting from permissions.

## C. Flipping autonomy flags by default

### C1. Flag mechanics
- Flags are live-settable via the registry (`feature_flag_service.go`); `GetFeatureFlagWithDefault` reads
  fresh. Preference: no env vars, global + per-scope override (memory: rollout-flags-live-settable).
  The three flags are not equivalent: `backlog:sdd-default-pipeline` is read via
  `config.LoadConfig().GetFeatureFlag(...)`, a **plain `GetFeatureFlag` (default false)**, at item
  creation only (`backlog_service_lifecycle.go:136-150`); the other two use `...WithDefault`
  (`config.go:556,571`). Flipping the default for the first requires switching to `...WithDefault` and
  registering `defaultValue` in `knownFeatureFlags`; flipping the const alone changes nothing.
  (`GetFeatureFlagWithDefault`'s explicit-false must still opt out, `config.go:502-503`.)
- A new default silently turns the feature on for every installation that never wrote the key, and
  there is no "was this explicit or inherited" bit in the UI. Surface the effective value and its
  source (default vs. explicit) in the panel, else "I never turned that on" incidents happen.
- Flag interaction: `autonomous_stall_nudge_enabled` AND `diagnose_nudge_enabled` (`config.go:547-571`
  comments); flipping one alone is a no-op, and `diagnose_nudge_enabled` is the kill switch for **every**
  autonomous pane write (`config.go:565-567`). Do not make the stall flag default-on without keeping the
  other flag as an independent off-switch. The UI should state the AND (requirements Rabbit Holes).

### C2. Known gaps that default-on would expose (`autonomous-stall-causes.md:151-166`, VERIFIED)
- Two nudgers can be active on one session (in-process nudge and `attemptBacklogNudge`,
  `session_driver.go:933`); the write lease serialises writes, not intent; each has its own cap.
- The nudge cap is a lifetime counter on the `(item, autonomous_stuck)` row; neither reopening the stuck
  row nor Reset clears it: after N nudges over an item's life, nudging silently stops. A default flip
  changes the point at which that happens.
- Stale-work detector and health-checker restarts still kill and respawn with no pane snapshot or nudge
  (O1-O3, O5): the flag helps driver-stop paths only. Do not claim "stalled sessions are nudged, not
  killed" in the decision record; the baseline in requirements.md is broader than the code scope.
- Pause and user stop never stop the driver; `buildTurnCallback` raw-writes `AutonomousTurn`.
- Typing into an unattended pane is a write to an LLM session: the memory note says gate such writes
  on `StatusIdle` plus a `StatusContext` allowlist, because `detection.StatusReady` is dead code. UNVERIFIED
  that the in-process nudge uses that gate; check before recommending default-on.
- Memory: `--allowedTools` is client-side only; an MCP tool an autonomous dispatch must not reach needs
  a caller-identity check at the handler. `diagnose_nudge_session` is exactly this surface; a default
  flip increases exposure, so verify the server-side check exists. UNVERIFIED.

### C3. `backlog:sdd-default-pipeline`
- The 2026-09-03 finding: the "sdd" pipeline triage prompt did not fit the 30-minute triage budget and
  stranded 12 items (`docs/tasks/backlog-feature-improvement.md:2340`); budget later raised to 3h
  (`:2446-2460`), but whether that fix is deployed and soaked is UNVERIFIED. Making sdd the default for
  every new item multiplies headless-pool load (5-slot pool, 2026-09-08 entry `:2188`) and token spend.
- Interaction with the WIP cap of 2 (memory: backlog WIP limit, OOM incident 2026-07-12/29): sdd mode is
  multi-phase and long-lived, so default-on raises concurrent session pressure; the cap counts only
  `in_progress`/`review` (`:1364-1380`, fresh-spawn blind spot), so default-on can exceed it invisibly.
- It never changes existing items (creation-time read), so rollback = flag flip, but items already
  created with `sdd` stay `sdd`; document that rollback is not retroactive.

### C4. Quota gate as a default (workstream B adjacent)
- `Quota.Enabled` default false, `AssumedWindowTokenBudget` 0 disables the proactive signal
  (`config/types.go:743-763`). Surfacing both in UI must show the 0 = "reactive only" coupling
  (requirements Rabbit Holes).
- Quota pitfalls (`quota-aware-backlog-gating/research/pitfalls.md`): needs hysteresis (config has it:
  `ConsecutiveTicksTo*`, `ResumeMarginPct`); needs `pausedByQuota` provenance so the auto-gate and the
  manual toggle do not stomp each other; never trust persisted "paused for quota" across restart;
  resume-notification has no copyable precedent in `capacity_monitor.go`; reactive-only signal cannot
  prevent the first hit. A settings UI that lets an operator edit these live adds a new writer; ensure
  one writer loop (that doc §summary 4) and that a mid-pause threshold edit re-derives, not applies
  retroactively without a tick.
- The "soft-budget threshold" in requirement 2: `EvaluateBudgetThreshold(itemID, stage, thresholdUSD *float64, cumulativeSpentUSD)`
  (`session/budget_warning.go:10`) takes a **per-item** `*float64` USD threshold, not a global setting.
  INFERRED: there may be no global value to put in a settings UI; scope may be a per-item field or a new
  default. UNVERIFIED (read `budget_warning.go` callers before planning B).

### C5. Decision-record pitfalls (success metric 4)
- Evidence for "soak" must be queryable. Open Question 3 is unresolved; `diagnose_nudge_session` outcomes
  and notification history may not retain enough. If evidence cannot be produced, the honest record is
  "stay off, exit criteria = X", which requirements already allows. Do not write "soaked" without a
  command and output (CLAUDE.md evidence rule; "recount every number").
- Check binary mtime/start time vs. merge time before calling a flag's behavior verified or failed
  (`docs/tasks/backlog-feature-improvement.md:2446-2460`).

## D. Config surface needs runtime proof (scope E)

- Rule: every stage-config/quota setting added to UI/proto needs a test that changes it and asserts a
  runtime difference. A naive repo check (grep proto fields against tests) is gameable and is the
  "self-testing guard" anti-pattern in memory (omnibar review). Make the check structural: a table of
  `setting -> test name` that the lint parses, plus the test must call the production entry point (e.g.
  `TransitionBacklogItemStatus`), not the engine in isolation; otherwise a test passes while the
  `dependencies.go` wiring still uses the wrong engine (today's exact bug: `server/dependencies.go:678`
  vs `:773`). Add a wiring test asserting the engine type held by `BacklogService` (there is a
  precedent: `server/dependencies_test.go` asserts pointer equality for the pipeline engine, per
  `dependencies.go` comment near `:683`). Avoid `BuildDependencies()` in tests (real network calls,
  memory: CI hermetic gotchas); use a narrow `ServerDependencies`.
- New `dupl`/gocyclo/gocognit gates apply to modified existing functions (memory: go-git plumbing
  gotchas). `TransitionBacklogItemStatus`/`backlog_service_lifecycle.go` are complexity hotspots, so
  extract rather than append.
- Proto/ent: `make proto-gen` and `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert`;
  never commit generated output; in a fresh worktree run `make build` first (memory: proto gen + CI
  gotchas). Run `make registry-generate` and commit feature JSON when RPCs/components change.
- Frontend: pnpm only; mobile + desktop layouts (memory); `Stages` settings already exist, so changes
  need Playwright specs that follow `e2e-test-conventions` (no `waitForTimeout`, testid locators).

## Checklist to carry into the plan

- [ ] Engine parity covers gates and error identity, not only edges (A1).
- [ ] Engine load/invalidate failure has a defined, logged, metered behavior that is not "deny all" (Top 5 #2).
- [ ] Every engine holder identified and either switched or documented (BacklogService, lifecycle listener, SyncLoop, MCP) (A4).
- [ ] System-required edges cannot be disabled by operator config (A5).
- [ ] Nil `stageConfigEngine` surfaces to the operator instead of a silent no-op (A3).
- [ ] Shadow mode is read-only, cheap, per-request single engine, and records binary build time (A6).
- [ ] Repo-setting lookup: host-aware, singleflight, three-valued with reason, ETag keeps parsed value, no per-render fetch, tagged origin (B).
- [ ] Each flag's default is declared once; effective value + source shown in UI; rollback and exit criteria written with real evidence (C).
- [ ] Wiring test + structural "config surface has proof" check (D).
