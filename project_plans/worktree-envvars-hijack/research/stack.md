# Research: Stack-level mechanics of the cross-session conversation bleed (half 2)

Scope: how the `claude` CLI / tmux stack could make a freshly-created session's
tmux pane show another live session's conversation content. Does not attempt
to root-cause half 1 (why `GetEffectiveRootDir()`/`resolvedPath` might resolve
to the bare repo path instead of a real worktree) — that is Server/App
territory — but this research shows the two halves plausibly share one root
cause (see "Most plausible mechanism" below).

## 1. Does stapler-squad ever omit `--resume` and rely on the CLI's own cwd-based continuation?

**VERIFIED — no.** `buildClaudeCommand` (`session/instance_tmux.go:364-371`) only
appends `--resume <id>` when it is handed a non-empty `claudeSessionID`:

```go
func (i *Instance) buildClaudeCommand(base, claudeSessionID string) string {
	parts := []string{base}
	if claudeSessionID != "" {
		parts = append(parts, "--resume", shellQuote(claudeSessionID))
	}
	...
```

That id comes from `initTmuxSession()` (`session/instance_tmux.go:686-690`):
`claudeSessionID = i.claudeSession.ConversationUUID` if `i.claudeSession != nil`,
else `""`. For a genuine first-time `CreateSession`, `i.claudeSession` is nil
unless one of two things populated it first:

- `opts.ResumeId` was set on the request (`session/instance.go:1178-1187`,
  "Handle ResumeId - set up claudeSession so the --resume flag gets added on
  Start()"). The repro's Request B did not report a `resumeId` field.
- `tryExtractConversationUUID()` ran and found a JSONL on disk (see §4 below).

Neither `--continue` nor `-c` (as a Claude Code flag, not the many unrelated
`sh -c`/`git -c` hits) appears anywhere in `session/*.go` — grepped the whole
tree for the literals `"--resume"`, `"-c"`, `"--continue"`, `"--session-id"`
outside test files; every non-`--resume` hit is `sh -c`/`git -c`/argparse
tables, unrelated to Claude session resumption.

`recoverConversationBeforeLaunch` (`session/instance.go:1421-1426`) — the one
function whose whole job is pre-seeding `claudeSessionID` before
`initTmuxSession()` reads it — explicitly no-ops when `firstTimeSetup` is
true:

```go
func (i *Instance) recoverConversationBeforeLaunch(firstTimeSetup bool) {
	if firstTimeSetup || i.pm().IsAlive() || i.HasClaudeSession() {
		return
	}
	i.tryExtractConversationUUID()
}
```

Confirming the triager's baseline. **Stapler-squad's own first-launch path
never embeds `--resume` for a fresh `CreateSession`, and never relies on the
CLI's bare cwd-based continuation either** — it always passes an explicit
flag or nothing.

## 2. Documented `claude` CLI behavior with no flags (external, UNVERIFIED against this exact install but consistent across multiple independent sources)

Per Anthropic's own docs and third-party guides (see Sources), running plain
`claude` with **no** `--resume`/`--continue`/`-c` **always starts a brand-new
conversation**, regardless of what `~/.claude/projects/<encoded-path>/`
already contains for that directory. Auto-attaching to the most recent
session in the cwd requires the explicit `--continue`/`-c` flag; a specific
prior conversation requires `--resume <uuid>` or the interactive picker
(`--resume` with no id).

This means: **if** the observed pane genuinely reflects the actual `claude`
process's own terminal output (which is what a tmux pane always is — the raw
stdout/stdin of whatever process occupies it, not something stapler-squad
re-renders), then the CLI itself resuming purely from a shared cwd, with
*no* `--resume`/`--continue` flag on the command line, is **not a documented
CLI behavior** and should not be assumed as the mechanism. Something must
have put an explicit resume flag (or attached the wrong process/pane) into
play. This substantially narrows the hunt back onto stapler-squad's own code
(§4) or a stapler-squad-level pane/process mixup outside this research's
scope (see "boundary" note at the end).

