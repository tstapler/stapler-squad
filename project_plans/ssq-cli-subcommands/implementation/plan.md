# Implementation Plan: ssq-cli-subcommands

**Feature**: Thin CLI wrapper over `SessionService` ConnectRPC for scriptable session lifecycle from shell/CI
**Date**: 2026-10-18
**Status**: Ready for implementation
**ADRs**: `decisions/adr-001-list-sessions-naming.md`, `decisions/adr-002-server-flag-placement.md`

---

## Domain Glossary

| Term | Definition |
|---|---|
| **ServerURL** | HTTP base URL of the running stapler-squad server; default `http://localhost:8543/api`; overridable via `--server` flag or `SSQ_SERVER` env var |
| **SessionID** | String identifier for a session — the server accepts either a UUID or the session's human-readable title interchangeably |
| **SteerMessage** | Free-text string injected mid-run into an autonomous session's input stream via `UpdateSession.steer_message` |
| **DiffStats** | Aggregated git statistics for a session's worktree: `added` lines, `removed` lines, and `content` (full unified diff) — returned by `GetSessionDiff` |
| **AutonomousMode** | Session driver mode (`autonomous_mode=true`) in which the agent runs without per-step human confirmation; `steer_message` is only meaningful in this mode |
| **ListSessions** | The RPC-backed, server-addressable session-listing verb provided by the new `list-sessions` subcommand, distinct from the existing `list` command that reads SQLite directly |
| **ConnectError** | A structured error returned by ConnectRPC, carrying a `connect.Code` and a human-readable `Message`; see `connectrpc.com/connect` |
| **protojson** | `google.golang.org/protobuf/encoding/protojson` — required for correct JSON marshalling of proto messages (enums become string names, not integers) |
| **Transaction Script** | PoEAA pattern for CLI commands: each command is a thin procedure that calls one RPC and formats the result; no domain model, no local state |

---

## Pattern Decisions

### Chosen approach: per-command `--server` flag + `ResolveServerURL()` helper (Option A)

Three alternatives were considered:

**Option A (chosen):** Per-command local `--server` flag in each `NewXxxCmd()` constructor; `cmd/commands/client.go` exports `ResolveServerURL(flagVal string) string` that reads `SSQ_SERVER` env var as fallback. Commands live in `cmd/commands/` as constructor functions. Registration in `main.go`'s `init()`.

**Option B (rejected):** Each command reads `SSQ_SERVER` env independently, no `--server` flag. Rejected: violates DRY; no way to target a non-default server from the command line without editing the environment.

**Option C (rejected):** Separate `ssq-client` binary. Rejected: adds a second binary artifact, inconsistent with the existing `stapler-squad`/`ssq` symlink pattern, splits documentation surface.

**Why not `rootCmd.PersistentFlags()`?** A persistent `--server` flag on `rootCmd` appears in the server-startup `--help` output, implying the server reads it — misleading for operators who run `stapler-squad` to start the daemon. Per-command flags scope the concept correctly. See `decisions/adr-002-server-flag-placement.md`.

### Constructor pattern: `NewXxxCmd() *cobra.Command`

Matching the precedent set by `newIntegrationCredentialCmd()` and `newContractBundleCmd()` in `main.go`: constructor functions return `*cobra.Command` with all flags registered internally, and are called from `init()` in `main.go`.

### Output strategy: `protojson` for all proto-typed responses, `text/tabwriter` for human tables

`encoding/json` serialises proto enums as integers; `protojson` serialises them as their string names. `DiffStats.content` is a raw string and prints directly in non-JSON mode. `text/tabwriter` for human-readable session lists (already used in `main.go`).

### `create-session` async contract: non-blocking by default, `--wait` for synchronous use

`CreateSession` returns immediately with status `SESSION_STATUS_CREATING`. Default: print the session ID and exit. `--wait` polls `GetSession` (via the same client) until status is no longer `SESSION_STATUS_CREATING`, with a configurable `--wait-timeout` (default 60s).

---

## Tech Debt Disposition

