# Implementation Plan: alias

**Feature**: Named session presets invoked via @alias-name in the omnibar
**Date**: 2026-06-20
**Status**: Ready for implementation
**ADRs**: [ADR-020 — alias @ trigger character](../../docs/adr/020-alias-at-trigger-character.md)

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Config storage | `[]AliasConfig` array on `Config` struct | existing `DirectoryRules []DirectoryRule` precedent | `map[string]AliasConfig` | Array preserves insertion order, matches DirectoryRules pattern, enables group browsing |
| Env var expansion | `${VAR_NAME}` → `os.Getenv` at session-create time; omit if unset | requirements spec | Expand at config-load time | Session-create-time is correct — env may differ between load and invocation |
| Wire gap fix | Add `EnvVars`+`CLIFlags` to `InstanceOptions`; inject via `ExtraEnv` at `initTmuxSession` | existing `ExtraEnv` pattern in `session/tmux/tmux.go` | Post-hoc `tmux setenv` | Post-hoc setenv misses already-running processes |
| Alias resolution | `config.ResolveAlias()` wrapping existing `ResolveDefaults()` | existing `ResolveDefaults()` pattern | Inline in session_service.go | Keeps resolution logic testable and co-located |
| Frontend detector | `AliasDetector` dynamically registered like `WorkflowDetector` | `WorkflowDetector` pattern in `OmnibarContext.tsx` | Static in `createDefaultRegistry()` | Alias list is runtime user data, not static |
| Alias palette UX | Extend existing `AtCommandDropdown` pattern | `WorkflowDetector` + `AtCommandDropdown` | New modal/panel | Least new UI surface; `@` UX already understood |
| AliasName (newtype) | `string` with validated pattern `^[\w-]+$` at load time | type-driven-design: parse-at-boundary | Raw unvalidated `string` | Prevents invalid names reaching the resolver |
| Proto field ordering | Fields 25 (`env_vars`), 26 (`cli_flags`), 27 (`alias_name`) | next available after `workflow_id = 24` | Non-sequential | Field numbers are permanent wire identifiers |
| ListAliases RPC | New RPC in `SessionService` (follows `GetSessionDefaults` pattern) | existing `GetSessionDefaults` | Reuse `GetSessionDefaults` | Separate concern; avoids bloating existing response |
| CLIFlags merge semantics | **Replace** at each resolution layer (global → dir → profile → alias); **append** only for invocation-time extra flags in `ResolveAlias` | matches `Program` field behavior in `mergeProfileInto` | Append at all layers | Appending raw strings across layers produces uninterpretable commands; replace is predictable. Invocation-time append is a separate, explicit step. |
| `ResolveAlias` return type | `(ResolvedDefaults, error)` only — `Path` promoted into `ResolvedDefaults` | SRP / arch review concern #3 | `(ResolvedDefaults, *AliasConfig, error)` | Returning `*AliasConfig` invites handler to bypass the resolution layer; `Path` belongs in `ResolvedDefaults` |
| Omnibar keyboard state | Single `activeDropdown` discriminated union (`"alias" \| "workflow" \| "search" \| null`) | arch review concern #6 | Per-dropdown boolean flags | Prevents impossible states (two dropdowns "active" simultaneously); eliminates keyboard handler conflicts |

---

## Dependency Visualization

```
Phase 1: Backend Wire Gap Fix
  proto fields 25,26
    → session/instance.go (EnvVars + CLIFlags fields)
    → session/instance_tmux.go (ExtraEnv injection + CLIFlags append)
    → server/services/session_service.go (wire resolved → instanceOpts)

Phase 2: Config Layer
  config/config.go (AliasConfig struct + Aliases field)
    → config/defaults.go (FindAlias + GetAliasesByGroup + expandEnvVars + ResolveAlias)
    → proto (AliasProto + ListAliases RPC)
    → server/services/defaults_service.go (ListAliases handler)

Phase 3: Frontend Detector  [depends on Phase 2 RPC]
  web-app/src/lib/omnibar/types.ts (InputType.Alias + AliasNotFound)
    → web-app/src/lib/hooks/useAliases.ts
    → web-app/src/lib/omnibar/detectors/AliasDetector.ts
    → web-app/src/lib/contexts/OmnibarContext.tsx (dynamic registration)

Phase 4: Frontend UI  [depends on Phase 3]
  web-app/src/components/ui/AliasPalette.tsx + .css.ts
    → web-app/src/lib/hooks/useAliasSuggestions.ts
    → web-app/src/components/sessions/Omnibar.tsx (palette + chip + placeholder)

Phase 5: Session Creation Wire-up  [depends on Phase 3 + Phase 1]
  web-app/src/lib/omnibar/actions/types.ts (create_alias_session)
    → web-app/src/lib/omnibar/actions/dispatch.ts
    → web-app/src/lib/contexts/OmnibarContext.tsx
    → web-app/src/lib/hooks/useSessionService.ts
    → proto (alias_name = 27)
    → server/services/session_service.go (alias resolution in CreateSession)
```

