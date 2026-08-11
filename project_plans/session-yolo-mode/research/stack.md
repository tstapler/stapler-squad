# Stack Research: Session Yolo/Auto-Approve Mode

## 0. CRITICAL FINDING — this mostly already exists as `auto_yes`

Before the touchpoint inventory below: the codebase already has an end-to-end `auto_yes`
boolean wired from proto → ent → Go handler → tmux command → frontend Omnibar toggle. It is
**not** a new concept to build from scratch — it's the closest possible prior art, and the
plan phase should decide explicitly whether this feature **extends `auto_yes`** or
**introduces a distinct `auto_approve` field** that composes with it. Gaps vs. the
requirements.md success criteria, as currently implemented:

| Requirement | `auto_yes` today |
|---|---|
| Toggle at creation | ✅ `OmnibarCreationPanel.tsx:802-810`, labeled "Auto-approve prompts (experimental)" |
| Correct per-agent flag | ⚠️ Partial — only affects Claude programs. `buildLaunchCommand` (`session/instance_tmux.go:105`) only injects a flag inside `buildClaudeCommand`; non-Claude (`plainProgram`, e.g. Aider) programs get **no flag at all** for `auto_yes` today. |
| Exact flag | ⚠️ Injects `--permission-mode bypassPermissions` (`session/instance_tmux.go:154-156`), not `--dangerously-skip-permissions` as named in the maki precedent / backlog item. Functionally similar (bypasses tool-approval) but a different flag name — worth confirming with the user/backlog author whether `bypassPermissions` mode is considered equivalent or whether `--dangerously-skip-permissions` specifically is wanted. |
| Session card badge | ❌ No `auto_yes`-driven badge found in `SessionCard.tsx` (badges present: External, GitHub, ReviewQueue, Status, Memory, Autonomous, Workflow — no Auto/Yolo badge). |
| Persisted, readable via API | ✅ ent field `auto_yes` (`session/ent/schema/session.go:46-47`), proto field 8, `session/storage.go:30`, `session/session.go:30`. |
| Toggle after creation | ❌ No `SetAutoYes`-triggered RPC call found wired to a post-creation session action/UI control (the Go setter `Instance.SetAutoYes` exists at `session/instance_actor_setters.go:329` but appears intended for `daemon.go` automation, not a user-facing "toggle this session" action). |
| Aider support | ❌ No Aider-specific detection anywhere in `session/` (`grep -rl Aider session/*.go` → no hits). Only `isClaude`/`classifyProgram` exist; there is no `programKind` case for Aider, so an Aider session's launch command is passed through unchanged (`plainProgram`) regardless of `auto_yes`. |

**Implication for planning**: the "Must Have" scope items (badge, per-agent flag mapping
including Aider, correct flag choice) are the real net-new work. The creation-time toggle,
persistence, and proto/ent plumbing are already done for Claude — the plan should treat this
as *extending* `classifyProgram`/`buildLaunchCommand` to add an Aider case and a badge, not
re-adding a `CreateSessionRequest.auto_yes`-equivalent field. Recommend explicitly deciding in
plan.md whether to reuse `auto_yes` (rename/re-label in UI to "Auto/Yolo" per requirements) or
add a second field — reusing avoids proto/ent churn and a second must-be-kept-in-sync toggle.

---

## 1. `proto/session/v1/session.proto` — `CreateSessionRequest`

Message starts at line 472. Full field list with numbers:

```protobuf
message CreateSessionRequest {
  string title = 1;
  string path = 2;
  string working_dir = 3;
  string branch = 4;
  string program = 5;
  string category = 6;
  string prompt = 7;
  bool auto_yes = 8;                    // <-- existing "auto-approve" flag
  string existing_worktree = 9;
  string resume_id = 10;
  string profile = 11;
  bool skip_defaults = 12;
  SessionType session_type = 13;
  reserved 14; reserved "one_off";      // deprecated, use session_type = SESSION_TYPE_ONE_OFF
  string initial_prompt = 15;
  bool one_shot = 16;
  string project_id = 17;
  // ... continues past line 531 (not fully enumerated here; next free number is >= 18)
```

Need to read past line 531 to get the true next-available field number — do this in plan
phase with `grep -n "= [0-9]*;" proto/session/v1/session.proto | sed -n '/CreateSessionRequest/,/^}/p'`
or just open the file at that message and scan to its closing `}`. (Not exhaustively
enumerated here to keep this doc from going stale if unrelated fields are added between now
and the plan phase — re-check field numbers immediately before writing `plan.md`.)