**Port `localhost:8080` in `get_session.go:25`** — this is the only pre-existing technical debt this feature touches. Root cause: hardcoded wrong default when the command was written. Fix: Phase 2 refactors `GetSessionCmd` to use `NewGetSessionCmd()` constructor with `ResolveServerURL()`. The fix is prerequisite to the shared-infrastructure story; no separate tracking ticket needed.

---

## Migration Plan

N/A — no schema or data changes. Additive CLI subcommands only.

---

## Observability Plan

All commands write errors to stderr and produce clean stdout output for pipelining. No structured logging; these are thin CLI shrapnel. The server-side RPC handlers already emit their own structured log lines.

- **Server not running:** `ResolveServerURL()` returns the URL; commands that receive `connect.CodeUnavailable` print `"error: server unreachable at <url> — is stapler-squad running?"` to stderr and exit non-zero.
- **Unknown session ID:** server returns `connect.CodeNotFound`; commands print `"error: session not found: <id>"`.
- **Steer on non-autonomous hidden session:** server returns `connect.CodeFailedPrecondition`; `steer` prints the verbatim server message.

---

## Risk Control

| Risk | Likelihood | Mitigation |
|---|---|---|
| `protojson` output diverges from `encoding/json` for callers already parsing `get-session` output | Low — only `get-session` existed and it used `encoding/json`, so existing consumers already see integer enums | Phase 2 keeps `--json` output of `get-session` identical to current behaviour (still uses `encoding/json` on the `Session` message) until a follow-up decides to migrate; document the divergence in `--help` |
| `create-session --wait` races against session deletion | Low | `GetSession` returning `CodeNotFound` while waiting is treated as an error, not as completion |
| `list-sessions` name collides with future rename of `list` | Low | `list` retains its `Use: "list"` string; `list-sessions` is a new verb; see ADR-001 |

---

## Unresolved Questions

1. **`get-session` JSON migration**: the current command uses `encoding/json` (enums as integers). Should Phase 2 switch it to `protojson`? This is a breaking change for any existing consumers. Decision deferred — Phase 2 keeps the existing marshaller for `get-session`; new commands use `protojson` from the start.
2. **`--wait` default timeout**: 60 s is a guess. The CLAUDE.md for this repo does not specify a service SLA. Leave at 60 s, configurable via `--wait-timeout`.

---

## Dependency Visualization

```
Phase 1: Shared Infrastructure
  cmd/commands/client.go     (new)
    └─ used by all new commands + retrofitted get-session

Phase 2: Bug Fix
  cmd/commands/get_session.go   (modified)
    └─ depends on Phase 1 (client.go)

Phase 3: New Commands (parallel — each depends only on Phase 1)
  cmd/commands/create_session.go  (new)
  cmd/commands/list_sessions.go   (new)
  cmd/commands/steer.go           (new)
  cmd/commands/get_diff.go        (new)
  main.go                         (modified — wire up 4 new commands + new get-session constructor)

Phase 4: Documentation (depends on Phase 3)
  README.md                       (modified — CLI reference section)
```

Phases 3 stories may be implemented concurrently; each is independent of the others within Phase 3.

---

## Phase 1: Shared Client Infrastructure

**Epic 1 — Shared Client Infrastructure**

### Story 1.1 — `cmd/commands/client.go`: server URL resolver, client constructor, error helper, proto JSON helper

**Files**: `cmd/commands/client.go` (new — ~80 LOC)
**Cost class**: S

**Acceptance criteria (Given-When-Then):**

- **Given** `SSQ_SERVER=http://myhost:9000/api` is set and `--server` flag is empty,  
  **When** `ResolveServerURL("")` is called,  
  **Then** it returns `"http://myhost:9000/api"`.

- **Given** `SSQ_SERVER` is unset and `--server` flag is `""`,  
  **When** `ResolveServerURL("")` is called,  
  **Then** it returns `"http://localhost:8543/api"`.

- **Given** `--server http://override:1234/api` flag value is passed,  
  **When** `ResolveServerURL("http://override:1234/api")` is called,  
  **Then** it returns `"http://override:1234/api"` regardless of `SSQ_SERVER`.

- **Given** a `connect.Error` with code `CodeUnavailable` and message `"connection refused"`,  
  **When** `FormatConnectError(err, serverURL)` is called,  
  **Then** it returns `"error: server unreachable at http://localhost:8543/api — is stapler-squad running?\ndetail: connection refused"`.

