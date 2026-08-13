# Code Archaeology Analysis: fuel-forge

**Analyzed**: 2026-04-08
**Source**: ~/IdeaProjects/fuel-forge (local, HEAD: 4d31af9)
**Working directory**: /tmp/archaeology-fuel-forge-1775608895

---

## 1. Executive Summary

Fuel Forge is a multi-agent orchestration engine for autonomous software development. It runs a pool of Claude Code worker agents ("polecats") in isolated git worktrees, coordinated through NATS pub/sub, with a Dolt versioned database as the source of truth and SQLite as a local cache. A single MCP server exposes all orchestration capabilities to both interactive users (via Claude Code, a Lanterna TUI, and a Compose Multiplatform client) and the agents themselves, creating a self-orchestrating loop: agents claim work items, write code, and trigger their own merge queue.

---

## 2. Tech Stack

| Layer | Technology | Evidence |
|-------|-----------|----------|
| Language | Kotlin (JVM 21) | `build.gradle.kts`, `forge-mcp/build.gradle.kts` |
| Framework | Ktor (HTTP/WS server), Exposed (ORM) | `forge-mcp/build.gradle.kts` deps |
| Database (source of truth) | Dolt 1.45.4 (MySQL-compatible, versioned) | `docker-compose.yml:18` |
| Database (local cache) | SQLite via JDBC | `forge-common/src/.../ForgeDatabase.kt` |
| Message Bus | NATS 2 with JetStream | `docker-compose.yml:4`, `forge-nats/` module |
| Serialization | Protobuf (service contracts) + kotlinx.serialization (JSON) | `proto/` dir, `build.gradle.kts` |
| Client UI | Compose Multiplatform (Desktop, Web/WASM, iOS, Android, Terminal TUI) | `settings.gradle.kts`, `client/` module |
| MCP Protocol | Kotlin MCP SDK | `forge-mcp/build.gradle.kts:mcp-kotlin-sdk` |
| Observability | Langfuse (LLM tracing), OTEL, LiteLLM proxy | `.env.example`, `services/observability/` |
| Build System | Gradle (KTS) 8.x | `gradlew`, `settings.gradle.kts` |
| CI/CD | GitHub Actions | `.github/workflows/` |
| Deployment | Docker Compose (local), AWS ECS Fargate (planned) | `docker-compose.yml`, `plans/containerized-agent-architecture.md` |

---

## 3. Domain Model

### Entities

| Entity | Key Fields | Relationships | Source |
|--------|-----------|---------------|--------|
| **Bead** | id, title, status, priority, assignee, storyPoints, parentId, acceptanceCriteria, fileScope | has-many BeadDeps, has-many Events | `ForgeSchema.kt:107` |
| **Polecat** (agent worker) | beadId, pid, worktree, branch, startedAt | works-on Bead, has AgentPhase | `SpawnService.kt:19` |
| **AgentPhase** | agent, phase, bead, commitHash, engine, langfuseTraceId | belongs-to Bead | `ForgeSchema.kt:18` |
| **Event** | ts, agent, event, bead, detail, team | append-only log | `ForgeSchema.kt:6` |
| **Merge** | agent, bead, commitHash, status, reviewStatus, retryCount | outcome of Polecat work | `ForgeSchema.kt:32` |
| **Warrant** | agent, status, reason, staleSince | issued by Witness for stalled agents | `ForgeSchema.kt:215` |
| **Review** | agent, bead, grade (A-F), verdict, criticalCount | quality gate for merges | `ForgeSchema.kt:228` |
| **Observation** | agent, beadId, category, fileScope, content | persistent notes across agent sessions | `ForgeSchema.kt:246` |
| **SessionCost** | agent, model, estimatedCostUsd, pricingSnapshot | API cost tracking | `ForgeSchema.kt:258` |
| **AgentMemory** | agent, key, content, category | cross-session agent memory | `ForgeSchema.kt:276` |
| **DraftBead** | id, title, status, acceptanceCriteria | staging area before live board | `ForgeSchema.kt:170` |
| **IntegrationBranch** | branch, agent, bead, status, headCommit, mergedAt | branch lifecycle tracking | `ForgeSchema.kt:317` |

