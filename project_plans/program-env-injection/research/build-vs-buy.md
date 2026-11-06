# Build vs. Buy: program-env-injection (verification run)

Scope: the fix already exists in `session/instance_tmux.go`; remaining work is
verification, regression coverage, and optional hardening. Prior research:
`project_plans/program-env-not-applied/research/build-vs-buy.md` (cited below as "prior").

## Verdict summary

Build nothing new, buy nothing. Reuse the in-repo helpers and the existing
regression tests; spend effort on running them against the pre-fix commit.

## Prior findings, re-checked against current source

| Prior finding | Status | Evidence (opened this run) |
|---|---|---|
| `-e K=V` pairs are separate argv elements, never a shell string, so no quoting library is needed | Still holds | `session/tmux/tmux_session_start.go:221-226` and `:639-644` append `"-e", kv` pairs |
| `go-shlex` is indirect only, no direct call sites | Still holds | `go.mod:73` lists it `// indirect`; no other env/tmux library in go.mod (only testify at `:47` is relevant) |
| `resolveExtraEnvVars()` -> `buildExtraEnv()` is the single shared point and should be extended, not bypassed | Still holds, and now realized | `session/instance_tmux.go:710`, `:729`; `claudeSettingsEnvOverrideArgs()` at `:756` calls `resolveExtraEnvVars()` |
| Snapshot via `atomic.Pointer` is stdlib, nothing to swap in | Still holds (not re-opened; unchanged scope) | prior path above; `.claude/rules/instance-lock-free-reads.md` |
| Other `exec.Cmd.Env` builders are a different mechanism and not reusable | Still holds | not re-audited per file; mechanism distinction is unchanged |
| "Wiring gap, fix in place" | No longer open | Fix is implemented; the prior doc predates it |
| (new vs prior) The fix adds a hand-built JSON payload plus shell quoting for `claude --settings` | New, not covered by prior | `session/instance_tmux.go:761` `json.Marshal`, `:766` `shellQuote(...)`; see Option 3 |

## 1. Existing OSS libraries / test helpers

Candidates: tmux Go bindings (`gotmux`, `go-tmux`), env-file libs (`godotenv`,
`envconfig`), `shellescape`/`go-shlex`.

- Pros: none needed for the mechanism. tmux `-e` is native and already used.
- Cons: tmux bindings wrap the same CLI and add a dependency whose version
  skew with the project's pinned/embedded tmux (`tmux.Binary()`,
  `session/tmux/binary.go:16`) is a new risk. Env-file libs parse `.env`
  files, which is not this problem. `go-shlex` is a splitter, not a quoter.
- Verdict: Not recommended. No library fits and none is imported today.

## 2. SaaS

N/A: this is local process env injection with no hosted-service analogue.

## 3. Bespoke code vs battle-tested mechanism

| Mechanism | Pros | Cons | Verdict |
|---|---|---|---|
| tmux `new-session -e K=V` (current) | Native, argv-isolated, visible in `tmux show-environment` (AC2), set before the pane process starts | Needs tmux >= 3.2 for `-e` (CI pins tmux via `TMUX_BIN`); applies to the session env, so a program that resets its own env can still lose it | Recommended (already implemented) |
| `os/exec` `Cmd.Env` on the tmux client | Stdlib | Only sets the tmux client's env, not the server/pane; the first session on a socket inherits from the server start, later ones do not. Does not satisfy AC2 | Not recommended |
| Wrapper `env K=V cmd` in the launch string | Works on any tmux version | Values pass through the shell and need quoting; invisible to `show-environment`; fails AC2 | Viable only as fallback for tmux < 3.2 |
| `claude --settings '{"env":{...}}'` via `json.Marshal` + `shellQuote` (current, #852) | `encoding/json` handles all escaping of the payload; `shellQuote` (`session/instance_tmux.go:433`) single-quotes and escapes `'` as `'\''`, which is the standard POSIX idiom | Hand-rolled quoter (~3 lines); a second, near-identical `shellQuote` exists in `session/tmux/ssh_runner.go:601` | Recommended; add one table test with `'`, `$`, backtick and newline values rather than swapping in a library |

Hardening worth considering (not required by the ACs): a test asserting the
`--settings` payload round-trips through `sh -c` for hostile values. This
checks the one piece of hand-rolled code in the fix.

## 4. Fork / adapt in-repo helpers

| Helper | Use here | Verdict |
|---|---|---|
| `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` (`server/services/session_service_create_test.go:673`) | Already does the end-to-end check: real tmux on an isolated socket, asserts `show-environment` and in-pane `printenv`; uses `tmux.Binary()`, which honors `TMUX_BIN` | Recommended: run it unchanged against the pre-fix commit for AC3 |
| `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars`, `TestClaudeSettingsEnvOverrideArgs_*`, `TestBuildClaudeCommand_IncludesSettingsEnvOverride` (`session/instance_tmux_test.go:1202-1278`) | Unit-level coverage of resolution order and the `--settings` override (AC4) | Recommended: reuse; extend only if a gap is found |
| `createTestStorage` (`server/services/session_service_test.go:42`) | Storage fixture for service tests | Viable if a new service-level case is added; not needed for existing ones |
| `envtest` (`envtest/envtest.go`: `ClearAmbientStaplerSquadStateEnv`, `NewIsolatedStateDir`) | Keeps ambient `STAPLER_SQUAD_*` state and the real `~/.stapler-squad` from leaking into tests | Recommended for any new test touching `config.LoadConfig()`, since `resolveExtraEnvVars` reads it |
| New bespoke tmux test harness | Duplicates the above | Not recommended |

## Recommendation

1. Verification (AC1-3): run the existing server and session tests at HEAD,
   then the server test at the pre-`cdfd4e5cf2` parent to prove it fails there.
   Reusing the existing test is cheaper and more trustworthy than a new one.
2. No new dependency. Optional: one hostile-value table test for
   `claudeSettingsEnvOverrideArgs` + `shellQuote`.
3. Optional cleanup, out of this item's scope: dedupe the two `shellQuote`
   copies.

Confidence: VERIFIED for file/line citations above (opened this run);
INFERRED for tmux `-e` minimum version (tmux 3.2 from memory, not checked
against tmux docs here).
