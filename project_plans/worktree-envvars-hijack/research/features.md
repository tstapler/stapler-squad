# Research: Features/Edge-cases — program vs. envVars divergence

Scope: does `program == "claude"` (built-in) vs. a registered custom program (e.g.
`"netflix-model-gateway"`) route a `SESSION_TYPE_NEW_WORKTREE` `CreateSession` call
through meaningfully different code, and is there an untested combination that
plausibly explains the reported cross-session content leak.

## 1. `config.ResolveProgramConfig` — what `IsCustom` gates

`config/defaults.go:211-227`:

```go
func ResolveProgramConfig(cfg *Config, program string) ResolvedProgram {
	if prog := FindProgramConfig(cfg, program); prog != nil {
		env := ExpandEnvVars(prog.Env)
		return ResolvedProgram{Command: prog.Command, CLIFlags: prog.CLIFlags, EnvVars: env, IsCustom: true}
	}
	return ResolvedProgram{Command: program, CLIFlags: "", EnvVars: nil, IsCustom: false}
}
```

`FindProgramConfig` (`config/defaults.go:190-200`) does a case-insensitive ID lookup
against `cfg.SessionDefaults.Programs`. For `program == "claude"` this returns `nil`
(not a registered custom program ID) → `IsCustom: false`, `Command: "claude"`. For
`program == "netflix-model-gateway"` it returns the matching `ProgramConfig` →
`IsCustom: true`, `Command: <the custom program's real binary/command>`.

`IsCustom` does **not** affect `Path`, `WorkingDir`, or session-type resolution
anywhere in `server/services/session_service.go`'s `CreateSession` — confirmed by
reading `resolveSessionType` (`session_service.go:3108-3132`), which only branches on
`msg.SessionType`, `msg.ExistingWorktree`, and `branch`, never `program`. Both repro
requests set `sessionType: SESSION_TYPE_NEW_WORKTREE` explicitly, so
`resolveSessionType` returns `session.SessionTypeNewWorktree` for **both** regardless
of program — the baseline's elimination of this function holds for the `program`
variable too, not just `envVars`.

What `IsCustom` **does** gate, all downstream in `session/instance_tmux.go` (launch
time, not creation time):

- `isClaude(program)` (`session/instance_tmux.go:149-160`) — resolves custom IDs to
  their underlying `Command` first, then checks whether any whitespace token's
  basename is `claude`. A custom program's `Command` is very unlikely to literally be
  `claude`, so `isClaude("netflix-model-gateway") == false` while
  `isClaude("claude") == true`.
- `isPi(program)` (`:167-178`) — same resolve-then-basename-match pattern for `pi`.
- `yoloFlagFor(program)` (`:193-204`) — same pattern, used by `AutoApproveSupported`
  (`:208-210`) and the auto-approve flag injection.
- `markWorkingDirTrusted` (`session/claude_trust.go:178-186`) — no-ops entirely
  (`if !isClaude(i.Program) { return }`) unless `isClaude` is true, so the custom
  program never gets its working dir pre-trusted in `~/.claude.json`.
- `matchLaunchBuilder(program)` (`session/instance_tmux.go:292-299`) — iterates
  `launchBuilders = []launchCommandBuilder{&claudeLaunchBuilder{}, &piLaunchBuilder{}}`
  (`:78`); `claudeLaunchBuilder.Matches` is `isClaude(program)` (`:83`). Only when this
  matches does `buildBaseLaunchCommand` call `i.buildClaudeCommand(base, claudeSessionID)`
  (`:85-91`, `:305-307`) instead of the generic `shellQuoteFields(program)` fallback
  (`:305,313`).

So `IsCustom`/`isClaude` is a **launch-time** command-construction gate, not a
creation-time path/session-type gate. It cannot by itself explain
`workingDir`/`activeDir`/`existingDir` reporting the bare repo path — those are set
by `setupFirstTimeWorktree` (`session/instance_worktree.go:53-90`), which is
unconditional on `Program` for `SessionTypeNewWorktree` (case at `:61-90` calls
`i.newWorktreeFromResolvedBase()` with no program check).

## 2. Every `program`-keyed conditional in `CreateSession` / `NewInstance` / `Start`

In `server/services/session_service.go`'s `CreateSession` (~2304-2900):

