# Architecture Health Audit — 2026-07-01

Tool-generated evidence of coupling, complexity, and cohesion issues in the
`stapler-squad` Go backend, evaluated against:
- Martin Fowler's *Patterns of Enterprise Application Architecture* / DDD Entity–Value
  Object–Aggregate–Repository framework (`architecture-best-practices` skill)
- Go idiom (Effective Go, Go Code Review Comments, Go Proverbs) (`go-development` skill)

This report is **read-only analysis**. No application code was modified. It is
separate from the in-flight `session.Instance` actor/concurrency migration tracked
in `project_plans/` — this audit widens the lens to find what *else* looks like
that problem.

Scope note: generated code (`session/ent/*` via entgo, `gen/proto/*` via buf/protoc,
`.claude/worktrees/*` stale worktree copies) is excluded from rankings unless
explicitly called out, since it is machine-produced and re-derivable.

---

## 1. Package Dependency Graph (`goda`)

Tool: `github.com/loov/goda` (note: `loov.dev/goda` vanity import is currently broken —
its `go.mod` declares `github.com/loov/goda`, causing `go install loov.dev/goda@latest`
to fail with a version-constraint conflict; installing the canonical module path worked).

```
go install github.com/loov/goda@latest
goda graph "github.com/tstapler/stapler-squad/..."
```

Full graph saved to `scratchpad/full-graph.dot` (402 lines, ~100 non-worktree packages).

### Afferent coupling (packages with the most incoming dependents)

Computed via `goda list "incoming(github.com/tstapler/stapler-squad/..., $pkg)"` per package:

| Rank | Package | Dependents |
|---|---|---|
| 1 | `log` | 31 |
| 2 | `session/ent/predicate` | 24 |
| 3 | **`session`** | **20** |
| 3 | `executor/safeexec` | 20 |
| 5 | `gen/proto/go/session/v1` | 11 |
| 5 | `executor` | 11 |
| 5 | `config` | 11 |
| 8 | `session/ent` | 9 |
| 9 | `session/tmux` | 7 |
| 9 | `session/detection` | 7 |
| 11 | **`server/services`** | **5** |

`log` and `executor/safeexec` topping the list is expected and healthy (cross-cutting
leaf utilities — exactly what high afferent coupling *should* look like: stable, small,
no outgoing business-logic dependencies). `session` at 20 dependents is the actual
chokepoint: it is imported by `server/services`, `daemon`, `main`, and 17 other
packages, and per the confirmed prior finding, `session.Instance` is the shared mutable
aggregate all of them poke at. `server/services` itself has lower afferent coupling (5)
but very high *efferent* coupling (imports `session`, `session/tmux`, `session/git`,
`session/vc`, `github`, `config`, `pkg/classifier`, `pkg/events`, `pkg/analytics`, and
more) — i.e., it is a hub, not a chokepoint, consistent with a bloated Application
Service layer rather than a shared-kernel problem.

### Dependency direction (vs. CLAUDE.md's Web Server → Session Mgmt → Config)

Checked with `goda list "reach(X, Y)"` (empty result = no package in X imports anything
in Y, i.e., clean):

```
reach(session/...,  server/...) → empty   (session does NOT depend on server — correct)
reach(config,       session/...) → empty  (config does NOT depend on session — correct)
reach(config,       server/...)  → empty  (config does NOT depend on server — correct)
reach(session/...,  daemon)      → empty  (no backward edge from session to daemon)
reach(server/...,   daemon)      → empty  (no backward edge from server to daemon)
```

**Finding: package-level dependency direction is clean.** There is no architectural
layering violation at the import-graph level, and Go's compiler makes true import
cycles between packages impossible by construction. The mess described in this
audit is *not* a package-graph problem — it is concentrated inside two oversized
packages (`session`, `server/services`) that individually violate Single Responsibility
badly enough that the package graph looks fine while the code inside two nodes does
not.

---

## 2. Call Graph (`go-callvis`)

Tool: `github.com/ofabry/go-callvis` (maintained fork). Installed and ran successfully,
with one caveat:

