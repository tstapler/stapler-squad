# Research: Architecture Pattern for Launcher Presets

## 1. Session-creation-registry — which of the 7 touchpoints apply

Full rule: `.claude/rules/session-creation-registry.md`. Launcher presets are a **prefill
shortcut over the existing directory/worktree session types** (per requirements.md
Constraints and Out-of-Scope), so this follows the `autonomous`-mode precedent
(`.claude/rules/session-creation-registry.md:22`) — reuse existing session type, add
parameters instead of an enum value.

| # | Touchpoint | File | Needed? | Why |
|---|---|---|---|---|
| 1 | Proto `SessionType` enum | `proto/session/v1/types.proto` | **No** | No new lifecycle. A preset resolves to `SESSION_TYPE_DIRECTORY` or `SESSION_TYPE_NEW_WORKTREE` exactly like today — same as `autonomous` reusing `SESSION_TYPE_DIRECTORY` (`.claude/rules/session-creation-registry.md:22`). |
| 2 | Proto request fields | `proto/session/v1/session.proto` (`CreateSessionRequest`, lines 472–573) | **Yes, narrowly** | Not a mode flag like `one_off`/`autonomous_mode` — the gap is representational: `CreateSessionRequest` has no way to carry >1 positional argv element safely. See §2 below for the exact field to add. `program` (field 5), `working_dir` (field 3), `cli_flags` (field 26) already exist and are reused as-is; only the "additional argv elements" concept is missing. |
| 3a | Path validation guard | `server/services/session_service.go` (~line 48 pattern) | **No** | A preset always resolves to a real `path`/`working_dir` before submission (it prefills the form; the user still submits through the normal directory/worktree flow), so the existing guard is untouched. |
| 3b | `switch req.Msg.SessionType` | `server/services/session_service.go` (~line 55) | **No** | No new enum value means no new case. |
| 3c | Mode-specific logic block | `server/services/session_service.go` (~line 1375–1435, where `program`/`CliFlags` are resolved) | **Yes, small** | If a `repeated string argv`/`extra_args`-style field is added (see §2), the handler needs to fold it into `instance.CLIFlags`-equivalent state the same way it already folds `req.Msg.CliFlags` at `server/services/session_service.go:1430-1435`. This is additive alongside the existing `cli_flags` append, not a new branch keyed on session type. |
| 4 | `session/instance.go` `SessionType` constants + `IsValid()` | `session/instance.go` | **No** | No new lifecycle constant — presets terminate in `SessionTypeDirectory`/`SessionTypeNewWorktree`, identical to sessions created by hand. |
| 5 | Frontend `OmnibarFormState.sessionType` union, `canSubmit`, `handleSubmit` | `web-app/src/components/sessions/Omnibar.tsx` | **No new union member** — but `handleSubmit`/form state need a new field (e.g. `argv`/`extraArgs`) threaded alongside `program`, the same way `cliFlags`/`autoYes` are threaded today, so a preset's extra args reach `OmnibarSessionData`. | Prefilling doesn't create a new mode; it just needs a carrier field for the extra argv, mirrored from touchpoint 2. |
| 6 | `SESSION_TYPES` radio group + hints | `web-app/src/components/sessions/OmnibarCreationPanel.tsx` | **No** | No new radio option — presets are a *selection above* the radio group that prefills whichever existing mode is already selected (or forces `directory`/`new_worktree` per the preset's `default_path`). A new "Presets" list/section is additive UI, not a `SESSION_TYPES` entry. |
| 7 | `OmnibarContext.tsx` `sessionTypeMap` + `useSessionService.ts` RPC body | `web-app/src/lib/contexts/OmnibarContext.tsx`, `web-app/src/lib/hooks/useSessionService.ts` | **No `sessionTypeMap` change; yes RPC-body passthrough** | `sessionTypeMap` only needs entries per `SessionType` enum value (touchpoint 1), which isn't changing. But same as touchpoint 5, the new argv/extra-args field needs to be threaded into the `createSession(...)` call body — same pattern as `oneOff: data.oneOff ?? false` (`.claude/rules/session-creation-registry.md:143`). |

**Net effect:** 2 of 7 touchpoints are truly untouched in the way the rule anticipates (proto
enum, `session/instance.go` constants); 3 are untouched in their primary role but need a small
adjacent addition (proto request field, backend fold-in logic, frontend field threading); 2
(`SESSION_TYPES` radio group, `sessionTypeMap`) don't need edits at all because presets don't
introduce a selectable mode — they operate as a prefill layer above the mode selector, exactly
as requirements.md's Constraints section anticipated by pointing at the `autonomous`
reference implementation.

This is confirmed by the **`GetSessionDefaults`/`profile` precedent already in the codebase**
(`proto/session/v1/session.proto:1735-1752`, `server/services/defaults_service.go`,
`server/services/session_service.go:1412` `config.ResolveDefaults(cfg, workingDir,
req.Msg.Profile)`): profiles are a named-preset-of-defaults mechanism that resolves
server-side into existing `CreateSessionRequest` fields (`program`, `auto_yes`, `tags`,
`env_vars` — see `ResolveDefaultsResponse`, `session.proto:1748-1752`) with **zero** touchpoint
1/4/6 changes and only carrier-field threading for touchpoint 2/3c/5/7. Launcher presets should
follow the same shape, not the `one_off`/`autonomous_mode` shape (which do add boolean flags
that alter *handler control flow*, not just data).

## 2. Proto field design: `argv` decomposition

Read in full: `proto/session/v1/session.proto:472-573` (`CreateSessionRequest`).

### Relevant existing fields
- `program` (string, field 5) — single program name, e.g. `"claude"`, `"codex"`.
- `cli_flags` (string, field 26) — "additional CLI flags appended to the program launch
  command. Applied on top of any defaults-resolved flags."
- `profile` (string, field 11) — resolves named defaults server-side via
  `config.ResolveDefaults`; the response shape (`ResolveDefaultsResponse`,
  `session.proto:1748-1752`) only carries `program` (single string) + `auto_yes` + `tags` +
  `env_vars` — **no multi-arg argv concept exists anywhere in the current schema.**

### How `program`/`cli_flags` actually reach the spawned process — traced end to end
1. `server/services/session_service.go:1430-1435` appends `req.Msg.CliFlags` onto a resolved
   `instanceCLIFlags` string (defaults-resolved flags + request flags, space-joined).
2. That string is stored verbatim on the instance: `session/instance.go:293`
   `CLIFlags string \`json:"cli_flags,omitempty"\`` and `session/instance.go:539-540`,
   threaded through `StartOptions` at `session/instance.go:642`.
3. At launch time, `session/instance_tmux.go:105-118` (`buildLaunchCommand`) builds the full
   command **as a shell string**:
   ```go
   for _, f := range strings.Fields(i.CLIFlags) {
       cmd = cmd + " " + shellQuote(f)
   }
   ```
   `strings.Fields` does a **naive whitespace split** — it has no concept of quoting, so a
   flag value containing a space (a path, a remote command like `ssh -t host 'cd ~/repo &&
   exec claude'`) cannot survive this split intact. Each split token is then independently
   `shellQuote()`-d (`session/instance_tmux.go:126-128`, POSIX single-quote escaping) before
   re-joining.
4. The final assembled command is *not* exec'd directly — it becomes the shell script handed
   to a real shell: `session/tmux/shell_handle.go:89`
   `tmux new-session -d -s {sessionName} -c {workDir} -- /bin/sh -c {command}`. The *entire*
   pipeline, including `i.Program` itself (`session/instance_tmux.go:111`
   `cmd = p.cmd` for non-claude programs — used completely unquoted/unsplit), is a shell
   string interpreted by `/bin/sh -c`.

### Conclusion: decompose into `program` + a new `repeated string extra_args`
- **Don't reuse `cli_flags`.** It's fundamentally string+whitespace-split, which is exactly
  the "shell-quoting ambiguity" the requirements (and herdr-web's prior art) call out as the
  reason to store `argv` as an array in the first place. Passing preset argv through
  `cli_flags` would silently reintroduce the bug this feature exists to avoid (e.g. a
  preset argv element `["ssh", "-t", "host", "cd ~/repo && exec claude"]` — the 4th element
  contains spaces and must stay one token; `strings.Fields` would shred it).
- **Don't reuse `profile`.** It resolves entirely server-side against `config.json`-defined
  profiles (`UpsertProfile`/`DeleteProfile` RPCs) — a different, mutable, RPC-editable
  mechanism explicitly out of scope for presets (requirements.md Out of Scope: "Editing
  presets through the UI").
- **Add `repeated string extra_args = 28;`** (next free field number — 27 is `alias_name`) to
  `CreateSessionRequest`. Semantics: `argv[0]` maps to existing `program` (field 5); `argv[1:]`
  maps to `extra_args`. This mirrors the `program` + `cli_flags` split that already exists,
  just with an array instead of a whitespace-joined string.
- **Downstream handling must NOT go through `strings.Fields`.** The fix threads `extra_args`
  as its own `[]string` on `session.Instance` (parallel to `CLIFlags string`) and
  `buildLaunchCommand` (`session/instance_tmux.go:105-118`) must iterate that slice directly
  — `for _, a := range i.ExtraArgs { cmd = cmd + " " + shellQuote(a) }` — skipping
  `strings.Fields` entirely for this path. `shellQuote` (`session/instance_tmux.go:126-128`)
  already exists and is exactly the primitive needed: each argv element is quoted
  independently before being joined into the `/bin/sh -c` string, so multi-word elements
  (`"cd ~/repo && exec claude"`) survive intact even though the ultimate transport is still a
  shell string (tmux's `/bin/sh -c` boundary is unavoidable given `session/tmux/shell_handle.go:89`
  — "argv-safe" here means "no ambiguity from naive splitting/re-parsing," not "literal
  `exec()`", since the whole pipeline is shell-string based at the tmux layer).
- Net new surface: 1 proto field, 1 Go struct field on `Instance`/`StartOptions`, ~3-line
  change in `buildLaunchCommand`. No new `SessionType`, no new RPC-level branching.

## 3. Integration points

### Config loading — `config/config.go` `LoadConfig()` pattern
- `config/config.go:782-804` (`LoadConfig`) / `847-` (`LoadConfigFromPath`): read file → JSON
  unmarshal → on `os.IsNotExist`, write and return `DefaultConfig()`; on any other error, log
  and fall back to `DefaultConfig()` (never returns an error to the caller — always degrades
  to a usable default). **This does not match the Must-Have "reject the whole file with a
  clear error on schema/JSON errors" requirement** — `LoadConfig()`'s silent-fallback-to-default
  behavior is the wrong template to copy verbatim for presets; the loader needs its own
  `LoadLauncherPresets(path) (*LauncherPresetsConfig, error)` that returns an error (not a
  silently-empty preset list) on malformed JSON/duplicate IDs/empty argv, per requirements.md
  Must Have #2. Log the error loudly at startup (`log.Error`) and continue serving with zero
  presets rather than crashing the server — analogous to how `LoadConfig()` degrades, but
  *visibly*, not silently.
- **Call-frequency pattern split already exists in this codebase and matters for the presets
  RPC design:**
  - *Fresh-per-call* pattern: `config.LoadConfig()` is called inline in most RPC handlers
    (`server/services/defaults_service.go:86,97,122,176,...`, `feature_flag_service.go:96,158`,
    `approval_service.go:93`) — no in-memory singleton/cache; every call re-reads
    `config.json` from disk. Cheap because it's a small JSON file.
  - *Cache-once-at-startup* pattern: `AvailablePrograms` (`config/config.go:736`) is
    deliberately detected **once at process start**, per `server/server.go:714` comment
    ("Detect available programs once at startup so /api/server-info never runs [PATH
    scanning] on hot path") — expensive because it involves filesystem `PATH` lookups.
  - Launcher presets are a small, cheap-to-parse JSON file, so they fit the **fresh-per-call**
    pattern, not the cache-once pattern. This has a direct payoff for the Nice-to-Have
    "live reload": if `GetLauncherPresets` simply re-reads and re-validates the file on every
    RPC call (like `config.LoadConfig()` does), the Must-Have #1 ("see it appear ... without
    restarting the server") is satisfied for free, and the `fsnotify` watcher
    (`session/unfinished/watcher.go`) becomes purely an optimization (push-based cache
    invalidation) rather than a correctness requirement — safe to defer to Nice-to-Have as
    requirements.md already scopes it.

### ConnectRPC service layer — `server/services/session_service.go`
- New RPC: `rpc GetLauncherPresets(GetLauncherPresetsRequest) returns
  (GetLauncherPresetsResponse) {}` in `proto/session/v1/session.proto`, alongside the existing
  `GetSessionDefaults`/`ResolveDefaults`/`ListWorkflows`-style read-only list RPCs (all
  clustered ~lines 248-380). Handler in `server/services/session_service.go`, following the
  `GetSessionDefaults` pattern (`server/services/session_service.go:3362-3392`): load config
  fresh, map to proto message, return — no mutation, no session-type branching.
- **This must be a ConnectRPC method, not folded into `/api/server-info`.** `AvailablePrograms`
  is exposed via the plain REST `/api/server-info` endpoint (`server/server.go:989-1021`,
  consumed by `useAvailablePrograms.ts`), a separate code path from ConnectRPC entirely.
  requirements.md explicitly calls for a `GetLauncherPresets` **RPC** "alongside (not
  replacing) the existing AvailablePrograms list" — i.e., presets get their own RPC surface
  distinct from the REST server-info mechanism `AvailablePrograms` already uses; the two
  should not be merged into one response shape.
- `server_service.go:1430-1435`'s existing `cli_flags` append block is the insertion point for
  folding `req.Msg.ExtraArgs` into the instance the same way (additive, not session-type-gated).

### Frontend Omnibar — `OmnibarCreationPanel.tsx` + `OmnibarContext.tsx`
- `useAvailablePrograms.ts` (`web-app/src/lib/hooks/useAvailablePrograms.ts`) is the fetch
  pattern to mirror for a new `useLauncherPresets.ts` hook — except it should call the new
  ConnectRPC method instead of `fetch("/api/server-info")`, since presets are RPC-backed, not
  REST-backed.
- `OmnibarCreationPanel.tsx:770-785` shows the existing "Program" `<select>` wired to
  `availablePrograms` — the new "Presets" section is sibling UI in the same Advanced
  Options collapsible area (or a new top-level section per requirements.md Must Have #3), and
  on selection calls `setFormField` for `program`, the new `extraArgs`/`argv` field, and
  `workingDir`/`path` (from `default_path`) — no new radio-group entry (touchpoint 6, N/A per
  §1).
- **Dynamic per-request-data detector pattern** (`OmnibarContext.tsx:75-114`,
  `WorkflowDetector`/`AliasDetector`) is the template for the Nice-to-Have `preset:<id>`
  detector: register/unregister a `PresetDetector` in a `useEffect` keyed on the fetched preset
  list, exactly like `workflowDetectorRef`/`aliasDetectorRef`. `AliasDetector`
  (`web-app/src/lib/omnibar/detectors/AliasDetector.ts`) is the closest structural analog —
  note it also uses a free-text `extraFlags` string parsed from user input
  (`AliasDetector.ts:19,102-113`), reinforcing that **user-typed** extra flags stay
  string-based by convention in this codebase; only **preset-authored** argv (JSON array,
  never user-typed at the terminal) needs the strict array/no-shell-split treatment from §2.

## 4. Data flow and consistency requirements

- **Startup vs. hot-reload:** validate once at startup (log loudly on failure, per
  requirements Must-Have #4 "fails loudly... rather than silently dropping presets or
  crashing"), but do not cache the parsed result as the source of truth for the RPC — re-parse
  on each `GetLauncherPresets` call (see §3 fresh-per-call rationale) so a corrected file is
  picked up without a restart, matching Must Have #1's "without restarting the server (or
  after an explicit reload)" wording, which already anticipates fresh-read-per-request as an
  acceptable interpretation of "explicit reload" (each RPC call ~= an explicit reload
  triggered by opening the Omnibar).
- **How presets reach the frontend:** new dedicated `GetLauncherPresets` RPC, not embedded in
  `GetSessionDefaults`/`ResolveDefaults` (those are about *server-computed* default resolution
  for a given working dir/profile, a different concern from a static, user-authored list) and
  not embedded in `/api/server-info` (that's the REST/`AvailablePrograms` auto-detection path,
  explicitly a "different mechanism" per requirements.md Context section).
  `GetLauncherPresets` should be read independently on Omnibar mount, same lifecycle as
  `useAvailablePrograms`'s startup fetch.
- **Consistency on malformed config:** whole-file reject (Must Have #4) means
  `LoadLauncherPresets` returns `(nil, err)` on any single bad preset (duplicate ID, empty
  `argv`, bad JSON) — no partial-apply. The RPC handler on error should return an empty list
  with the error surfaced via logs (server-side) — requirements.md doesn't specify whether the
  RPC itself should return a connect error vs. an empty list with a warning; Phase 3 should
  decide, but returning a connect error would let the frontend surface "presets file is
  invalid" directly in the Omnibar UI, which better serves "fails loudly... surfaced to the
  user" (Must Have #4) than a silent empty list plus a server log the user never sees.