- `program := req.Msg.Program` (`:2452`) — plain assignment, not a branch.
- Alias branch: `if program == "" { program = resolved.Program }` (`:2466-2468`) and
  the non-alias branch's identical `if program == "" { program = resolved.Program }`
  (`:2494-2496`) — only fills in an *empty* program from defaults; does not branch on
  a *non-empty* program's value.
- `config.ResolveProgramConfig(cfg, program)` / `if resolvedProg.IsCustom` (`:2521-2531`)
  — merges the custom program's `EnvVars` into `instanceEnvVars` (only filling keys not
  already set by request/alias/defaults — see note below). No `Path`/`WorkingDir`/
  `SessionType` effect.
- `if req.Msg.AutoApprove && !session.AutoApproveSupported(program)` (`:2596-2599`) —
  validation-only; returns `CodeInvalidArgument`, doesn't affect worktree/path
  resolution and neither repro request sets `AutoApprove`.
- `tmux.NewTmuxSessionWithServerSocket(..., program, ...)` /
  `NewTmuxSessionWithPrefix(..., program, ...)` (`:2796,2798`) — **only inside the
  `remoteRequested` block** (`:2652` `if remoteRequested {`); neither repro request
  targets a remote host, so this is out of scope here.
- `instanceOpts.Program: program` (`:2840`) — passed straight through to
  `session.InstanceOptions`, no branching.

In `session/instance.go`'s `NewInstance`/`Start` path: no `program`-value branch found
(only `opts.Program: program` at `session/instance.go:1093`, a plain assignment; the
`log.Info` calls at `:1470,1723` just log it). The actual program-value branches live
one layer further down, in `session/instance_tmux.go` (Section 1 above) and are all
reached from `initTmuxSession`/`buildLaunchCommand`, i.e. **inside** `Start()`'s tmux
bring-up, not in `NewInstance`'s worktree/path setup.

**Conclusion for Q2**: no `program`-keyed conditional touches `Path`/`WorkingDir`/
`SessionType` resolution anywhere in the traced call graph. Every `program`-keyed
branch found affects only: (a) auto-approve flag validity, (b) which
`launchCommandBuilder` builds the tmux launch string, (c) whether `--settings`/
`--mcp-config`/`--resume` flags get added, (d) whether the working dir gets
pre-trusted in `~/.claude.json`. This rules out a *direct* `program`-conditioned
path/session-type bug in the traced functions, but does NOT rule out an *indirect*
interaction — see the ranked list at the end.

## 3. Existing test coverage for NEW_WORKTREE × program × envVars

```
grep -rn "NEW_WORKTREE" server/services/*_test.go
```
Files matching: `session_service_remote_test.go`,
`session_service_stream_terminal_remote_test.go`,
`session_service_remote_target_proto_test.go`, `session_service_create_test.go`. All
the remote-target files are (as the name says) `remoteRequested` scenarios — out of
scope for the local repro.

`server/services/session_service_create_test.go` (the local-path test file) has zero
occurrences of `IsCustom` and only 4 `Program:` field usages total:
`Program: "codex"` (`:138`, an unsupported-program-for-auto-approve negative test),
`Program: "claude"` (`:155`, an auto-approve-IS-supported positive test), and two more
`Program: "claude"` (`:308, :614`) in duplicate-title / GitHub-URL tests. **None of
these four tests also set `SessionType: SESSION_TYPE_NEW_WORKTREE` or `EnvVars`.** The
one `NEW_WORKTREE` test in this file (`:37`,
`TestResolveSessionType_ExplicitNewWorktree`) tests `resolveSessionType` as a bare
unit function — it doesn't go through `CreateSession` at all and sets no `Program` or
`EnvVars`.

A repo-wide search for tests that exercise `ResolveProgramConfig`/`IsCustom` at all
(not just combined with worktree+envVars) returned nothing — `grep -rln
"ResolveProgramConfig\|IsCustom" **/*_test.go` had no matches (the glob itself matched
zero files in this shell, but the earlier full-repo grep of production code already
established these are only read from `config/defaults.go`,
`session/instance_tmux.go`, and `session_service.go`, none of which have a
custom-program-ID test fixture visible in `server/services/*_test.go`).

**Coverage gap confirmed**: there is no test for `CreateSession` with
`SessionType: SESSION_TYPE_NEW_WORKTREE` + a **registered custom program ID**
(`IsCustom: true`) + non-empty `EnvVars`, nor its built-in-`claude` counterpart with
the same `EnvVars`. This is exactly the combination in the two repro requests, and
it's untested on either side of the divergence.