### Entity Relationship Summary

```
Plan.md → (parsed) → DraftBeads → (published) → Beads
                                                    │
                                             BeadDeps (DAG)
                                                    │
                                    Scheduler → ReadyBeads
                                                    │
                               Polecat (claims) → AgentPhase
                                                    │
                                               git worktree
                                                    │
                                            Merges → Reviews
                                                    │
                                           IntegrationBranch
                                                    │
                                        Events (append-only log)
```

---

## 4. API / Interface Contracts

### MCP Tools (exposed to Claude Code and agents)

| Tool | Sub-actions | Purpose | Source |
|------|-------------|---------|--------|
| `forge_dashboard` | — | Full overview: tasks, agents, merges, CI | `ForgeTools.kt` |
| `forge_tasks` | seed, claim, complete, ready, dep_tree, reset, mountain | Dependency-aware task management | `TaskTools.kt` |
| `forge_ci` | build, rebuild, merge_status, run | CI/CD operations | `CiTools.kt` |
| `forge_agents` | spawn, stop, list, nudge, phase, log, purge | Agent lifecycle | `AgentTools.kt` |
| `forge_pool` | status, scale, synthesize, scheduler | Pool sizing and stale detection | `PoolTools.kt` |
| `forge_plan` | seed | Parse plan.md → Kanban board | `PlanTools.kt` |
| `forge_backlog` | create, list, update | Backlog authoring | `BacklogTools.kt` |
| `forge_backlog_refine` | analyze, suggest | Backlog grooming with LLM | `BacklogRefineTools.kt` |
| `forge_debug` | health, config, db_stats | Diagnostics | `DebugTools.kt` |
| `forge_pause` | pause, resume | Dispatch control | `PauseTools.kt` |
| `forge_resources` | suspend, resume, kill_idle | Rig resource management | `ResourceTools.kt` |
| `forge_formula` | run, status | Formula (workflow recipe) execution | `FormulaTools.kt` |

### REST Endpoints (Ktor HTTP, port 3100)

| Method | Path | Purpose | Source |
|--------|------|---------|--------|
| GET | `/api/forge` | Dashboard data (tasks, agents, merges) | `ApiHandlers.kt` |
| GET | `/health` | Service health check | `KtorServer.kt` |
| WS | `/ws` | Real-time push to client surfaces | `KtorServer.kt` |
| POST | `/mcp` | MCP HTTP transport entry point | `KtorServer.kt` |
| POST | `/intake` | Plan intake from desktop app | `ForgeRoutes.kt` |

### NATS Topics

| Subject Pattern | Direction | Purpose | Source |
|----------------|-----------|---------|--------|
| `service.*.capabilities` | Publish (service → core) | Service registration | `NatsServiceBridge.kt:55` |
| `service.*.health` | Publish (service → core) | Heartbeat | `NatsServiceBridge.kt:84` |
| `service.{name}.execute.{action}` | Request/Reply | Service action invocation | `NatsServiceBridge.kt:97` |
| `agent.{beadId}.progress` | Publish | Agent streaming output | `containerized-agent-architecture.md` |
| `agent.{beadId}.complete` | Publish | Agent completion signal | `containerized-agent-architecture.md` |

### Proto-Defined Service Contracts

| Service | Key Messages | Source |
|---------|-------------|--------|
| `spawn` | `agent.spawn`, `agent.stop`, `workspace.create` | `proto/forge/services/spawn.proto` |
| `ci` | build, test, merge | `proto/forge/services/ci.proto` |
| `git` | branch, merge, diff | `proto/forge/services/git.proto` |
| `gastown` | dispatch, polecat status | `proto/forge/services/gastown.proto` |
| `llm` | inference routing | `proto/forge/services/llm.proto` |
| `observability` | cost tracking, Langfuse | `proto/forge/services/observability.proto` |
| `session` | session state | `proto/forge/services/session.proto` |
| `figma` | design reads | `proto/forge/services/figma.proto` |