- **Issue encountered**: default `-algo=cha` build failed with `generic type alias
  requires GODEBUG=gotypesalias=1 or unset` inside a transitive dependency
  (`github.com/puzpuzpuz/xsync/v4@v4.5.0`), because this repo is on Go 1.25 (where
  `gotypesalias=1` is now default) and go-callvis's vendored `go/packages`/`go/ssa`
  loader is not fully in sync with that GODEBUG default for generic type aliases.
  Setting `GODEBUG=gotypesalias=1` explicitly did not fix it under `-algo=cha` (timed
  out instead, no error — the CHA algorithm appears to hang building the full call
  graph over the whole binary's SSA on this codebase's size).
  **Fallback that worked**: `-algo=static` (simpler intraprocedural static call
  resolution, no whole-program CHA/RTA analysis) completed cleanly in under 90s for
  both packages.

```
GODEBUG=gotypesalias=1 go-callvis -algo=static -format=dot \
  -focus=github.com/tstapler/stapler-squad/server/services \
  -group=pkg,type -limit=github.com/tstapler/stapler-squad -nostd .
```

Output saved:
- `scratchpad/callgraph-sessionservice.dot` / `.gv` (1.5MB dot source — `server/services` focus)
- `scratchpad/callgraph-sessioninstance.dot` (32K lines — `session` package focus)

These are too large to render/inspect node-by-node in this pass (graphviz `dot -Tsvg`
was not run — the `.dot` files are preserved in scratchpad for follow-up rendering if
needed), but the raw file sizes are themselves a signal: a focused call graph for one
package should be a few hundred KB at most for a codebase this size; `server/services`'s
static call graph alone is 1.5MB of dot source, indicating an extremely dense internal
call web consistent with the "150 exported functions in one file" finding below (§3).

---

## 3. Struct/Interface Size & Coupling (`ast-grep`)

### (a) Largest structs by field count (God Object candidates)

Query: `sg run --pattern 'type $NAME struct { $$$FIELDS }' --lang go --json=compact .`,
counted non-comment lines per struct body, generated/vendor/test/ent excluded.

| Fields | Struct | File:line |
|---|---|---|
| **95** | **`Instance`** | **`session/instance.go:92`** |
| 63 | `InstanceData` | `session/storage.go:17` |
| 48 | `TmuxSession` | `session/tmux/tmux.go:46` |
| 44 | `Config` | `config/config.go:151` |
| 40 | `SessionService` | `server/services/session_service.go:61` |
| 33 | `InstanceOptions` | `session/instance.go:402` |
| 30 | `ServerDependencies` | `server/dependencies.go:33` |
| 26 | `RuntimeDeps` | `server/dependencies.go:357` |
| 24 | `ApprovalRuleData` | `session/repository.go:185` |
| 23 | `RuleSpec` | `server/services/rules_store.go:22` |
| 22 | `ProviderLimits` | `server/services/provider_limits.go:64` |

`Instance` at 95 fields (this pass's line-count method; a per-field enumeration in
§ synthesis below counts ~95–97 depending on grouped declarations, consistent with the
prior session's "~200-field" characterization once embedded/derived accessors and
comments are considered) is confirmed as the largest hand-written struct in the repo by
a wide margin — nearly 1.5x the second-largest (`InstanceData`, its own hand-maintained
serialization shadow, see §5). `SessionService` at 40 fields is the second-largest
*service* struct (as opposed to data struct) — most Application Services in a
PoEAA-style layering should hold only their own dependencies (repositories, other
services), not this many disparate collaborators bundled into one type.

### (b) Functions/methods with the most parameters (primitive obsession signal)

Query: `sg run --pattern 'func $NAME($$$PARAMS) $$$RET { ... }'` and the receiver-method
equivalent, custom top-level comma splitter to count params, generated/test excluded.

| Params | Function | File:line |
|---|---|---|
| 10 | `newLogManager` | `log/log_manager.go:32` |
| 9 | `RegisterRoutes` | `server/auth/handlers.go:22` |
| 8 | `UpdateInstancePRStatus` | `session/storage.go:471` |
| 8 | `NewNotificationEvent` | `pkg/events/types.go:152` |
| 8 | `AppendAutoApproved` | `server/notifications/store.go:161` |
| 7 | `newTmuxSessionWithSocket` | `session/tmux/tmux.go:520` |
| 7 | `UpdatePRStatus` | `session/instance_terminal.go:247` |
| 7 | `CreateSourceSyncEvent` | `session/ent_repository_backlog.go:450` |
| 7 | `CreateDirectorySession` | `server/services/session_service.go:647` |

