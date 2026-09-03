# Implementation Plan: antigravity-history-translator

**Feature**: Robust CLI history translation and inhibition integration for Stapler squad
**Date**: 2026-08-08
**Status**: Ready for implementation
**ADRs**: ADR-001-sqlite-storage.md

---

## Dependency Visualization
```text
[Epic 1.1: Storage & Models] -> [Epic 1.2: History Parsers]
[Epic 1.2: History Parsers] -> [Epic 2.1: Inhibition Engine]
[Epic 2.1: Inhibition Engine] -> [Epic 2.2: CLI Integration]
```

---

## Phase 1: Core History Engine
### Epic 1.1: Core Data Models and Storage Setup
**Goal**: Establish the canonical history event model and initialize the SQLite database for robust storage.

#### Story 1.1.1: Canonical History Model
**As a** system component, **I want** a unified history event model, **so that** different parsers can output a standard format.
**Acceptance Criteria**:
- Go structs defined for HistoryEvent with fields for command, timestamp, directory, exit code, and program source.
**Files**: `internal/history/models.go`

##### Task 1.1.1a: Define Models (~3 min)
- Create `internal/history/models.go`.
- Define `Event` struct with `db` tags for SQLite and `json` tags.
- Add a unique composite key/hash. For Bash without timestamps, define a fallback deduplication strategy using strictly contiguous command deduplication and content hashing (dropping the unstable line-number dependency).
- Files: `internal/history/models.go`

#### Story 1.1.2: SQLite Database Initialization
**As a** system component, **I want** an initialized SQLite database, **so that** history events can be durably stored and queried.
**Acceptance Criteria**:
- SQLite schema initialized on startup.
- Basic Insert and Query methods implemented.
**Files**: `internal/storage/sqlite.go`, `internal/storage/sqlite_test.go`

##### Task 1.1.2a: Implement SQLite Repo (~5 min)
- Implement `NewSQLiteStore(dsn string)` using simple `CREATE TABLE IF NOT EXISTS` for robust schema initialization without migration concurrency issues.
- Configure SQLite connection with WAL mode, `busy_timeout`, and `_txlock=immediate` for high concurrency, and implement exponential backoff retry logic.
- Implement `InsertEvent(e *history.Event) error` and batch inserts wrapped in transactions (`InsertEvents`) for the initial ingestion phase to prevent lock contention.
- Implement a database retention policy (TTL) to automatically prune history events older than a specified duration.
- Files: `internal/storage/sqlite.go`

### Epic 1.2: History Parsers
**Goal**: Implement parsers for Claude Code and Antigravity history files, converting them into canonical events.

#### Story 1.2.1: Claude Code History Parser
**As a** translator, **I want** to parse Claude Code history files, **so that** I can import rich session history.
**Acceptance Criteria**:
- Correctly parses `~/.claude/history.jsonl` format.
**Files**: `internal/parser/claudecode.go`, `internal/parser/claudecode_test.go`

##### Task 1.2.1a: Implement Claude Code Parser (~5 min)
- Write a streaming JSONL parser to handle Claude Code history.
- Ensure strict streaming to avoid memory spikes on large files.
- Explicitly add environment variable resolution for locating the `.claude` directory.
- Files: `internal/parser/claudecode.go`

#### Story 1.2.2: Antigravity History Parser
**As a** translator, **I want** to parse Antigravity history files, **so that** I can import Antigravity command history.
**Acceptance Criteria**:
- Parses JSONL transcripts from `~/.gemini/antigravity-cli/brain/<conversation-id>/.system_generated/logs/transcript.jsonl`.
**Files**: `internal/parser/antigravity.go`, `internal/parser/antigravity_test.go`

##### Task 1.2.2a: Implement Antigravity Parser (~5 min)
- Write a streaming JSONL parser to handle Antigravity transcripts.
- Map the JSONL `content` and `type` fields accurately to canonical events.
- Files: `internal/parser/antigravity.go`

### Epic 1.3: History Exporters
**Goal**: Implement exporters to translate and inject history back into target programs.

#### Story 1.3.1: Export to Program Formats
**As a** translator, **I want** to export canonical history to Claude Code and Antigravity formats, **so that** target programs can ingest the translated history.
**Acceptance Criteria**:
- Exporters generate valid `history.jsonl` and `transcript.jsonl` formats.
**Files**: `internal/exporter/claudecode.go`, `internal/exporter/antigravity.go`

##### Task 1.3.1a: Implement Exporters (~5 min)
- Convert `HistoryEvent` to target JSONL formats (using original commands, NOT sanitized ones).
- Implement safe file injection relying strictly on standard POSIX `O_APPEND` (atomic for writes under `PIPE_BUF`).
- Files: `internal/exporter/claudecode.go`, `internal/exporter/antigravity.go`

#### Story 1.3.2: Export to Agent Context
**As a** translator, **I want** to export history for AI agents, **so that** they receive context securely.
**Acceptance Criteria**:
- Exports sanitized events (with inhibition rules applied) in a format suitable for agents.
**Files**: `internal/exporter/agent.go`

##### Task 1.3.2a: Implement Agent Exporter (~4 min)
- Convert `HistoryEvent` to JSON/Agent format, applying inhibition engine rules.
- Implement time-bounding (e.g., last 24 hours), pagination, or semantic filtering (top-k) to prevent LLM context overload.
- Integrate directly with Stapler Squad agent context injection endpoints or standard JSON stdout (ensuring strict separation of stdout for data and stderr for logs).
- Files: `internal/exporter/agent.go`

## Phase 2: Inhibition & CLI Integration
### Epic 2.1: Stapler Squad Inhibition Integration
**Goal**: Apply inhibition rules to filter out sensitive or restricted commands before storage or agent context provision.

#### Story 2.1.1: Inhibition Engine
**As a** security control, **I want** to apply inhibition rules to parsed history, **so that** sensitive data is not leaked to agents.
**Acceptance Criteria**:
- Engine explicitly defines that it redacts events (sanitizing), replacing secrets with exact placeholders like `[REDACTED_CREDENTIAL]`. This clear strategy allows the agent to handle redacted payloads correctly without hallucinating.
- Define explicit data flow: Original commands are kept ONLY in the local SQLite storage. Agent exporter strictly outputs ONLY sanitized commands to target contexts, while program exporters output original commands.
- Implements basic rules using established regex sets (e.g., TruffleHog's open source rules), preceded by a fast-path heuristic check to prevent severe latency degradation.
**Files**: `internal/inhibition/engine.go`, `internal/inhibition/engine_test.go`

##### Task 2.1.1a: Implement Engine Logic (~4 min)
- Create `Engine` struct.
- Implement `ApplyRules(e *history.Event) (*history.Event, error)` keeping original command intact.
- Files: `internal/inhibition/engine.go`

### Epic 2.2: CLI Command Implementation
**Goal**: Wire the components into the anti-gravity CLI using Cobra.

#### Story 2.2.1: Translate Command
**As a** user, **I want** a CLI command to trigger history translation, **so that** I can manually sync history between programs.
**Acceptance Criteria**:
- `antigravity translate-history --source=claudecode --target=antigravity` executes pipeline.
**Files**: `cmd/translate.go`

##### Task 2.2.1a: Implement Translate Cmd (~5 min)
- Setup Cobra command `translateCmd`.
- Wire parser, inhibition engine, and storage.
- Files: `cmd/translate.go`