## 3. Does `ANTHROPIC_BASE_URL` / any `envVars` change `claude`'s argv or resume behavior?

**VERIFIED — no, it's pure passthrough**, and it's structurally incapable of
reaching the `--resume` logic:

- `resolveExtraEnvVars()` (`session/instance_tmux.go:585-599`) merges
  program-level env (`config.ResolveProgramConfig`, only when the program ID
  `IsCustom`) with instance-level `snap.EnvVars` (request-supplied), instance
  wins on key collision. It only returns a `map[string]string` — never reads
  or affects `SessionType`, path resolution, or `claudeSessionID`.
- `buildExtraEnv()` (`instance_tmux.go:604-614`) turns that map into
  `KEY=VALUE` strings injected via tmux's own `-e` flags at
  `new-session` time (`session/tmux/tmux_session_start.go:214-221`,
  `newSessionArgs = append(newSessionArgs, "-e", kv)`) — this is tmux's
  native per-session environment feature; nothing here touches the launch
  command string.
- `claudeSettingsEnvOverrideArgs()` (`instance_tmux.go:631-642`) additionally
  serializes the same map into a `--settings '{"env": {...}}'` JSON blob, so
  the value also wins over a global `~/.claude/settings.json`'s `env` block
  (regression test for issue #852 — Netflix's wrapper `settings.json` was
  silently discarding a plain inherited `ANTHROPIC_BASE_URL`). This flag is
  appended unconditionally alongside — not instead of — the `--resume` logic;
  `buildClaudeCommand` computes the `--resume` flag from `claudeSessionID`
  completely independently (see §1), never from `envVars`.