`UpdateSessionRequest` (relevant for the "toggle after creation" Should-Have) already has a
similar optional-bool pattern for a comparable per-session flag:
```protobuf
// line ~612
optional bool autonomous_mode = 10;
```
This is the pattern to follow if `auto_approve`/`auto_yes` needs a post-creation update path:
add `optional bool auto_yes = N;` (or `auto_approve`) to `UpdateSessionRequest`, mirroring how
`autonomous_mode` does it (see `server/services/session_service.go:1804-1805` for the handler
pattern: `if req.Msg.AutonomousMode != nil && *req.Msg.AutonomousMode != instance.AutonomousMode { ... }`).

## 2. `proto/session/v1/types.proto` — `SessionType` enum

```protobuf
// line 354
enum SessionType {
  SESSION_TYPE_UNSPECIFIED = 0;
  SESSION_TYPE_DIRECTORY = 1;
  SESSION_TYPE_NEW_WORKTREE = 2;
  SESSION_TYPE_EXISTING_WORKTREE = 3;
  SESSION_TYPE_NEW_PROJECT = 4;
  SESSION_TYPE_ONE_OFF = 5;
}
```

Confirmed: `autonomous_mode` is **not** in this enum — it's a separate `bool` field,
declared at `proto/session/v1/types.proto:203` (`bool autonomous_mode = 60;`, on whatever
message that block belongs to — likely a session state/snapshot message) and again as a
create-request field at `proto/session/v1/session.proto:557` (`bool autonomous_mode = 23;`)
and as the `optional bool` update-request field at `session.proto:612` (`optional bool
autonomous_mode = 10;`). This is exactly the precedent the requirements.md `auto_approve`
field should follow (bool flag, not a new `SessionType` value) — and it's also exactly the
shape `auto_yes` already has, reinforcing finding #0.

## 3. `session/ent/schema/session.go` — full field list

File: `session/ent/schema/session.go` (180 lines). Relevant existing bool fields (pattern to
copy for a new field, if the plan decides a second field is needed):

```go
field.Bool("auto_yes").
    Default(false),
field.Bool("autonomous_mode").
    Default(false).
    Comment("Crew autonomy mode — when true, the Fixer injects correction prompts without user confirmation."),
field.Bool("is_expanded").
    Default(true),
field.Bool("one_shot").
    Default(false).
    Comment("When true, runs claude in -p mode; session exits after task completes."),
field.Bool("hidden").
    Default(false).
    Comment("..."),
```

Full field list (name: type) for reference: title(String,Unique,NotEmpty), uuid(String,Optional,Default("")),
path(String,NotEmpty), working_dir(String,Optional), branch(String,Optional), status(Int),
height(Int,Optional), width(Int,Optional), created_at(Time), updated_at(Time),
**auto_yes(Bool,Default false)**, **autonomous_mode(Bool,Default false)**, prompt(String,Optional),
program(String,NotEmpty), existing_worktree(String,Optional), category(String,Optional),
is_expanded(Bool,Default true), session_type(String,Optional), tmux_prefix(String,Optional),
last_terminal_update/last_meaningful_output/last_added_to_queue/last_viewed/last_acknowledged
(Time,Optional,Nillable), last_output_signature(String,Optional), mcp_server_url(String,Optional),
initial_prompt(String,Optional), one_shot(Bool,Default false), last_user_response/
processing_grace_until/last_prompt_detected(Time,Optional,Nillable), last_prompt_signature(String,Optional),
hidden(Bool,Default false), pause_reason(String,Optional), workflow_id(String,Optional),
archived_at(Time,Optional,Nillable), github_pr_url/github_pr_number/github_owner/github_repo,
session_artifacts(String,Optional,Default "").

**If a new `auto_approve` field is needed** (as distinct from `auto_yes`), the pattern is:
```go
field.Bool("auto_approve").
    Default(false).
    Comment("Bypasses tool-approval prompts for the launched agent (Claude: --permission-mode bypassPermissions; Aider: --yes). Opt-in only."),
```
placed alongside `auto_yes`/`autonomous_mode`, then regenerate with the mandatory
`--feature sql/upsert` flag per `.claude/rules/ent-schema-generation.md`.

## 4. Where the tmux launch command is built — exact injection point

File: `session/instance_tmux.go`.