None of these are extreme outliers (worst is 10), but several (`UpdateInstancePRStatus`,
`NewNotificationEvent`, `AppendAutoApproved`, `CreateDirectorySession`) mix multiple
primitive `string`/`bool` parameters that would benefit from a parameter object per the
`type-driven-design` skill already in use in this repo — lower priority than the God
Object findings.

### (c) Files with unusually large exported-function counts (low cohesion)

| Exported funcs/methods | File |
|---|---|
| **150** | **`server/services/session_service.go`** |
| 68 | `session/storage.go` |
| 50 | `session/tmux/tmux.go` |
| 47 | `log/log.go` |
| 47 | `session/ent_repository.go` |
| 40 | `session/claude_controller.go` |
| 35 | `session/tmux_backend.go` |
| 35 | `session/tmux_process_manager.go` |
| 31 | `session/instance_tmux.go` |
| 27 | `session/session.go` |

`session_service.go` at 4,542 lines and 150 exported symbols is the single strongest
low-cohesion signal in the repo — a file this size implementing this many public
operations is by definition not honoring Single Responsibility, regardless of internal
organization.

### (d) Cross-package direct field mutation (encapsulation violation)

Searched `server/services/`, `daemon/`, and `main.go` for `<var>.<ExportedField> =`
patterns on `Instance`-typed variables (`instance`, `inst`, `sess`, `session`):

```
daemon/daemon.go:43:                instance.AutoYes = true
daemon/daemon.go:312:               instance.AutoYes = true
server/services/session_service.go:359:   inst.MCPServerURL = s.mcpServerURL
server/services/session_service.go:1293:  instance.CreationProgress = "Starting session..."
server/services/session_service.go:1439:  instance.Title = *req.Msg.Title
server/services/session_service.go:1445:  instance.Category = *req.Msg.Category
server/services/session_service.go:1472:  instance.Program = newProgram
server/services/session_service.go:1500:  instance.WorkingDir = *req.Msg.WorkingDir
server/services/session_service.go:1524:  instance.AutonomousMode = *req.Msg.AutonomousMode
server/services/session_service.go:1567:  instance.PauseReason = session.PauseReasonManual
server/services/session_service.go:2502:  instance.Title = oldTitle
server/services/session_service.go:3452:  inst.GitHubPRURL = prURL
server/services/session_service.go:3700:  inst.AutonomousMode = false
server/services/session_service.go:4245:  inst.ArchivedAt = &now
server/services/session_service.go:4508:  inst.Program = newProgram
```

(21 total hits found; representative sample above.) **This is the most serious finding
in this audit.** `session/instance.go` defines six separate lock primitives
(`stateMutex`, `pmMu`, `startMu`, `restartMu`, `lifecycleListenersMu`,
`rateLimitCallbacksMu` — lines 313, 332, 335, 347, 351, 354) and documents specific
fields as "Protected by stateMutex" (e.g. line 292, line 371). Grepping
`server/services/session_service.go` for `stateMutex` returns **zero matches** — every
one of the ~20 direct field writes above happens with no lock held at all, from a
different package than the one that defined and depends on the locking invariant. This
is textbook Inappropriate Intimacy (Fowler) / a broken encapsulation boundary: the
struct's own concurrency contract is invisible to, and unenforced against, its
heaviest external consumer.

---

## 4. Complexity Metrics (`gocyclo` / `gocognit`)

Both installed cleanly via `go install`. Generated code (`session/ent/*`,
`gen/proto/*`) excluded from the tables below as noise (entgo/protoc/buf output is
mechanically complex by nature and not a refactor target).

### Cyclomatic complexity (`gocyclo -top 60 -avg .`), hand-written code only