---

## Phase 1: Backend Wire Gap Fix

### Epic 1.1: Thread EnvVars + CLIFlags from defaults resolution into tmux session

**Goal**: Make the already-computed `resolved.EnvVars` and `resolved.CLIFlags` values actually reach the tmux session. Currently `ResolveDefaults` produces these values but `session_service.go` discards them.

#### Story 1.1.1: Add EnvVars and CLIFlags to InstanceOptions and Instance

**As a** backend developer, **I want** `InstanceOptions` to carry `EnvVars` and `CLIFlags`, **so that** callers can pass session-level env and flag overrides without touching tmux directly.

**Acceptance Criteria**:
- `InstanceOptions` has `EnvVars map[string]string` and `CLIFlags string` fields
- `Instance` struct carries these values from `NewInstance`
- Unit test: `NewInstance` with `EnvVars` populates the field

**Files**: `session/instance.go`

##### Task 1.1.1a: Add fields to InstanceOptions and Instance (~3 min)
- Add `EnvVars map[string]string` and `CLIFlags string` after `WorkflowID string` in `InstanceOptions` (~line 487)
- Add same fields to `Instance` struct (~line 111)
- In `NewInstance()`, populate `instance.EnvVars = opts.EnvVars` and `instance.CLIFlags = opts.CLIFlags`
- Files: `session/instance.go`

#### Story 1.1.2: Apply EnvVars via tmux ExtraEnv at session creation

**As a** developer, **I want** `Instance.EnvVars` injected as tmux session env at new-session time, **so that** the child process inherits the correct environment from startup.

**Acceptance Criteria**:
- `initTmuxSession()` converts `i.EnvVars` to `KEY=VALUE` strings appended to `session.ExtraEnv`
- Empty map produces no extra env entries
- Env vars are set at new-session time, not post-hoc

**Files**: `session/instance_tmux.go`

##### Task 1.1.2a: Inject EnvVars into ExtraEnv in initTmuxSession (~3 min)
- In `initTmuxSession()`, before `tb.TmuxManager().SetSession(session)`, iterate `i.EnvVars` and append `fmt.Sprintf("%s=%s", k, v)` to `session.ExtraEnv`
- Files: `session/instance_tmux.go`

#### Story 1.1.3: Apply CLIFlags in buildLaunchCommand

**As a** developer, **I want** `Instance.CLIFlags` appended to the program command, **so that** config-level flags reach the AI process.

**Acceptance Criteria**:
- `buildLaunchCommand` appends `i.CLIFlags` when non-empty
- Flags appended after all other flags (last-value-wins for duplicate flags)
- Works for all programs, not just claude

**Files**: `session/instance_tmux.go`

##### Task 1.1.3a: Append CLIFlags in buildLaunchCommand (~2 min)
- In `buildLaunchCommand()` before `return program`: if `i.CLIFlags != ""`, append `" " + i.CLIFlags`
- Files: `session/instance_tmux.go`

#### Story 1.1.4: Add proto fields 25+26 and wire in session_service.go

**As a** developer, **I want** `env_vars` and `cli_flags` in `CreateSessionRequest`, **so that** the frontend can pass these values explicitly.

**Acceptance Criteria**:
- `CreateSessionRequest` has `map<string,string> env_vars = 25` and `string cli_flags = 26`
- `session_service.go` applies `resolved.EnvVars`/`resolved.CLIFlags` then merges explicit request fields on top
- `make generate-proto` regenerates Go and TypeScript bindings