---

## 5. Functional Requirements (Derived)

| ID | Requirement | Source Artifact | Confidence |
|----|------------|-----------------|------------|
| FR-01 | System SHALL manage a backlog of work items (Beads) with dependency tracking | `ForgeSchema.kt:99-103`, `TaskTools.kt` | High |
| FR-02 | System SHALL parse markdown plan files into structured Beads and seed the board | `AdaptivePlanParser.kt`, `PlanTools.kt` | High |
| FR-03 | System SHALL spawn worker agents (polecats) in isolated git worktrees, one per Bead | `SpawnService.kt`, `WorktreeManager.kt` | High |
| FR-04 | System SHALL enforce Bead dependency ordering (only dispatch when all deps done) | `ForgeSchema.kt:94-97`, `GastownEventSync.kt` | High |
| FR-05 | System SHALL process a merge queue, running CI and applying AST-based conflict resolution | `MergeQueueProcessor.kt`, `AstMergeResolver.kt` | High |
| FR-06 | System SHALL track agent cost per session and emit structured observability events | `SessionCostEstimator.kt`, `LangfuseClient.kt` | High |
| FR-07 | System SHALL detect stalled agents (Witnesses) and issue warrants for recovery | `ForgeSchema.kt:215-225`, `CLAUDE.md:stall-checklist` | High |
| FR-08 | System SHALL expose all orchestration capabilities as MCP tools accessible to Claude Code | `ForgeMcpServer.kt:224-241` | High |
| FR-09 | System SHALL provide a real-time dashboard to all client surfaces via WebSocket | `KtorServer.kt`, `ApiHandlers.kt` | High |
| FR-10 | System SHALL support operating modes (idle/eco/standard/turbo/max) to cap concurrency | `CLAUDE.md:96-104` | High |
| FR-11 | System SHALL auto-schedule ready Beads via a dependency-aware scheduler | `GastownEventSync.kt:autoScheduleReadyBeads`, `CLAUDE.md:114` | High |
| FR-12 | System SHALL sanitize user-controlled text fields to prevent prompt injection | `PromptInjectionDefense.kt` | High |
| FR-13 | System SHALL apply per-agent per-tool rate limiting with sliding window algorithm | `RateLimiter.kt` | High |
| FR-14 | System SHALL persist agent observations and memories across sessions | `ForgeSchema.kt:246-285` | High |
| FR-15 | System SHALL support backlog drafting (staging area before publishing to live board) | `ForgeSchema.kt:169-195`, `BacklogTools.kt` | High |

---

## 6. Non-Functional Requirements (Derived)

