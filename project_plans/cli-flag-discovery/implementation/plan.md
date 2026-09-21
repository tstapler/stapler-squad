# Implementation Plan: cli-flag-discovery

**Feature**: `ProbeProgram` RPC that verifies a program's binary and parses `<binary> --help` into flags; surfaced as a found/not-found badge, flag autocomplete, unknown-flag warnings and tap-friendly descriptions in Program Config and (badge plus saved-flag validation) in session creation.
**Date**: 2026-09-21 (repaired after adversarial + architecture review; see "Review repair log" at the end)
**Status**: Ready for implementation (ADR-001 owner sign-off PENDING; implementation proceeds on coordinator decision)
**ADRs**: [ADR-001](../decisions/ADR-001-probe-any-command-with-hardened-execution.md), [ADR-002](../decisions/ADR-002-probe-runner-on-safeexec-with-immediate-sigkill.md)
**Requirements**: `../requirements.md` (AC numbers 1-10 below refer to its Acceptance criteria list; its "Interpretation notes" section records how AC2, AC3, AC6, AC7, AC8 and AC10 are read here). UX decisions D1-D5 from `../design/ux.md` (section "Decisions the plan should confirm") are adopted or dropped as stated in Flagged Choice 9. Complexity: 2 (no field in requirements.md; treated as 2).

---

## Flagged Choices (reviewer: challenge these)

1. **Security model = any command, hardened execution plus a request guard (ADR-001).** The pitfalls agent preferred a saved-only allow-list; that cannot cover the unsaved form being edited in Program Config (AC5), so it was rejected. The `:8543` listener has no auth (`research/pitfalls.md` section 1), so the server enforces AC8, not the UI: only a bare command name or an absolute path is accepted, target must be a regular, executable, non-world-writable file, POST-only, loopback Host/Origin guard, one audit log line per probe. Owner sign-off is **PENDING human reviewer sign-off**; implementation proceeds on coordinator decision.
2. **AC7 is scoped down (recorded, not dropped).** Session creation shows the same missing-binary badge plus validation of the selected program's **saved `cli_flags`**. It does **not** validate alias `extraFlags`: `OmnibarCreationPanel.tsx` has no `extraFlags` prop or reference (grep, 2026-09-21); `extraFlags` exists only in `Omnibar.tsx:1485,1508,1881` in the alias-apply path, which never reaches the picker/panel. Plumbing it would touch two hotspot files for a value that is appended after the fact. See Unresolved Questions #5 and the traceability row. No new flags input is added; the picker is a `<select>` (`OmnibarCreationPanel.tsx:942`).
3. **Session-creation RPC input = client resolves program ID to command (rejected: `program_id` on the request).** AC1 fixes the signature as `ProbeProgram(command)`; the server accepts any command anyway (ADR-001), so `program_id` adds a second server path with no security gain. Cost: `ProgramOption` (`web-app/src/lib/constants/programs.ts:1-5`) and `useAvailablePrograms` (`useAvailablePrograms.ts:19-23`) carry `command` and `cliFlags` (task 2.3.1a).
4. **Runner** built from `safeexec.CommandContextPG` with `cmd.Cancel` overridden to immediate group SIGKILL and `Setsid` (ADR-002); `ShortLivedCmd` rejected because it cannot cap output.
5. **New `FlagCombobox` component instead of extending `AutocompleteInput`.** `AutocompleteInput` is 199 lines, single-token, lacks `role="combobox"` (`research/ux.md` section 0) and has five other users. `web-app/src/components/history/HistorySearchInput.tsx:174` is the ARIA reference. Tracked duplication cost: task 3.1.2c extracts the shared listbox key handling or files a follow-up issue.
6. **Unknown-flag warnings are suppressed when zero flags parsed, or when the probed command is a wrapper**, and worded "not listed in --help".
7. **Wrapper commands (`env`, `sudo`, `npx`, `uv`, `uvx`, `nice`, `time`, `exec`, `nohup`, `xargs`, `command`) are detected, not probed.** `Resolve` skips leading `NAME=value` tokens; if the first real token's basename is a known wrapper the server confirms it exists but does not run `--help` (`npx --help` may hit the network and `env --help` describes env, not the wrapped program) and returns `is_wrapper=true`. The UI shows "Wrapper command (`env`): flags for the wrapped program are not checked." and suppresses autocomplete and warnings. The built-in Proxy entry (`programs.ts:16`, begins `env -u ...`) is the test case.
8. **Feature-flag gating: none.** Additive and non-blocking; see Risk Control.
9. **UX decisions from `design/ux.md`**: D1 adopted (Enter in the command field runs Check and does not submit; task 2.2.1a). D2 adopted (copy "Checked on this server only" shown in the badge detail for every probe, since the probe only ever sees the server host; task 2.1.2a). D3 adopted (inline description on the active option, no button inside `role=option`; tasks 4.2.2a). D4 **dropped**: no "Did you mean / Use it" and no "Edit in Program Config" link. D5 adopted (Check button on desktop and mobile). Also dropped as scope drift: `docs/reference/program-probe.md` and its `CLAUDE.md` index row. Kept: `docs/registry/features/` regeneration (repo rule) and the e2e spec (AC10 requires it; AC9 proof).
10. **PATH: not the bare server PATH.** Program lookup mirrors how `config/config.go` finds programs: through the user's shell (`config/config.go:1213-1221` and `:1288-1296` run `$SHELL -c "source ~/.zshrc &>/dev/null || true; which <cand>"`; bash uses `~/.bashrc`; other shells plain `which`). The probe derives the login-shell PATH (task 1.1.4d; success cached with TTL, failures retried, `$SHELL` unset falls back to server PATH + `~/.local/bin`, `/usr/local/bin`) with the same `source ~/.zshrc`/`~/.bashrc` prefix but a fixed script that prints `$PATH` between sentinel markers (no user input reaches a shell, so ADR-001's no-shell envelope for user text holds), then uses `shellPATH dirs + server PATH dirs` (deduped, shell first) both for lookup and as the child `PATH`. Alias/function-only programs (for example `proxy-claude` when defined only as an alias, candidate list `config.go:1208,1286`) cannot be executed by the probe and are reported `NOT_FOUND` with explanatory copy "Not found as an executable on the server's PATH. Shell aliases and functions are not checked; the program may still work when launched from your shell." The full shell resolution used by `GetClaudeCommand` is deliberately not adopted (it would need a shell for user text).
11. **Timeout is a measured default, not a constant (pre-mortem P1-1).** AC3's 3s stays the default. Task 1.1.3a captures cold and warm `--help` wall time for claude, aider, gemini, agy and gh and records the numbers in the "Measured `--help` timings" table below. Decision rule: if **any** target exceeds about 1.5s cold, `Limits.Timeout` becomes overridable per probe (a `slowTools` map keyed by basename in `config/clihelp`, raised to a value derived from the measurement, e.g. 2x the worst cold time capped at 10s) while the default stays 3s; if a target still cannot finish inside a sensible bound, add a `PENDING` status (RPC returns fast, flight continues under `WithoutCancel`, client re-polls) instead of a bigger timeout. Whichever branch is taken is recorded in that table and in the repair log. `TIMEOUT` is shown as "Found: /path. Timed out reading flags — try Check again." (never as "no flags"), cached 60s (was 30s; still short so a busy-machine timeout heals), and an explicit Check bypasses a cached `TIMEOUT`. A real-binary smoke test (skipped when the binary is absent) guards the decision.
12. **Scripts are not run without an explicit Check (pre-mortem P1-2; ADR-001 addition, sign-off PENDING).** `Prober` reads the first bytes of the resolved file: native binary (ELF/Mach-O) may run on an implicit probe; a `#!` shebang or unrecognized file returns `NEEDS_CONFIRM` without executing. Execution needs request field `confirm_execute=true`, sent only by the Check button or Enter in the command field (never by blur). The session-creation picker never executes on selection: it sends `resolve_only=true` (found-only, cached result if any, else `NEEDS_CONFIRM`) and offers a `Check` button. Confirmed (real path, mtime, size) targets are remembered for the process lifetime so later blur probes of the same file need no second click. Cost: claude, gemini and aider are shebang scripts, so each needs one click the first time; recorded in Unresolved Questions.

### Measured `--help` timings (fill during Task 1.1.3a; go/no-go)

| Target | Cold (s) | Warm (s) | Under 1.5s cold? | Notes |
|---|---|---|---|---|
| claude | UNMEASURED | UNMEASURED | - | - |
| aider | UNMEASURED | UNMEASURED | - | - |
| gemini | UNMEASURED | UNMEASURED | - | - |
| agy | UNMEASURED | UNMEASURED | - | - |
| gh | UNMEASURED | UNMEASURED | - | - |

Decision (fill in): _default 3s kept_ / _`slowTools` override added: ..._ / _`PENDING` status added_.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `Command` | The raw user string in `ProgramConfigProto.command` (`session.proto:2526-2534`) | Not argv; only the first non-env-assignment token is ever used |
| `Target` | Result of a successful `Resolve`: `{Name string; IsAbsolute bool; Wrapper bool}` | Go struct in `config/clihelp/resolve.go`; constructible only by `Resolve` |
| `ResolveError` | Typed const set: `EMPTY`, `ONLY_ENV_ASSIGNMENTS`, `UNBALANCED_QUOTE`, `INVALID_TOKEN` (NUL/newline/leading `-`), `RELATIVE_PATH` (contains a separator but is not absolute) | `Resolve` returns `(Target, ResolveError)`; the zero value means success and `Target` is only valid then; all map to `NOT_FOUND` |
| `ResolvedPath` | Absolute path returned by `lookPath`/`checkExecutable`: regular, executable, not world-writable file, after symlink eval | Newtype `type ResolvedPath string` |
| `LoginPath` | Ordered dir list: user's login-shell PATH first, then server PATH, deduped; derived once | Used for lookup and as child `PATH` |
| `HelpText` | Merged, ANSI-stripped stdout+stderr of `<ResolvedPath> --help`, capped at 256KB | Newtype `type HelpText string` |
| `Flag` | Parsed option: `Name` (primary long, or `-x` if no long), `Short`, `Aliases []string`, `TakesValue`, `Description` | Go type in `parser.go`; proto `FlagInfo` |
| `ProbeStatus` | Enum: `FOUND_PARSED`, `FOUND_NO_FLAGS`, `NOT_FOUND`, `TIMEOUT`, `ERROR`, `BUSY`, `NEEDS_CONFIRM` | Proto enum mirrored by a Go type; exhaustive switch in `probeResultToProto`. `NEEDS_CONFIRM` = found but not executed (script, or resolve-only with no cached result) |
| `Prober` | Struct in `config/clihelp/prober.go` holding injected `lookPath`, `run`, `stat`, `readHead`, `home`, `loginPath`, cache, confirmed-set, semaphore, singleflight group | `NewProber(opts...)`; built in `NewDefaultsService()` with login-PATH derivation **off**; production wiring starts it explicitly (Task 1.1.4d) |
| `Limits` | `{Timeout time.Duration; MaxBytes int}`; defaults 3s / 256KiB; per-tool override via `slowTools` if Task 1.1.3a measurement requires (Flagged Choice 11) | Passed to `RunFunc` on every call so tests can assert plumbing |
| `RunFunc` | `func(ctx context.Context, path ResolvedPath, lim Limits) (RunOutput, error)` | Injection seam; default is the package `Run` |
| `RunOutput` | `{Text HelpText; Truncated bool; TimedOut bool}` | Exit code intentionally absent |
| `runSpec` | Unexported `{path; args; extraEnv; lim}` consumed by `runWith` | Test-only seam (task 1.1.2b): production `Run` always builds `args=["--help"]`, `extraEnv=nil` |
| `capWriter` | `io.Writer` keeping the first N bytes, sets `truncated`, always returns `len(p), nil`, calls `onOverflow` once | Shared by stdout and stderr (`cmd.Stdout == cmd.Stderr`, so `os/exec` serializes writes; no mutex) |
| `cacheKey` | `{RealPath string; MtimeNanos int64; Size int64}` | Comparable struct |
| `ProbeResult` | `Status`, `ResolvedPath`, `Flags`, `Truncated`, `IsWrapper` | Mapped 1:1 to `ProbeProgramResponse` |
| `ProbeGuard` | HTTP middleware in `server/middleware` wrapping the `ProbeProgram` procedure only; installed in `Start()`'s chain only (never `StartRemote()`), when `authMiddleware == nil` | POST-only, loopback Host/Origin (origins read lazily at request time) |
| `useProbeProgram` | React hook: `(command) => {state, check()}` with request token + AbortSignal | `state` is `ProbeUiState` |
| `ProbeUiState` | Union: `idle`, `checking`, `found{path, flags, status, isWrapper}`, `needsConfirm{path}`, `notFound`, `busyOrError`, `transportError` | Transport failure and ERROR/BUSY are never rendered as not-found |
| `ProbeStatusBadge` | Shared component: icon + text in a `role="status"` region | Used by ProgramsManager and ProgramProbeSection |
| `ProgramProbeSection` | Component owning hook + badge + saved-flag warning for one `ProgramOption` | The only addition to `OmnibarCreationPanel.tsx` is one JSX element |
| `FlagCombobox` | Always-rendered single `<input>` with ARIA 1.2 combobox attributes toggled | Never swapped for a plain input |
| `FlagInfoButton` | 44px `aria-expanded` disclosure button toggling an inline description | Placed outside `role=option` (warnings list, "Available flags" disclosure) |
| `unknown flag` | A `-`-token in cli_flags whose name is not in the parsed set (after `--x=v`, `-abc`, `--no-x`, `--`, alias handling) | Warning only |