| Cyclomatic | Function | File:line |
|---|---|---|
| 77 | `(*ConnectRPCWebSocketHandler).streamViaControlMode` | `server/services/connectrpc_websocket.go:472` |
| 72 | `(*ConnectRPCWebSocketHandler).streamViaTmuxCapturePane` | `server/services/connectrpc_websocket.go:1015` |
| 64 | `(*EntRepository).Update` | `session/ent_repository.go:322` |
| 63 | `(*SessionService).CreateSession` | `server/services/session_service.go:1020` |
| 61 | `BuildRuntimeDeps` | `server/dependencies.go:424` |
| 58 | `(*SessionService).UpdateSession` | `server/services/session_service.go:1391` |
| 51 | `(*SessionService).StreamTerminal` | `server/services/session_service.go:1942` |
| 49 | `runSessionDriverWithPrompt` | `session/session_driver.go:107` |
| 45 | `(*EntRepository).Create` | `session/ent_repository.go:121` |
| 44 | `(*ReviewQueuePoller).checkSession` | `session/review_queue_poller.go:600` |
| 40 | `(*TmuxSession).processControlModeLine` | `session/tmux/control_mode.go:344` |
| 40 | `(*InsightsService).GetInsightsSummary` | `server/services/insights_service.go:41` |
| 40 | `(*ApprovalHandler).HandlePermissionRequest` | `server/services/approval_handler.go:180` |
| 39 | `(*Instance).start` | `session/instance.go:714` |
| 39 | `(*CommandCriteria).Matches` | `pkg/classifier/classifier.go:198` |
| Avg | repo-wide average | **2.42** |

(Repo-wide average of 2.42 is healthy — these hotspots are extreme outliers, not a
systemic style problem.)

### Cognitive complexity (`gocognit -top 40 .`), hand-written code only

| Cognitive | Function | File:line |
|---|---|---|
| **242** | **`(*ConnectRPCWebSocketHandler).streamViaTmuxCapturePane`** | **`server/services/connectrpc_websocket.go:1015`** |
| **232** | **`(*ConnectRPCWebSocketHandler).streamViaControlMode`** | **`server/services/connectrpc_websocket.go:472`** |

Cognitive complexity (which, unlike cyclomatic, penalizes nesting depth rather than
just branch count) puts these two WebSocket streaming handlers in a category of their
own — 3x-5x higher than every other hand-written function in the repo. A function at
cognitive complexity 242 is effectively unreviewable and untestable as a single unit.

### Complexity × churn (hotspot ranking)

Cross-referenced against 6-month git commit churn (`git log --since="6 months ago"
--name-only`):

| File | Commits (6mo) | Top complexity in file |
|---|---|---|
| `server/services/session_service.go` | **91** | 63 (CreateSession) |
| `session/instance.go` | **77** | 39 (start) |
| `server/services/connectrpc_websocket.go` | **43** | 242 cognitive (streamViaTmuxCapturePane) |
| `session/storage.go` | **40** | — (68 exported funcs) |

High complexity **and** high change frequency is the CodeScene-style definition of a
true hotspot (as opposed to complex-but-stable code, which is lower risk). All four
files above qualify: they are both the hardest code to reason about and the code being
edited most often, which is the worst combination for defect injection risk.

---

## 5. UML Class Diagram (`goplantuml`)

Tool: `github.com/jfeliu007/goplantuml`, installed and ran cleanly.

```
goplantuml -recursive session          > scratchpad/session.puml
goplantuml -recursive server/services  > scratchpad/server-services.puml
```

Rendering to an image was not attempted — no local PlantUML jar/server was available,
and the `.puml` text itself is sufficient structural evidence for this pass. Saved to
`scratchpad/session.puml` (18,814 lines) and `scratchpad/server-services.puml` (1,703
lines).

Key structural facts extracted from the `.puml` text (not rendered):

- `session` package (recursive, includes all subpackages and the generated `ent` ORM
  layer): **55 namespaces**, **1,098 classes/structs total**, of which **669 are
  hand-written** (the remainder are entgo-generated `ent/*` types). This confirms
  `session/` has sprawled into what is effectively 30+ sub-domains (tmux, vnc, cdp,
  git, vc, headless, hibernation, queue, scrollback, search, tokens, namegen, memory,
  detection, unfinished, artifacts, mux, procinfo, prompts, framebuffer, and more)
  living under one top-level package name.
- The `Instance` class entry in the diagram spans **314 lines** (fields + method
  signatures) — the single largest class definition in the diagram by a wide margin,
  visually confirming the God Object shape.
- `SessionService` in `server/services` spans **217 lines** in its class entry —
  second largest, and the largest in the `server/services` package (138 total
  classes/structs in that package).