- **`buildLaunchCommand`** (line 105) — top-level entry point, called from `initTmuxSession`
  (line 258) and twice from `session/instance.go` (lines 1451, 1573, both setting
  `i.LaunchCommand`). It classifies `i.Program` via `classifyProgram` (line 63) into a sealed
  `programKind` sum type: `claudeProgram{base}` or `plainProgram{cmd}` (lines 53-59).
- **`classifyProgram`** (line 63) delegates to **`isClaude`** (line 74), which checks
  `filepath.Base(token) == "claude"` over `strings.Fields(program)` — i.e., it's a substring/
  basename match on the program string, not a stored enum. **This is the closest existing
  "detected agent" concept** (see Q5 below) but it currently only distinguishes
  Claude-vs-everything-else; there is no Aider case.
- **`buildClaudeCommand`** (lines 133-166) is where all Claude-specific flags get appended,
  including the existing yolo-adjacent flag:
  ```go
  // session/instance_tmux.go:154-156
  if i.AutoYes {
      parts = append(parts, "--permission-mode", PermissionModeBypassPermissions)
  }
  ```
  `PermissionModeBypassPermissions = "bypassPermissions"` is defined at `session/instance.go:429`.
- Non-Claude programs go through the `plainProgram` branch (line 110-111 in
  `buildLaunchCommand`): `cmd = p.cmd` — the raw program string, completely unmodified. **This
  is the exact gap**: an Aider (or other) session with `auto_yes`/`auto_approve` set today gets
  no flag injected at all.

**This is where the new per-agent flag logic must go**: extend `programKind` with an
`aiderProgram` case (or a more general `agentKind` concept), add an `isAider` detector
analogous to `isClaude`, and add a `buildAiderCommand` (or a shared "append yolo flag by agent
kind" helper) that appends the Aider equivalent (`--yes`/similar — confirm exact flag name in
Aider's own CLI docs during planning, not assumed here) when the flag is set.

## 5. Existing "detected agent" concept

There is **no enum or stored field** that distinguishes Claude vs Aider vs other agents.
The only existing concept is the runtime string-matching function:

```go
// session/instance_tmux.go:74
func isClaude(program string) bool {
    for _, token := range strings.Fields(program) {
        if filepath.Base(token) == "claude" {
            return true
        }
    }
    return false
}
```

consumed by `classifyProgram` (line 63) to produce the sealed `programKind` sum type
(`claudeProgram` / `plainProgram`, lines 53-59). No `isAider` or third `programKind` variant
exists anywhere in `session/*.go` (confirmed via `grep -rn "Aider\|isAider" session/*.go` →
no matches outside this research). The plan must add the Aider-detection case here, following
the same "parse once at the boundary, trust internally" sealed-sum-type pattern the file's own
comment (line 51-52) documents as the intended idiom — i.e., don't add ad-hoc `isAider(...)`
guards scattered through `buildLaunchCommand`; add a third `programKind` variant instead.

## 6. `go.mod` — versions

```
module github.com/tstapler/stapler-squad
go 1.26.3

connectrpc.com/connect v1.19.0
entgo.io/ent v0.14.5
github.com/go-git/go-git/v5 v5.14.0
```

No version guessing needed for the plan — use these exact versions when referencing API
shapes (e.g. ent v0.14.5 codegen flags, connect v1.19.0 handler signatures).

---

## Open questions to resolve in plan.md (flagging now, not deciding here)

1. **Reuse `auto_yes` or add `auto_approve`?** Reusing avoids a second proto/ent field and a
   confusing pair of near-duplicate toggles: relabel the existing Omnibar checkbox from
   "Auto-approve prompts (experimental)" to match whatever the requirements/backlog item wants
   surfaced, and extend the existing field's *behavior* (Aider support, badge, post-creation
   toggle) rather than adding a parallel field. If product intent is genuinely for
   `auto_approve` to be a distinct, more aggressive setting than the existing `bypassPermissions`
   mode, a second field is justified — but that's a product decision, not a stack constraint.
2. **Exact flag string per agent**: confirm whether Claude should switch from
   `--permission-mode bypassPermissions` to literally `--dangerously-skip-permissions` (per the
   backlog item's wording and the maki precedent), and confirm Aider's actual bypass flag name
   (not verified against Aider's CLI in this research pass — plan/implementation phase should
   check Aider's `--help` or docs directly rather than assume `--yes`).