| ID | Category | Requirement | Evidence | Confidence |
|----|----------|------------|----------|------------|
| NFR-01 | Performance | NATS sync loop runs on 5s fast / 30s slow intervals | `.env.example:FORGE_FAST_SYNC_MS=5000` | High |
| NFR-02 | Performance | Dolt direct JDBC reads with bd CLI fallback (eliminates subprocess per read) | `DoltConnection.kt:init()` | High |
| NFR-03 | Scalability | Operating modes support 2-8 concurrent agents on a single machine | `CLAUDE.md:forge-mode eco/turbo/max` | High |
| NFR-04 | Scalability | Architecture designed for AWS ECS Fargate Spot scaling to 50 agents | `plans/containerized-agent-architecture.md:Phase3` | Medium |
| NFR-05 | Reliability | Sync loop has belt-and-suspenders: MCP loop + bash fallback + keepalive script | `CLAUDE.md:auto-dispatch-sources` | High |
| NFR-06 | Reliability | Circuit breaker for beads: 3 failures → permanently blocked, with manual clear | `CLAUDE.md:stall-checklist` | High |
| NFR-07 | Reliability | NATS reconnects infinitely with 2s wait | `NatsServiceBridge.kt:maxReconnects(-1)` | High |
| NFR-08 | Reliability | Transaction retry wrapper for SQLite concurrency | `ForgeSchema.kt:TransactionRetry` (file exists) | High |
| NFR-09 | Security | iptables DROP-all firewall for containerized agents (allowlist-only) | `plans/containerized-agent-architecture.md:init-firewall.sh` | High |
| NFR-10 | Security | Per-agent budget cap (`--max-budget-usd`) enforced by Claude Code | `containerized-agent-architecture.md:MAX_BUDGET_USD=5` | High |
| NFR-11 | Security | Prompt injection defense on all user-controlled text rendered in LLM context | `PromptInjectionDefense.kt` | High |
| NFR-12 | Observability | Full MCP tool audit log (tool name, action, agent, params, status, duration) | `ForgeSchema.kt:AuditLog:349-361` | High |
| NFR-13 | Observability | Langfuse LLM tracing with span IDs linked to beads and agent sessions | `ForgeSchema.kt:langfuseTraceId` in AgentPhases | High |
| NFR-14 | Cost | Blended cost model: ~$30/hr at turbo (Opus crew, Sonnet polecats, Haiku infra) | `CLAUDE.md:cost-model` | High |

---

## 7. Architectural Patterns

| Pattern | Evidence | Assessment |
|---------|----------|------------|
| **Event-Driven (NATS pub/sub)** | All inter-service communication via NATS topics; services register capabilities and emit health | Well-implemented as the service mesh backbone |
| **CQRS (read/write split)** | Dolt writes via bd CLI (preserves commit semantics); reads via direct JDBC or CLI | Partially implemented; prevents Dolt write amplification |
| **MCP-First API** | Every orchestration capability exposed as MCP tool, not REST | Strong pattern: agents and humans use the same interface |
| **Dual-Store Cache** | Dolt = source of truth; SQLite = local cache synced by GastownEventSync | Creates divergence risk but enables offline/fast reads |
| **Service Registry** | Services self-register via NATS capability announcements; registry routes actions | Clean NATS-native service discovery |
| **Claim-Lock Concurrency** | SQLite `claim_locks` table prevents concurrent Bead claiming races | Pragmatic use of SQLite's serializable isolation |
| **Pipeline Pattern** | `PipelineRunner` / `PipelineStep` / `StepResult` for composable CI merge steps | Clean; AST merge step plugs into pipeline |
| **AST-Based Conflict Resolution** | `AstMergeResolver` does 2-way and 3-way semantic merge of Kotlin declarations | Novel and unique; handles the "two agents add different functions to same file" case |
| **Signal File Fallback** | MCP sampling → log notification → pending signals in `.forge/signals/` | Three-tier degradation for LLM wakeup when sampling is unsupported |
| **Identity Pool** | `IdentityPool` assigns stable agent personas (name, avatar, color) | Enables consistent agent tracking across sessions |

---

## 8. Notable Design Choices

### 8.1 MCP as the Universal Bus
- **Decision**: Every capability (tasks, CI, agents, backlog, resources) exposed as MCP tools, consumed identically by interactive users and autonomous agents.
- **Alternatives**: REST API for users, separate internal API for agents.
- **Likely rationale**: Avoids maintaining two API surfaces. Agents and humans literally speak the same language.
- **Evidence**: `ForgeMcpServer.kt:224-241`, `CLAUDE.md:mcp-tools`

### 8.2 Dolt as Versioned Source of Truth
- **Decision**: All Bead state goes through Dolt (MySQL-compatible with git semantics — every write is a commit).
- **Alternatives**: Postgres, plain MySQL, SQLite-only.
- **Likely rationale**: Git-versioned database gives an audit trail of all task state changes. Also aligns philosophically with agents that work via git.
- **Evidence**: `docker-compose.yml:18`, `DoltConnection.kt`, `CLAUDE.md:dolt-routing`