- **Given** a proto message `*sessionv1.Session` with `status = SESSION_STATUS_ACTIVE`,  
  **When** `MarshalProtoJSON(msg)` is called,  
  **Then** the returned bytes contain `"status": "SESSION_STATUS_ACTIVE"` (string name, not integer).

**Implementation notes:**

```go
// cmd/commands/client.go
package commands

import (
    "fmt"
    "net/http"
    "os"

    "connectrpc.com/connect"
    "github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
    "google.golang.org/protobuf/encoding/protojson"
    "google.golang.org/protobuf/proto"
)

const defaultServerURL = "http://localhost:8543/api"

// ResolveServerURL returns the server base URL in priority order:
//  1. flagVal (non-empty --server flag)
//  2. SSQ_SERVER environment variable
//  3. defaultServerURL (http://localhost:8543/api)
func ResolveServerURL(flagVal string) string { ... }

// NewSessionClient constructs a ConnectRPC SessionServiceClient.
func NewSessionClient(serverURL string) sessionv1connect.SessionServiceClient {
    return sessionv1connect.NewSessionServiceClient(http.DefaultClient, serverURL)
}

// FormatConnectError formats a ConnectRPC error for stderr output.
// Includes an actionability hint for CodeUnavailable.
func FormatConnectError(err error, serverURL string) string { ... }

// MarshalProtoJSON marshals a proto.Message to JSON bytes using protojson
// (enums as string names, not integers).
func MarshalProtoJSON(msg proto.Message) ([]byte, error) {
    return protojson.MarshalOptions{Multiline: true, EmitUnpopulated: false}.Marshal(msg)
}
```

---

## Phase 2: Bug Fix — `get-session` Port Correction

**Epic 2 — Bug Fix: get-session uses wrong default port**

### Story 2.1 — Refactor `GetSessionCmd` to use shared infrastructure

**Files**: `cmd/commands/get_session.go` (modified)
**Cost class**: XS

**Root cause**: Line 25 hard-codes `"http://localhost:8080/api"`; the correct default port is `8543` per `main.go:320`.

**Acceptance criteria (Given-When-Then):**

- **Given** a running server on `localhost:8543`,  
  **When** `ssq get-session my-session` is run without `--server`,  
  **Then** the command succeeds and returns session JSON (previously would return a connection error from port 8080).

- **Given** `SSQ_SERVER=http://localhost:9999/api`,  
  **When** `ssq get-session my-session` is run,  
  **Then** the command connects to port 9999.

- **Given** `ssq get-session my-session --server http://remotehost:8543/api`,  
  **When** the command runs,  
  **Then** it connects to `remotehost:8543`.

**Implementation notes:**

Refactor `GetSessionCmd` to `NewGetSessionCmd() *cobra.Command`. Register `--server` flag. Replace the hard-coded URL literal with `ResolveServerURL(serverFlag)`. Keep `encoding/json` for the existing `Session` JSON output to avoid breaking changes (see Unresolved Questions §1). Update `main.go:935` from `commands.GetSessionCmd` to `commands.NewGetSessionCmd()`.

---

## Phase 3: New CLI Commands

**Epic 3 — New CLI Subcommands**

All stories in Epic 3 are independent and may be implemented in parallel. Each depends on Epic 1 (Phase 1) being complete.

---

### Story 3.1 — `ssq create-session` subcommand

**Files**: `cmd/commands/create_session.go` (new), `main.go` (add `commands.NewCreateSessionCmd()`)
**Cost class**: M

**Acceptance criteria (Given-When-Then):**

- **Given** a running server and `ssq create-session --title "fix-auth" --path ~/code/myrepo`,  
  **When** the command runs,  
  **Then** it prints `session-id: <uuid>` to stdout and exits 0.

- **Given** `ssq create-session --title "fix-auth" --path ~/code/myrepo --json`,  
  **When** the command runs,  
  **Then** stdout is valid JSON with `"status": "SESSION_STATUS_CREATING"` and `"id": "<uuid>"`; no other output on stdout.

