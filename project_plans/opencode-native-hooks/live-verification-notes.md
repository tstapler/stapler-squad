# Live verification notes: `patchOpenCodeHooks()`

Captured 2026-08-11 against opencode CLI 1.4.0, `@opencode-ai/plugin` resolved 1.3.10, on this
dev machine, using a throwaway plugin file (not committed — deleted after testing).

## 1. Plugin registration mechanism (criteria 1/2) — CONFIRMED

`~/.config/opencode/plugins/<name>.js` (plural, global, no config-file registration) auto-loads
with zero entries in `opencode.json`. Verified: dropped a trivial plugin there, ran `opencode run`
with a prompt that triggers a tool call, observed the hook fire with no `opencode.json` changes
and no `opencode plugin <module>` CLI invocation. This confirms `plan.md`'s recommendation (over
`research/architecture.md`'s alternative, the `opencode.json` `plugin` array, whose local-file-path
support was unconfirmed) — `patchOpenCodeHooks()` should write to this directory.

Decoy note reconfirmed: `~/.config/opencode/plugin/` (singular) exists on this machine and is
unrelated/inert — the installer must target `plugins/` (plural) exactly.

## 2. `tool.execute.before` throw-to-block — CONFIRMED (highest-risk assumption in the design)

A plugin whose handler unconditionally throws blocked every subsequent tool call in the session:
`opencode run` showed `✗ bash failed` / `Error: SSQ_SMOKE_TEST_FIRED tool=bash` for each attempt,
and the agent could not complete the task. This is the single highest-risk unverified assumption
flagged in `research/architecture.md` §2 and `research/stack.md` §1 — now verified live, not just
via docs.

## 3. Non-blocking (allow) path — CONFIRMED

A non-throwing plugin (`fs.appendFileSync` logging only) let `opencode run` complete normally.
Logged payload shape matches `stack.md`'s cited type: `{tool, sessionID, callID}` — confirmed
`input.tool` values are lowercase (`"bash"`, `"read"`, `"task"`), not the Claude-style
`"Bash"`/`"Read"` capitalization.

## 4. Subagent (task-tool) coverage — CONFIRMED FIRES (refutes opencode issue #5894's original claim, as the repo collaborator argued)

Prompted the primary agent to delegate to `@general` (a real subagent per `opencode agent list`)
to run a bash command. Hook log showed **two** separate entries: `{"tool":"task",...}` for the
delegation itself (primary session), then `{"tool":"bash",...}` for the subagent's own bash call,
under a **different** `sessionID` than the parent. Both fired `tool.execute.before`. This matches
the repo collaborator's rebuttal in `pitfalls.md` §1 (the original #5894 bypass report was a
test-methodology artifact) — independently reproduced here against the actual target version
(opencode 1.4.0), not just relied upon from the GitHub thread.

## 5. `batch`-tool blind spot — NOT REPRODUCED (inconclusive, not proof of absence)

Asked the agent to run two bash commands "using your batch tool if you have one." The default
`build` agent had no distinct batch tool available — it issued two separate `bash` tool calls,
and **both** were individually intercepted (two log entries, two distinct callIDs). This does not
positively confirm `pitfalls.md` §1's cited `batch.ts` code path doesn't exist somewhere in
opencode's tool surface (e.g. a different agent mode or a future `batch` tool might expose it) —
only that the default agent's tool set on this opencode version didn't expose a bypassable batch
path in this test. Recorded as a known, documented gap in the ADR/plan rather than asserted as a
solved problem.

## 6. End-to-end test with the real `patchOpenCodeHooks()`-generated plugin — CONFIRMED, found and fixed one real bug

Built the actual `cmd/ssq-hooks` binary from this branch, ran the real `install open-code`
against this machine (binary + config backed up first, restored after — see Cleanup), and drove
it through a real `opencode run` session with no synthetic/mocked classifier:

- `git push origin HEAD --dry-run` and a plain file write both completed normally (AutoAllow
  path unaffected).
- `rm -rf /` — the *model itself* refused before any tool call was attempted (its own training,
  not ssq-hooks); not usable as an interception proof for that reason, so this was not counted
  as a passing test either way.
- A `.env` file write via the `write` tool was **not blocked** on the first attempt — found live,
  not by a synthetic unit test: OpenCode's `write`/`edit` tool args use camelCase `filePath`
  (`{"content":"...","filePath":"/tmp/.env"}`, confirmed via a second debug probe plugin logging
  raw `output.args`), but `pkg/classifier/classifier.go`'s `FilePattern` rules match against
  `payload.ToolInput["file_path"]` (snake_case, Claude's convention) — every OpenCode
  `.env`/`.git` write-protection rule was silently never matching (empty-string lookup, no
  error, no log). Fixed with `normalizeOpenCodeToolInput()` in `parseOpenCodePayload()` (copies
  `filePath` → `file_path` when the latter is absent) — a fourth-adapter-layer fix, no change to
  `classifier.Classify()` itself. Re-verified after the fix: same `.env` write via `opencode run`
  was blocked, reason (`seed-deny-env-write`) surfaced verbatim to the model, no file created on
  disk. A plain non-`.env` write in the same session still succeeded (allow path unaffected by
  the fix). Regression tests: `TestParseOpenCodePayload_NormalizesCamelCaseFilePath`,
  `TestParseOpenCodePayload_PrefersExistingSnakeCaseFilePath`,
  `TestParseOpenCodePayload_EnvFileWriteClassifiesAsAutoDeny` (`cmd/ssq-hooks/main_test.go`).
- Migration path also confirmed live in the same pass: the real install replaced a pre-existing
  `~/.local/bin/open-code` bash-wrapper (from this machine's actual prior install) — removed
  cleanly, no error, plugin still installed correctly alongside it.

This is the concrete example of why `.claude/CLAUDE.md`'s "run it, don't read it" rule exists —
the `filePath`/`file_path` mismatch would not have been caught by any of the unit tests written
before this pass (all used correctly-shaped synthetic payloads); only a real opencode session
hitting a real classifier rule surfaced it.

## Cleanup

All throwaway/debug plugin files (`ssq-smoke-test.js`, `zz-args-probe.js`) were deleted from
`~/.config/opencode/plugins/` after verification. The machine's pre-existing live
`~/.local/bin/ssq-hooks` and `~/.local/bin/open-code` were backed up before the real
`install open-code` run above and restored byte-for-byte afterward (sha256 verified) — this
branch's build was not left installed as the live hook binary; shipping that is what the PR
merge is for, not an ad hoc local install during verification.