### 8.3 NATS for Service Mesh
- **Decision**: All capability services (spawn, CI, git, figma, LLM, observability, session, gastown) are NATS participants that self-register.
- **Alternatives**: gRPC service mesh, HTTP REST between services, monolith.
- **Likely rationale**: NATS's request-reply pattern is simpler than gRPC for a JVM monorepo. Services can join/leave without code changes.
- **Evidence**: `NatsServiceBridge.kt`, `forge-nats/` module, `docker-compose.yml`

### 8.4 AST-Based Merge Conflict Resolution
- **Decision**: When two agents produce conflicting git merges, use AST parsing (not text diff) to detect whether the conflict is safe to auto-resolve (disjoint function additions).
- **Alternatives**: Always require human review, use LLM to resolve conflicts, reject and re-queue.
- **Likely rationale**: The dominant conflict pattern in multi-agent work is "two agents each added a new function to the same file." Git sees line overlap; AST sees no semantic conflict. Auto-resolution enables fully autonomous merge queues.
- **Evidence**: `AstMergeResolver.kt`, `AstMergeStep.kt`, `MergeQueueProcessor.kt`

### 8.5 Three-Tier Signal Fallback
- **Decision**: MCP server tries `createMessage()` (sampling), then `sendLoggingMessage()`, then queues signal for poll-based retrieval.
- **Alternatives**: Single mechanism, require specific Claude Code version.
- **Likely rationale**: MCP sampling support varies by client. Graceful degradation ensures signals always get delivered.
- **Evidence**: `ForgeMcpServer.kt:568-618`

### 8.6 Compose Multiplatform for All Clients
- **Decision**: Single Kotlin codebase targets Desktop, Web/WASM, iOS, Android, and Terminal TUI.
- **Alternatives**: React web + native mobile apps + separate TUI.
- **Likely rationale**: Maximizes code reuse, keeps all surfaces in Kotlin (same language as server), enables one developer to maintain all surfaces.
- **Evidence**: `settings.gradle.kts` (includes `client`, `fuel-ui`, `fuel-app`), `README.md:architecture`

---

## 9. Problems to Solve / Technical Debt

### Blocking (Must fix before adoption)

| # | Problem | Location | Impact | Suggested Fix |
|---|---------|----------|--------|---------------|
| 1 | **Broken cross-module references** — ci module's `WorktreeMergePipeline.kt.broken`, mcp module's `AgentTools.kt` has circular dep on server | `CLAUDE.md:known-build-issues`, `AgentTools.kt` | Server won't build with ci/mcp modules enabled | Resolve circular dep: extract shared interface to forge-common; fix PipelineRunner import |
| 2 | **Spawn service runs on host, not Docker** | `SpawnService.kt:8-16`, `docker-compose.yml` (no spawn service) | Breaks containerization goal; requires Java 21 + Claude CLI on every dev machine | Implement `DockerSpawner.kt` (Phase 2 plan already exists in `plans/containerized-agent-architecture.md`) |
| 3 | **Three diverging data stores** | `CLAUDE.md:data-coordination` | Dolt/SQLite/gt divergence causes "bead not found" errors and stalled dispatchers | Add reconciliation health check as startup gate; make divergence self-healing via GastownEventSync |

### Significant (Should fix early)