- **Given** `ssq create-session --title "fix-auth" --path ~/code/myrepo --branch feat/auth --program aider --category backend --prompt "Fix the auth bug" --auto-yes --one-shot`,  
  **When** the command runs,  
  **Then** `CreateSessionRequest` is populated with `branch="feat/auth"`, `program="aider"`, `category="backend"`, `prompt="Fix the auth bug"`, `auto_yes=true`, `one_shot=true`.

- **Given** `ssq create-session --title "fix-auth" --path ~/code/myrepo --wait`,  
  **When** the command runs and the session transitions to `SESSION_STATUS_ACTIVE` within 60 s,  
  **Then** the command prints `session-id: <uuid>` and exits 0.

- **Given** `ssq create-session --title "fix-auth" --path ~/code/myrepo --wait`,  
  **When** the async creation pipeline fails and the session reaches `SESSION_STATUS_FAILED`,  
  **Then** stderr prints `"error: session creation failed: fix-auth reached terminal state SESSION_STATUS_FAILED"` and exit code is non-zero (1).

- **Given** `ssq create-session --title "fix-auth" --path ~/code/myrepo --wait --wait-timeout 5s`,  
  **When** the session has not left `SESSION_STATUS_CREATING` after 5 s,  
  **Then** the command prints `"error: timed out waiting for session to become active"` to stderr and exits non-zero.

- **Given** `--title` is omitted,  
  **When** the command is invoked,  
  **Then** cobra prints a usage error and exits non-zero.

- **Given** `--path` is omitted,  
  **When** the command is invoked,  
  **Then** cobra prints a usage error and exits non-zero.

**Flags**: `--title` (required string), `--path` (required string), `--branch`, `--program`, `--category`, `--prompt`, `--auto-yes` (bool), `--one-shot` (bool), `--wait` (bool), `--wait-timeout` (duration, default 60s), `--json`, `--server`/`-s`.

**Non-JSON output format** (one key–value per line, pipeable):
```
session-id: 3f2a1bc4-dead-beef-cafe-000000000001
title:      fix-auth
status:     SESSION_STATUS_CREATING
path:       /home/user/code/myrepo
```

**`--wait` polling**: call `GetSession` every 2 s; apply two-branch exit logic:
- `SESSION_STATUS_ACTIVE` → exit 0, print session info
- `SESSION_STATUS_FAILED`, `SESSION_STATUS_PERMANENTLY_FAILED`, `SESSION_STATUS_CRASHED` → stderr error `"error: session creation failed: <title> reached terminal state <status>"` + exit 1
- Any other non-CREATING status → treat as Active (exit 0) to be forward-compatible
- Timeout reached while still CREATING → stderr `"error: timed out waiting for session to become active"` + exit 1
Respect `--wait-timeout`.

---

### Story 3.2 — `ssq list-sessions` subcommand

**Files**: `cmd/commands/list_sessions.go` (new), `main.go` (add `commands.NewListSessionsCmd()`)
**Cost class**: S

**Rationale for `list-sessions` vs overriding `list`**: the existing `list` command reads SQLite directly — it works without a running server and is the fallback for operators debugging a crashed instance. The new `list-sessions` command requires a running server and adds `--json`, `--status`, `--search`, `--category` filters. See `decisions/adr-001-list-sessions-naming.md`.

**Acceptance criteria (Given-When-Then):**

- **Given** a running server with two active and one paused session,  
  **When** `ssq list-sessions` is run,  
  **Then** stdout contains a tab-separated table with columns `ID`, `TITLE`, `STATUS`, `PATH` for all three sessions; exit 0.

- **Given** `ssq list-sessions --json`,  
  **When** the command runs,  
  **Then** stdout is a valid JSON array where each element has `"status": "SESSION_STATUS_ACTIVE"` (string name); exit 0.

- **Given** `ssq list-sessions --status active`,  
  **When** the command runs,  
  **Then** `ListSessionsRequest.status` is set to `SESSION_STATUS_ACTIVE` and only active sessions are returned.

- **Given** `ssq list-sessions --search "auth"`,  
  **When** the command runs,  
  **Then** `ListSessionsRequest.search_query` is set to `"auth"`.

- **Given** no sessions exist,  
  **When** `ssq list-sessions` is run,  
  **Then** stdout prints `"No sessions found."` and exits 0.