**Files**: `proto/session/v1/session.proto`, `server/services/session_service.go`

##### Task 1.1.4a: Add proto fields and regenerate (~4 min)
- Add after field 24 in `CreateSessionRequest`: `map<string,string> env_vars = 25;` and `string cli_flags = 26;`
- Run `make generate-proto`
- Files: `proto/session/v1/session.proto`

##### Task 1.1.4b: Wire resolved and explicit EnvVars/CLIFlags in session_service.go (~4 min)
- In `!req.Msg.SkipDefaults` block (~line 1053), after `resolved := config.ResolveDefaults(...)`:
  ```go
  instanceOpts.EnvVars = resolved.EnvVars
  instanceOpts.CLIFlags = resolved.CLIFlags
  ```
- After the block, merge explicit request fields:
  ```go
  for k, v := range req.Msg.EnvVars { instanceOpts.EnvVars[k] = v }
  if req.Msg.CliFlags != "" { instanceOpts.CLIFlags += " " + req.Msg.CliFlags }
  ```
- Files: `server/services/session_service.go`

##### Task 1.1.4c: Unit test for EnvVars wire-through (~5 min)
- Verify `CreateSession` with `env_vars: {"FOO":"bar"}` results in `instanceOpts.EnvVars["FOO"] == "bar"`
- Files: `server/services/session_service_test.go` (or new `session_service_envvars_test.go`)

---

## Phase 2: Config Layer

### Epic 2.1: AliasConfig struct and config integration

**Goal**: Define the alias schema in Go, expose lookup/group helpers, wire `${VAR}` expansion, expose via RPC.

#### Story 2.1.1: Add AliasConfig struct and Aliases field to Config

**As a** user, **I want** to define aliases in `config.json`, **so that** I can create named session presets.

**Acceptance Criteria**:
- `AliasConfig` struct with all fields: `Name`, `Group`, `Path`, `Description`, `Profile`, `Program`, `AutoYes`, `Tags`, `EnvVars`, `CLIFlags`
- `Name` validation pattern `^[\w-]+$` documented in comment
- `Config.Aliases []AliasConfig` with `json:"aliases,omitempty"`
- `LoadConfigFromPath` initializes nil `Aliases` to `[]AliasConfig{}`

**Files**: `config/config.go`

##### Task 2.1.1a: Add AliasConfig struct and wire into Config (~4 min)
- Add `AliasConfig` struct after `DirectoryRule` definition (~line 435)
- Add `Aliases []AliasConfig \`json:"aliases,omitempty"\`` to `Config` struct
- In `LoadConfigFromPath`, add nil-init: `if cfg.Aliases == nil { cfg.Aliases = []AliasConfig{} }`
- Files: `config/config.go`

#### Story 2.1.2: Add alias lookup helpers to config/defaults.go

**As a** backend developer, **I want** `FindAlias`, `GetAliasesByGroup`, `expandEnvVars`, and `ResolveAlias`, **so that** the session service can resolve aliases cleanly.

**Acceptance Criteria**:
- `FindAlias(cfg, name)` returns pointer or nil; case-insensitive
- `GetAliasesByGroup(cfg)` returns `map[string][]AliasConfig`; ungrouped use key `""`
- `expandEnvVars(m)` expands `${VAR}` via `os.Getenv`; omits key when unset
- `ResolveAlias(cfg, name, branch, label, extraFlags)` returns `(ResolvedDefaults, error)` — `Path` is promoted into `ResolvedDefaults.Path` (new field); do NOT return `*AliasConfig` (SRP: handler must not bypass the resolution layer)
- All four have unit tests in `config/defaults_test.go`

**Files**: `config/defaults.go`, `config/defaults_test.go`

##### Task 2.1.2a: Add FindAlias and GetAliasesByGroup (~4 min)
- `FindAlias`: iterate `cfg.Aliases`, `strings.EqualFold(a.Name, name)`
- `GetAliasesByGroup`: iterate, key by `a.Group`
- Files: `config/defaults.go`