No branch anywhere in this call graph reads `len(envVars)` or `EnvVars`
content to decide whether to add `--resume`, `--continue`, or any other flag.
`ANTHROPIC_BASE_URL` specifically is not a value the `claude` CLI is known to
interpret for session/conversation selection (it's a routing var).

## 4. Could the same cold-start-uuid-loss detector (`tryExtractConversationUUID`/`DetectByPath`) run somewhere in the first-time-setup path this triager didn't check?

**This is the strongest lead found.** `tryExtractConversationUUID()`
(`session/instance_claude.go:380-456`) falls back to
`detector.DetectByPath(effectivePath)` (line 414) whenever the fast
live-process-inspection path is unavailable. `effectivePath :=
i.GetEffectiveRootDir()` (line 409) — the exact field half 1 of this bug
report says wrongly resolved to the bare repo path. `DetectByPath` scans
`~/.claude/projects/<encoded effectivePath>/` for the **newest** conversation
JSONL with no ownership check — if `effectivePath` is the bare repo path
that another live session is *also* using, `DetectByPath` will legitimately
find and return that other session's most-recent JSONL, and
`tryExtractConversationUUID` will attach its UUID to `i.claudeSession`
(lines 443-455):

```go
i.claudeSessionMu.Lock()
i.mu.Lock()
if i.claudeSession == nil {
	i.claudeSession = &ClaudeSessionData{}
}
i.claudeSession.ConversationUUID = info.ConversationUUID
i.HistoryFilePath = info.HistoryFilePath
...
```

Once that happens, **any subsequent relaunch** — `initTmuxSession()` after a
`KillSession()`, a cold restore, or any path that rebuilds
`i.LaunchCommand` via `currentLaunchCommand()` (`instance_tmux.go:254-256`,
which reads `i.GetConversationUUID()` fresh) — will embed
`--resume <foreign-uuid>` into the real `claude` invocation, and the CLI
will then legitimately (per its own documented `--resume` semantics) render
that other session's conversation, including turns written after this
instance's creation, because `--resume` reopens the live JSONL, not a
snapshot. This exactly matches the reported symptom ("turns that postdated
the new session's creation").

Call sites of `tryExtractConversationUUID`, all grepped
(`session/agy_adapter.go:59`, `session/claude_adapter.go:60`,
`session/instance_workspace.go:80`, `session/instance.go:1425/1568/1845`):

- `instance.go:1425` is `recoverConversationBeforeLaunch`, gated off for
  `firstTimeSetup` (§1) — ruled out for the *initial* launch.
- `instance.go:1568` and `instance.go:1845` are both inside the **cold
  restore** branches of `startLocked`/legacy `start()` (the `if` branches
  guarded by `!i.pm().IsAlive()` after the `!firstTimeSetup` gate one level
  up — confirmed by reading the surrounding `else { // firstTimeSetup... }`
  block at `instance.go:1585-1668`, which contains **no** call to
  `tryExtractConversationUUID`). Not on the fresh-CreateSession path either.
- `claude_adapter.go:60` (`ClaudeAdapter.Import`) and `agy_adapter.go:59`
  fire on-demand when something calls `Import` to read conversation turns —
  used by `history_transfer.go:35` (session restart/fork lineage transfer)
  and `instance_checkpoint.go:56` (checkpoint creation). Neither is called
  automatically as part of a plain `CreateSession`/`Start(firstTimeSetup=true)`
  flow in the code paths this research read — but both are plausible if the
  repro's Request B session was ever restarted, forked, or checkpointed
  shortly after creation (worth the Server/App research agent checking
  request logs for a second `Start`/`RestartedFromSessionID`/checkpoint call
  against this session).
- `instance_workspace.go:80` fires from `SwitchWorkspace` — also not part of
  plain session creation.

**Conclusion for this section:** none of the six call sites fire during a
plain, single `CreateSession` → `Start(firstTimeSetup=true)` flow as read in
this codebase. If `DetectByPath`'s wrong-directory misattachment (or
`opts.ResumeId`, §1) is in fact the mechanism, something *after* the initial
launch — a relaunch, adapter `Import`, or a second `Start()` — must have
run. This is the concrete follow-up question for whichever research thread
covers session lifecycle/timeline for this specific backlog item's repro.

## 5. Tmux session/pane naming — is it ever keyed by path instead of Title?

**VERIFIED — no.** `NewSessionName(title, prefix)` (`session/tmux/tmux.go:392-394`)
is the single source of truth for sanitized tmux session names, built purely
from `title` + `prefix`:

```go
func NewSessionName(title, prefix string) SessionName {
	return SessionName{value: toStaplerSquadTmuxNameWithPrefix(title, prefix)}
}
```

`wireTmuxSession` (`session/instance_tmux.go:646-676`) calls
`tmux.NewTmuxSessionWithPrefix(snap.Title, program, tmuxPrefix, ...)` —
`snap.Title`, not path. The local `Start()` path
(`session/tmux/tmux_session_start.go:214`) creates
`new-session -d -s t.sanitizedName ... -c workDir ...` — `-s` (session name)
is `t.sanitizedName` (title-derived), `-c` (working directory) is a
*separate*, independent argument that only sets the pane's cwd, not its
identity. The remote path (`tmux_session_start.go:485`,
`createRemoteSession`) is identical in shape.

Grepped the whole repo for any path-keyed session/pane lookup
(`SessionByPath`, `FindSessionByPath`, `sessionByWorkDir`, `GetSessionByPath`,
`tmuxSessionForPath`) — zero hits. Combined with `CreateSession`'s existing
duplicate-title `AlreadyExists` validation (noted in requirements.md's
baseline), **pane/session-name collision is not a plausible mechanism** —
confirmed, not just assumed.

## 6. New finding not in the original baseline: `program` identity does not actually differentiate the two repro requests

Requirements.md (line 26) flags `program: "netflix-model-gateway"` (Request A)
vs. `program: "claude"` (Request B) as an untested variable. This research
found the two are **not actually different from the CLI-launch-code's point
of view**. `isClaude()` (`session/instance_tmux.go:149-160`) resolves custom
program IDs through `config.ResolveProgramConfig` *before* checking the
basename:

```go
func isClaude(program string) bool {
	cfg := config.LoadConfig()
	if res := config.ResolveProgramConfig(cfg, program); res.IsCustom {
		program = res.Command
	}
	for _, token := range strings.Fields(program) {
		if filepath.Base(token) == "claude" {
			return true
		}
	}
	return false
}
```

And the "netflix-model-gateway" custom program's registered config
(`session/instance_tmux_test.go:1252-1257`, `TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars`)
is:

```go
seedCustomProgram(t, config.ProgramConfig{
	ID:      "netflix-model-gateway",
	Command: "claude",
	Env:     map[string]string{"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"},
})
```

— i.e. its `Command` **is** literally `"claude"`, and its pre-registered env
var is the exact same `ANTHROPIC_BASE_URL`/port the reporter passed manually
in Request B's `envVars`. Both requests therefore resolve to `isClaude() ==
true` and go through `claudeLaunchBuilder`/`buildClaudeCommand` identically;
the only structural difference is *how* `ANTHROPIC_BASE_URL` got into
`resolveExtraEnvVars()`'s merged map (program-config-sourced vs.
request-sourced) — which §3 already shows is inert with respect to
`--resume`/argv. This means `program` identity, on its own, is **not** a
valid differentiator for the CLI-launch code path either — the real
differentiator is very likely ambient repo/session state (the bare repo path
already hosting a live session) combined with whatever made `resolvedPath`
diverge between the two requests, which is outside this research's stack
scope.

## Most plausible mechanism for the cross-attach symptom, ranked by confidence

1. **INFERRED (medium-high confidence):** Request B's `i.claudeSession`
   picked up a foreign `ConversationUUID` via
   `tryExtractConversationUUID()`'s `DetectByPath` fallback
   (`session/instance_claude.go:408-431`), scanning
   `~/.claude/projects/<encoded bare-repo-path>/` because
   `GetEffectiveRootDir()` had already collapsed to the bare repo path (half
   1's bug). A subsequent relaunch, adapter `Import`, or second `Start()`
   then embedded `--resume <foreign-uuid>` into the real `claude`
   invocation via `buildClaudeCommand` (`instance_tmux.go:364-371`), and the
   CLI faithfully resumed that live JSONL — explaining both "shows another
   session's content" and "including turns that postdated this session's
   creation" (a `--resume` reopens the live file, not a snapshot). Not fully
   VERIFIED because none of `tryExtractConversationUUID`'s six call sites
   fire on a plain, single `CreateSession(firstTimeSetup=true)` — a second
   lifecycle event (relaunch/import/restart/checkpoint) for this exact
   session is the missing link, and this research did not have access to a
   request-level timeline to confirm one occurred.
2. **VERIFIED (high confidence): ruled out.** The raw `claude` CLI
   autonomously resuming purely from a shared cwd with *no* `--resume`/
   `--continue` flag is not documented CLI behavior (§2) — every source
   checked agrees plain `claude` always starts fresh.
3. **VERIFIED (high confidence): ruled out.** tmux session/pane name
   collision — names are strictly `Title`-derived, pre-validated unique, and
   no code anywhere keys a tmux lookup by path (§5).
4. **VERIFIED (high confidence): ruled out.** `ANTHROPIC_BASE_URL`/`envVars`
   altering `claude`'s argv or resume decision directly — the env-var
   pipeline (tmux `-e` + `--settings` override) is structurally
   disconnected from the `--resume` flag logic (§3).
5. **VERIFIED, corrects a baseline assumption:** `program` identity
   (`netflix-model-gateway` vs. `claude`) does not actually change whether
   the session goes through the Claude launch-command path — both resolve to
   the literal `claude` binary (§6). The real differentiator between the two
   requests is elsewhere (likely path/worktree resolution state — Server/App
   scope, not stack).

Sources for §2 (external, not this repo):
- [Manage sessions — Claude Code Docs](https://code.claude.com/docs/en/sessions)
- [Claude Code --continue and --resume: Never Lose Your Context Again](https://pasqualepillitteri.it/en/news/366/claude-code-continue-resume-guide)
- [What is the --resume Flag in Claude Code | ClaudeLog](https://claudelog.com/faqs/what-is-resume-flag-in-claude-code/)
- [claude should resume the last session by default; add --new for fresh sessions · Issue #52556 · anthropics/claude-code](https://github.com/anthropics/claude-code/issues/52556)