## 4. Alias resolution and empty `AliasName`

`server/services/session_service.go:2457-2504`: the alias-resolving branch
(`config.ResolveAlias`, `:2459-2487`) only runs `if req.Msg.AliasName != ""`. Both
repro requests, per the requirements doc, show no `alias_name` in their JSON bodies —
i.e. both take the `else` branch (`:2488-2504`), calling `config.ResolveDefaults(cfg,
workingDir, req.Msg.Profile)` directly. There is **no hidden default-alias-matching by
program** anywhere in this code: `ResolveAlias` is only reachable by name
(`config.FindAlias`, `config/defaults.go:180-187`, a case-insensitive `Name` lookup),
never by `program` value or any implicit fallback. Confirmed by reading
`config.ResolveAlias` in full (`config/defaults.go:275-312`) — it takes
`(cfg, aliasName, branch, label, extraFlags)` and starts with `alias := FindAlias(cfg,
aliasName); if alias == nil { return ..., ErrAliasNotFound }`; nothing resolves an
alias from `program`.

With `AliasName` empty in both requests, both go through the **identical** code path
at `:2488-2504` (`config.ResolveDefaults`); the only inputs that can make its output
differ between the two requests are `workingDir` (`req.Msg.WorkingDir`, or
`resolvedPath` if that's empty) and `req.Msg.Profile` — both hidden fields not shown
in the repro JSON (see Q5).

## 5. Hidden-field audit — plausible unshown-field explanations

Fields on `CreateSessionRequest` (`proto/session/v1/session.proto:685-830+`) not
mentioned in the repro's shown JSON bodies, that a real web-UI/MCP client could
plausibly set differently between the two requests and that route through
program-adjacent or worktree-adjacent code:

- **`working_dir` (field 3)**: read at `session_service.go:2489-2492` — if set, it (not
  `resolvedPath`) is fed into `config.ResolveDefaults`'s directory-rule matching
  (`config/defaults.go:78-93`, longest-prefix match against
  `sd.DirectoryRules`). A directory rule scoped to the repo path could plausibly set
  its own `Program`/`EnvVars`/`CLIFlags` defaults, independent of what the client sent
  — worth ruling in/out since it's the one place `program` and directory-scoped
  config genuinely interact before this function's output is merged.
- **`profile` (field 11)**: `config.ResolveDefaults`'s Layer 4 (`config/defaults.go:96-101`)
  — an explicit profile requested by one client but not the other applies a
  `ProfileDefaults` override (potentially setting `Program`, `EnvVars`, `CLIFlags`,
  even absent an alias).
- **`skip_defaults` (field 12)**: `session_service.go:2457` — if request B (or A) set
  this `true`, the entire `ResolveDefaults`/`ResolveAlias`/`ResolveProgramConfig`
  block is skipped outright, changing `instanceEnvVars`/`instanceCLIFlags` composition
  substantially. Not shown in either repro body, so unconfirmed either way.
- **`create_if_missing` (field 18)**: only affects `SessionTypeDirectory`
  (`:2570-2582`), irrelevant to `NEW_WORKTREE`, but worth ruling out if the actual
  server-side `sessionType` ended up `Directory` rather than `NewWorktree` for some
  other reason not yet found (see ranked list).
- **`existing_worktree` (field 9)**: if request B accidentally carried a non-empty
  `existing_worktree` (e.g. stale web-UI form state from a previous session-creation
  attempt reused via `SwitchProgram`/`restart_from_session_id`-adjacent flows), it
  would silently override the session type via `resolveSessionType`
  (`:3125-3126`, `if msg.ExistingWorktree != "" { return SessionTypeExistingWorktree }`)
  — but this branch is only reached if `msg.SessionType ==
  SESSION_TYPE_UNSPECIFIED`, and both repro requests set it explicitly to
  `NEW_WORKTREE`, so this particular field is *ruled out* unless the client's proto
  serialization somehow left `session_type` unset despite the reporter's JSON showing
  it set (worth a byte-for-byte capture in Phase 3 to eliminate any client-side
  serialization mismatch).
- **`restart_from_session_id` / `confirm_restart_with_live_source`** (fields 32-33):
  not mentioned in either repro body; if request B's client silently attached a stale
  `restart_from_session_id` (e.g. leftover from a "restart" UI action state), it feeds
  `resolveRestartSource` (`session_service.go:2397`) which can substitute in a
  *different* `resolvedPath` derived from another live/persisted session — directly
  consistent with "the repo path happened to already host a live, unrelated Claude
  Code session." This is the single most direct mechanistic match to the observed
  symptom among the hidden fields and should be the first thing Phase 3's repro script
  captures/rules out (dump the *exact* wire-level protobuf bytes of request B, not
  just the JSON summary in the bug report).

Phase 3 repro-script guidance: capture the **raw request bytes** (not a
human-summarized JSON) for both Request A and Request B — ideally by adding a
temporary debug log of `req.Msg.String()` (proto text format, so every field
including zero-value-but-explicitly-set fields — note proto3 does NOT distinguish
"unset" from "zero value" for scalar fields like `restart_from_session_id ""` vs. not
sent, so this alone won't resolve that ambiguity for string fields; a wire-format dump
or field-presence check via reflection would be needed to be fully certain) at the top
of `CreateSession`, then diff the two captures field-by-field. That directly answers
whether `working_dir`, `profile`, `skip_defaults`, or `restart_from_session_id` differ,
which no other research angle can settle from code-reading alone.

## Ranked list: most likely divergent code paths, `program=claude` vs.
`program=netflix-model-gateway`, otherwise-identical NEW_WORKTREE request

1. **Hidden field, not `program` at all — `restart_from_session_id` /
   `working_dir` differing between the two real client requests** (Q5). This is
   the only mechanism found in this angle's scope that can directly explain a
   *different resolved path* pointing at a pre-existing live session's directory.
   Everything below is a `program`-conditioned difference that changes *launch-time
   behavior* but was not shown to touch `Path`/`WorkingDir`/`SessionType`.
2. **`isClaude`-gated `matchLaunchBuilder` → `buildClaudeCommand`
   (`session/instance_tmux.go:83,281-287,364-402`)**: only `program=claude` runs
   `claudeSettingsEnvOverrideArgs()` (`:631-641`), which is the one place `envVars`
   (`ANTHROPIC_BASE_URL` in the repro) and `program=claude` interact in the same
   function call — request A (custom program, no envVars) never reaches this code at
   all, request B (claude + envVars) is the only repro request that does. Does not by
   itself explain the path-hijack symptom, but is the strongest **combined-variable**
   interaction found and the most direct candidate for "why B and not A" if the actual
   bug is launch-command-construction rather than worktree/path resolution (e.g. a
   malformed `--settings`/`--resume` flag somehow causing tmux/claude to attach
   differently) — Phase 3 should capture the *actual* shell command string
   `buildLaunchCommand` produced for request B and check it for a spurious
   `--resume <uuid-of-other-session>` or similar cross-wired flag.
3. **`markWorkingDirTrusted` firing only for claude (`session/claude_trust.go:178-186`)**:
   pre-trusts the working directory in `~/.claude.json` only when `isClaude`. If the
   "trusted" bare-repo-path entry pre-dates this session (from the other live claude
   session) and claude's own trust-directory matching is prefix/path-based rather than
   exact, this is a plausible (but unconfirmed — outside this angle's file scope)
   secondary contributor to why claude's pane could end up rooted at/attached to the
   bare repo rather than the worktree; the custom program path never touches
   `~/.claude.json` at all, so it's structurally immune to this class of bug.
4. **`ResolveProgramConfig`/`IsCustom` merge order at `session_service.go:2521-2531`**:
   ruled low-likelihood — it only ever *adds* keys not already present in
   `instanceEnvVars`, and only for `IsCustom == true` programs, so it cannot explain
   B's (built-in, non-custom) behavior at all; only relevant if it turns out A's
   correct behavior depends on this merge in a way B's built-in path structurally
   cannot replicate (asymmetric coverage, not a bug per se).
5. **Untested-combination risk generically (Q3)**: given zero existing test coverage
   for NEW_WORKTREE + custom-program + envVars on *either* side of the divergence, it
   is possible the bug is a **pre-existing dormant defect** exposed only by this exact
   combination, rather than a "custom program handles it correctly, claude doesn't"
   asymmetry — Phase 3's repro script should also test a third case
   (`program=claude`, `envVars` set, but session_type=DIRECTORY or no envVars) to
   isolate whether `NEW_WORKTREE` itself is required to reproduce, independent of
   `program`/`envVars`.