| # | Problem | Location | Impact | Suggested Fix |
|---|---------|----------|--------|---------------|
| 1 | **External CLI dependency** (`gt`, `bd`) — paths hardcoded to `/opt/homebrew/bin/gt` | `ForgeMcpServer.kt:125-128`, `GastownBridge.kt` | Not portable; requires Gastown toolchain installed on host | Abstract behind feature flag; provide Docker-only mode that doesn't need gt/bd |
| 2 | **Complex zombie polecat recovery** — requires 8-step manual runbook | `CLAUDE.md:zombie-polecat-recovery` | Operations burden; frequent enough to need documentation | Implement automatic recovery in GastownEventSync (auto-kill zombie processes, not just dir cleanup) |
| 3 | **SQLite `claim_locks` stale lock risk** — no TTL enforcement at DB level | `ForgeSchema.kt:162` | Crashed agent can permanently lock a Bead | Add `WHERE acquiredAt < datetime('now', '-5 minutes')` to cleanup cron in sync loop |
| 4 | **No formal API versioning** for MCP tools | `ForgeMcpServer.kt:211` | Breaking changes will silently corrupt agent behavior | Add version field to tool schemas; add version to `implementation("fuel-forge", "0.3.0")` |
| 5 | **Polling-heavy sync loop** — 5s/30s intervals regardless of activity | `GastownEventSync.kt:schedule` | Unnecessary load; delayed reactions to state changes | Switch to event-driven sync: NATS events trigger targeted SQLite updates |

### Minor (Fix when convenient)

| # | Problem | Location | Impact | Suggested Fix |
|---|---------|----------|--------|---------------|
| 1 | **Thin test coverage** — mainly forge-common; forge-mcp has 3 tests | `forge-mcp/src/test/kotlin/` | Regressions in tool handlers go undetected | Add integration tests for each MCP tool action |
| 2 | **Hardcoded user path in CLAUDE.md** | `CLAUDE.md:49` — `/Users/alexandermurphy/gt/...` | Config drift for other developers | Template with `$HOME` variable or read from rc config |
| 3 | **Chat messages table** with no apparent reader | `ForgeSchema.kt:ChatMessages:65-76` | Schema bloat; unclear lifecycle | Remove or document the intended consumer |
| 4 | **Convoys and MountainEaters** tables with sparse documentation | `ForgeSchema.kt:301-347` | Cognitive overhead for future maintainers | Add doc comments explaining the convoy/mountain-eater feature |
| 5 | **`decks.zip` in repo root** (2.5MB binary) | Root directory listing | Bloats clone size | Move to git-lfs or external asset storage |

---

## 10. Adoption Considerations

### Prerequisites
- Gastown toolchain (`gt`, `bd` CLIs) for full dispatch functionality
- Java 21+ for building the Kotlin monorepo
- Docker (any runtime) for NATS + Dolt services
- Claude Code CLI for agent spawning (Phase 0/1) or Bedrock/API key for containers (Phase 2+)
- Understanding of the Bead/Polecat/Refinery/Deacon vocabulary before reading code

### Estimated Complexity
- **Codebase size**: 607 files, ~350 Kotlin source files
- **External dependencies**: 15+ runtime deps (NATS, Dolt, Exposed, Ktor, MCP SDK, Protobuf, Langfuse, LiteLLM)
- **External service integrations**: Anthropic API / Bedrock, Langfuse, Dolt Cloud, Figma (via WebSocket relay), GitHub
- **Data migration needs**: Minimal for new installs; Dolt schema managed via bd CLI; SQLite schema created on first run

### Recommended Next Steps
1. Fix the ci/mcp module circular dependency (tracked SE-477-480) to get a clean build
2. Implement Phase 2 containerized agents (DockerSpawner) — design is already complete in `plans/containerized-agent-architecture.md`
3. Replace polling sync loop with event-driven NATS triggers for SQLite updates

---

## 11. Lessons for Stapler-Squad

### High-Value Patterns to Adopt