**Flags**: `--status` (string: `active`/`paused`/`all`, default `all`), `--category`, `--search`, `--json`, `--server`/`-s`.

**Status flag mapping**: `"active"` → `SESSION_STATUS_ACTIVE`, `"paused"` → `SESSION_STATUS_PAUSED`, `"all"` → omit the optional field.

**Human output** (tabwriter, 2-space padding):
```
ID                                    TITLE       STATUS                  PATH
3f2a1bc4-dead-beef-cafe-000000000001  fix-auth    SESSION_STATUS_ACTIVE   /home/user/code/myrepo
```

---

### Story 3.3 — `ssq steer` subcommand

**Files**: `cmd/commands/steer.go` (new), `main.go` (add `commands.NewSteerCmd()`)
**Cost class**: S

**Acceptance criteria (Given-When-Then):**

- **Given** a running session in autonomous mode,  
  **When** `ssq steer 3f2a1bc4-dead-beef-cafe-000000000001 --message "Focus on the auth module"`,  
  **Then** `UpdateSession` is called with `id="3f2a1bc4..."`, `steer_message="Focus on the auth module"`, exit 0.

- **Given** `ssq steer 3f2a1bc4 --message "Steer text"` succeeds,  
  **When** checking stdout,  
  **Then** it prints `"ok"` (non-JSON default) or valid JSON `UpdateSessionResponse` (`--json`).

- **Given** the session is a hidden background session (server returns `CodeFailedPrecondition`),  
  **When** `ssq steer` is run,  
  **Then** stderr shows the verbatim server message and exit code is non-zero.

- **Given** `--message` is omitted,  
  **When** the command is invoked,  
  **Then** cobra prints a usage error and exits non-zero.

- **Given** the session ID positional arg is omitted,  
  **When** the command is invoked,  
  **Then** cobra prints `"requires exactly 1 arg(s), only received 0"` and exits non-zero.

**Flags**: `--message`/`-m` (required string), `--json`, `--server`/`-s`.

**`--help` note** (verbatim in `Long` field): `"Steering injects the message into the session's input. On an autonomous session (autonomous_mode=true) the message enters the driver's command queue. On a non-autonomous session it is typed directly into the PTY pane as if the user pressed Enter — the agent sees it immediately but the driver does not queue it. Hidden backlog sessions that are not in review state return an error; surface the server error message verbatim."`

**P1 pre-mortem fix:** Story 3.3 must verify actual server behavior for non-autonomous sessions. Add an acceptance criterion:

- **Given** a session in non-autonomous mode (autonomous_mode=false),  
  **When** `ssq steer <session-id> --message "hello"` is run,  
  **Then** the command exits 0 and prints `"ok"`, confirming the server accepted the PTY injection — the help text must accurately document this PTY-injection behavior so callers are not surprised.

---

### Story 3.4 — `ssq get-diff` subcommand

**Files**: `cmd/commands/get_diff.go` (new), `main.go` (add `commands.NewGetDiffCmd()`)
**Cost class**: S

**Acceptance criteria (Given-When-Then):**

- **Given** a session with a modified file,  
  **When** `ssq get-diff 3f2a1bc4-dead-beef-cafe-000000000001`,  
  **Then** stdout is the raw unified diff from `DiffStats.content`; exit 0.

- **Given** `ssq get-diff 3f2a1bc4 --json`,  
  **When** the command runs,  
  **Then** stdout is valid JSON `GetSessionDiffResponse` with `"diff_stats"` containing `"added"`, `"removed"`, `"content"` fields; exit 0.

- **Given** the session has no changes (`DiffStats.content` is empty),  
  **When** `ssq get-diff` is run without `--json`,  
  **Then** stdout is empty and exit code is 0 (suitable for `test -z "$(ssq get-diff ...)"` guards).

- **Given** the session does not exist,  
  **When** `ssq get-diff <unknown-id>`,  
  **Then** stderr prints the not-found error and exit code is non-zero.

**Flags**: `--json`, `--server`/`-s`.

**Default output**: `DiffStats.content` printed verbatim to stdout. If content is empty, no output. Suitable for piping to `patch`, `delta`, etc.