##### Task 2.1.2b: Add expandEnvVars (~3 min)
- Regexp `\$\{([^}]+)\}` to match `${VAR}` tokens in values
- `os.Getenv(varName)`; if result empty, omit key AND emit `log.Warn("alias env var not set", "key", k)` (arch concern: silent omission must produce a warning)
- Files: `config/defaults.go`

##### Task 2.1.2c: Add ResolveAlias (~5 min)
- `ResolveAlias(cfg *Config, aliasName, branch, label, extraFlags string) (ResolvedDefaults, error)`
- Add `Path string` and `Branch string` and `SessionLabel string` to `ResolvedDefaults` struct (needed since we no longer return `*AliasConfig`)
- Call `ResolveDefaults(cfg, alias.Path, alias.Profile)`, then `mergeProfileInto` with alias inline fields
- **CLIFlags semantics (blocker fix #3)**: resolution layers use REPLACE semantics (existing `mergeProfileInto` behavior — correct, no change needed); `extraFlags` is APPENDED via `result.CLIFlags += " " + extraFlags` as a final explicit step; document this distinction in a comment
- Apply `expandEnvVars` on `result.EnvVars`
- Set `result.Path = alias.Path`, `result.Branch = branch`, `result.SessionLabel = label`
- Files: `config/defaults.go`

##### Task 2.1.2d: Unit tests for all helpers (~5 min)
- `FindAlias`: found, not found, case-insensitive
- `GetAliasesByGroup`: grouped, ungrouped, mixed
- `expandEnvVars`: set var expands, unset var key omitted (with warning), literal passthrough
- `ResolveAlias`: resolution order verification (global → dir → profile → alias); CLIFlags replace-then-append semantics
- Files: `config/defaults_test.go`

#### Story 2.1.3: Expose aliases via ListAliases RPC

**As a** frontend developer, **I want** a `ListAliases` RPC, **so that** the omnibar can populate its alias palette.

**Acceptance Criteria**:
- `AliasProto` message mirrors `AliasConfig`
- `ListAliases` RPC returns all configured aliases
- Handler in `defaults_service.go` maps `[]AliasConfig` → proto

**Files**: `proto/session/v1/session.proto`, `server/services/defaults_service.go`, `server/services/session_service.go`

##### Task 2.1.3a: Define AliasProto and ListAliases in proto (~4 min)
- Add `message AliasProto { string name = 1; string group = 2; string path = 3; string description = 4; string profile = 5; string program = 6; bool auto_yes = 7; repeated string tags = 8; }`
- Add `message ListAliasesRequest {}` and `message ListAliasesResponse { repeated AliasProto aliases = 1; }`
- Add `rpc ListAliases(ListAliasesRequest) returns (ListAliasesResponse) {}` to service
- Run `make generate-proto`
- Files: `proto/session/v1/session.proto`

##### Task 2.1.3b: Implement ListAliases handler (~3 min)
- Add `ListAliases` to `DefaultsService` in `server/services/defaults_service.go`
- Map `config.AliasConfig` → `sessionv1.AliasProto` for each alias in `config.LoadConfig().Aliases`
- Register/delegate from `session_service.go`
- Files: `server/services/defaults_service.go`, `server/services/session_service.go`

---

## Phase 3: Frontend Detector

### Epic 3.1: AliasDetector with grammar parsing

**Goal**: Parse the full `@alias-name[:branch][ label][ --flags]` grammar; return `InputType.Alias` or `InputType.AliasNotFound`; do not fall through to session search.

#### Story 3.1.1: Add Alias and AliasNotFound to InputType

**As a** frontend developer, **I want** `InputType.Alias` and `InputType.AliasNotFound`, **so that** the omnibar dispatches alias invocations correctly.

**Acceptance Criteria**:
- `InputType.Alias = "alias"` and `InputType.AliasNotFound = "alias_not_found"` added
- Corresponding entries in `INPUT_TYPE_INFO`

**Files**: `web-app/src/lib/omnibar/types.ts`

##### Task 3.1.1a: Extend InputType enum (~3 min)
- Add `Alias = "alias"`, `AliasNotFound = "alias_not_found"`, and `AliasBrowse = "alias_browse"` to `InputType`
- `AliasBrowse` is returned for `@name` without trailing space (completion mode) — prevents `SessionSearchDetector` from claiming `^@` input (**blocker fix #1**)
- Add `INPUT_TYPE_INFO` entries (icon `"@"`, labels `"Alias"` / `"Alias Not Found"` / `"Alias Browse"`)
- Files: `web-app/src/lib/omnibar/types.ts`

#### Story 3.1.2: Add useAliases hook

**As a** frontend developer, **I want** a typed alias list from the server, **so that** the detector and palette reference live config data.

**Acceptance Criteria**:
- `AliasEntry` interface mirrors `AliasProto` (name, group, path, description, program, autoYes, tags)
- `useAliases()` returns `{ aliases: AliasEntry[], loading: boolean, error: Error | null }`
- Fetches `ListAliases` RPC on mount; no polling

**Files**: `web-app/src/lib/hooks/useAliases.ts`

##### Task 3.1.2a: Create useAliases hook (~4 min)
- Create `web-app/src/lib/hooks/useAliases.ts`
- `AliasEntry` interface, `useAliases()` hook calling `client.listAliases({})`
- Files: `web-app/src/lib/hooks/useAliases.ts`

#### Story 3.1.3: Implement AliasDetector class

**As a** developer, **I want** `AliasDetector` that correctly parses the alias grammar, **so that** alias invocations are detected and dispatched.

**Acceptance Criteria**:
- Grammar: `^@([\w-]+)(?::([^\s]+))?(?:\s+((?!--)[^\n]+?))?(?:\s+(--\S.*?))?$`
- Known alias + trailing space → `InputType.Alias` with `metadata: { aliasName, branch, label, extraFlags }`
- Unknown alias (no match in list) → `InputType.AliasNotFound` with `metadata: { slug }`
- Bare `@` or `@name` without space → returns null (completion mode, not invocation)
- Priority 36; NOT in `createDefaultRegistry()` — dynamically registered

**Files**: `web-app/src/lib/omnibar/detectors/AliasDetector.ts`, `web-app/src/lib/omnibar/detectors/AliasDetector.test.ts`

##### Task 3.1.3a: Create AliasDetector (~5 min)
- Create `web-app/src/lib/omnibar/detectors/AliasDetector.ts`
- Constructor takes `aliases: AliasEntry[]`
- **Must claim ALL `^@` input — never return null for input starting with `@`** (blocker fix: prevents `SessionSearchDetector` from claiming `@foo` input)
- Three return modes:
  - `"@"` alone → `InputType.AliasBrowse` (open browse palette)
  - `^@[\w-]+$` (no trailing space) → `InputType.AliasBrowse` with `metadata.partial` (completion mode)
  - `^@[\w-]+\s` (has space) → parse full grammar; return `InputType.Alias` (found) or `InputType.AliasNotFound` (not found)
- Full grammar for invocation mode: name, optional `:branch`, optional label (text before first `--`), optional `--extra-flags`
- Lookup name case-insensitively in `this.aliases`
- Files: `web-app/src/lib/omnibar/detectors/AliasDetector.ts`

##### Task 3.1.3b: Unit tests for AliasDetector (~5 min)
- Test IDs `T-UNIT-TS-030` through `T-UNIT-TS-040`
- Cases: found with space, found with branch, found with label, found with flags, not found, no space → `AliasBrowse` (NOT null), bare `@` → `AliasBrowse`
- Files: `web-app/src/lib/omnibar/detectors/AliasDetector.test.ts`

#### Story 3.1.4: Dynamically register AliasDetector in OmnibarContext

**As a** developer, **I want** `AliasDetector` registered when the alias list loads, **so that** detection reflects current config.

**Acceptance Criteria**:
- `OmnibarContext.tsx` imports `useAliases` + `AliasDetector`
- Uses `useEffect` + `useRef` pattern identical to `WorkflowDetector` registration
- Re-registers when `aliases` array reference changes

**Files**: `web-app/src/lib/contexts/OmnibarContext.tsx`

##### Task 3.1.4a: Register AliasDetector dynamically (~3 min)
- Add `const { aliases } = useAliases()` and `useEffect` mirroring `WorkflowDetector` pattern
- Files: `web-app/src/lib/contexts/OmnibarContext.tsx`

---

## Phase 4: Frontend Omnibar UI

### Epic 4.1: Alias palette, resolution chip, empty/not-found states

**Goal**: Two-phase alias UI — grouped palette when browsing, flat fuzzy filter when typing, inline resolution chip when matched.

#### Story 4.1.1: Create AliasPalette component

**As a** user, **I want** to see my aliases in a grouped palette when I type `@`, **so that** I can browse without memorizing names.

**Acceptance Criteria**:
- Grouped by `group`; ungrouped aliases first with no section header
- Within group: config-file order
- Empty state: "No aliases yet — add them in config.json" (`data-testid="alias-palette-empty"`)
- When typing `@sometext`: flat fuzzy-filtered list
- Uses `.css.ts` per CSS architecture; `data-testid` locators throughout

**Files**: `web-app/src/components/ui/AliasPalette.tsx`, `web-app/src/components/ui/AliasPalette.css.ts`

##### Task 4.1.1a: Create AliasPalette and styles (~5 min)
- Create `AliasPalette.tsx` with props: `aliases`, `input`, `selectedIndex`, `onSelect`
- Grouped view when `input === "@"`; flat fuzzy view when `input` matches `^@[\w-]+$`
- Create `AliasPalette.css.ts` using `vars` from `theme.css`
- Files: `web-app/src/components/ui/AliasPalette.tsx`, `web-app/src/components/ui/AliasPalette.css.ts`

##### Task 4.1.1b: Create useAliasSuggestions hook (~3 min)
- Create `web-app/src/lib/hooks/useAliasSuggestions.ts`
- Exports: `isAliasBrowse`, `isAliasCompletion`, `filteredAliases`, `complete(alias)`
- `complete` returns `"@" + alias.name + " "` (trailing space triggers invocation mode)
- Files: `web-app/src/lib/hooks/useAliasSuggestions.ts`

#### Story 4.1.1b: Introduce activeDropdown discriminated union (prerequisite for 4.1.2)

**As a** developer, **I want** a single `activeDropdown` state variable, **so that** alias palette and session-search navigation cannot conflict.

**Acceptance Criteria**:
- `activeDropdown: "alias" | "workflow" | "search" | null` state added to `Omnibar`
- All existing keyboard handlers (ArrowUp/Down, Enter, Escape) gate on `activeDropdown` value
- No boolean flags (`showAliasPalette`, `showAtDropdown`) — the discriminated union is the sole source of truth
- TypeScript type system prevents adding a new dropdown without updating all handlers

**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 4.1.1b: Add activeDropdown state (~3 min)
- Add `const [activeDropdown, setActiveDropdown] = useState<"alias" | "workflow" | "search" | null>(null)`
- Migrate existing `atSuggestIndex` / workflow dropdown logic to gate on `activeDropdown === "workflow"`
- Files: `web-app/src/components/sessions/Omnibar.tsx`

#### Story 4.1.2: Integrate AliasPalette into Omnibar

**As a** user, **I want** the alias palette in the omnibar with keyboard navigation, **so that** I can select aliases without a mouse.

**Acceptance Criteria**:
- `AliasPalette` renders when `isAliasBrowse || isAliasCompletion`
- ArrowUp/Down cycles through aliases; Enter advances input to `@aliasname ` (completion)
- Mutually exclusive with `AtCommandDropdown` in alias mode

**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 4.1.2a: Wire AliasPalette into Omnibar.tsx (~5 min)
- Import `AliasPalette`, `useAliasSuggestions`, `useAliases`
- Add `aliasSuggestIndex` state; handle ArrowUp/Down/Enter for alias selection
- Show `AliasPalette` conditionally; suppress `AtCommandDropdown` when alias mode active
- Files: `web-app/src/components/sessions/Omnibar.tsx`

#### Story 4.1.3: Resolution chip and not-found state

**As a** user, **I want** to see a chip showing the resolved alias as I type, **so that** I know what session will be created before pressing Enter.

**Acceptance Criteria**:
- `InputType.Alias` → chip: `@foo → ~/projects/foo [:branch]`
- `InputType.AliasNotFound` → error chip: `No alias '@foo'`; session search results suppressed

**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 4.1.3a: Add resolution chip rendering (~4 min)
- Add cases for `InputType.Alias` and `InputType.AliasNotFound` in the detection info section
- Block session search results when `InputType.AliasNotFound`
- Files: `web-app/src/components/sessions/Omnibar.tsx`

#### Story 4.1.4: Update omnibar placeholder text

**As a** user, **I want** the omnibar placeholder to mention `@alias`, **so that** I discover the feature from the UI.

**Acceptance Criteria**:
- Placeholder updated to include `@alias` example

**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 4.1.4a: Update placeholder (~2 min)
- Update `placeholder` prop to include `"@alias"` in examples
- Files: `web-app/src/components/sessions/Omnibar.tsx`

---

## Phase 5: Session Creation Wire-up

### Epic 5.1: Route alias selection through dispatch → createSession

**Goal**: When user confirms an alias, dispatch `create_alias_session` → RPC → backend resolves alias → session created.

#### Story 5.1.1: Add create_alias_session to OmnibarAction union

**As a** developer, **I want** a `create_alias_session` action type, **so that** alias invocations dispatch cleanly.

**Acceptance Criteria**:
- `{ type: "create_alias_session"; aliasName: string; branch?: string; label?: string; extraFlags?: string }` added to union
- TypeScript compile fails if dispatch switch is missing the case

**Files**: `web-app/src/lib/omnibar/actions/types.ts`

##### Task 5.1.1a: Add union variant (~2 min)
- Add `create_alias_session` variant to `OmnibarAction`
- Files: `web-app/src/lib/omnibar/actions/types.ts`

#### Story 5.1.2: Add dispatch case for create_alias_session

**As a** developer, **I want** `dispatchOmnibarAction` to handle `create_alias_session`, **so that** the action creates a session with alias context.

**Acceptance Criteria**:
- `case "create_alias_session":` calls `deps.createSession` with `aliasName` field
- Unit test in `dispatch.test.ts`

**Files**: `web-app/src/lib/omnibar/actions/dispatch.ts`, `web-app/src/lib/omnibar/actions/dispatch.test.ts`

##### Task 5.1.2a: Add dispatch case (~3 min)
- Add `case "create_alias_session":` forwarding `aliasName`, `branch`, `label` as `title`
- Files: `web-app/src/lib/omnibar/actions/dispatch.ts`

##### Task 5.1.2b: Unit tests (~3 min)
- `describe("create_alias_session")` block in `dispatch.test.ts`
- Files: `web-app/src/lib/omnibar/actions/dispatch.test.ts`

#### Story 5.1.3: Thread aliasName from form to RPC to backend

**As a** developer, **I want** `aliasName` threaded from form submission through `useSessionService` to the proto, **so that** the backend resolves the alias.

**Acceptance Criteria**:
- `OmnibarSessionData.aliasName?: string` added
- `OmnibarContext.handleCreateSession` passes `aliasName`
- `useSessionService.createSession` passes `alias_name` to RPC
- `string alias_name = 27` in `CreateSessionRequest`
- `session_service.go` calls `config.ResolveAlias` when `req.Msg.AliasName != ""`

**Files**: `web-app/src/components/sessions/Omnibar.tsx`, `web-app/src/lib/contexts/OmnibarContext.tsx`, `web-app/src/lib/hooks/useSessionService.ts`, `proto/session/v1/session.proto`, `server/services/session_service.go`

##### Task 5.1.3a: Add alias_name = 27 to proto and regenerate (~2 min)
- Add `string alias_name = 27;` to `CreateSessionRequest`; run `make generate-proto`
- Files: `proto/session/v1/session.proto`

##### Task 5.1.3b: Add aliasName to OmnibarSessionData (~2 min)
- Add `aliasName?: string` to `OmnibarSessionData` interface
- Files: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 5.1.3c: Thread through OmnibarContext and useSessionService (~3 min)
- `OmnibarContext`: pass `aliasName: data.aliasName` to `createSession()`
- `useSessionService`: pass `aliasName: request.aliasName` to RPC body
- Files: `web-app/src/lib/contexts/OmnibarContext.tsx`, `web-app/src/lib/hooks/useSessionService.ts`

##### Task 5.1.3d: Handle alias_name in CreateSession handler (~6 min)
- **Blocker fix #2 — path guard**: The existing guard `if !req.Msg.OneOff && ... && req.Msg.Path == ""` at line ~965 must add `&& req.Msg.AliasName == ""`. Pathless aliases (e.g. `{"name": "quick", "cli_flags": "..."}`) are valid and must not 400.
- **Arch concern #5 — double-apply**: When `req.Msg.AliasName != ""`, **skip** the existing `ResolveDefaults` call at line ~1053. Route exclusively through `ResolveAlias` (which calls `ResolveDefaults` internally). Add early-return branch at the top of the defaults block: `if req.Msg.AliasName != "" { resolved, err := config.ResolveAlias(...); ... goto applyResolved }`.
- Call `config.ResolveAlias(cfg, req.Msg.AliasName, req.Msg.Branch, req.Msg.Title, "")` when `AliasName != ""`
- If alias not found: return `connect.CodeNotFound`
- Use `resolved.Path` as `resolvedPath` (if non-empty and `req.Msg.Path` is empty)
- Populate `program`, `instanceOpts.EnvVars`, `instanceOpts.CLIFlags`, `tags` from resolved result
- Files: `server/services/session_service.go`

#### Story 5.1.4: Wire Enter key for alias detection results

**As a** user, **I want** pressing Enter on an alias detection result to create the session, **so that** the full flow works end-to-end.

**Acceptance Criteria**:
- `InputType.Alias` + Enter → dispatch `create_alias_session`
- `InputType.AliasNotFound` + Enter → no-op (error chip already shown)

**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 5.1.4a: Wire Enter key for Alias type (~4 min)
- In `handleSubmit` / Enter handler, add `InputType.Alias` case dispatching `create_alias_session`
- Block submission for `InputType.AliasNotFound`
- Files: `web-app/src/components/sessions/Omnibar.tsx`

#### Story 5.1.5: Feature registry and e2e test stub

**As a** developer, **I want** the alias feature registered in the feature registry with a Playwright stub, **so that** CI coverage tracking is satisfied.

**Acceptance Criteria**:
- `docs/registry/features/alias.json` created
- `tests/e2e/alias.spec.ts` created with `// @feature session:create, alias:invoke` header and skeleton test

**Files**: `docs/registry/features/alias.json`, `tests/e2e/alias.spec.ts`

##### Task 5.1.5a: Create feature registry entry (~2 min)
- Create `docs/registry/features/alias.json`
- Files: `docs/registry/features/alias.json`

##### Task 5.1.5b: Create e2e test stub (~3 min)
- Create `tests/e2e/alias.spec.ts` with feature annotation and `test.skip` skeleton
- Files: `tests/e2e/alias.spec.ts`

---

## Flagged Risks and Review Resolutions

### Resolved blockers (from adversarial review)

1. **[RESOLVED] Fallthrough bug**: `AliasDetector` previously returned `null` for `@foo` (no space), letting `SessionSearchDetector` claim `^@` input. Fixed: `AliasDetector` now returns `InputType.AliasBrowse` for all `^@` input without a space, never `null`. See Task 3.1.3a.

2. **[RESOLVED] Path guard fires before alias resolution**: Pathless aliases would 400. Fixed: Task 5.1.3d adds `&& req.Msg.AliasName == ""` to the path guard at line ~965.

3. **[RESOLVED] CLIFlags replace vs append ambiguity**: Resolution layers use **replace** (existing `mergeProfileInto` behavior — correct). Invocation-time `extraFlags` are **appended** as an explicit final step in `ResolveAlias`. Documented in Pattern Decisions table and Task 2.1.2c.

### Remaining risks

4. **`@` collision with WorkflowDetector (priority 25 vs 36)**: Workflows win when slug = alias name. One-way door. Documented in ADR-020.

5. **Wire gap fix is a prerequisite (Phase 1)**: `EnvVars`/`CLIFlags` from `ResolveDefaults` are currently silently discarded. Phase 2+ depends on this working. Must be tested in isolation before building on top.

6. **`${VAR_NAME}` drops unset keys with a warning**: `log.Warn` emitted (non-blocking). See Task 2.1.2b.

7. **No new SessionType enum value**: Aliases resolve to existing types (`directory`, `new_worktree`). The 7-touchpoint session creation registry checklist is NOT triggered. Verified against `.claude/rules/session-creation-registry.md`.