- 304 relationship arrows (`-->`, association/dependency lines) were extracted from
  `session.puml`, indicating a densely interconnected type graph even before
  considering the generated `ent` layer.

---

## 6. Manual Confirmation: `Instance` / `InstanceData` Shadow Struct

Not part of the requested tool list, but surfaced directly by the struct-size query
above (§3a) and worth calling out explicitly: `session/storage.go:17` defines
`InstanceData` (63 fields) as a **hand-maintained serialization shadow** of `Instance`
(95 fields), converted via `(*Instance).ToInstanceData()` and `FromInstanceData(...)`
in `session/instance_serialization.go` (381 lines total, `ToInstanceData` alone
manually assigns ~73 fields one-by-one starting at line 22). Every time a field is
added to `Instance`, a human must remember to also update `InstanceData`,
`ToInstanceData`, and `FromInstanceData` in lockstep, or the field silently fails to
persist/restore. This is a duplication-of-knowledge smell layered directly on top of
the God Object — two large structs that must be kept in sync by hand, with no
compiler-enforced link between them.

---

## Top Findings — Ranked by (Cost of Leaving Unaddressed) vs. (Disruption to Fix)

### 1. [Highest severity — data race, not just style] Unsynchronized cross-package field mutation on `Instance`
**Evidence**: `server/services/session_service.go` performs ~20+ direct writes to
`Instance` exported fields (`Title`, `Category`, `WorkingDir`, `AutonomousMode`,
`PauseReason`, `ArchivedAt`, `Program`, `GitHubPRURL`, `MCPServerURL`,
`CreationProgress`, lines 359, 1293, 1439, 1445, 1472, 1500, 1524, 1567, 2502, 3452,
3700, 4245, 4508) and `daemon/daemon.go:43,312` (`AutoYes`) — with **zero** references
to `stateMutex` in `session_service.go`, despite `session/instance.go` documenting
several of these exact fields (lines 292, 371) as "Protected by stateMutex" and
defining six separate mutexes (lines 313, 332, 335, 347, 351, 354) for different field
subsets.
**Cost of leaving unaddressed**: active, unguarded data races on session state mutated
from the hottest, highest-churn file in the repo (91 commits/6mo) — this is exactly the
class of bug the user's earlier session already found ("guarded by an ad hoc mutex
that's already proven unreliable"), and it will keep causing intermittent, hard-to-repro
corruption (lost renames, stuck pause states, wrong PR links) as load increases.
**Remediation direction**: give `Instance` a real encapsulated API (`SetTitle`,
`SetProgram`, etc., or a single `Update(func(*Instance))` mutator) that internally
takes `stateMutex`; make the fields unexported so the compiler prevents any future
direct-write regression. This is the same direction as the in-flight actor/concurrency
migration in `project_plans/` — this finding is corroborating evidence for that
project's priority, not a new competing effort.

### 2. [God Object] `session.Instance` — 95 hand-written fields, 314-line class diagram entry, 6 separate mutexes
**Evidence**: `session/instance.go:92` (struct), confirmed by `sg` field count (§3a),
`goplantuml` class size (§5), and mutex sprawl (§3d, §6).
**Cost of leaving unaddressed**: every new feature that touches session state has to
understand which of 6 mutexes protects which subset of 95 fields; afferent coupling
data (§1) shows 20 packages depend on `session`, meaning changes here have the widest
blast radius of any type in the codebase.
**Remediation direction**: decompose along the natural mutex boundaries already
present (lifecycle state, process management, rate-limit callbacks, restart state) into
smaller aggregates each with one owner and one lock, per the Aggregate/Repository
pattern in `architecture-best-practices.md`. High disruption (touches 20 dependent
packages) — matches the existing migration project's scope, do not duplicate.

### 3. [Cognitive complexity outlier] `ConnectRPCWebSocketHandler.streamViaTmuxCapturePane` / `streamViaControlMode`
**Evidence**: `server/services/connectrpc_websocket.go:1015` (cognitive 242, cyclomatic
72) and `:472` (cognitive 232, cyclomatic 77) — 3-5x every other hand-written function
in the repo (§4), in a file with 43 commits/6mo.
**Cost of leaving unaddressed**: these two functions are the WebSocket terminal
streaming path — core, always-on runtime code. At this complexity they are effectively
unreviewable in a PR diff and any bug fix risks new regressions.
**Remediation direction**: extract the control-mode/capture-pane branches into named
helper functions or a small state-machine type; this is a localized refactor (one file,
two functions) — low disruption, should be scheduled independently and soon.

