# Architecture Research: Per-Session Auto-Approve ("Yolo") Mode

Phase 2 research for `project_plans/session-yolo-mode/requirements.md`. No prior
`code-hotspot-analysis` run exists for this area; this is fresh code reading.

## 1. Where the tmux launch command is assembled

`session/instance_tmux.go`. Entry point is `Instance.buildLaunchCommand` (session/instance_tmux.go:105-119):

```go
func (i *Instance) buildLaunchCommand(claudeSessionID string) string {
	var cmd string
	switch p := classifyProgram(i.Program).(type) {
	case claudeProgram:
		cmd = i.buildClaudeCommand(p.base, claudeSessionID)
	case plainProgram:
		cmd = p.cmd
	default:
		panic(fmt.Sprintf("unknown programKind %T", p))
	}
	for _, f := range strings.Fields(i.CLIFlags) {
		cmd = cmd + " " + shellQuote(f)
	}
	return cmd
}
```

`classifyProgram` (instance_tmux.go:63-68) parses `i.Program` (the raw command
string, e.g. `"claude"`, `"claude --model sonnet"`, `"aider --model ollama_chat/..."`)
into a sealed sum type: `claudeProgram{base}` if `isClaude()` matches (basename of
any whitespace token == `"claude"`), otherwise `plainProgram{cmd}` — **which today
means "aider and everything else" get zero flag-injection treatment**; the raw
program string is returned unchanged and only the *user-typed* `CLIFlags` field
(free-text, not structured) gets appended.

`buildClaudeCommand` (instance_tmux.go:133-166) is where flags get appended for
Claude specifically — this is the existing precedent block:

```go
if i.AutoYes {
    parts = append(parts, "--permission-mode", PermissionModeBypassPermissions)
}
```

`initTmuxSession` (instance_tmux.go:249-279) calls `buildLaunchCommand` and hands
the result straight to `tmux.NewTmuxSessionWithPrefix`/`NewTmuxSessionWithServerSocket`.
This is the single injection point — anything that needs to alter the spawned
command line goes through `buildLaunchCommand`/`buildClaudeCommand`, not through
`initTmuxSession` or the tmux package itself.

## 2. Existing "which agent is this" concept

**Weaker than a first-class detection layer — it's inline string classification,
Claude-only.** No enum, no `AgentKind` string field, no generalized detector for
"claude vs aider vs other" at the command-builder level.

- `classifyProgram(program string) programKind` (instance_tmux.go:63-68) — a sealed
  2-way sum type (`claudeProgram` / `plainProgram`) computed fresh from `i.Program`
  on every `buildLaunchCommand` call. It only distinguishes "is claude" from
  "is not claude" — it has no `aiderProgram` case.
- `isClaude(program string)` (instance_tmux.go:74-81) — the actual detection
  logic: splits `i.Program` on whitespace, checks each token's `filepath.Base()`
  against the literal string `"claude"`.
- A **separate**, unrelated "Aider" concept already exists elsewhere in the
  codebase but is not wired to command building at all:
  - `session/tmux/tmux.go:32` — `const ProgramAider = "aider"`, used only for
    prompt-detection heuristics (`tmux.go:1462`, `strings.HasPrefix(t.program, ProgramAider)`).
  - `session/detection/binaries/aider.go` — `AiderDetector` (dtypes.BinaryDetector)
    used by the status/output-pattern detection subsystem (`session/detection/registry.go:10`),
    not by session launch.
  - `server/mcp/tools_lifecycle.go:36`, `tools_github.go:91-92` — MCP tool schemas
    accept `program: "claude" | "aider"` as an enum for session creation, but this
    is just the raw `Program` string passed straight through to `i.Program`; no
    structured "AgentKind" value is derived or stored.

**Implication for the plan phase**: per-agent flag mapping can be a simple
`map[string]string` keyed by the *same string-matching primitive* `isClaude`/
`classifyProgram` already uses (program binary basename), extended with an
`isAider` check of the same shape. No new detection subsystem is required —
`classifyProgram` needs a third case (`aiderProgram`) or, more simply per §3
below, a lookup function that takes `i.Program` and returns the flag to append.
Do **not** reach for `session/detection/binaries.AiderDetector` — that's a
different subsystem (pane-content pattern matching for status detection), not
program-string classification, and pulling it in would be a layering violation
for what is a one-line lookup.

## 3. Recommended design for per-agent flag lookup