**Stats summary line** (printed to stderr when `DiffStats.content` is non-empty and stdout is a TTY only, never when piped): `"# +N -M lines"`. Using stderr prevents contaminating the diff stream.

---

### Story 3.5 — Wire all new commands in `main.go`

**Files**: `main.go` (modified — `init()` only)
**Cost class**: XS

Replace `commands.GetSessionCmd` reference and add four new commands:

```go
// main.go init()
rootCmd.AddCommand(commands.NewGetSessionCmd())       // replaces commands.GetSessionCmd (Phase 2)
rootCmd.AddCommand(commands.NewCreateSessionCmd())   // Phase 3 Story 3.1
rootCmd.AddCommand(commands.NewListSessionsCmd())    // Phase 3 Story 3.2
rootCmd.AddCommand(commands.NewSteerCmd())           // Phase 3 Story 3.3
rootCmd.AddCommand(commands.NewGetDiffCmd())         // Phase 3 Story 3.4
```

**Acceptance criteria:**

- **Given** `ssq --help`,  
  **When** the command runs,  
  **Then** stdout lists `create-session`, `list-sessions`, `steer`, `get-diff`, and `get-session` in the available commands.

- **Given** `ssq create-session --help`,  
  **When** the command runs,  
  **Then** stdout contains `--server`, `--title`, `--path`, `--json` flags.

---

## Phase 4: Documentation

**Epic 4 — Documentation**

### Story 4.1 — Update `README.md` CLI reference section

**Files**: `README.md` (modified)
**Cost class**: XS

**Acceptance criteria:**

- **Given** the README's CLI reference section,  
  **When** reviewed,  
  **Then** it includes usage examples for each new subcommand with the correct default server URL (`localhost:8543`).

- Examples cover: `create-session`, `list-sessions`, `steer`, `get-diff`, corrected `get-session`.

- Each example uses the `--server` flag to show how to target a non-default instance.

- **P1 pre-mortem fix:** Every CI/automation example in the README MUST show `--wait` on `create-session`. A callout box must warn: "In CI pipelines, always use `--wait` — without it the session is still initializing when subsequent `steer` or `get-diff` calls run." The `--help` Long description for `create-session` must include a `CI usage note` about `--wait`.

**Minimum example set:**

```bash
# List all sessions on the local server
ssq list-sessions

# List active sessions only, JSON output
ssq list-sessions --status active --json

# Create a new session
ssq create-session --title "fix-login-bug" --path ~/code/myapp --branch fix/login

# Create and wait for it to be active
ssq create-session --title "fix-login-bug" --path ~/code/myapp --wait

# Steer an in-flight autonomous session
ssq steer <session-id> --message "Ignore the database layer, focus only on the API"

# Get the current diff for a session
ssq get-diff <session-id>

# Get diff as JSON
ssq get-diff <session-id> --json

# Target a non-default server
ssq list-sessions --server http://myremote:8543/api
# or
SSQ_SERVER=http://myremote:8543/api ssq list-sessions
```

---

## Story / Task Summary

| Phase | Epic | Story | Cost | Files |
|---|---|---|---|---|
| 1 | E1 | 1.1 — Shared client infrastructure | S | `cmd/commands/client.go` (new) |
| 2 | E2 | 2.1 — Fix `get-session` port bug | XS | `cmd/commands/get_session.go` |
| 3 | E3 | 3.1 — `create-session` command | M | `cmd/commands/create_session.go` (new) |
| 3 | E3 | 3.2 — `list-sessions` command | S | `cmd/commands/list_sessions.go` (new) |
| 3 | E3 | 3.3 — `steer` command | S | `cmd/commands/steer.go` (new) |
| 3 | E3 | 3.4 — `get-diff` command | S | `cmd/commands/get_diff.go` (new) |
| 3 | E3 | 3.5 — Wire commands in `main.go` | XS | `main.go` |
| 4 | E4 | 4.1 — README CLI reference | XS | `README.md` |

**Totals**: 4 epics, 8 stories, 8 tasks (one task per story — each story is self-contained). 5 new files, 2 modified files (`cmd/commands/get_session.go`, `main.go`), 1 doc update.