Glossary term count: 24 (recounted with awk over this table).

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Approach: probe design | Server-side prober + client hook, pure parser at the core | Ports and Adapters | (B) Client-side curated flag specs; (C) LLM parses help text | B lags flag churn; C adds latency, cost, egress (`research/build-vs-buy.md`). Precision limited by heuristics, so warnings stay soft |
| `clihelp.ParseHelp` | Pure function, table-driven line handlers | Fowler (Transaction Script) | Third-party parser | No OSS lib parses arbitrary help; keeps gocognit below 40 |
| `Prober` | Service with injected function fields (`lookPath`, `run`, `stat`) | GoF Strategy via func values | Reuse `config.CommandExecutor` (`config/executor.go:13`) | No size cap, stdin or env control |
| Runner | Adapter over `safeexec.CommandContextPG` plus unexported `runWith(runSpec)` seam | GoF Adapter | `executor.ShortLivedCmd`; edit `sigkillGrace` | ADR-002; seam makes the re-exec tests reachable |
| Probe cache | Bounded map + TTL under mutex; `singleflight.DoChan` on `context.WithoutCancel` | PoEAA Identity-Map-like | Unbounded `sync.Map`; leader on request ctx | Shims never change mtime; a cancelled leader would poison followers and the cache |
| `ResolvedPath`, `HelpText` | Newtypes | type-driven-design | raw `string` | Prevent passing user input where a resolved path is expected |
| `ProbeStatus`, `ResolveError` | Sum types (typed consts, exhaustive switch; proto enum) | type-driven-design | booleans plus message string | UI must distinguish states; compile-checked mapping; `found` derived in one place |
| `ProbeProgram` handler | Service Layer, thin adapter delegating from `SessionService` | PoEAA | Logic inside `DefaultsService` | `defaults_service.go` is 794 lines, 16 commits/6mo |
| Request guard | HTTP middleware scoped to one procedure | Chain of Responsibility | Connect interceptor | Interceptors do not see the `Host` header (`r.Host`); middleware does |
| `ProbeUiState` | Discriminated union | type-driven-design | `{loading, error, data}` booleans | Not-found vs error vs transport failure cannot be conflated |
| `useProbeProgram` | Custom hook, request token + `AbortController` | React idiom | Per-component fetch | Both forms need it; jscpd |
| Omnibar integration | `ProgramProbeSection` component | Extract Component | Inline hook/effects in the 1040-line panel | Hotspot containment (architecture review) |
| Descriptions on touch | Disclosure button outside options; inline description for the active option | WAI-ARIA disclosure | Radix Tooltip; button inside option | Tooltips are hover-only; nested-interactive axe rule blocks CI |

---

## Tech Debt Disposition
*(from `research/architecture.md`, "Prior analysis and hotspots")*

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `server/services/defaults_service.go` (794 lines, 16 commits/6mo) | Mid-sized, growing service | **Isolate via seam** | All logic in `config/clihelp`; file gains one thin handler, one field, one setter (task 1.2.2a) |
| `server/services/session_service.go` (5000+ lines) | God object with per-RPC delegation | **Isolate via seam** | One 3-line delegating wrapper only (task 1.2.2b) |
| `web-app/src/components/sessions/OmnibarCreationPanel.tsx` (1040 lines, 36 commits) | Real hotspot | **Isolate via seam** | Gains exactly one JSX element (`<ProgramProbeSection>`) at `:951`; all state, effect and validation live in `ProgramProbeSection` (tasks 2.3.2a-b, sequenced before any panel edit) |
| `server/server.go` | Large wiring file | **Isolate via seam** | One guard wrap around `inner` in `Start()` (`:1374-1377`) only, not at route registration (`:414`, shared mux) and not in `StartRemote()` (`:1643-1646`); logic in `server/middleware/probeguard.go` |
| `ProgramsManager.tsx` (486 lines, 1 commit) | Small, stable | **Extend as-is** | Adds hook + badge + combobox usage |
| `proto/session/v1/session.proto` (89 commits) | Large shared file | **Extend as-is** | Additive-only |

---

## Migration Plan
N/A: no schema or data changes. Proto change is additive; existing clients unaffected.

## Observability Plan
- **Audit log (AC8)**: exactly one `slog` line per probe attempt, emitted in `Prober.Probe` for every outcome (including rejected, not-found, busy, cache hit): `level=Info`, msg `program_probe`, fields `resolved_path` (or `""`), `command_token` (the first token after env-assignment skipping; never env values, never later args), `status`, `flags`, `duration_ms`, `cache_hit`, `truncated`, `is_wrapper`, `confirmed` (request had `confirm_execute`), `resolve_only`, `remote_addr` (from the guard via context). `NEEDS_CONFIRM` logs at Info like every other outcome. Not-found and cache hits are Info too, since the line is the audit record (ADR-002 forgoes the `executor` audit). A test asserts one line per call using a `slog` capture handler.
- **Metrics**: none new; the BUSY path logs at Warn.
- **Alerts**: none.

## Risk Control
- **Feature flag**: not gated. Justification: additive RPC, non-blocking UI, save never blocked. (Repo memory: rollout flags must be live-settable, not env vars; none introduced.)
- **Rollback procedure**: revert the PR; proto change is additive.
- **Staged rollout**: full rollout on merge.