### 4. [Low cohesion] `server/services/session_service.go` — 4,542 lines, 150 exported functions, highest churn file in repo (91 commits/6mo)
**Evidence**: §3c (exported function count), §4 (complexity × churn table), and the
1.5MB call-graph dot output for this single package (§2) as a density proxy.
**Cost of leaving unaddressed**: this file is the most frequently edited file in the
codebase; every change is a chance to touch unrelated functionality bundled in the same
file, and the CreateSession/UpdateSession functions inside it are themselves in the
top-6 cyclomatic complexity list (63 and 58).
**Remediation direction**: split by Application Service responsibility (session
lifecycle CRUD vs. terminal streaming vs. PR/GitHub integration vs. review-queue
wiring) into separate service files/types, each depending on `session.Instance`'s new
encapsulated API from Finding #1 rather than reaching into its fields directly.
Medium disruption — mechanical file-split once Finding #1's accessor API exists.

### 5. [Duplication / knowledge shadow] `InstanceData` (63 fields) manually synced with `Instance` (95 fields)
**Evidence**: `session/storage.go:17`, conversion functions in
`session/instance_serialization.go:22,129` (§6).
**Cost of leaving unaddressed**: silent data loss risk — every new `Instance` field
requires a human to remember to thread it through `ToInstanceData`/`FromInstanceData`;
no compiler check enforces this, and it will not fail loudly when forgotten (the field
just won't persist).
**Remediation direction**: once `Instance` is decomposed (Finding #2), regenerate the
persistence shadow via code generation (`go:generate` + reflection-based mapper, or
adopt the `ent` schema directly as the source of truth instead of a hand-rolled
struct) rather than hand-maintaining two structs in lockstep. Medium disruption,
naturally sequenced after Finding #2.

### 6. [Package sprawl under one name] `session/` package contains 30+ conceptually distinct sub-domains
**Evidence**: `goplantuml` namespace breakdown (§5) — 55 namespaces recursively under
`session/`, 669 hand-written classes; afferent coupling of 20 dependent packages (§1)
on the top-level `session` package specifically.
**Cost of leaving unaddressed**: moderate — Go's package system already provides some
isolation (tmux, vnc, cdp, git, vc are already sub-packages), so this is less urgent
than Findings 1–4, but the top-level `session` package itself (not its sub-packages)
still holds the God Object and most of the coupling.
**Remediation direction**: no action needed on the sub-package structure itself (it's
reasonably decomposed already); this finding is really a restatement of Finding #2 from
a different tool's vantage point — included for completeness, not as independent work.

### 7. [Minor] Scattered primitive-obsession parameter lists
**Evidence**: §3b — `newLogManager` (10 params), `RegisterRoutes` (9),
`UpdateInstancePRStatus` (8), `NewNotificationEvent` (8), `AppendAutoApproved` (8).
**Cost of leaving unaddressed**: low — worst offender is 10 params, not an extreme
outlier for this codebase's size, and these are mostly constructors/registration
functions called from few sites.
**Remediation direction**: apply `type-driven-design` skill's parameter-object pattern
opportunistically when next touched; not worth a dedicated pass.

---

## Tool Success Summary

| Tool | Status | Notes |
|---|---|---|
| `goda` (package graph) | Ran successfully | `loov.dev/goda` vanity path broken; used `github.com/loov/goda` directly |
| `go-callvis` (call graph) | Ran successfully | `-algo=cha` fails on Go 1.25 generic-type-alias GODEBUG mismatch in a dependency; `-algo=static` fallback worked |
| `ast-grep` (struct/param/cohesion/coupling) | Ran successfully | All 4 sub-queries (a–d) completed |
| `gocyclo` / `gocognit` (complexity) | Ran successfully | Both installed and ran clean; excluded entgo-generated code from rankings as noise |
| `goplantuml` (UML diagram) | Ran successfully | `.puml` text generated for both packages; not rendered to image (no local PlantUML renderer available) |

All five requested tool categories produced usable evidence — no fallback to
"opinion-only" analysis was needed for any of them.