| Pattern | Fuel-Forge Approach | Stapler-Squad Opportunity | Priority |
|---------|--------------------|--------------------------| ---------|
| **Prompt injection defense** | `PromptInjectionDefense.kt` sanitizes all user-controlled strings (bead titles, descriptions, acceptance criteria) before LLM rendering | Sanitize session titles, branch names, and tags before they appear in approval requests or hook content | **High** |
| **Per-agent rate limiting** | `RateLimiter.kt` — sliding window, per-session per-tool, configurable defaults and overrides | Rate-limit approval request handlers per session to prevent approval spam from runaway agents | **High** |
| **MCP tool audit log** | `AuditLog` table records every tool call: tool name, sub-action, agent, params, status, duration | Add audit log table to stapler-squad's session storage for debugging session history | **Medium** |
| **Agent memory persistence** | `AgentMemories` table: key-value per agent, survives session restarts | Store agent-specific context hints in stapler-squad's DB so new sessions can inherit context | **Medium** |
| **AST-based merge conflict resolution** | `AstMergeResolver` detects whether two agents' changes are semantically disjoint (both added different functions) | When stapler-squad detects merge conflicts in branches, attempt AST-level auto-resolution before escalating to human | **Medium** |
| **Three-tier signal fallback** | MCP sampling → log notification → poll queue | Stapler-squad's approval requests could similarly degrade: WebSocket push → SSE → poll endpoint | **Medium** |
| **Operating mode concept** | `forge-mode idle/eco/standard/turbo/max` — operator controls concurrency | Add a `max-concurrent-sessions` setting to stapler-squad with named presets | **Low** |
| **Stall detection / warrants** | Witness service issues warrants for agents with no commits in N minutes | Detect sessions that haven't written output in N minutes and surface them in the UI as "stalled" | **Low** |

### Architecture Observations

**What fuel-forge does dramatically better:**
1. **Observability** — Langfuse tracing, cost-per-session, token burn rate visible per agent. Stapler-squad has logs; fuel-forge has telemetry.
2. **Backlog workflow** — Plan markdown → parsed → drafted → published to live board. Stapler-squad starts sessions manually; fuel-forge starts them from a structured backlog.
3. **Merge automation** — Full autonomous merge queue with AST conflict resolution, review grades (A-F), retry cycles. Stapler-squad surfaces diffs but doesn't automate merges.
4. **Security posture** — Prompt injection defense, budget caps, iptables firewall, non-root agents. Stapler-squad has none of these.

**What stapler-squad does better:**
1. **Operational simplicity** — Single Go binary, no `gt`/`bd` dependency, no 3-store data coordination problem.
2. **Terminal streaming fidelity** — Control-mode tmux streaming with ANSI support. Fuel-forge's polecats write to worktrees; terminal output isn't streamed back.
3. **Live session attachment** — `ssq-mux` PTY multiplexer for monitoring external Claude sessions. Fuel-forge agents are fire-and-forget.
4. **Test coverage** — Stapler-squad has benchmarks and targeted unit tests; fuel-forge's test suite is sparse outside forge-common.

**Where fuel-forge's complexity becomes a liability:**
- The 3-data-store problem (Dolt + SQLite + gt) creates a class of divergence bugs that required a 12-step recovery runbook. The single-source-of-truth principle stapler-squad follows (sessions.json or DB) is harder to get wrong.
- External CLI dependencies (`gt`, `bd`) make fuel-forge non-portable. Every stapler-squad session is self-contained.
- The zombie polecat problem is tmux + process management at scale — exactly the problem stapler-squad solved cleanly with its session lifecycle model.

### Immediate Actions

1. **Add `PromptInjectionDefense` equivalent to stapler-squad** — any session title, branch name, or tag that flows into an approval request body should be sanitized. This is a one-file addition.

2. **Add per-session rate limiting to approval handlers** — `approval_handler.go` should reject more than N approval requests per second from any single session to prevent runaway agents from flooding the approval queue.

3. **Instrument session cost** — Stapler-squad can log token counts from Claude's usage headers. Fuel-forge's `SessionCostEstimator` is a good reference for estimating cost from session duration + model.

4. **Consider adopting the signal/poll fallback pattern** — The three-tier degradation (WebSocket push → notification → poll) is a resilience improvement worth adding to stapler-squad's approval notification path.

---

**Cleanup reminder**: Analysis working directory at `/tmp/archaeology-fuel-forge-1775608895` can be removed:
```bash
cd ~/IdeaProjects/fuel-forge && git worktree remove /tmp/archaeology-fuel-forge-1775608895
```