Per `.claude/rules/interface-pollution-checklist.md`, avoid a `FlagProvider`
interface with `ClaudeFlagProvider`/`AiderFlagProvider` implementations — that's
exactly the speculative-interface smell (#1) the rule calls out: two
implementations known in advance, no runtime polymorphism needed, one call site.

Simplest concrete design, colocated in `session/instance_tmux.go` next to
`classifyProgram`/`isClaude`:

```go
// yoloFlagByAgent maps each supported agent's basename to the CLI flag that
// bypasses its tool/permission-approval prompts. Extend this map (not a new
// interface) when a new agent needs auto-approve support.
var yoloFlagByAgent = map[string]string{
	"claude": "--dangerously-skip-permissions",
	"aider":  "--yes",
}

// yoloFlagFor returns the auto-approve flag for the given program string's
// detected agent binary, or "" if the agent has no known flag (unsupported
// agent — auto_approve is a no-op rather than an error, since Scope explicitly
// limits this pass to Claude Code and Aider).
func yoloFlagFor(program string) string {
	for _, token := range strings.Fields(program) {
		if flag, ok := yoloFlagByAgent[filepath.Base(token)]; ok {
			return flag
		}
	}
	return ""
}
```

Injection sites:
- `buildClaudeCommand` gains `if i.AutoApprove { parts = append(parts, "--dangerously-skip-permissions") }`
  alongside the existing `i.AutoYes` block — OR, to stay agent-generic in one
  place rather than duplicating per-branch, `buildLaunchCommand` calls
  `yoloFlagFor(i.Program)` once and appends it after the per-agent command is
  built, for both the `claudeProgram` and `plainProgram` branches. The latter is
  less duplicative and doesn't require `plainProgram`'s branch (today just
  `p.cmd` verbatim) to grow agent-specific logic.
- Reuses `shellQuote`/`strings.Fields` machinery already in this file — no new
  parsing primitives needed.

This keeps the "detect agent → look up flag" logic to ~15 lines, matches the
existing `isClaude` string-matching style exactly, and needs no mocking/testing
seam beyond a table-driven test over `yoloFlagFor`.

## 4. Data flow — layer-by-layer status

| Layer | Exists today? | Evidence |
|---|---|---|
| Proto `CreateSessionRequest.auto_approve` | **Missing** — needs adding | `proto/session/v1/session.proto:472-528`; highest used field number in `CreateSessionRequest` is `27` (`alias_name`), so next field is **28**. Field `14` is `reserved "one_off"` (line 519) — do not reuse. |
| Proto `Session.auto_approve` (read-back) | **Missing** — needs adding | `proto/session/v1/types.proto` `message Session` highest field is `71` (`workspace_key`); next is **72**. |
| Proto `UpdateSessionRequest.auto_approve` | **Missing** — needs adding | `session.proto` `UpdateSessionRequest` fields go 1-11 (`steer_message`); next is **12**. Follow the `optional bool` pattern used by `autonomous_mode = 10`/`rate_limit_enabled = 8` (optional-bool-as-explicit-toggle, not implicit "provided" via zero value). |
| ent `Session.AutoApprove` column | **Missing** — needs adding | Template: `session/ent/schema/session.go:46`, `field.Bool("auto_yes")`. Regenerate with `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` per `.claude/rules/ent-schema-generation.md`. |
| In-memory `session.Instance.AutoApprove` (bool field) | **Missing** — needs adding | Template: `session/instance.go:128-129` (struct field) + `session/instance.go:465-466` (`InstanceOptions.AutoYes`) + `instance.go:604` (wired in constructor). |
| Setter (`Instance.SetAutoApprove`) | **Missing** — needs adding | Template: `session/instance_actor_setters.go:317-329`, `SetAutoYes`. |
| `ToInstanceData()` / ent Create+Update `SetAutoApprove(...)` round-trip | **Missing** — needs adding | Template: `session/instance_serialization.go:62` (`AutoYes: snap.AutoYes`) and `session/ent_repository.go:155,367` (`SetAutoYes(data.AutoYes)` in both Create and Update paths) + `ent_repository.go:1047` (read-back `AutoYes: sess.AutoYes` when loading from ent into `InstanceData`). |
| Command builder reads it | **Missing** — needs adding | See §3 above; injection point is `buildClaudeCommand`/`buildLaunchCommand` in `session/instance_tmux.go`, exactly where `i.AutoYes` is already read (instance_tmux.go:154-156). |
| Service-layer create wiring (`server/services/session_service.go`) | **Missing** — needs adding | Template: `one_off` handling at session_service.go ~510-615 (per requirements.md) and `AutonomousMode` wiring at line 1503 (`AutonomousMode: req.Msg.AutonomousMode` passed into instance options at construction). |
| `UpdateSession` toggle-after-creation | **Missing** — needs adding | Template: the `autonomous_mode` block at session_service.go:1802-1816, or the simpler `rate_limit_enabled` block at 1789-1800 (no live side-effect, just `instance.Set...()` + append to `updatedFields`) — auto_approve's "Should Have" toggle (requirements.md line 38-40) explicitly does *not* need a live in-place effect on the running process, so the `rate_limit_enabled` shape (persist-only, no live driver start/stop) is the closer template than `autonomous_mode`'s (which also starts/stops a live driver). |

**Naming collision risk worth flagging to the plan phase**: the existing
`auto_yes` proto field/Go field is documented as "Auto-approve prompts without
user interaction" (session.proto:496) and today drives *both* a `TapEnter`
auto-press-enter behavior (session/instance.go:128, "automatically press enter
when prompted") *and* `--permission-mode bypassPermissions` for Claude only
(instance_tmux.go:154-156) — which is conceptually adjacent to but distinct from
the new `--dangerously-skip-permissions` / `--yes` "skip prompts entirely"
semantics requirements.md asks for. `auto_approve` and `auto_yes` as sibling
field names on the same struct/proto message is a foot-gun for future readers
(and for an LLM autocompleting the wrong one) — the plan phase should pick a
name that's clearly distinguishable in code (e.g. `AutoApprove`/`auto_approve`
is probably fine since it's already what requirements.md specifies, but the
plan doc should call out the existing `AutoYes` neighbor explicitly in a comment
at the new field's declaration, the way `session/instance.go:128` documents its
own field, so nobody merges the two later).

## 5. Does `UpdateSession`'s toggle need to persist to ent, not just in-memory?

**Yes**, and the existing pattern already does this uniformly — there is no
"in-memory only" mutable field exempted from ent persistence. Every field
`UpdateSession` mutates (`title`, `category`, `tags`, `program`, `working_dir`,
`rate_limit_enabled`, `autonomous_mode`) is written to the `instance` struct via
a `Set*` method, then **all** mutated instances flow through one shared call:

```go
// server/services/session_service.go:1874
if err := s.storage.SaveInstances(instances); err != nil { ... }
```

`Storage.SaveInstances` (session/storage.go:257-282) is not a JSON-file cache —
it upserts into the ent repository directly (`s.repo.Update(ctx, data)`, falling
back to `s.repo.Create` if not found), where `data := inst.ToInstanceData()`
serializes the in-memory struct (including e.g. `AutoYes: data.AutoYes` per
instance_serialization.go:62) into the ent write. So: **`auto_approve` needs
the same `SetAutoApprove` setter + inclusion in `ToInstanceData()` + the
corresponding ent `Session.SetAutoApprove(...)` call added to both the Create
and Update branches in `session/ent_repository.go`** (mirroring lines 155 and
367 for `auto_yes`) for the toggle to survive a service restart. Skipping any
one of those three would make the toggle appear to work (UI reflects new state
immediately from the in-memory instance) but silently revert on next process
restart/reload — exactly the class of bug `.claude/CLAUDE.md`'s "read a mutation
back before claiming it happened" rule warns about.

## 6. Simple CRUD vs. Event-Command-Policy table

**Simple CRUD — confirmed, table not warranted.** Rationale:

- Single actor: the session owner (only stakeholder per requirements.md — "User
  (Self) — primary and only stakeholder"). No multi-party workflow, no approval
  chain, no cross-session coordination.
- Single state transition shape: a plain bool flipped at creation time or via one
  `UpdateSession` call, with no side effects beyond "append this string to the
  next-spawned command line." Contrast with `autonomous_mode`, which *does*
  warrant more than pure CRUD because toggling it starts/stops a live
  `AutonomousDriver` (session_service.go:1809-1814) — a real side-effecting state
  machine. `auto_approve` explicitly has no such live effect (requirements.md
  "Should Have": "takes effect on next launch/restart... not a live in-place
  flag swap").
- No policy/business-rule branching: the "correct flag" is a pure function of
  `(auto_approve bool, detected agent string) -> flag string`, not a decision
  that depends on accumulated events, other sessions' state, or external
  approval. `yoloFlagFor` in §3 is the entire "policy."
- The requirements.md Out-of-Scope section explicitly rules out the shapes that
  would justify heavier modeling (no new SessionType, no approval-logic change,
  no retroactive live-process mutation).

An Event-Command-Policy table would be justified if, e.g., auto-approve mode
had to be *revoked* by a different actor mid-session, or required an audit
trail of who enabled it and why, or gated on org-level policy — none of which
requirements.md asks for.