## Unresolved Questions
- [ ] **ADR-001 owner sign-off: PENDING human reviewer sign-off (Tyler).** Implementation proceeds on the coordinator's decision; if sign-off changes the model, escalate to the additive option B. Blocks nothing in the plan.
- [x] ~~Which PATH do tmux sessions launch with~~ Resolved: Flagged Choice 10 (login-shell PATH derived once + server PATH; alias-only reported NOT_FOUND with explanatory copy). Remaining verification (implementer, task 1.1.4d): confirm on the dev machine that the derived PATH contains `~/.local/bin`.
- [ ] `git --help` under a pipe may spawn `man`; with the sanitized env (`PAGER=cat`, `MANPAGER=cat`) verify and, if it does, use `git -h` output as the negative fixture — blocks task 1.1.3a; owner: implementer.
- [ ] `preset-program-warning` (`OmnibarCreationPanel.tsx:952`) has no other references in `web-app/src` or `tests/e2e` (architecture review, verified); default: keep the same `data-testid` on the not-found variant — nothing blocks.
- [ ] **AC7 `extraFlags` scoped down** (Flagged Choice 2): needs a one-line note in `requirements.md` AC7 or Tyler's acceptance; owner: coordinator. Default: plan ships saved-`cli_flags` validation only.
- [ ] Does `--help` for claude/aider/gemini write to `$HOME` (config, update checks)? UNVERIFIED (adversarial review). Implementer records the result during fixture capture (task 1.1.3a). If it does, point `HOME` at the temp dir for the child (lookup still uses the real home).
- [ ] `ProbeGuard` "is the listener loopback-bound" input: implementer reads `Server.httpServer.Addr`/`config.ListenAddress` (`config/config.go:212-214`, default `localhost:8543` at `:848`); exact accessor UNVERIFIED. Wiring point is now decided (Start() chain only, Story 1.2.3); only this accessor remains open. **Reachability note (pre-mortem #3):** if the UI is reached on `:8543` through a LAN or Tailscale hostname, that name must be allowed via `SetHostnames`, or the guard 403s every probe. The guard rejection must render as "Couldn't check right now." (never "not found"), which is covered by the hook's `PermissionDenied`/403 mapping test (Task 2.1.1b) and by `TestProbeGuard_should_Pass_When_HostPublishedViaSetHostnames` (Task 1.2.3b). Resolve this question in the first task of Story 1.2.3 and record which Host values the deployed instance's guard sees (manual instance with `STAPLER_SQUAD_INSTANCE`); log the rejecting reason and Host at Warn.
- [ ] **Measured `--help` timings** (Flagged Choice 11): the timing table is UNMEASURED until Task 1.1.3a runs; the timeout decision (keep 3s / `slowTools` override / `PENDING` status) is made there and recorded in that table. Blocks Task 1.1.4b's final TTL/limit values only.
- [ ] **`NEEDS_CONFIRM` first-check cost** (Flagged Choice 12): claude, gemini and aider are shebang scripts, so each needs one explicit Check the first time. Needs Tyler's acceptance together with ADR-001 sign-off (PENDING); if rejected, the fallback is an allow-list of known CLI basenames that skip confirmation, which is a security-model change and must go through ADR-001.
- [ ] **AC8 / AC2 / AC3 / AC6 / AC7 / AC10 interpretation** is recorded in `requirements.md` "Interpretation notes"; it is a reading, not an approval (PENDING human review).

## Dependency Visualization

```
Phase 1 (backend)
 1.1.1 resolve ─┐
 1.1.2 runner ──┼─► 1.1.4 prober ─► 1.2.2 handler+wrapper ─► registry
 1.1.3 parser ──┘   (1.1.4d loginPath)   ▲
 1.2.1 proto ─────────────────────────────┤
 1.2.3 ProbeGuard ────────────────────────┘
        │  (proto-gen makes TS client available)
        ▼
Phase 2 (binary status; AC5, AC7 badge)
 2.1.1 hook ─► 2.1.2 badge ─► 2.2.1 ProgramsManager
                          └─► 2.3.1 ProgramOption ext ─► 2.3.2 ProgramProbeSection ─► panel (1 line)
        ▼
Phase 3 (autocomplete; AC6 part 1)
 3.1.1 token helpers ─► 3.1.2 FlagCombobox ─► 3.1.3 wire in ProgramsManager
        ▼
Phase 4 (warnings + descriptions; AC6 part 2, AC7 saved flags, AC9)
 4.1.1 validateFlags ─► 4.1.2 wire warnings (PM + ProgramProbeSection)
 4.2.1 FlagInfoButton ─► 4.2.2 descriptions (inline active option + warnings list)
        ▼
Phase 5: e2e (5.1, proves AC9) ─► gates (5.2)
```

## AC-to-Task Traceability

| AC | Tasks |
|----|-------|
| 1 (RPC, fields) | 1.2.1a, 1.2.1b, 1.2.2a, 1.2.2c, 1.2.2e, 1.1.4a |
| 2 (LookPath, `~`, found=false not error) | 1.1.1a, 1.1.1b, 1.1.4a, 1.1.4c, 1.1.4d, 1.2.2c |
| 3 (3s timeout, no stdin, 256KB cap, no shell, cache by path+mtime) | 1.1.2a-d, 1.1.3a (timing go/no-go, smoke test), 1.1.4a-c |
| 4 (parser, aliases, empty not error) | 1.1.3a-d, 1.2.1a |
| 5 (Program Config indicator on blur) | 2.1.1a-b, 2.1.2a-b, 2.2.1a-b, 5.1a-b |
| 6 (autocomplete, warn on unknown, description) | 3.1.1a-b, 3.1.2a-c, 3.1.3a, 4.1.1a-b, 4.1.2a, 4.2.1a-b, 4.2.2a |
| 7 (session creation badge + flag validation; **scoped down: saved `cli_flags` only, `extraFlags` not validated**, see Unresolved Questions #5) | 2.3.1a, 2.3.2a-b, 4.1.2b |
| 8 (security; timeout and oversize tests; scripts need explicit Check) | 1.1.1a-b, 1.1.2a-d, 1.1.4b-g, 1.2.1a, 1.2.3a-b, 2.1.1a-b, 2.3.2a, 5.1a-b, ADR-001, ADR-002 |
| 9 (mobile tap, no hover) | 4.2.1a-b, 4.2.2a, 3.1.2a (44px rows), 5.1b (**e2e is the AC9 proof**; jest is a unit substitute only) |
| 10 (Go parser fixtures; jest tests) | 1.1.3a-d, 2.1.1b, 2.1.2b, 2.2.1b, 2.3.2c, 3.1.2b, 4.1.1b, 4.2.1b, 5.1a-b |

---

## Acceptance criteria with Given-When-Then

- **AC1**: *Given* a `Prober` built with fake `lookPath` returning `/usr/bin/aider` and fake `run` returning aider fixture text, wired into a real `DefaultsService` handler (task 1.2.2e), *When* the client calls `ProbeProgram{command:"aider --model x"}`, *Then* the response has `found=true`, `resolved_path="/usr/bin/aider"`, and `flags` containing `FlagInfo{name:"--model", takes_value:true, description:"Specify the model to use in the main chat"}` (asserted against the committed fixture).
- **AC2**: *Given* a `Prober` with `WithHome("/home/tyler")` and no `~/bin/nope` file, *When* `ProbeProgram{command:"FOO=1 ~/bin/nope --x"}` is called, *Then* the RPC succeeds with `found=false`, `probe_status=NOT_FOUND`, `flags=[]`, and `stat`/`lookPath` was called with `/home/tyler/bin/nope` (env prefix skipped, `~` expanded).
- **AC3**: *Given* the re-exec helper program (task 1.1.2b seam) in mode `bigout` (1MB to stdout and stderr) and mode `hang` (ignores SIGTERM, blocks), with `Limits{Timeout:80ms, MaxBytes:256KiB}`, *When* each runs through `runWith`, *Then* `bigout` returns `Truncated=true`, `len(Text)==262144` promptly; `hang` returns `TimedOut=true` within 500ms with its process group gone; *And* mode `flood` (infinite writer) with a 2s timeout returns `Truncated=true` in under 1s (early kill on overflow); *And* given a fake `stat`/`run`, a second `Probe` of an unchanged binary does not call `run` (cache hit) while a changed mtime does. The end-to-end 3s default is asserted as `DefaultLimits().Timeout==3*time.Second` and `MaxBytes==256<<10`; *And* if Task 1.1.3a's measurement adds a `slowTools` override, a probe of an overridden basename receives the raised `Limits` while others still get 3s; *And* a `TIMEOUT` result renders "Timed out reading flags — try Check again." (not the no-flags copy), is cached 60s, and an explicit Check bypasses it.
- **AC4**: *Given* `testdata/help/tmux.txt` (usage error, no options table), *When* `ParseHelp` is called, *Then* it returns an empty slice and no error; *And* for `testdata/help/claude.txt` it returns a flag with `Name:"--allowedTools"`, `Aliases:["--allowed-tools"]`, `TakesValue=true`.
- **AC5**: *Given* Program Config with `prog-command-input` focused and `probeProgram` mocked to return `{found:true, resolvedPath:"/usr/bin/claude", probeStatus:FOUND_PARSED}`, *When* the user types `claude` and blurs the field (or presses Enter, or clicks `prog-command-check`), *Then* `prog-command-status` (role=status) reads "Found: /usr/bin/claude" with a check icon and the detail "Checked on this server only"; for `NOT_FOUND` it reads "Not found as an executable on the server's PATH. Shell aliases and functions are not checked; the program may still work when launched from your shell." and Save remains enabled; Enter does not submit the form; *And* given a result of `NEEDS_CONFIRM` (script), *Then* the badge reads "Found: <path>. Not run yet: this program is a script. Select Check to run it with --help and read its flags." until `prog-command-check` (or Enter) is used, and blur alone never runs it.
- **AC6**: *Given* probed flags `[--model (takes value, "Model to use"), --verbose]` and `prog-flags-input` containing `--mo` with caret at end, *When* the user presses Down then Enter, *Then* `aria-activedescendant` pointed at option `--model` beforehand, the active option's description "Model to use" was displayed inline, and the value becomes `--model `; *And* given value `--verbos --model x`, *Then* `prog-flags-warning` (referenced by `aria-describedby`) reads "--verbos is not listed in `claude --help`" and no warning appears for `--model x`; *And* given a wrapper result (`is_wrapper=true`) no warning and no suggestions appear.
- **AC7 (scoped down)**: *Given* the omnibar picker on program `aider` whose saved `cli_flags` is `--yes-always --bogus` and whose probe returns flags including `--yes-always`, *When* the Program select changes to `aider` (a resolve-only request; nothing is executed), *Then* a `role=status` badge shows the resolved path (with a `Check` control if the result is `NEEDS_CONFIRM`), and after an explicit Check (or a cached prior result) a warning says "--bogus is not listed in `aider --help`"; selecting a program whose binary is missing shows the not-found badge in place of the old `preset-program-warning` text. Alias `extraFlags` are **not** validated (Flagged Choice 2).
- **AC8**: *Given* `command="claude; rm -rf ~"`, *When* probed, *Then* the token is the literal `claude;`, no shell runs (the login-PATH derivation script is a constant), and the result is `NOT_FOUND`; *And* `./x`, `bin/x`, and `../x` return `NOT_FOUND` (relative paths never resolved) and a directory, a non-executable file and a world-writable file return `NOT_FOUND`; *And* `helpSpec(path, lim)` yields `args==["--help"]` and `env` derived by `probeEnv(parent)` from a parent environ containing `GITHUB_TOKEN`/`ANTHROPIC_API_KEY` contains neither; *And* the helper `printenv` mode run through `runWith` reports no `GITHUB_TOKEN` and a cwd that is an empty temp dir; *And* a non-POST request or a request with `Host: evil.example` or `Origin: https://evil.example` to the guarded procedure on the unauthenticated listener gets 405/403 and never reaches the handler; *And* exactly one `program_probe` log line is emitted per call; *And* given a `#!` shebang script whose body writes a marker file, *When* probed without `confirm_execute` (blur) or with `resolve_only`, *Then* the status is `NEEDS_CONFIRM`, `found=true`, `resolved_path` is set, and the marker file does not exist; *When* the same script is probed with `confirm_execute=true`, *Then* the marker file exists and flags are parsed; *And* a native binary (ELF/Mach-O magic) is run on an implicit probe.
- **AC9**: *Given* a Playwright context with `viewport 375x667`, `hasTouch: true`, and flags with descriptions, *When* the user taps `prog-flag-info-button` next to a warned flag or in the "Available flags" disclosure, *Then* the description appears inline, `aria-expanded="true"`, and the button's bounding box is at least 44x44. The jest `FlagInfoButton` test (click only, no `mouseover`) is a unit-level substitute and does not by itself satisfy AC9; task 5.1b blocks AC9 sign-off.
- **AC10**: *Given* the committed fixtures (claude, aider, gh, gh-pr-list, rg, uv, gemini, agy, plus negatives git and tmux), *When* `go test ./config/clihelp -race` and `cd web-app && pnpm exec jest --testPathPatterns="useProbeProgram|ProbeStatusBadge|ProgramProbeSection|FlagCombobox|ProgramsManager|FlagInfoButton|flagTokens|validateFlags" --no-coverage` run, *Then* both pass.

---

## Phase 1: Backend (`config/clihelp`, proto, handler, guard)

### Epic 1.1: `config/clihelp` package
**Goal**: A pure, injectable, hardened probe with a never-fails parser (AC1-4, AC8).

#### Story 1.1.1: Command resolution
**As a** server, **I want** to reduce a raw command string to one binary token, **so that** nothing else in the string is ever executed and only bare names or absolute paths are ever looked up.
**Acceptance Criteria**:
- Env prefixes skipped, `~`/`~/` expanded, quotes handled, invalid tokens rejected (AC2, AC8)
  - *Given* `FOO=1 "~/my bin/x" --y`, *When* `Resolve` runs with home `/h`, *Then* `Target.Name=="/h/my bin/x"`, `IsAbsolute`, and no args are retained.
- Relative paths rejected (AC8)
  - *Given* `./x`, `bin/x`, `../x`, `~bob/x`, *When* `Resolve` runs, *Then* it returns `RELATIVE_PATH` and lookup is never attempted.
- Wrappers flagged (Flagged Choice 7)
  - *Given* `env -u CLAUDE_CODE_USE_BEDROCK ANTHROPIC_BASE_URL=x claude` and `npx foo`, *When* resolved, *Then* `Wrapper==true` for both (first real token `env`/`npx`; `env` with only assignments prefix still skipped first).
**Files**: `config/clihelp/resolve.go`, `config/clihelp/resolve_test.go`

##### Task 1.1.1a: Implement `Resolve` (~5 min)
- `Resolve(command, home string) (Target, ResolveError)`; zero `ResolveError` means success and `Target` is only meaningful then. Skip leading `NAME=value` tokens; quote-aware tokenizer (unbalanced -> `UNBALANCED_QUOTE`); reject NUL/newline, empty, leading `-`; expand `~` and `~/` only (`~user` -> `RELATIVE_PATH`). After expansion the token must either contain no path separator (bare name) or be absolute (`filepath.IsAbs`); otherwise `RELATIVE_PATH`. `Wrapper` set when `filepath.Base(name)` is in the wrapper set. `home` is a parameter (injectable; AC2). Helpers `splitTokens`, `skipEnvAssignments`, `expandHome` keep gocognit under 40.
- Files: `config/clihelp/resolve.go`

##### Task 1.1.1b: Table tests (~4 min)
- Cases: empty, only-assignments, `FOO=1 claude`, `claude; rm -rf ~` (literal token), `-x`, NUL, unbalanced quote, `~`, `~/bin/t`, `~bob`, quoted path with space, `./x`, `bin/x`, `../x`, absolute path, `env ... claude` and `npx` wrapper flags, the built-in Proxy command from `programs.ts:16`.
- Files: `config/clihelp/resolve_test.go`

#### Story 1.1.2: Bounded runner with a reachable test seam
**As a** server, **I want** a runner that executes `<path> --help` with a hard limit, sanitized env and a 256KB cap, **so that** no probe can hang, leak secrets, or exhaust memory (AC3, AC8), **and** every one of those properties is testable against a real child process.
**Acceptance Criteria**:
- Oversized output truncated, drained or killed, flagged (AC3, AC8)
  - *Given* the helper in `bigout` mode via `runWith`, *When* run with cap 256KiB, *Then* `Truncated=true`, `len(Text)==262144`, and it returns without timing out; *Given* `flood` mode and a 2s timeout, *Then* it returns in under 1s (killed on overflow).
- Hung process killed with its group; bounded wall time (AC3, AC8)
  - *Given* `hang` mode with timeout 80ms, *Then* `TimedOut=true`, return under 500ms, and `syscall.Kill(-pid, 0)` eventually yields ESRCH; *Given* `orphan` mode (child exits immediately leaving a grandchild in the group), *Then* after `runWith` returns the group is gone (kill-after-Wait on the normal path).
- Environment, cwd, session (AC8)
  - *Given* `printenv` mode, *Then* (with the poisoned `runSpec.parentEnv`) output contains none of the parent's non-allowlisted vars and cwd is an empty temp dir removed after the run; *Given* `sid` mode, *Then* `Getsid(0)==Getpid()` (child is its own session leader, no controlling terminal).
**Files**: `config/clihelp/capwriter.go`, `config/clihelp/runner.go`, `config/clihelp/runner_unix.go`, `config/clihelp/runner_other.go`, `config/clihelp/runner_test.go`, `config/clihelp/runner_helper_test.go`

##### Task 1.1.2a: `capWriter` (~3 min)
- `capWriter{buf bytes.Buffer; max int; truncated bool; onOverflow func()}`; `Write` appends up to remaining, on first overflow sets `truncated` and calls `onOverflow` once, always returns `len(p), nil` so the child never blocks. One instance is assigned to both `cmd.Stdout` and `cmd.Stderr`; `os/exec` copies via a single goroutine when the writers are identical, so no mutex is needed (add a comment; verify with `-race`).
- Files: `config/clihelp/capwriter.go` (+ `capwriter_test.go`)

##### Task 1.1.2b: `runWith` seam + `Run` (~6 min)
- Types: `Limits{Timeout time.Duration; MaxBytes int}`, `DefaultLimits()` (3s, 256<<10), `RunFunc` (glossary). `limitsFor(base Limits, basename string) Limits` applies the `slowTools` override (empty until Task 1.1.3a's measurement justifies entries); `Prober` passes its result to `run`. Unexported `runSpec{path ResolvedPath; args []string; extraEnv []string; parentEnv []string; inheritEnv bool; lim Limits}`. `parentEnv` is the environ `runWith` feeds to `probeEnv` (nil means `os.Environ()`, the production value); tests set it to a poisoned list so the no-leak proof runs through the real runner. `inheritEnv` is used only by `deriveLoginPath` (Task 1.1.4d).
- `helpSpec(path, lim) runSpec` returns `args = []string{"--help"}` (package-level `var helpArgs`-free literal; not configurable) and `extraEnv=nil`. Exported `Run(ctx, path, lim)` is exactly `runWith(ctx, helpSpec(path, lim))`. Nothing in the RPC path can set `args` or `extraEnv`; only `_test.go` files construct a `runSpec` directly. `runWith` accepts `extraEnv` entries only with the prefix `CLIHELP_TEST_` (others dropped), which is the "permitted marker var" the helper needs.
- `probeEnv(parent []string, path string, loginPath string) []string`: allowlist `PATH`(=loginPath), `HOME`, `TERM=dumb`, `NO_COLOR=1`, `CI=1`, `COLUMNS=200`, `LANG=C.UTF-8`, `LC_ALL=C.UTF-8`, `PAGER=cat`, `MANPAGER=cat`; nothing else. Takes the parent environ as a parameter so tests need no `t.Setenv`.
- `runWith`: `ctx, cancel := context.WithTimeout(ctx, lim.Timeout)`; `cmd := safeexec.CommandContextPG(ctx, string(spec.path), spec.args...)`; then override `cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}` (Setsid also creates a new process group with pgid==pid; drops `Setpgid` to avoid the SIGTTIN case documented at `executor/safeexec/safeexec_pg.go:29-31`); `cmd.Dir` = `os.MkdirTemp` dir removed after; `cmd.Env = probeEnv(spec.parentEnv or os.Environ() when nil, ...) + test extraEnv` (or the full parent env when `inheritEnv`); `cmd.Stdin = nil`; `cmd.Stdout = cmd.Stderr = cw`; `cmd.Cancel = killGroup` (immediate SIGKILL to `-pid`, ignoring ESRCH); `cmd.WaitDelay = 200*time.Millisecond` (overrides safeexec's 2s `DefaultWaitDelay`, `executor/safeexec/safeexec.go:26`, so worst-case wall time is about `Timeout + 200ms`); `cw.onOverflow = killGroup` (early kill on cap overflow, so `yes` cannot burn the full timeout). After `Wait` returns, on every path, call `killGroup` once more (grandchildren surviving a normal exit). `TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)`; a `context.Canceled` result returns a non-nil error (never partial success). Ignore exit code unless the process failed to start.
- `runner_unix.go` (`//go:build !windows`): `killGroup(cmd)`; `runner_other.go`: closure `func() error { return cmd.Process.Kill() }` (assigned after `Start` or wrapped so `Process` is non-nil at call time).
- Files: `config/clihelp/runner.go`, `config/clihelp/runner_unix.go`, `config/clihelp/runner_other.go`

##### Task 1.1.2c: Re-exec helper (~4 min)
- `runner_helper_test.go` (`//go:build !windows`, package `clihelp`): `TestMain` checks `os.Getenv("CLIHELP_TEST_HELPER")`; when set it runs the mode and `os.Exit`s, otherwise `m.Run()`. Modes: `bigout` (1MB to stdout and stderr, exit 3), `flood` (write until killed), `hang` (ignore SIGTERM, print `pid=<n>` and flush, block), `orphan` (start a grandchild re-exec in `hang` mode, print its pid, exit 0), `printenv` (print sorted env keys and `Getwd`), `sid` (print `Getsid(0)`, `Getpid()`). Tests call `runWith(ctx, runSpec{path: ResolvedPath(os.Args[0]), args: []string{"-test.run=^$"}, extraEnv: []string{"CLIHELP_TEST_HELPER=<mode>"}, lim: ...})`. This re-exec path is the same convention as `executor/safeexec/safeexec_sigkill_helper_test.go:1-20`, with the marker var passed through the permitted prefix because the sanitized env would otherwise drop it.
- Files: `config/clihelp/runner_helper_test.go`

##### Task 1.1.2d: Runner tests (~5 min)
- In `runner_test.go` using the helper: truncation and exact length, `TimedOut` within 500ms, `flood` early-kill under 1s, `orphan` group gone after return (`require.Eventually` polling `syscall.Kill(-pid,0)==ESRCH`, no `time.Sleep`), `printenv` (non-vacuous: the test passes `runSpec.parentEnv` containing `GITHUB_TOKEN=leak`, `ANTHROPIC_API_KEY=leak` plus the helper marker via `extraEnv`, runs the `printenv` mode through the real `runWith`, and asserts the child's printed env keys contain neither; a control assertion proves the poison was present in `parentEnv` and that an allowlisted key like `HOME` did arrive; plus a pure `probeEnv(parent)` test), cwd empty and removed, `sid` equals pid, cancelled ctx returns an error (not success). Plus pure tests: `helpSpec` returns args exactly `["--help"]` and nil extraEnv; `probeEnv` drops `GITHUB_TOKEN`/`ANTHROPIC_API_KEY`; `runWith` drops an `extraEnv` entry without the `CLIHELP_TEST_` prefix; one smoke test of the real exported `Run` against the test binary with `--help` proves the production path reaches a real process (Go test binary answers `--help` with usage text containing `-test.run`).
- Files: `config/clihelp/runner_test.go`

#### Story 1.1.3: Help parser
**As a** user, **I want** `--help` text turned into a flag list, **so that** autocomplete and validation have data; unparseable text must yield `[]`, never an error (AC4).
**Acceptance Criteria**:
- Golden fixtures parse to exact expected flags (AC4, AC10)
  - *Given* `testdata/help/gh-pr-list.txt`, *When* `ParseHelp` runs, *Then* it returns `{Name:"--assignee", Short:"-a", TakesValue:true, Description:"Filter by assignee"}` among others.
- Aliases (AC4)
  - *Given* `testdata/help/claude.txt`, *Then* `--allowedTools` carries `Aliases:["--allowed-tools"]` (long-long aliases are aliases on one entry, never separate entries).
- Negatives and adversarial input are safe (AC4)
  - *Given* a 256KB single line of `-` characters, *When* `ParseHelp` runs, *Then* it returns within 100ms with an empty slice.
**Files**: `config/clihelp/parser.go`, `config/clihelp/parser_lines.go`, `config/clihelp/parser_test.go`, `config/clihelp/parser_fuzz_test.go`, `config/clihelp/testdata/help/*.txt`

##### Task 1.1.3a: Capture fixtures (~5 min)
- Run real `--help` (stdout+stderr merged, `NO_COLOR=1`, `PAGER=cat`, `MANPAGER=cat`) for: `claude` (commander), `aider` (argparse), `gh`, `gh pr list` (cobra), `rg` and `uv` (clap), `gemini` (yargs), `agy` (Go `flag`), plus negatives `tmux` and `git` (see Unresolved Questions). Record whether any of them wrote under `$HOME` (Unresolved Questions #6). Name `<tool>-<version>.txt`; add one ANSI-coloured variant. Capture command noted in a `parser_test.go` comment only.
- **Timing go/no-go (pre-mortem P1-1, Flagged Choice 11).** For each of claude, aider, gemini, agy and gh, record wall time of `<tool> --help` cold (first run after `sync; echo 3 > /proc/sys/vm/drop_caches`-equivalent or a fresh boot, or the first run after the tool was last untouched; note which) and warm (median of 5), using the sanitized env, into the "Measured `--help` timings" table in this plan. Decision rule: if any target exceeds about 1.5s cold, add the `slowTools` per-basename `Limits.Timeout` override (Task 1.1.2b), or the `PENDING` status if no bound is sensible; AC3's 3s stays the default unless the measurement contradicts it. Record the decision in the table and repair log. Also record which of the five are shebang scripts (`head -c2`), since those need `NEEDS_CONFIRM` (Flagged Choice 12).
- **Real-binary smoke test**: `TestProbe_should_ReturnFoundParsed_When_RealClaudeOrAiderInstalled` in `config/clihelp/prober_realbin_test.go` builds a real `Prober` with `confirm_execute=true`, iterates claude, aider, gemini if `exec.LookPath` finds them, and asserts `FOUND_PARSED` within the configured limit; `t.Skip` when none is installed (so CI without the tools stays green). Never runs on picker or blur paths.
- Files: `config/clihelp/testdata/help/*.txt`, `config/clihelp/prober_realbin_test.go`

##### Task 1.1.3b: `ParseHelp` core (~5 min)
- Precision-first: candidate line only if, after `stripANSI`, indentation <= 8 and it starts with `-`. Helpers: `stripANSI` (CSI/OSC, `\r`, `X\b` overstrike), `splitLines` (bufio.Scanner with enlarged buffer; skip lines >2KB), `parseFlagLine` (`-x, --long`, `--long, -x`; `--[no-]long` emits `--long` with alias `--no-long`; extra long forms go to `Aliases`), `valueHint` (`<v>`, `=V`, ALLCAPS, cobra type word, `[=V]`; unknown -> false), `joinContinuation`, `dedupe` (prefer entry with description). Caps: 500 flags, description 300 chars. Never returns error.
- Files: `config/clihelp/parser.go`, `config/clihelp/parser_lines.go`

##### Task 1.1.3c: Golden and negative tests (~5 min)
- Table test: fixture -> expected `[]Flag` subset (names, short, aliases, takes_value) and minimum count; `tmux`/`git` expect empty; every name and alias matches `^--?[A-Za-z0-9][\w-]*$`.
- Files: `config/clihelp/parser_test.go`

##### Task 1.1.3d: Fuzz and adversarial (~3 min)
- `FuzzParseHelp` (no panic, bounded result, name-regex invariant) plus adversarial cases (256KB of `-`, no newlines, 10k tiny lines) with a 100ms bound.
- Files: `config/clihelp/parser_fuzz_test.go`

#### Story 1.1.4: Prober (cache, singleflight, semaphore, PATH)
**As a** server, **I want** `Probe(ctx, command)` to compose resolve, lookup, cache, run and parse, **so that** the handler stays thin (AC1-3, AC8).
**Acceptance Criteria**:
- Found/not-found mapping and caching (AC2, AC3)
  - *Given* fake `lookPath` returning not found, *When* `Probe("nope")` is called twice, *Then* both return `NOT_FOUND`, `run` is never called and nothing is cached.
  - *Given* a found binary probed twice with unchanged mtime, *Then* `run` was called once; after the fake `stat` mtime changes it is called again.
- Execution gate (AC8, Flagged Choice 12)
  - *Given* a shebang script target, *When* `Probe` is called without `ConfirmExecute` or with `ResolveOnly`, *Then* the status is `NEEDS_CONFIRM` and `run` is never called; *When* called with `ConfirmExecute`, *Then* `run` is called once and later implicit probes of the same (path, mtime, size) run; a native binary runs implicitly.
- Limits plumbing (AC3)
  - *Given* `WithLimits(Limits{Timeout:80ms, MaxBytes:1024})` and a fake `RunFunc`, *When* `Probe` runs, *Then* the fake received exactly that `Limits`.
- Cancellation safety (architecture blocker)
  - *Given* a fake `run` that blocks until released, *When* the caller's ctx is cancelled mid-probe, *Then* that call returns `ERROR` promptly and nothing is cached for the cancelled call; *When* the fake is then released and the command re-probed, *Then* the result is the real parsed result (never a sticky `ERROR`/`FOUND_NO_FLAGS`), and a follower that joined the same flight also receives the real result.
- Concurrency and busy (AC3)
  - *Given* 10 concurrent probes of one uncached binary under `-race`, *Then* `run` is called once, all 10 get the real result (none `BUSY`: followers do not consume slots), and at most 2 distinct probes run in parallel across binaries; *Given* 2 blocked flights holding both slots whose callers are all cancelled, *Then* a third distinct-binary probe still gets `BUSY` (a cancelled caller does not free the slot; only flight completion does); *Given* the semaphore is saturated, *Then* an extra distinct-binary probe returns `BUSY` immediately and a retry after a slot frees returns a real result (BUSY is never cached).
- PATH (Flagged Choice 10)
  - *Given* an executable present only in a directory returned by the injected login-PATH function (not in the fake server PATH), *When* `Probe` runs, *Then* it is found and `run` receives that path; *Given* a command that exists only as a shell alias (no executable), *Then* `NOT_FOUND`.
**Files**: `config/clihelp/types.go`, `config/clihelp/prober.go`, `config/clihelp/cache.go`, `config/clihelp/loginpath.go`, `config/clihelp/prober_test.go`, `config/clihelp/prober_confirm_test.go`, `config/clihelp/loginpath_test.go`

##### Task 1.1.4a: `Prober` core (~6 min)
- `NewProber(opts ...Option)`; options `WithLookPath`, `WithRun(RunFunc)`, `WithStat`, `WithLimits`, `WithClock`, `WithHome`, `WithLoginPath(func() []string)`; defaults: `lookInDirs(name, LoginPath)` for bare names (skips non-regular, non-executable), `Run` (as the default `RunFunc`; the prober always passes its own `Limits` on each call), `os.Stat`, `os.UserHomeDir`.
- `Probe(ctx, command, ProbeOpts{ConfirmExecute, ResolveOnly bool})`: `Resolve` (failure -> `NOT_FOUND`) -> if absolute use it, else `lookPath` (treat `exec.ErrDot` as not found) -> `checkExecutable(path)` (regular file, executable bit, `mode&0o002==0` i.e. not world-writable; failure -> `NOT_FOUND`) -> `filepath.EvalSymlinks` -> stat -> if `Wrapper`, return `FOUND_NO_FLAGS` with `IsWrapper=true` **without running** -> cache lookup (a hit is returned even for `ResolveOnly`) -> **execution gate (Task 1.1.4f): if `ResolveOnly`, or the target is not native and not in the confirmed set and `ConfirmExecute` is false, return `NEEDS_CONFIRM` with `found` path, without running** -> flight (see 1.1.4b; the non-blocking 2-slot semaphore is acquired **inside the flight body, leader only**; full -> the flight returns `BUSY`, shared with followers, not cached; the slot is released when the flight ends, not when a caller cancels) -> `ParseHelp` -> status (`TIMEOUT` if timed out; `FOUND_PARSED` if flags>0; else `FOUND_NO_FLAGS`; run start failure or `context.Canceled` -> `ERROR`). One audit log line per call (Observability Plan). Call-site `found` is not stored; it is derived in `probeResultToProto` only.
- Files: `config/clihelp/types.go`, `config/clihelp/prober.go`

##### Task 1.1.4b: Cache + singleflight (~5 min)
- `cache.go`: mutex-guarded map by `cacheKey`, max 128 entries (evict oldest), TTL 10 min for `FOUND_PARSED`/`FOUND_NO_FLAGS`, 60s for `TIMEOUT` (lengthened from 30s so a slow tool is not re-run every half minute; a probe with `ConfirmExecute` bypasses a cached `TIMEOUT`, so the user's Check always retries); `NOT_FOUND`, `ERROR`, `BUSY` and `NEEDS_CONFIRM` are never stored. After `run`, re-stat and skip storing if mtime changed (TOCTOU note).
- `singleflight.Group` (`golang.org/x/sync`, `go.mod:222` indirect; `go mod tidy` makes it direct) keyed by the cache key string, using `DoChan`. The flight body first try-acquires the semaphore slot (leader only; followers never touch it, so duplicate probes of one binary share one slot and one result, and cancelled callers cannot exceed the 2-process cap because the slot is tied to the flight's lifetime), then runs `run` with `context.WithoutCancel(ctx)` (its own `Limits.Timeout` bounds it), releases the slot on return, stores the result, and returns it; each caller `select`s on the channel or its own `ctx.Done()`. A caller whose ctx is done returns `ERROR` (`context.Canceled` neither sets `TimedOut` nor caches). The flight still completes and caches its valid result for the next caller.
- Files: `config/clihelp/cache.go`, `config/clihelp/prober.go`, `go.mod`, `go.sum`

##### Task 1.1.4c: Prober tests (~6 min)
- Fakes only (no process): found/not-found, `ErrDot`, directory, non-executable, world-writable, relative path, cache hit/mtime invalidation/TTL via fake clock, `TIMEOUT` 60s TTL and Check-bypass, `NOT_FOUND`/`ERROR` never cached, singleflight count, semaphore saturation -> `BUSY` then retry succeeds, 10 concurrent same-binary probes all get the real result (leader-only slot), cancelled callers do not free a slot early, **flight exits after all callers cancel** (fake released, then `goleak.VerifyNone`/`require.Eventually` on a flight-done signal or `runtime.NumGoroutine` baseline, no sleeps), **cancel-then-reprobe**, follower-of-cancelled-leader, limits plumbing, wrapper skip (`run` never called for `env ... claude` and `npx`), first-token-only (`claude --dangerously-skip-permissions` runs `--help` only), one audit log line per call via a slog capture handler. Run `go test ./config/clihelp -race`.
- Files: `config/clihelp/prober_test.go`

##### Task 1.1.4d: Login-shell PATH (~5 min)
- `loginpath.go`: `deriveLoginPath(ctx, shell string) []string` mirrors `config/config.go:1213-1221`: for zsh `source ~/.zshrc &>/dev/null || true; <print PATH between sentinels>`, for bash the `~/.bashrc` variant, otherwise just the sentinel print; the script is a constant (no user text). It runs through the **same runner machinery as the probe** (`runWith` with a `runSpec{path: shell, args: ["-c", script], lim: {Timeout: 2s, MaxBytes: 64KiB}}`: `Setsid`, group SIGKILL on timeout and after exit, stdin nil, capped output), not bare `safeexec.CommandContext`, so `.zshrc` background helpers (nvm, ssh-agent, direnv) cannot outlive the timeout or hold the pipe. Unlike the probe, this spec keeps the user's real env (the shell needs `HOME`, etc.), so `runSpec` carries an `inheritEnv bool` used only by `deriveLoginPath` (never set by `helpSpec`; test asserts `helpSpec` leaves it false). **Caching**: no `sync.Once`. A `loginPathCache` stores only a successful result (with a 10 min TTL); a failure or timeout is not cached and is retried at most once per 30s (fake clock in tests). **Hermetic by default (pre-mortem #4):** `NewDefaultsService()` and `NewProber()` do **not** start derivation; the default `loginPath` is server PATH plus fallback dirs, so tests never spawn a shell or read a real rc file. Production wiring calls an explicit `prober.StartLoginPathDerivation()` from the point where `main.go` wires the service (next to the `SetOrigins`/`SetHostnames` calls, `main.go:435-450`, or `BuildDependencies`; implementer picks and records it). A probe arriving before it finishes uses the server PATH plus fallback dirs for that call only. **Sentinel-delimited output:** the constant script is `source ~/.zshrc &>/dev/null || true; printf '__CLIHELP_PATH_START__%s__CLIHELP_PATH_END__' "$PATH"` (bash and plain variants likewise); only the span between the markers is parsed, and the result is discarded (treated as failure, retried per the 30s rule) if either marker is missing, so rc stdout noise (p10k prompt, `echo`, nvm banners) cannot inject garbage directories. Result merged: shell dirs first, then `os.Getenv("PATH")` dirs, then (always appended) fallback dirs `~/.local/bin` and `/usr/local/bin` if absent, deduped. **`$SHELL` unset or `/bin/sh`**: skip shell derivation (script is just the sentinel `$PATH` print under `/bin/sh`, no rc sourcing) and use server PATH + fallback dirs; do not assume bash (a deliberate difference from `config.go:1281-1284`, which defaults to `/bin/bash`). **bash early-return**: `source ~/.bashrc` non-interactively often returns early (`[ -z "$PS1" ] && return`); tolerated, as `|| true` already ignores it, and the result then degrades to server PATH + fallback dirs (documented in Not-found copy caveat). `shell` is a parameter so tests do not touch the environment. Expose the merged list as the child `PATH` too.
- Files: `config/clihelp/loginpath.go`

##### Task 1.1.4e: Login-PATH tests (~4 min)
- `loginpath_test.go`: a stub `zsh`-named shell script in `t.TempDir()` echoing a fixed PATH containing a temp bin dir; assert the dir is included first and deduped with server PATH; failing/timed-out shell -> falls back to server PATH + fallback dirs and the failure is **not cached** (next call after 30s on the fake clock re-derives and succeeds); a stub shell that backgrounds `sleep` and exits does not delay return past the timeout and the group is gone afterwards; empty `$SHELL` -> fallback dirs (`~/.local/bin`, `/usr/local/bin` via `WithHome`) present, no rc sourcing; a stub bash whose rc returns early still yields server PATH + fallback dirs; prober test (1.1.4c) covers found-via-shell-PATH and alias-only NOT_FOUND. **Pollution and hermeticity**: a stub shell that prints banner text (and a fake `p10k` line) to stdout before the sentinel span yields only the span's directories; a stub that prints no markers is treated as failure (falls back, not cached); `TestNewDefaultsService_should_SpawnNoProcessAndNoGoroutine_When_Constructed` (in `defaults_service_probe_test.go`, with `goleak`) proves a test-constructed service starts no derivation, and only `StartLoginPathDerivation()` does.
- Files: `config/clihelp/loginpath_test.go`, `server/services/defaults_service_probe_test.go`

##### Task 1.1.4f: Execution gate (native vs script, confirm) (~5 min)
- `readHead(path) ([]byte, error)` injected via `WithReadHead` (default opens the file and reads 4 bytes; failure -> treat as not native). `isNative(head)`: ELF `7f 45 4c 46`, Mach-O `cf fa ed fe`/`ce fa ed fe`/`fe ed fa ce`/`fe ed fa cf`, fat `ca fe ba be` (all others, including `#!` and text, are non-native). `Prober` keeps a bounded `confirmed` set of `cacheKey` values (128, evict oldest, process lifetime, not TTL'd; a changed mtime/size is a different key so needs re-confirmation). Gate rules, in order: cache hit -> return it (`ResolveOnly` included); `ResolveOnly` -> `NEEDS_CONFIRM`; native -> run; in `confirmed` -> run; `ConfirmExecute` -> add to `confirmed`, run; else `NEEDS_CONFIRM`. `NEEDS_CONFIRM` is `found` (path returned), never cached, never runs `run`. Wrapper targets keep their existing never-executed path. Files: `config/clihelp/prober.go`, `config/clihelp/types.go`.

##### Task 1.1.4g: Execution-gate tests (~5 min)
- Fakes: shebang head -> `NEEDS_CONFIRM` and `run` never called on blur, on `ResolveOnly`, and on a repeated implicit probe; `ConfirmExecute` -> `run` called once, then a later implicit probe of the same key runs without confirmation (confirmed set) while a changed mtime returns `NEEDS_CONFIRM` again; native head runs implicitly; `ResolveOnly` with a cache hit returns the cached flags; `readHead` error -> non-native; `NEEDS_CONFIRM` not cached. **Real-process marker test** (`prober_confirm_test.go`, real `Run`, `WithLoginPath` injected, temp dir 0700): write an executable `#!/bin/sh` script that touches an absolute marker path; `Probe` (implicit) -> `NEEDS_CONFIRM` and `os.Stat(marker)` is `ErrNotExist`; `Probe` with `ConfirmExecute` -> marker exists. Also `ConfirmExecute` bypasses a cached `TIMEOUT` (fake clock).
- Files: `config/clihelp/prober_test.go`, `config/clihelp/prober_confirm_test.go`

### Epic 1.2: Proto, guard and RPC wiring
**Goal**: `ProbeProgram` reachable from the web client and safe to expose on `:8543` (AC1, AC2, AC8).

#### Story 1.2.1: Proto
**As a** client author, **I want** typed request/response messages, **so that** the UI can distinguish found, timeout, no-flags, busy and wrapper (`research/ux.md` section 4).
**Acceptance Criteria**:
- Additive proto (AC1)
  - *Given* the edited `session.proto`, *When* `make proto-gen && go build ./...` runs, *Then* it compiles and `probeProgram` exists on the generated TS client.
**Files**: `proto/session/v1/session.proto`

##### Task 1.2.1a: Edit proto (~4 min)
- Add `rpc ProbeProgram(ProbeProgramRequest) returns (ProbeProgramResponse) {}` after `DeleteProgramConfig` (`session.proto:571`); **do not** set `idempotency_level = NO_SIDE_EFFECTS` (that would enable Connect GET; ADR-001). Add after `ProgramConfigProto` (`:2526`): `ProbeProgramRequest{string command=1; bool confirm_execute=2; bool resolve_only=3}` (`confirm_execute`: the user explicitly asked to run this program's `--help`, required for scripts; `resolve_only`: resolve and stat only, never execute); `FlagInfo{string name=1; string short=2; bool takes_value=3; string description=4; repeated string aliases=5}`; `enum ProbeStatus{UNSPECIFIED=0; FOUND_PARSED=1; FOUND_NO_FLAGS=2; NOT_FOUND=3; TIMEOUT=4; ERROR=5; BUSY=6; NEEDS_CONFIRM=7}` (names prefixed `PROBE_STATUS_`); `ProbeProgramResponse{bool found=1; string resolved_path=2; repeated FlagInfo flags=3; ProbeStatus probe_status=4; bool truncated=5; bool is_wrapper=6}`. No renumbering. Document in a proto comment that `found` is derived from `probe_status` (true iff `FOUND_PARSED`, `FOUND_NO_FLAGS`, `TIMEOUT`, `NEEDS_CONFIRM`); `NEEDS_CONFIRM` means the file exists but was not executed; `ERROR` and `BUSY` mean "could not determine" (`found=false`, but clients must not render them as "not found").
- Files: `proto/session/v1/session.proto`

##### Task 1.2.1b: Generate and build (~3 min)
- `make proto-gen`, `go build ./...`. Do not `git add -f` anything under `gen/`, `web-app/src/gen/` (gitignored).
- Files: none committed

#### Story 1.2.2: Handler, wrapper, registry
**As a** client, **I want** the RPC served by `SessionService`, **so that** the generated handler interface compiles and the endpoint works.
**Acceptance Criteria**:
- `found=false` is a normal response (AC2)
  - *Given* a `DefaultsService` with a fake prober returning `NOT_FOUND`, *When* `ProbeProgram{command:"nope"}` is called, *Then* err is nil and `found=false`.
- Empty command is the only RPC error (AC1)
  - *Given* `command:""`, *Then* `connect.CodeInvalidArgument`.
- Single derivation of `found`
  - *Given* every `ProbeStatus` value, *When* `probeResultToProto` runs, *Then* `found` equals the table (`FOUND_PARSED`, `FOUND_NO_FLAGS`, `TIMEOUT`, `NEEDS_CONFIRM` true; `NOT_FOUND`, `ERROR`, `BUSY` false) and no other code assigns `Found`.
**Files**: `server/services/defaults_service.go`, `server/services/session_service.go`, `server/services/defaults_service_probe_test.go`, `docs/registry/features/*.json`

##### Task 1.2.2a: Handler + prober field + setter (~5 min)
- In `defaults_service.go` add field `prober *clihelp.Prober`, default built in `NewDefaultsService()` (`:62`, signature unchanged), `SetProber(p)`, and handler `ProbeProgram` preceded by `// +api: program_config:probe` (existing markers `:666,690,758`). Map `clihelp.ProbeResult` through one exhaustive `switch` helper `probeResultToProto` (the only place `Found` is set). The handler passes `req.ConfirmExecute` and `req.ResolveOnly` into `ProbeOpts` unchanged; it adds no logic of its own. `NewDefaultsService()` builds the prober with login-PATH derivation off (Task 1.1.4d).
- Files: `server/services/defaults_service.go`

##### Task 1.2.2b: Delegating wrapper (~2 min)
- Add `func (s *SessionService) ProbeProgram(ctx, req) { return s.defaultsSvc.ProbeProgram(ctx, req) }` beside `ListProgramsConfig` (`session_service.go:5215-5217`).
- Files: `server/services/session_service.go`

##### Task 1.2.2c: Handler tests (~4 min)
- Fake prober via `SetProber`: found, not-found (nil error), timeout, busy/error (found=false, status preserved), `NEEDS_CONFIRM` (found=true), `confirm_execute`/`resolve_only` reach the prober's `ProbeOpts` unchanged, wrapper flag, empty command error, flags+aliases mapping, and the `found`/status invariant table test.
- Files: `server/services/defaults_service_probe_test.go`

##### Task 1.2.2d: Registry (~3 min)
- `make registry-generate`; review and commit changed `docs/registry/features/**` files only (specific paths, not `git add -A`). Commit `session.proto` too.
- Files: `docs/registry/features/backend/*.json` (as changed)

##### Task 1.2.2e: End-to-end handler test with real Prober (~3 min)
- One test constructs a real `clihelp.Prober` with fake `lookPath`/`run` (aider fixture text) and calls the handler, asserting the AC1 response shape exactly.
- Files: `server/services/defaults_service_probe_test.go`

#### Story 1.2.3: `ProbeGuard` (AC8 on the unauthenticated listener)
**As an** owner, **I want** the probe RPC to refuse cross-site and DNS-rebinding requests on `:8543`, **so that** a web page cannot drive `<path> --help` on my machine.
**Verified facts** (opened 2026-09-21): the request chain is `otelhttp -> Logging -> CORSWithOrigins -> Compress -> [auth] -> mux` (`server/server.go:1372-1382`); `CORSWithOrigins` only sets `Access-Control-*` headers for allow-listed origins and never rejects a request (`server/middleware/cors.go:9-40`); there is no Host or Origin check anywhere in `server/middleware/`; `auth` is only added when `authMiddleware != nil`. Connect-go rejects unsupported `Content-Type` with 415, and serves unary GET only for procedures marked `NO_SIDE_EFFECTS` (task 1.2.1a keeps this RPC off that list). So: cross-origin JSON POST is already stopped by CORS preflight failing, but DNS rebinding (same-origin from the browser's view, attacker-controlled `Host`) is **not** stopped by existing middleware and needs the guard. The handler is mounted at `"/api"+path` (`server.go:414-415`).
**Wiring facts** (opened 2026-09-21): one mux (`s`) serves both listeners. `Start()` builds `inner := s` plus `authMiddleware` when non-nil (`server.go:1374-1377`), `StartRemote()` builds `inner := s` plus its `authMW` argument (`:1643-1646`, the `:8444` TLS listener with `middleware.Auth`, Host is the LAN/Tailscale name). `main.go` never calls `SetupAuth` (grep: no match), so `:8543` has `authMiddleware == nil`. `SetOrigins`/`SetHostnames` run after `NewServerWithDeps` (`main.go:435-450`), and `:414` is route registration inside the constructor, so any config captured there is empty/stale. Also `s` is the outer handler here, so `r.URL.Path` at the `inner` layer is the full `/api/session.v1.SessionService/ProbeProgram` (the `/api` `StripPrefix` happens inside the mux, later); wrapping the inner Connect `handler` at `:414` would see the stripped path and never match.
**Decision (ADR-001)**: install the guard in `Start()`'s chain only, wrapping `inner` when `s.authMiddleware == nil`; never in `StartRemote()` (authenticated, auth is the boundary; a Host check there would 403 the mobile app). Configuration is read lazily per request through accessors (`s.GetOrigins()`, `s.GetHostnames()`, the bound address), not captured at construction.
**Acceptance Criteria**:
- *Given* `Start()`'s chain with `authMiddleware == nil`, *When* a request to `/api/session.v1.SessionService/ProbeProgram` uses method GET, or `Host: evil.example:8543`, or `Origin: https://evil.example`, *Then* the guard returns 405/403 and the inner handler is not called; *Given* `Host: localhost:8543` or `127.0.0.1:8543` or `[::1]:8543` and either no `Origin` or one in `s.GetOrigins()` read at request time (including origins added by `SetOrigins` after construction), *Then* it passes through; *Given* `Host` is a name published through `SetHostnames` (LAN access on `:8543`, owner-optional, default allowed), *Then* it passes; *Given* the server is bound to a non-loopback address with no auth middleware, *Then* the guard returns 403 for every request to this procedure.
- *Given* `StartRemote()`'s chain (authenticated), *Then* it does not contain the guard: a POST with `Host: onyx.staplerhome.internal:8444` and valid auth reaches the handler. *Given* `Start()` with `authMiddleware != nil` (not the current `main.go` wiring), *Then* the guard is not installed either (auth is the boundary).
**Files**: `server/middleware/probeguard.go`, `server/middleware/probeguard_test.go`, `server/server.go`, `server/server_probeguard_test.go`

##### Task 1.2.3a: Middleware + wiring (~6 min)
- `ProbeGuard(procedurePath string, cfg ProbeGuardConfig) func(http.Handler) http.Handler` where `ProbeGuardConfig{LoopbackBound func() bool; AllowedOrigins func() []string; AllowedHosts func() []string}` (functions, evaluated per request, so late `SetOrigins`/`SetHostnames` are honored); applies only when `r.URL.Path == procedurePath` (full `/api`-prefixed path); order: non-POST -> 405; non-loopback bind -> 403; Host hostname not in `{localhost,127.0.0.1,::1}` plus `AllowedHosts()` -> 403; `Origin` present and not loopback or in `AllowedOrigins()` -> 403. Log rejections at Warn with `remote_addr`.
- In `Start()` (`server.go:1374-1377`) add `if s.authMiddleware == nil { inner = middleware.ProbeGuard(probeProcedurePath, cfg)(inner) }` inside the existing `inner` construction, so the guard sits inside CORS/Logging like auth would. Do NOT touch `StartRemote()` and do NOT wrap at `:414`. Factor the chain construction into a small `func (s *Server) localChain() http.Handler` (or equivalent) so the chain is testable without binding a port. Resolve the bound-address accessor (Unresolved Questions #7).
- Files: `server/middleware/probeguard.go`, `server/server.go`

##### Task 1.2.3b: Guard tests (~4 min)
- `httptest` table: GET, wrong Host, wrong Origin, loopback Host with/without allowed Origin, IPv6 Host, `SetHostnames`-published Host (`TestProbeGuard_should_Pass_When_HostPublishedViaSetHostnames`, so LAN/Tailscale access on `:8543` works), origin added after construction is honored (lazy read), non-loopback bind, other procedures untouched; plus one integration-style test mounting the guard in front of the real Connect handler asserting a wrong-Host POST never reaches the service.
- Files: `server/middleware/probeguard_test.go`

##### Task 1.2.3c: Per-listener chain tests (~4 min)
- `server/server_probeguard_test.go`: (1) the chain `Start()` builds (via the factored helper) with `authMiddleware == nil` returns 403 for `POST /api/session.v1.SessionService/ProbeProgram` with `Host: evil.example:8543`, and 200/pass-through with `Host: localhost:8543`; (2) the chain `StartRemote()` builds (same helper style, with a stub `authMW` that admits) does NOT 403 a POST with `Host: onyx.staplerhome.internal:8444`, proving the guard is absent; (3) `srv.SetOrigins([...])` called after `NewServerWithDeps` is honored by (1) (lazy read).
- Files: `server/server_probeguard_test.go`

---

## Phase 2: Binary found/not-found in both UIs (ships first; AC5, AC7)

### Epic 2.1: Shared hook and badge
**Goal**: One implementation used by both forms (jscpd margin).

#### Story 2.1.1: `useProbeProgram`
**As a** form, **I want** a hook that probes a command on demand and drops stale responses, **so that** the badge never shows a result for an old command.
**Acceptance Criteria**:
- Stale responses ignored (AC5)
  - *Given* probes for `clau` then `claude` where the first resolves last, *Then* state reflects `claude` only.
- Transport failure, permission denial and server ERROR/BUSY are distinct from not-found (AC5)
  - *Given* the RPC rejects (network error, or Connect `permission_denied`/HTTP 403 from `ProbeGuard`, e.g. `listen_address=0.0.0.0`), *Then* state is `transportError` and the badge reads "Couldn't check" (never "not found"); *Given* `probeStatus=ERROR|BUSY`, *Then* state is `busyOrError`; never `notFound`.
**Files**: `web-app/src/lib/hooks/useProbeProgram.ts`, `web-app/src/lib/hooks/useProbeProgram.test.ts`

##### Task 2.1.1a: Hook (~5 min)
- `useProbeProgram(command: string, mode: "program-config" | "picker")` returns `{state: ProbeUiState, check(opts?: {explicit?: boolean}): void}`. Request fields: `explicit` (Check button or Enter) sends `confirm_execute: true`; blur/selection sends neither; `mode: "picker"` always sends `resolve_only: true` unless `explicit` (then `confirm_execute: true`, `resolve_only: false`). `status NEEDS_CONFIRM` maps to `needsConfirm{path}` (never memoized; an explicit check replaces it). `check` ignores empty command, skips if unchanged and settled, increments a request-token ref, aborts the previous `AbortController`, calls `client.probeProgram({command}, {signal})` (client per `ProgramsManager.tsx:77`), keeps a per-command in-memory memo (never memoizes `busyOrError`/`transportError`), resets to `checking` on command change, aborts on unmount. `// +feature: settings-programs` in first 10 lines.
- Files: `web-app/src/lib/hooks/useProbeProgram.ts`

##### Task 2.1.1b: Hook tests (~4 min)
- Fake timers + two deferred promises resolved out of order; abort on unmount; transport error; **`ConnectError` with `Code.PermissionDenied` (and a plain 403-shaped rejection) maps to `transportError`, never `notFound`, asserted on the state and on the rendered "Couldn't check" copy via the badge**; ERROR/BUSY mapping; memo hit; error not memoized; request-shape assertions on the mocked client: blur sends neither flag, `check({explicit:true})` sends `confirm_execute:true`, picker mode sends `resolve_only:true`; `NEEDS_CONFIRM` maps to `needsConfirm` and is not memoized. One shared mock module `web-app/src/lib/hooks/__mocks__/probeProgramMock.ts` reused by later tests (jscpd).
- Files: `web-app/src/lib/hooks/useProbeProgram.test.ts`, `web-app/src/lib/hooks/__mocks__/probeProgramMock.ts`

#### Story 2.1.2: `ProbeStatusBadge`
**As a** user, **I want** an accessible status line, **so that** found/not-found is clear without color (`research/ux.md` section 3).
**Acceptance Criteria**:
- Icon plus text in `role=status` (AC5)
  - *Given* `found{path:"/usr/bin/claude"}`, *Then* text "Found: /usr/bin/claude", `aria-hidden` check icon and detail "Checked on this server only"; *Given* `FOUND_NO_FLAGS`, *Then* neutral "Couldn't read flags from --help; flag suggestions unavailable."; *Given* `TIMEOUT`, *Then* "Found: <path>. Timed out reading flags — try Check again." with found retained, distinct from the no-flags copy; *Given* `needsConfirm{path}`, *Then* "Found: <path>. Not run yet: this program is a script. Select Check to run it with --help and read its flags." with a `Check` control (44px); *Given* `isWrapper`, *Then* "Wrapper command (<token>): flags for the wrapped program are not checked."; *Given* `notFound`, *Then* the alias-aware copy from AC5; *Given* `busyOrError` or `transportError`, *Then* "Couldn't check right now." with Retry (never "not found"; `transportError` adds the non-loopback-bind sentence, Task 2.1.2a).
**Files**: `web-app/src/components/ui/ProbeStatusBadge.tsx`, `web-app/src/components/ui/ProbeStatusBadge.css.ts`, `web-app/src/components/ui/ProbeStatusBadge.test.tsx`

##### Task 2.1.2a: Component (~5 min)
- Props `{state, checkedToken, onRetry, onConfirm, testId}`; renders every `ProbeUiState` variant per `research/ux.md` section 4 plus the wrapper, busy and `needsConfirm` variants above (`needsConfirm` shows a 44px `Check` button calling `onConfirm`); Retry button 44px on `transportError`/`busyOrError`; `busyOrError` and `transportError` both lead with "Couldn't check right now."; `transportError` adds one sentence noting the server may refuse probes when it listens on a non-loopback address without auth; static "Checking..." under reduced motion; `warning`/`warningBg`/`warningText` tokens (`styles/theme.css.ts:100-102`); "Checked: <token>" and full path on expand; `// +feature:` header.
- Files: `web-app/src/components/ui/ProbeStatusBadge.tsx`, `web-app/src/components/ui/ProbeStatusBadge.css.ts`

##### Task 2.1.2b: Badge tests (~3 min)
- Each variant renders expected text and role; icons `aria-hidden`; Retry calls `onRetry`; not-found copy mentions aliases; ERROR/BUSY never shows "Not found".
- Files: `web-app/src/components/ui/ProbeStatusBadge.test.tsx`

### Epic 2.2: Program Config
**Goal**: AC5.

#### Story 2.2.1: Badge in ProgramsManager
**As a** user editing a program, **I want** an indicator when I leave the command field, **so that** typos surface before save.
**Acceptance Criteria**:
- Blur, Enter, or Check triggers the probe; save never blocked (AC5, AC9; UX D1, D5)
  - *Given* `prog-command-input` = `claude`, *When* it blurs, *Or* Enter is pressed (default form submit prevented), *Or* `prog-command-check` (44px, visible on desktop and mobile) is clicked, *Then* `prog-command-status` shows the result and Save stays enabled on `notFound`.
**Files**: `web-app/src/components/settings/ProgramsManager.tsx`, `web-app/src/components/settings/ProgramsManager.css.ts`, `web-app/src/components/settings/__tests__/ProgramsManager.test.tsx`

##### Task 2.2.1a: Wire (~5 min)
- At `ProgramsManager.tsx:313-321` add `onBlur={() => check()}` (implicit: never confirms a script), `onKeyDown` Enter -> `preventDefault(); check({explicit:true})` (D1; Enter is defined as Check), a `prog-command-check` `onClick={() => check({explicit:true})}`, and pass `onConfirm` from the badge's `Check` control (also explicit), `autoCapitalize="off" autoCorrect="off" spellCheck={false}` on `prog-command` and `prog-flags` (`:326-334`), a 44px `prog-command-check` button (D5), and `<ProbeStatusBadge testId="prog-command-status">` under the field with `aria-describedby`. Do not move focus.
- Files: `web-app/src/components/settings/ProgramsManager.tsx`, `web-app/src/components/settings/ProgramsManager.css.ts`

##### Task 2.2.1b: Tests (~4 min)
- Extend `__tests__/ProgramsManager.test.tsx` with the shared mock: blur -> found, not-found copy, transport error, busy, wrapper, Check button, Enter runs Check and does not submit, Save enabled.
- Files: `web-app/src/components/settings/__tests__/ProgramsManager.test.tsx`

### Epic 2.3: Session creation
**Goal**: AC7 (badge and saved-flag part), contained in a new component.

#### Story 2.3.1: Carry command and cli_flags on `ProgramOption`
**As a** picker, **I want** each option's command, **so that** the client can resolve ID to command (Flagged Choice 3).
**Acceptance Criteria**:
- *Given* `listProgramsConfig` returns `{id:"aider", command:"aider", cliFlags:"--yes-always"}`, *When* `useAvailablePrograms` maps it, *Then* the option has `value:"aider"`, `command:"aider"`, `cliFlags:"--yes-always"`; static `PROGRAMS` entries keep working with the new fields optional.
**Files**: `web-app/src/lib/constants/programs.ts`, `web-app/src/lib/hooks/useAvailablePrograms.ts`

##### Task 2.3.1a: Extend type and mapper (~3 min)
- Add optional `command?: string; cliFlags?: string` to `ProgramOption` (`programs.ts:1-5`); populate in the mapper (`useAvailablePrograms.ts:19-23`). Update the existing hook test if it asserts exact shape.
- Files: `web-app/src/lib/constants/programs.ts`, `web-app/src/lib/hooks/useAvailablePrograms.ts`

#### Story 2.3.2: `ProgramProbeSection` in the panel
**As a** user creating a session, **I want** the same missing-binary status and saved-flag check for the selected program, **so that** I do not waste a session slot.
**Acceptance Criteria**:
- Status follows the picker (AC7)
  - *Given* the picker on a program whose command is missing, *When* the Advanced Options section is open and the selection changes, *Then* the not-found badge appears below `<select id="omnibar-program">`, and the old `preset-program-warning` text is not rendered at the same time.
- Panel hotspot stays flat
  - *Given* the diff, *Then* `OmnibarCreationPanel.tsx` gains one import and one JSX element (`<ProgramProbeSection option={selectedOption} />`), no new state, effects or hooks.
**Files**: `web-app/src/components/sessions/ProgramProbeSection.tsx`, `web-app/src/components/sessions/ProgramProbeSection.test.tsx`, `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 2.3.2a: `ProgramProbeSection` component (~5 min)
- Props `{option: ProgramOption | undefined}`. Owns `useProbeProgram(option?.command, "picker")`, the selection-change effect calling `check()` (resolve-only: **never executes a program on selection**; not per keystroke), a `Check` control (via the badge's `onConfirm`) that calls `check({explicit:true})`, `ProbeStatusBadge`, and (task 4.1.2b) the saved-flag warning. Preserves `data-testid="preset-program-warning"` on the not-found variant. Behavior when the RPC is unavailable (`transportError`): falls back to nothing (the old `isProgramRecognized` list-membership check at `OmnibarCreationPanel.tsx:350` is removed with the span it fed). `// +feature:` header.
- Files: `web-app/src/components/sessions/ProgramProbeSection.tsx`

##### Task 2.3.2b: Panel wiring (~2 min)
- Replace the `!isProgramRecognized` span at `OmnibarCreationPanel.tsx:951-952` with `<ProgramProbeSection>`; delete `isProgramRecognized` (`:350`) if it has no other use (grep first).
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 2.3.2c: Tests (~4 min)
- `ProgramProbeSection.test.tsx` reusing the shared probe mock: not-found, found, wrapper, busy, and no duplicate warning; selection sends `resolve_only:true` and never `confirm_execute`; the `Check` control sends `confirm_execute:true`; a `NEEDS_CONFIRM` result shows the badge and no flag warning. Update `OmnibarCreationPanel.test.tsx` only for the removed span.
- Files: `web-app/src/components/sessions/ProgramProbeSection.test.tsx`, `web-app/src/components/sessions/OmnibarCreationPanel.test.tsx` (path confirmed at task time)

---

## Phase 3: Flag autocomplete (AC6, part 1)

### Epic 3.1: Token-at-caret combobox
**Goal**: Completion only after a probe with flags exists; never remounts the input.

#### Story 3.1.1: Pure token helpers
**As a** combobox, **I want** functions for the current token and replacement, **so that** completion works mid-string.
**Acceptance Criteria**:
- *Given* value `--yes --mo` and caret 9, *When* `tokenAtCaret` runs, *Then* it returns `{start:6,end:10,text:"--mo"}`; *When* `applyCompletion` inserts `--model` (takes_value) *Then* the result is `--yes --model ` (trailing space; `=` not used). `filterFlags` matches `name`, `short` and `aliases`.
**Files**: `web-app/src/lib/flags/flagTokens.ts`, `web-app/src/lib/flags/flagTokens.test.ts`

##### Task 3.1.1a: Helpers (~4 min)
- `tokenAtCaret(value, caret)`, `applyCompletion(value, token, flag)`, `filterFlags(flags, text)` (prefix, then substring, over name/short/aliases). Only tokens starting with `-` trigger.
- Files: `web-app/src/lib/flags/flagTokens.ts`

##### Task 3.1.1b: Tests (~3 min)
- Table tests: caret at start/middle/end, quoted values, empty value, `--` terminator, alias match.
- Files: `web-app/src/lib/flags/flagTokens.test.ts`

#### Story 3.1.2: `FlagCombobox`
**As a** user, **I want** flag suggestions with descriptions while typing, **so that** I recognize instead of recall.
**Acceptance Criteria**:
- ARIA 1.2 behaviour (AC6, AC9)
  - *Given* flags `[--model, --verbose]` and value `--mo`, *When* the user presses Down, *Then* the input has `role="combobox"`, `aria-expanded="true"`, `aria-controls`, `aria-autocomplete="list"`, `aria-activedescendant` = option id of `--model`; Escape closes without clearing; option rows are at least 44px; tapping an option does not blur-trigger a re-probe (`onMouseDown` preventDefault); options contain no interactive children.
- No remount / focus loss
  - *Given* the input focused with a caret while `flags` goes from `[]` to a populated list (probe resolves), *Then* the DOM `<input>` element is the same node (same reference), still focused, and the caret is unchanged; only ARIA attributes and the listbox change.
**Files**: `web-app/src/components/ui/FlagCombobox.tsx`, `web-app/src/components/ui/FlagCombobox.css.ts`, `web-app/src/components/ui/FlagCombobox.test.tsx`

##### Task 3.1.2a: Component (~6 min)
- Model attributes on `web-app/src/components/history/HistorySearchInput.tsx:174`. Props `{id, value, onChange, flags, testId, describedBy}`; **always renders the same `<input>`** (when `flags` is empty it simply sets no `role="combobox"`/`aria-expanded` and renders no listbox, without changing element type or key); keys Down/Up (wrap), Enter/Tab accept, Escape close, Alt+Down open; listbox in-flow with max-height on narrow viewports (`research/ux.md` section 6); the active option's description is shown inline in the option row (UX D3), rows hold text only.
- Files: `web-app/src/components/ui/FlagCombobox.tsx`, `web-app/src/components/ui/FlagCombobox.css.ts`

##### Task 3.1.2b: Tests (~5 min)
- Keyboard flow, ARIA attributes, mouse-down selection, empty-flags behavior, same-node/focus-preserved test when `flags` arrives late (`rerender`, compare element identity and `document.activeElement`), `jest-axe` `nested-interactive`/`aria-required-children` check if `jest-axe` is already a dependency (else rely on the existing Axe e2e).
- Files: `web-app/src/components/ui/FlagCombobox.test.tsx`

##### Task 3.1.2c: Shared listbox key handling (~4 min)
- Extract the up/down/wrap/Enter/Escape key-to-index reducer to `web-app/src/lib/flags/useListboxNav.ts` (pure reducer + test) and use it from `FlagCombobox`; if adopting it in `HistorySearchInput.tsx` is more than a mechanical swap, file a follow-up issue instead and note its number in the PR. This makes the three-combobox duplication a tracked cost.
- Files: `web-app/src/lib/flags/useListboxNav.ts`, `web-app/src/lib/flags/useListboxNav.test.ts`

#### Story 3.1.3: Use it in Program Config
**As a** user, **I want** suggestions in Default CLI Flags, **so that** I can discover flags.
**Acceptance Criteria**:
- *Given* probe `found` with flags, *When* the user types `--mo` in `prog-flags-input`, *Then* suggestions appear; *Given* `notFound`, zero flags or `isWrapper`, *Then* the field behaves as before (no listbox).
**Files**: `web-app/src/components/settings/ProgramsManager.tsx`, `web-app/src/components/settings/__tests__/ProgramsManager.test.tsx`

##### Task 3.1.3a: Wire (~4 min)
- Replace the plain input at `ProgramsManager.tsx:326-334` with `FlagCombobox` (keep `id="prog-flags"` and `data-testid="prog-flags-input"`); pass `flags` (empty when `isWrapper`); add test cases.
- Files: `web-app/src/components/settings/ProgramsManager.tsx`, `web-app/src/components/settings/__tests__/ProgramsManager.test.tsx`

---

## Phase 4: Unknown-flag warnings and descriptions (AC6 part 2, AC7 saved flags, AC9)

### Epic 4.1: Non-blocking validation
**Goal**: Warn without false positives.

#### Story 4.1.1: `validateFlags`
**As a** user, **I want** typos flagged softly, **so that** I notice them without being blocked.
**Acceptance Criteria**:
- *Given* known `{--model, --verbose, -v}` and input `--model=x -v --no-verbose --verbos -abc -- --anything`, *When* `validateFlags` runs, *Then* unknown is `["--verbos"]` (handles `=value`, bundled shorts, `--no-` negation of known, alias names, and the `--` terminator); *Given* zero known flags or a wrapper result, *Then* it returns `[]`.
**Files**: `web-app/src/lib/flags/validateFlags.ts`, `web-app/src/lib/flags/validateFlags.test.ts`

##### Task 4.1.1a: Implement (~5 min)
- Pure `validateFlags(input, flags): string[]`; prefers false negatives (precision-first stays: a known-valid flag the parser missed yields only the soft "is not listed ... may still work" warning, never a block, an error style or an `aria-invalid`; tested in validation.md); skips value tokens after a `takes_value` flag; matches `name`/`short`/`aliases`; an unknown `takes_value` never causes a following-token warning.
- Files: `web-app/src/lib/flags/validateFlags.ts`

##### Task 4.1.1b: Tests (~3 min)
- Table tests for each edge, quoted values, alias hit (`--allowed-tools`), and the built-in Proxy entry (wrapper -> no warnings).
- Files: `web-app/src/lib/flags/validateFlags.test.ts`

#### Story 4.1.2: Surface warnings
**As a** user, **I want** the warning next to the field, **so that** I see it before saving or launching.
**Acceptance Criteria**:
- Program Config (AC6)
  - *Given* `--verbos` typed, *Then* `prog-flags-warning` reads "--verbos is not listed in `claude --help`. It may still work (hidden or subcommand flags)." is referenced by `aria-describedby`, `aria-invalid` is not set, and there is no `role="alert"`.
- Session creation (AC7, scoped down)
  - *Given* selected program `aider` with saved `cli_flags` `--yes-always --bogus`, *When* probe returns flags, *Then* `omnibar-flags-warning` lists only `--bogus`. Alias `extraFlags` are not validated.
**Files**: `web-app/src/components/settings/ProgramsManager.tsx`, `web-app/src/components/sessions/ProgramProbeSection.tsx`, tests for both

##### Task 4.1.2a: Program Config warning (~4 min)
- Compute `validateFlags(formData.cli_flags, flags)`; render text element with id referenced by `aria-describedby` on the combobox.
- Files: `web-app/src/components/settings/ProgramsManager.tsx`, `web-app/src/components/settings/__tests__/ProgramsManager.test.tsx`

##### Task 4.1.2b: Session creation warning (~3 min)
- Inside `ProgramProbeSection` validate `option.cliFlags` (task 2.3.1a) against the probe flags and render `omnibar-flags-warning` below the badge. No change to `OmnibarCreationPanel.tsx`. Extend `ProgramProbeSection.test.tsx`.
- Files: `web-app/src/components/sessions/ProgramProbeSection.tsx`, `web-app/src/components/sessions/ProgramProbeSection.test.tsx`

### Epic 4.2: Tap-friendly descriptions
**Goal**: AC9, no hover dependency, no invalid ARIA.

#### Story 4.2.1: `FlagInfoButton`
**As a** mobile user, **I want** to tap an info button to read a flag's description, **so that** I am not blocked by missing hover.
**Acceptance Criteria**:
- *Given* a flag with description "Model to use", *When* the info button (44x44px) is tapped, *Then* the description renders inline, `aria-expanded="true"`, `aria-controls` points at it, and tapping again collapses it.
**Files**: `web-app/src/components/ui/FlagInfoButton.tsx`, `web-app/src/components/ui/FlagInfoButton.css.ts`, `web-app/src/components/ui/FlagInfoButton.test.tsx`

##### Task 4.2.1a: Component (~4 min)
- `<button type="button" data-testid="prog-flag-info-button" aria-expanded aria-controls>` toggling an inline `<div id>`; truncate long descriptions (full on expand); no Radix, no hover handler.
- Files: `web-app/src/components/ui/FlagInfoButton.tsx`, `web-app/src/components/ui/FlagInfoButton.css.ts`

##### Task 4.2.1b: Unit tap test (~3 min)
- `fireEvent.click`/`userEvent` only (no `mouseover`); assert expand/collapse and 44px min size via style tokens. This is a unit substitute; AC9 is proven by task 5.1b.
- Files: `web-app/src/components/ui/FlagInfoButton.test.tsx`

#### Story 4.2.2: Place descriptions (no button inside an option)
**As a** user, **I want** each suggestion and each warned flag to show its description, **so that** the name alone is not all I have.
**Acceptance Criteria**:
- *Given* the list is open, *Then* the active option row shows its description inline and rows contain no `<button>`; *Given* warned unknown flags or an "Available flags (N)" disclosure under the field, *Then* each entry has a `FlagInfoButton` outside any `role=option`; axe reports no `nested-interactive`.
**Files**: `web-app/src/components/ui/FlagCombobox.tsx`, `web-app/src/components/settings/ProgramsManager.tsx`, tests

##### Task 4.2.2a: Compose (~4 min)
- Decision (UX D3): inline description for the active option (also exposed via `aria-describedby` on the input pointing at the description node); `FlagInfoButton` used only in the warning list and an "Available flags" disclosure below the field (outside `role=listbox`). No Right-arrow handling, no `tabIndex={-1}` button in rows. Update tests.
- Files: `web-app/src/components/ui/FlagCombobox.tsx`, `web-app/src/components/settings/ProgramsManager.tsx`, `web-app/src/components/ui/FlagCombobox.test.tsx`

---

## Phase 5: End-to-end and gates

### Epic 5.1: Playwright
**Goal**: One spec proving the user path and AC9 (AC5, AC9).

#### Story 5.1.1: Program Config e2e
**As a** maintainer, **I want** one e2e spec, **so that** the wiring from blur to badge is guarded in CI and AC9 is proven on a touch viewport.
**Acceptance Criteria**:
- *Given* the isolated e2e server and a fixture script `probe-fixture` (created in the test dir, prints `--alpha  first flag` and `--beta <v>  second`) given by absolute path, *When* the test types its path in `prog-command-input` and blurs, *Then* `prog-command-status` has text "Found:" and "Not run yet" (the fixture is a shebang script, so nothing ran and the fixture's marker file does not exist); *When* `prog-command-check` is clicked, *Then* the marker exists, and typing `--alp` in `prog-flags-input` lists `--alpha`; a nonexistent path shows "Not found"; and in a context with `viewport: {width:375,height:667}, hasTouch:true, isMobile:true`, `tap()` on `prog-flag-info-button` shows the description with `aria-expanded="true"` and a bounding box of at least 44x44 (AC9 proof).
**Files**: `tests/e2e/pages/ProgramsSettingsPage.ts`, `tests/e2e/cli-flag-discovery.spec.ts`, `tests/e2e/fixtures/probe-fixture.sh`

##### Task 5.1a: Page helper + fixture (~4 min)
- `ProgramsSettingsPage` with `data-testid`/ARIA locators only; fixture binary a tiny shell script `chmod +x`, not `claude` (e2e server env may lack it), owned by the test user and not world-writable (the probe's own checks apply); its body first touches a marker file (path passed through a constant in the script, inside the test dir) so the spec can prove blur does not run it and Check does.
- Files: `tests/e2e/pages/ProgramsSettingsPage.ts`, `tests/e2e/fixtures/probe-fixture.sh`

##### Task 5.1b: Spec (~5 min)
- First line `// @feature program_config:probe, settings-programs`; no `waitForTimeout`; `expect(locator).toHaveText`; one desktop and one touch-emulated 375px test (the AC9 proof); also a wrong-Host POST check via `request.post` with `Host: evil.example` expecting 403. Run `cd tests/e2e && npx playwright test cli-flag-discovery.spec.ts`.
- Files: `tests/e2e/cli-flag-discovery.spec.ts`

### Epic 5.2: Gates
**Goal**: Green `make ready`.

#### Story 5.2.1: Gates
**As a** maintainer, **I want** passing gates, **so that** the feature ships mergeable.
**Acceptance Criteria**:
- *Given* the finished branch, *When* `make ready`, `cd web-app && pnpm run lint:duplicates` and `pnpm exec jest --no-coverage` run, *Then* all pass and jscpd stays at or below 0.12%.
**Files**: none new

##### Task 5.2.1a: Run gates (~5 min)
- `make ready-complexity-gate`, `make ready`, `go test ./config/clihelp ./server/services ./server/middleware -race`, `pnpm run lint:duplicates` (in `web-app/`, pnpm only). Fix any jscpd finding by extracting to the shared mock/helpers, not by raising the threshold. Commit specific files only. No `docs/reference/program-probe.md` (dropped, Flagged Choice 9); ADRs are the record of the security envelope.
- Files: none new

---

## Review repair log

Findings from `adversarial-review.md` (AR) and `architecture-review.md` (ARCH), and where each is resolved.

| Finding | Resolution | Where |
|---|---|---|
| AR blocker 1: ADR-001 does not enforce AC8 | Bare name or absolute path only, regular/executable/not world-writable, POST-only, loopback Host/Origin guard (verified not enforced by existing middleware), audit line per probe; sign-off recorded as PENDING | Flagged Choices 1, 10; Stories 1.1.1, 1.2.3; Task 1.1.4a; Observability Plan; ADR-001 |
| AR blocker 2 / ARCH: runner tests unreachable, `RunFunc`/`Limits` mismatch | `runWith(runSpec)` seam with `CLIHELP_TEST_` marker env; `helpSpec` test for exact `--help`; `RunFunc` now takes `Limits`; AC3/AC8 proofs re-specified | Story 1.1.2, Tasks 1.1.2b-d, Story 1.1.4 (limits plumbing AC), AC3, AC8 |
| AR blocker 3: PATH | Login-shell PATH derived once (mirrors `config/config.go:1213-1221,1288-1296`), alias-only -> NOT_FOUND with copy | Flagged Choice 10, Tasks 1.1.4d-e, AC5, Task 2.1.2a |
| ARCH blocker: singleflight leader cancelled | `DoChan` on `context.WithoutCancel`, cancelled call -> ERROR never cached, cancel-then-reprobe test | Task 1.1.4b-c, Story 1.1.4 ACs, ADR-002 |
| ARCH: semaphore rejection cached | New `BUSY` status, never cached | Proto 1.2.1a, Tasks 1.1.4a-c |
| AR: runner process gaps (Setsid, kill after exit, cap kill, wall time, shared writer) | `Setsid`, post-`Wait` group kill, `onOverflow` kill, `WaitDelay=200ms`, no mutex in `capWriter` | Tasks 1.1.2a-b, ADR-002 |
| ARCH: `found` vs `probe_status` | `probeResultToProto` is the single derivation; table test; ERROR/BUSY semantics documented | Task 1.2.1a, Story 1.2.2, Task 1.2.2c |
| AR: AC4 aliases | `FlagInfo.aliases = 5`; parser and validators use it | Tasks 1.2.1a, 1.1.3b-c, 3.1.1a, 4.1.1a |
| AR: FlagCombobox focus loss | Same `<input>` always rendered; identity/focus test | Task 3.1.2a-b |
| AR: button inside `role=option` | Inline active-option description (D3); `FlagInfoButton` outside options | Story 4.2.2, Task 4.2.2a |
| ARCH: OmnibarCreationPanel hotspot | `ProgramProbeSection` owns hook/badge/warning; panel gains one element | Tech Debt Disposition, Story 2.3.2 |
| AR: AC9 proof | Playwright touch-emulated 375px is the proof; jest is a unit substitute | AC9, Tasks 4.2.1b, 5.1b |
| AR/ARCH: AC7 `extraFlags` | Verified panel does not receive `extraFlags`; scoped down and recorded, not dropped | Flagged Choice 2, Unresolved Questions, traceability row 7, AC7 |
| AR: wrapper false warnings | Wrapper detection, no `--help` run, `is_wrapper`, suppressed suggestions/warnings, Proxy-entry test | Flagged Choice 7, Tasks 1.1.1a-b, 1.1.4a, 2.1.2a, 4.1.1b |
| UX D1, D2, D3, D5 adopted; D4 dropped | See Flagged Choice 9 | Tasks 2.2.1a, 2.1.2a, 4.2.2a; "Did you mean"/"Edit in Program Config" removed |
| AR: scope drift (reference doc, "Did you mean") | Removed; registry JSON and e2e kept | Flagged Choice 9, Task 5.2.1a |
| ARCH: `Resolve` returns Target with reason | `Resolve` returns `(Target, ResolveError)`, target only valid on success | Glossary, Task 1.1.1a |
| AR: `home` injectable; runner_other closure; PAGER reconciliation; end-to-end AC1 test | `WithHome`; closure form; ADR-001 lists `PAGER`/`MANPAGER`; Task 1.2.2e | Tasks 1.1.1a, 1.1.2b, 1.2.2e, ADR-001 |
| ARCH: three comboboxes duplication | Listbox reducer extraction or follow-up issue | Task 3.1.2c |
| AR: `git --help`/`$HOME` side effects; TOCTOU | Recorded as verify-at-capture; TOCTOU re-stat before caching | Unresolved Questions, Tasks 1.1.3a, 1.1.4b |
| AR: `validation.md` contradicts plan | Rewritten | `implementation/validation.md` |
| ARCH nit: paths for `HistorySearchInput`/`Omnibar` | Full paths added | Flagged Choice 2, Task 3.1.2a |
| ARCH nit: glossary count | Recounted: 24 | Domain Glossary |

### Iteration 2 (re-review of the repairs above)

Findings from the "Re-review" sections of `adversarial-review.md` (AR2) and `architecture-review.md` (ARCH2).

| Finding | Resolution | Where |
|---|---|---|
| AR2 BLOCKER: `ProbeGuard` at route registration (`server.go:414`) sits on the mux shared by `:8543` and `:8444`, so it 403s the authenticated remote listener or leaves `:8543` unguarded; auth/origins not yet configured; wrong path after `StripPrefix` | Guard installed in `Start()`'s chain only (around `inner`, `server.go:1374-1377`, when `authMiddleware == nil`), never in `StartRemote()` (`:1643-1646`), matching the full `/api`-prefixed path; origins/hostnames/bind read lazily per request (funcs in `ProbeGuardConfig`), `SetHostnames` names allowed; chain factored into a testable helper; tests: `Start()` chain has guard and 403s `Host: evil.example:8543`, `StartRemote()` chain does not and passes `Host: onyx...:8444`, late `SetOrigins` honored | Story 1.2.3 (wiring facts, ACs), Tasks 1.2.3a-c, glossary, Tech Debt table, Unresolved Q #7, ADR-001 |
| ARCH2: semaphore taken per caller before the flight | Slot try-acquired inside the flight body (leader only), released on flight end; BUSY shared through the flight result; tests: 10 same-binary probes all get the real result, cancelled callers do not free a slot | Task 1.1.4a-c, Story 1.1.4 concurrency AC, ADR-002 |
| ARCH2: repair-log ADR-002 citation had no matching decision | ADR-002 now records the `WithoutCancel` + `DoChan` + leader-only-slot decision | ADR-002 |
| ARCH2: no proof the flight goroutine exits after all callers cancel | `goleak`/`Eventually` test added, no sleeps | Task 1.1.4c, validation.md |
| AR2 concern: login-shell PATH via bare `safeexec.CommandContext`, `sync.Once` caches transient failure, `$SHELL` unset, bash early-return | Runs through `runWith` machinery (Setsid + group kill, 2s, capped) with `inheritEnv`; success-only cache with TTL and 30s retry; goroutine kick from `NewDefaultsService()`; `$SHELL` unset/`/bin/sh` -> server PATH + `~/.local/bin`, `/usr/local/bin`; bash early-return tolerated; tests with stub shells | Flagged Choice 10, Tasks 1.1.4d-e, glossary |
| AR2 concern: `printenv` no-leak proof vacuous through `runWith` | `runSpec.parentEnv` (poisoned in tests) plus control assertions | Tasks 1.1.2b, 1.1.2d, Story 1.1.2 |
| AR2 concern: 403/`permission_denied` shown as "not found" | Maps to `transportError`, "Couldn't check" copy; hook test for `PermissionDenied` | Story 2.1.1 AC, Tasks 2.1.1b, 2.1.2a |
| AR2 note: `Setsid` override drops `Pdeathsig` | Recorded as an accepted limitation | ADR-002 |

### Iteration 2 residual concerns (non-blocking, carry into implementation)

Both reviewers ended CONCERNS with no blocker. The implementer must handle:

- Name a `remoteChain` helper for `StartRemote()` so the "remote chain has no guard" test exercises real code (Task 1.2.3c).
- `LoopbackBound` must treat the hostname `localhost` as loopback (`Addr` is the raw `localhost:8543`); parse Host with `net.SplitHostPort` plus IPv6 bracket handling. Unresolved Q #7 (bound-address accessor) is still open.
- A non-loopback bind with no auth disables the probe RPC permanently, by design; note it in ADR-001.
- Loopback Origin on any port passes the guard (CORS preflight still applies). `s.origins` is read lazily but is race-free only because `SetOrigins` runs before `Start()`.
- ADR-001 sign-off is PENDING human review; the AC7 scope-down (saved `cli_flags` only) also needs reviewer acceptance.

## Phase 4 repair log

Findings from SDD phase 4 (`pre-mortem.md`, `validation.md` consistency pass) and where each is resolved. ADR-001 sign-off remains PENDING human review throughout.

| Finding | Resolution | Where |
|---|---|---|
| BLOCKER 1: `requirements.md` AC8 "only the command being configured" vs plan/ADR-001 (any UI-supplied command, guarded) | AC text untouched; "Interpretation notes" added to `requirements.md` (AC8 as one command per request from the form/selection, first token, `--help` only, hardened env/cwd, local-UI guard; AC2 custom `lookInDirs`; AC3 "no shell" scope; AC6 tooltip as tap-to-reveal; AC7 saved `cli_flags` only; AC10 git negative). No approval claimed | `requirements.md`, header line 7, Unresolved Questions |
| BLOCKER 2: `ux.md` S7 log msg/level vs plan Observability | `ux.md` S7 now `program_probe`, Info for all outcomes, Warn for BUSY; `validation.md` UX-34 row aligned | `design/ux.md` S7, `validation.md` |
| BLOCKER 3: `ux.md` S6 wording vs plan AC5 / Task 2.1.2a | S6 uses the alias-aware NOT_FOUND copy and "Couldn't check right now." for ERROR/BUSY/transport; wrapper and timeout rows added | `design/ux.md` S6, Task 2.1.2a |
| `ux.md` pruning: "Did you mean / Use it" (S3, UX-21), "Edit in Program Config" (S5, UX-31), extraFlags (UX-30); S2/S4 vs plan D3; S5 fallback; `ResolveReason`; "isn't listed" wording | Removed or aligned (UX-21 left as a withdrawn stub to keep numbering); no button in `role=option`, no Radix tooltip, description inline plus `FlagInfoButton` in warnings and "Available flags" disclosure; S5 falls back to nothing; `ResolveError`; "is not listed in --help" everywhere | `design/ux.md`, `validation.md` UX rows |
| P1-1: 3s timeout may be too short for aider/claude/gemini | Flagged Choice 11: measured cold/warm timing go/no-go in Task 1.1.3a with decision rule (> ~1.5s cold => `slowTools` `Limits` override or `PENDING`; 3s default kept unless contradicted), table to be filled at capture; TIMEOUT copy "Timed out reading flags — try Check again", TTL 30s -> 60s, explicit Check bypasses cached TIMEOUT; real-binary smoke test that skips when absent | Flagged Choice 11, Tasks 1.1.2b, 1.1.3a, 1.1.4b, AC3, Unresolved Questions |
| P1-2: implicit execution of arbitrary scripts | Flagged Choice 12 and ADR-001 addition: shebang/non-native returns `NEEDS_CONFIRM` without executing; run needs `confirm_execute` (Check click or Enter, never blur); picker sends `resolve_only`, never executes on selection; confirmed set per (path, mtime, size); new proto enum value 7 and request fields 2, 3; hook/badge/section/e2e updated; marker-file tests (fake and real process) | Flagged Choice 12, Tasks 1.1.4a, 1.1.4f-g, 1.2.1a, 1.2.2a, 2.1.1a-b, 2.1.2a, 2.2.1a, 2.3.2a, 5.1a, Story 5.1.1, AC5/AC7/AC8, ADR-001, `ux.md` S1/S5/S6/UX-42, `validation.md` |
| P2 #3: guard vs LAN/Tailscale hostnames on `:8543` | Unresolved Q #7 notes `SetHostnames` requirement and "Couldn't check" (not "not found"); added `TestProbeGuard_should_Pass_When_HostPublishedViaSetHostnames` | Unresolved Questions, Task 1.2.3b, `validation.md` |
| P2 #4: login-shell PATH goroutine in tests; rc stdout noise | `NewDefaultsService()`/`NewProber()` no longer start derivation; explicit `StartLoginPathDerivation()` at production wiring; PATH printed between `__CLIHELP_PATH_START__`/`__CLIHELP_PATH_END__` sentinels, missing markers = failure; tests for polluted output and a hermetic constructor | Tasks 1.1.4d-e, Flagged Choice 10, glossary (`Prober`), `validation.md` |
| P2 #5: parser false positives | Precision-first stays; Task 4.1.1a notes the soft-warning-only behavior; validation row for a known-valid flag not parsed | Task 4.1.1a, `validation.md` |

Not adopted from the pre-mortem: the low-coverage parser guard (fewer than ~60% of `-`-lines parsed => suppress warnings) and a per-program warning dismiss. Both remain optional follow-ups if warning noise appears.

Still open after this pass (carried in Unresolved Questions): ADR-001 sign-off and the AC8/AC7 readings (PENDING human review); the measured timing table (UNMEASURED until Task 1.1.3a); acceptance of the first-check click cost for shebang-script CLIs (Flagged Choice 12); the guard bound-address accessor (Q #7).
