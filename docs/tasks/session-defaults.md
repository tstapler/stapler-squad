# Session Defaults & Profiles

Status: Ready for Implementation
Created: 2026-04-13
Branch: `claude-squad-session-default-configuration`

---

## Epic Overview

**User Value:** A new session can be created with zero manual field entry when defaults
are configured. Named profiles let the user capture frequently-used configurations and
apply them in one click. Per-directory defaults auto-populate the form based on the
working directory.

**Success Metrics:**
- Zero manual field entry for any session created in a directory with a matching rule
- Profile selection pre-fills all form fields in ≤200 ms
- Settings page saves persist without data loss across restarts and concurrent tabs
- `ResolveDefaults` returns correct merged result for 100% of precedence test cases

**Scope (in):** Global defaults, named profiles, directory rules, Settings page Defaults
section, create-session dialog integration (profile selector, source badges, "Save as
Profile" shortcut), `ResolveDefaults` RPC.

**Scope (out):** Cloud/cross-machine sync, REST API for programmatic mutation, `.stapler-squad.json`
per-repo file (deferred to follow-up).

**Constraints:** No new npm or Go packages; persist in existing `config.json`;
all new React styles in `.css.ts` (vanilla-extract); no breaking changes to existing
config files.

---

## Architecture Decisions

| # | File | Summary |
|---|------|---------|
| ADR-001 | `project_plans/session-defaults/decisions/ADR-001-config-extension-strategy.md` | Extend `config.json` with `session_defaults` struct; add `config_version`; apply flock to config writes |
| ADR-002 | `project_plans/session-defaults/decisions/ADR-002-precedence-algorithm.md` | Precedence: global → directory (longest-prefix) → profile; tags union; scalars override |
| ADR-003 | `project_plans/session-defaults/decisions/ADR-003-api-rpc-design.md` | Granular RPCs on existing SessionService; `ResolveDefaults` returns source metadata for per-field badges |

---

## Story 1: Go Data Model & Precedence Logic

**User Value:** The backend can store, retrieve, and resolve session defaults correctly
across all three layers (global / directory / profile) with no data migration required
for existing users.

**Acceptance Criteria:**
- `config.LoadConfig()` initializes all collection fields; no caller nil-panics
- `ResolveDefaults(workingDir, profileName)` returns correct merged result for all
  precedence combinations
- Existing `config.json` files load without error; `DefaultProgram` continues to work
- Config writes are protected by flock

---

### Task 1.1 — Config types + LoadConfig hardening [2h] ✅ Completed

**Objective:** Add `SessionDefaults`, `ProfileDefaults`, `DirectoryRule` types to
`config/config.go`; initialize collections in `LoadConfig()`; add `config_version`;
apply flock pattern from `state.go` to config writes.

**Context boundary:**
- Primary: `config/config.go` (~400 lines relevant)
- Supporting: `config/state.go` (flock reference implementation, ~50 lines)

**Prerequisites:** None — this is the first task.

**Implementation approach:**
1. Add types after the existing `Config` struct:
   ```go
   type SessionDefaults struct {
       Program        string                      `json:"program,omitempty"`
       AutoYes        bool                        `json:"auto_yes,omitempty"`
       Tags           []string                    `json:"tags,omitempty"`
       EnvVars        map[string]string           `json:"env_vars,omitempty"`
       CLIFlags       string                      `json:"cli_flags,omitempty"`
       Profiles       map[string]ProfileDefaults  `json:"profiles,omitempty"`
       DirectoryRules []DirectoryRule             `json:"directory_rules,omitempty"`
   }

   type ProfileDefaults struct {
       Name        string            `json:"name"`
       Description string            `json:"description,omitempty"`
       Program     string            `json:"program,omitempty"`
       AutoYes     bool              `json:"auto_yes,omitempty"`
       Tags        []string          `json:"tags,omitempty"`
       EnvVars     map[string]string `json:"env_vars,omitempty"`
       CLIFlags    string            `json:"cli_flags,omitempty"`
       CreatedAt   time.Time         `json:"created_at"`
       UpdatedAt   time.Time         `json:"updated_at"`
   }

   type DirectoryRule struct {
       Path     string         `json:"path"`
       Profile  string         `json:"profile,omitempty"`
       Overrides ProfileDefaults `json:"overrides,omitempty"`
   }
   ```
2. Add `ConfigVersion int` and `SessionDefaults SessionDefaults` fields to `Config` struct
3. In `LoadConfig()`, after `json.Unmarshal`, initialize nil collections:
   ```go
   if cfg.SessionDefaults.Profiles == nil {
       cfg.SessionDefaults.Profiles = make(map[string]ProfileDefaults)
   }
   if cfg.SessionDefaults.EnvVars == nil {
       cfg.SessionDefaults.EnvVars = make(map[string]string)
   }
   ```
4. Copy flock acquire/release pattern from `state.go` into `SaveConfig()`
5. Add `SaveConfig(cfg Config) error` function if not already exported (check existing)

**Validation:**
- Unit: existing `config_test.go` passes; add test loading a config without `session_defaults` field — verify no panic and collections are non-nil
- Unit: save + reload round-trip preserves `SessionDefaults` fields
- Success: `go test ./config/...` green

---

### Task 1.2 — ResolveDefaults pure function [2h] ✅ Completed

**Objective:** Implement `ResolveDefaults(cfg Config, workingDir, profileName string) ResolvedDefaults`
as a pure function in `config/config.go` (or a new `config/defaults.go`).

**Context boundary:**
- Primary: `config/config.go` (types from Task 1.1)
- Supporting: ADR-002 algorithm spec (precedence, merge rules)

**Prerequisites:** Task 1.1 complete (types exist).

**Implementation approach:**
1. Define return type:
   ```go
   type ResolvedDefaults struct {
       Program          string
       AutoYes          bool
       Tags             []string
       EnvVars          map[string]string
       CLIFlags         string
       UsedGlobal       bool
       UsedDirectory    bool
       UsedProfile      bool
       MatchedDirectory string
   }
   ```
2. Implement `findClosestDirectoryRule(rules []DirectoryRule, workingDir string) *DirectoryRule`:
   - Call `filepath.EvalSymlinks` on both paths before comparison
   - Return rule with longest matching `Path` prefix
3. Implement `mergeProfileInto(target *ResolvedDefaults, src ProfileDefaults)`:
   - Scalar fields: if src non-zero, overwrite target
   - Tags: union (append + dedup)
   - EnvVars: merge; src key overwrites target key
4. Implement `ResolveDefaults`:
   - Seed from `cfg.SessionDefaults` global fields (UsedGlobal = true)
   - Apply legacy `cfg.DefaultProgram` fallback if Global.Program is empty
   - Apply directory rule if matched; if rule has Profile, apply that profile first
   - Apply request's profileName (user-selected profile wins last)
5. Return `ResolvedDefaults` with source flags set

**Validation:**
- Unit: table-driven tests covering:
  - No rules, no profile → global defaults returned
  - Directory rule match (exact + prefix) → directory overrides applied
  - Profile selected → profile wins over directory
  - Tags union across all three layers
  - Symlink path equivalence
- Success: `go test ./config/... -run TestResolveDefaults` green

---

## Story 2: ConnectRPC API

**User Value:** Frontend can fetch resolved defaults and manage the defaults configuration
(global settings, profiles, directory rules) through well-defined RPC endpoints.

**Acceptance Criteria:**
- `ResolveDefaults` RPC returns correct merged defaults and source metadata
- CRUD RPCs for profiles and directory rules persist changes to config.json
- `CreateSessionRequest` supports optional `profile` field
- All new RPCs registered and callable from a browser ConnectRPC client

---

### Task 2.1 — Proto messages & RPCs [2h] ✅ Completed

**Objective:** Add new message types and RPC definitions to
`proto/session/v1/session.proto`.

**Context boundary:**
- Primary: `proto/session/v1/session.proto`
- Supporting: `proto/session/v1/types.proto` (reference for existing message style)

**Prerequisites:** None — proto work is independent of Go implementation.

**Implementation approach:**
1. Add `profile string = 11` and `skip_defaults bool = 12` to `CreateSessionRequest`
2. Add message types:
   - `ProfileDefaults` (program, auto_yes, tags, env_vars, cli_flags, name, description, created_at, updated_at)
   - `DirectoryRule` (path, profile, overrides ProfileDefaults)
   - `SessionDefaultsConfig` (program, auto_yes, tags, env_vars, cli_flags, profiles map, directory_rules repeated)
   - `ResolvedDefaults` (all scalar/collection fields + used_global, used_directory, used_profile, matched_directory)
   - Request/Response pairs for: `GetSessionDefaults`, `ResolveDefaults`, `UpdateGlobalDefaults`, `UpsertProfile`, `DeleteProfile`, `UpsertDirectoryRule`, `DeleteDirectoryRule`
3. Add RPCs to `SessionService`:
   ```protobuf
   rpc GetSessionDefaults(GetSessionDefaultsRequest) returns (GetSessionDefaultsResponse) {}
   rpc ResolveDefaults(ResolveDefaultsRequest) returns (ResolveDefaultsResponse) {}
   rpc UpdateGlobalDefaults(UpdateGlobalDefaultsRequest) returns (UpdateGlobalDefaultsResponse) {}
   rpc UpsertProfile(UpsertProfileRequest) returns (UpsertProfileResponse) {}
   rpc DeleteProfile(DeleteProfileRequest) returns (DeleteProfileResponse) {}
   rpc UpsertDirectoryRule(UpsertDirectoryRuleRequest) returns (UpsertDirectoryRuleResponse) {}
   rpc DeleteDirectoryRule(DeleteDirectoryRuleRequest) returns (DeleteDirectoryRuleResponse) {}
   ```
4. Run `make generate-proto` and verify generated Go + TS client code compiles

**Validation:**
- `make generate-proto` exits 0
- `go build ./...` green after generation
- Success: no compilation errors

---

### Task 2.2 — DefaultsService handler implementation [3h] ✅ Completed

**Objective:** Implement all defaults-related RPC handlers in a new
`server/services/defaults_service.go`.

**Context boundary:**
- Primary: `server/services/defaults_service.go` (new file, ~200 lines)
- Supporting: `server/services/config_service.go` (handler pattern reference),
  `config/config.go` (ResolveDefaults + types from Stories 1.1/1.2)

**Prerequisites:** Tasks 1.1, 1.2, 2.1 complete.

**Implementation approach:**
1. Define `DefaultsService struct {}` with constructor `NewDefaultsService()`
2. Implement `GetSessionDefaults`: load config, map `SessionDefaults` to proto response
3. Implement `ResolveDefaults`: call `config.ResolveDefaults(cfg, req.WorkingDir, req.ProfileName)`,
   map result to proto; handle `filepath.EvalSymlinks` error gracefully (log + continue)
4. Implement `UpdateGlobalDefaults`: load config, apply proto fields to `cfg.SessionDefaults`
   global fields, call `config.SaveConfig(cfg)`
5. Implement `UpsertProfile` / `DeleteProfile`: load config, mutate `Profiles` map, save
6. Implement `UpsertDirectoryRule` / `DeleteDirectoryRule`: load config, find/replace by path
   in `DirectoryRules` slice, save
7. Helper `protoToProfileDefaults` and `profileDefaultsToProto` converters

**Error handling pattern:** match existing handlers (`connect.NewError(connect.CodeInternal, err)`)

**Validation:**
- Unit: mock config load/save; test each handler with happy path + error case
- Integration: start test server, call `ResolveDefaults` with a seeded config, verify response
- Success: `go test ./server/services/... -run TestDefaults` green

---

### Task 2.3 — Register DefaultsService in server.go [1h] ✅ Completed

**Objective:** Wire `DefaultsService` into the HTTP mux in `server/server.go`.

**Context boundary:**
- Primary: `server/server.go` (registration block, ~20 lines)
- Supporting: `server/services/defaults_service.go` (from Task 2.2)

**Prerequisites:** Task 2.2 complete.

**Implementation approach:**
1. Instantiate `NewDefaultsService()` in the `Deps` or inline in `NewServer`
2. Register methods on `SessionService` handler — check if `DefaultsService` methods can
   be embedded or if `SessionService` needs to gain the new methods directly (match
   existing architecture; likely add to `SessionService` struct or compose via embedding)
3. Verify all new RPC paths appear in the mux

**Validation:**
- `make build` green
- `make restart-web` starts without panic
- Manual: `curl -s http://localhost:8543/session.v1.SessionService/GetSessionDefaults` returns
  a valid (empty) response

---

## Story 3: Create-Session Dialog Integration

**User Value:** The session creation form pre-populates with resolved defaults on open,
shows a profile selector, displays per-field source badges, and offers a "Save as
Profile" shortcut — eliminating repetitive manual entry.

**Acceptance Criteria:**
- Form fields pre-populated within 200 ms of dialog open for directories with rules
- Profile selector updates all unedited form fields when changed
- Each pre-filled field shows a subtle source badge (global / directory / profile)
- "Save as Profile" in Review step captures current form values and creates a new profile

---

### Task 3.1 — `useSessionDefaults` hook [2h] ✅ Completed

**Objective:** Create `web-app/src/lib/hooks/useSessionDefaults.ts` — fetches
`ResolveDefaults` and exposes resolved defaults + per-field source metadata.

**Context boundary:**
- Primary: `web-app/src/lib/hooks/useSessionDefaults.ts` (new file)
- Supporting: `web-app/src/lib/hooks/useRepositorySuggestions.ts` (hook pattern reference),
  generated ConnectRPC client

**Prerequisites:** Task 2.1 complete (generated TS client exists).

**Implementation approach:**
1. Define `FieldSource = 'global' | 'directory' | 'profile' | 'user'`
2. Hook signature:
   ```typescript
   function useSessionDefaults(workingDir: string, profileName?: string): {
     defaults: ResolvedDefaults | null;
     fieldSources: Record<keyof SessionFormData, FieldSource>;
     loading: boolean;
     error: string | null;
   }
   ```
3. Call `resolveDefaults({ workingDir, profileName })` using the generated client
4. Re-fetch on `workingDir` or `profileName` change (useEffect dep array)
5. Derive `fieldSources` from `used_global`, `used_directory`, `used_profile` flags
   in the response; assign `'user'` for fields not coming from any default layer

**Validation:**
- Unit: mock ConnectRPC client; test that changing `profileName` triggers re-fetch;
  test `fieldSources` derivation for each flag combination
- Success: hook returns correct `defaults` and `fieldSources` for mocked responses

---

### Task 3.2 — Profile selector + defaults injection in SessionWizard [3h] ✅ Completed

**Objective:** Add profile selector dropdown to the Configuration step of `SessionWizard.tsx`;
wire `useSessionDefaults` to pre-populate form fields; track per-field edit state so
profile changes don't overwrite user-edited fields.

**Context boundary:**
- Primary: `web-app/src/components/sessions/SessionWizard.tsx`
- Supporting: `web-app/src/lib/hooks/useSessionDefaults.ts` (Task 3.1),
  `web-app/src/components/sessions/SessionWizard.css.ts` (new, styles)

**Prerequisites:** Task 3.1 complete.

**Implementation approach:**
1. Add `selectedProfile` state (`useState<string>('')`) to `SessionWizard`
2. Call `useSessionDefaults(repositoryPath, selectedProfile)` in the wizard body
3. Track `editedFields = useRef(new Set<keyof SessionFormData>())` — add field key
   on `onChange`; clear on profile change
4. On `defaults` change: `reset({ ...defaultValues, ...defaults, ...editedFieldValues })`
   — preserve only explicitly edited fields
5. Add profile selector `<select>` in step 2 (Configuration) — populate options from
   `getSessionDefaults()` called once on wizard mount; add "None" as first option
6. Show "Pre-filled from defaults" notice in dialog header when `defaults !== null`
7. Add styles to `SessionWizard.css.ts` using vanilla-extract + `vars` tokens

**Validation:**
- Manual: open create dialog for a directory with a configured default → fields pre-filled
- Manual: switch profile → unedited fields update; edited fields preserved
- Manual: no defaults configured → "no defaults configured" hint visible
- Success: `make restart-web` + visual verification

---

### Task 3.3 — Per-field source badges [2h] ✅ Completed

**Objective:** Display subtle source indicators on pre-populated form fields in
`SessionWizard.tsx` (tooltip: "from global defaults" / "from Work profile" /
"from /projects/foo rule").

**Context boundary:**
- Primary: `web-app/src/components/sessions/SessionWizard.tsx`
- Supporting: `web-app/src/components/ui/Tooltip.tsx` (if exists, else inline `title=`)
- New: `web-app/src/components/sessions/SourceBadge.tsx` + `SourceBadge.css.ts`

**Prerequisites:** Task 3.2 complete (`fieldSources` available in wizard).

**Implementation approach:**
1. Create `SourceBadge` component: takes `source: FieldSource` + optional `detail: string`;
   renders a small inline label with tooltip; hidden when `source === 'user'`
2. Styles in `SourceBadge.css.ts`: use `vars.color.textMuted` / `vars.fontSize.xs`;
   no hardcoded hex values
3. Place `<SourceBadge source={fieldSources.program} detail={...} />` next to Program field label
4. Repeat for tags, autoYes, cli_flags
5. `detail` string derived from `matchedDirectory` or profile name from response

**Validation:**
- Visual: badge visible next to pre-filled fields, hidden on user-edited fields
- Accessibility: badge text readable; tooltip usable with keyboard
- Success: `make restart-web` + visual check

---

### Task 3.4 — "Save as Profile" in Review step [2h] ✅ Completed

**Objective:** Add "Save as Profile…" button in the Review step of `SessionWizard`
that opens a modal, accepts a name + description, and calls `UpsertProfile`.

**Context boundary:**
- Primary: `web-app/src/components/sessions/SessionWizard.tsx`
- Supporting: `web-app/src/components/ui/Modal.tsx` (existing or inline modal pattern),
  generated ConnectRPC client

**Prerequisites:** Task 2.2 complete (UpsertProfile RPC available).

**Implementation approach:**
1. Add `showSaveProfileModal` boolean state
2. "Save as Profile…" button in Review step action bar (before "Create Session")
3. Modal: name input (required), description textarea (optional), "Set as global default"
   checkbox; on submit calls `upsertProfile({ ...currentFormValues, name, description })`
4. On success: close modal, show brief success toast; profile now available in selector
5. Styles in `SessionWizard.css.ts`

**Validation:**
- Manual: fill wizard, click "Save as Profile…", enter name, save → profile appears in
  selector on next wizard open
- Error: duplicate name → server returns error → toast shown
- Success: `make restart-web` + manual flow test

---

## Story 4: Settings Page — Defaults Section

**User Value:** Users can manage global defaults, named profiles, and directory rules
from a dedicated Settings page without leaving the browser.

**Acceptance Criteria:**
- `/settings` route renders without error
- Global defaults section saves and immediately reflected in next `ResolveDefaults` call
- Profiles list shows name, description, edit/delete; create/edit form works inline
- Directory rules list shows path → profile/override; add/remove works

---

### Task 4.1 — `/settings` route + SettingsNav skeleton [2h] ✅ Completed

**Objective:** Create the Next.js route and layout for the Settings page at
`web-app/src/app/settings/`.

**Context boundary:**
- Primary: `web-app/src/app/settings/page.tsx` (new),
  `web-app/src/app/settings/layout.tsx` (new)
- Supporting: `web-app/src/app/config/layout.tsx` (existing layout pattern reference)

**Prerequisites:** None — route skeleton is independent.

**Implementation approach:**
1. Create `web-app/src/app/settings/layout.tsx` with `<SettingsNav>` sidebar
2. Create `web-app/src/app/settings/page.tsx` that redirects to `/settings/defaults`
3. Create `web-app/src/app/settings/defaults/page.tsx` as the target page (empty shell)
4. `SettingsNav` component: links to "Defaults" section; future sections stubbed
5. Add "Settings" link to main nav (check `web-app/src/app/layout.tsx` or nav component)
6. Styles in `.css.ts` files; use `vars` tokens; no module.css

**Validation:**
- `make restart-web` → `/settings` navigable; sidebar visible; no 404
- Success: page renders without console errors

---

### Task 4.2 — GlobalDefaults section [3h] ✅ Completed

**Objective:** Implement the Global Defaults section in the Settings page — program
selector, tags input, env vars key/value editor, CLI flags input — wired to
`GetSessionDefaults` (load) and `UpdateGlobalDefaults` (save).

**Context boundary:**
- Primary: `web-app/src/app/settings/defaults/page.tsx`,
  `web-app/src/components/settings/GlobalDefaultsForm.tsx` (new),
  `web-app/src/components/settings/GlobalDefaultsForm.css.ts` (new)
- Supporting: `web-app/src/components/sessions/SessionWizard.tsx` (existing field components
  for program selector, tags input)

**Prerequisites:** Task 4.1 complete (route exists), Task 2.2 complete (RPCs available).

**Implementation approach:**
1. `GlobalDefaultsForm` component: loads current defaults via `getSessionDefaults()` on
   mount; local form state mirrors proto fields
2. ProgramSelector: reuse `PROGRAMS` constant + `<select>` from SessionWizard
3. TagsInput: pill-based input (can reuse or adapt existing tag editor)
4. EnvVarsEditor: key/value table with add row / delete row buttons
5. CLIFlagsInput: plain text input
6. "Save" button calls `updateGlobalDefaults`; success toast; error toast on failure
7. Form pre-populated from `getSessionDefaults` response on mount

**Validation:**
- Manual: save a program change → create session dialog pre-fills that program
- Unit: form renders pre-populated values from mock RPC response
- Success: `make restart-web` + visual verification

---

### Task 4.3 — Profiles Manager section [3h] ✅ Completed

**Objective:** Implement the Profiles section in Settings — list of profiles (name,
description, created date), inline create/edit form, delete with confirmation.

**Context boundary:**
- Primary: `web-app/src/components/settings/ProfilesManager.tsx` (new),
  `web-app/src/components/settings/ProfilesManager.css.ts` (new)
- Supporting: `web-app/src/app/settings/defaults/page.tsx`

**Prerequisites:** Task 4.2 complete (page structure exists), Task 2.2 complete.

**Implementation approach:**
1. `ProfilesManager` component: loads profiles from `getSessionDefaults().profiles`
2. `ProfilesList`: each row shows name, description, last-updated, edit/delete buttons
3. `ProfileForm` (inline, appears on "New Profile" click or "Edit"): fields matching
   `ProfileDefaults` (program, auto_yes, tags, env_vars, cli_flags, name, description)
4. Save calls `upsertProfile`; delete calls `deleteProfile` with confirmation dialog
5. Optimistic UI: add/remove from local list on success before re-fetch
6. "Last used" date not in scope for this iteration (defer)

**Validation:**
- Manual: create profile → appears in list → appears in create-session profile selector
- Manual: delete profile → removed from list; create-session selector no longer shows it
- Success: `make restart-web` + end-to-end flow

---

### Task 4.4 — Directory Rules Manager section [3h] 🚧 In Progress — BLOCKING BUILD

**Objective:** Implement the Directory Rules section in Settings — list of rules
(path, profile, overrides), add/edit/delete, path validation.

**Context boundary:**
- Primary: `web-app/src/components/settings/DirectoryRulesManager.tsx` (new),
  `web-app/src/components/settings/DirectoryRulesManager.css.ts` (new)
- Supporting: `web-app/src/app/settings/defaults/page.tsx`

**Prerequisites:** Task 4.3 complete (page structure exists), Task 2.2 complete.

**Build blocker:** `web-app/src/app/settings/defaults/page.tsx` imports `DirectoryRulesManager`
from `@/components/settings/DirectoryRulesManager` which does not yet exist. The build will
fail until this component is created. This is the single remaining task blocking a shippable build.

**Implementation approach:**
1. `DirectoryRulesManager` component: loads rules from `getSessionDefaults().directory_rules`
2. `RulesList`: each row shows truncated path, profile name (if any), edit/delete
3. `DirectoryRuleForm`: path text input (validate it's an absolute path); profile selector
   (optional, selects from existing profiles); optional overrides section (same fields as
   ProfileDefaults but all optional)
4. Save calls `upsertDirectoryRule`; delete calls `deleteDirectoryRule`
5. Path validation: must start with `/` (macOS/Linux); show error inline if not

**Validation:**
- Manual: add rule for `/Users/tyler/projects/foo` → open create session dialog with
  that path → fields pre-filled from rule
- Manual: longest-prefix: add rules for `/projects` and `/projects/foo` → directory
  `/projects/foo/bar` uses `/projects/foo` rule
- Success: `make restart-web` + end-to-end flow

---

## Known Issues (Identified During Planning)

### 🐛 Bug-001: config.json unprotected concurrent writes [SEVERITY: High]

Concurrent saves from two browser tabs performing Settings updates will race. Last write
wins silently; the other save's data is lost.

**Mitigation:** Apply `flock` in Task 1.1 (SaveConfig). After Task 1.1, all config saves
go through the lock. Defer flock on reads unless profiling shows contention.

**Files affected:** `config/config.go`

---

### 🐛 Bug-002: Nil panic on DirectoryRules / Profiles after load [SEVERITY: High]

Old `config.json` files have no `session_defaults` key. Go will zero-value the struct,
leaving `Profiles` map and `DirectoryRules` slice as nil. Any range or map-access in
`ResolveDefaults` panics.

**Mitigation:** Initialize in `LoadConfig()` after unmarshal (Task 1.1). All callers
get safe non-nil collections.

**Files affected:** `config/config.go`, `config/defaults.go`

---

### 🐛 Bug-003: Symlink mismatch in directory matching [SEVERITY: Medium]

`/home/user/projects/myrepo` (symlink) vs `/mnt/ssd/code/myrepo` (real path) produce
different hashes and different prefix matches, so directory rules don't apply.

**Mitigation:** Call `filepath.EvalSymlinks()` in `findClosestDirectoryRule` before
comparison (Task 1.2). Log warning + continue on symlink resolution error.

**Files affected:** `config/defaults.go`

---

### 🐛 Bug-004: Profile change overwrites user-edited form fields [SEVERITY: High]

If user partially fills the create-session form, then switches profile, react-hook-form
`reset()` would overwrite their edits.

**Mitigation:** Track `editedFields` ref in SessionWizard (Task 3.2). Only re-populate
fields that have not been explicitly edited.

**Files affected:** `web-app/src/components/sessions/SessionWizard.tsx`

---

### 🐛 Bug-005: Tags merge semantics violated at the UI layer [SEVERITY: Medium]

Tags pre-populated from defaults are displayed as editable pills. If the user removes a
default tag and clicks "Save as Profile", the saved profile may incorrectly include all
default tags (from the merged result) rather than only the user's changes.

**Mitigation:** "Save as Profile" saves the form's current state — which includes the
user's explicit edits including tag removals. Document that "Save as Profile" captures
the merged + overridden state. (Full diff-save is deferred.)

**Files affected:** `web-app/src/components/sessions/SessionWizard.tsx`

---

## Dependency Visualization

```
Task 1.1 (Config types + flock)
    │
Task 1.2 (ResolveDefaults function)
    │
    ├─── Task 2.1 (Proto definitions)  ◄─── can start in parallel with 1.1/1.2
    │         │
    │    Task 2.2 (DefaultsService handlers)  ◄─── needs 1.1 + 1.2 + 2.1
    │         │
    │    Task 2.3 (Register in server.go)
    │         │
    ├─────────┤
    │         │
Task 3.1 (useSessionDefaults hook)      Task 4.1 (/settings route skeleton)
    │                                         │
Task 3.2 (Profile selector + injection)  Task 4.2 (GlobalDefaults section)
    │                                         │
Task 3.3 (Source badges)             Task 4.3 (Profiles Manager)
    │                                         │
Task 3.4 (Save as Profile)           Task 4.4 (Directory Rules Manager)

Stories 3 and 4 can proceed in parallel after Story 2 completes.
Within Story 3: 3.1 → 3.2 → 3.3, 3.4 (3.3 and 3.4 parallel).
Within Story 4: 4.1 → 4.2 → 4.3 → 4.4 (sequential).
```

---

## Integration Checkpoints

**After Story 1:**
- `go test ./config/...` passes including new ResolveDefaults table tests
- Load an old `config.json` — no panic, all collections initialized

**After Story 2:**
- `make build` green
- `make restart-web` starts
- Manual: `GetSessionDefaults` RPC returns valid empty response
- Manual: `UpsertProfile` → `GetSessionDefaults` shows new profile

**After Story 3:**
- End-to-end: set global default program → open create-session dialog → program pre-filled
- End-to-end: create profile → select in dialog → all fields pre-filled → create session

**Final sign-off:**
- All 4 success criteria from Epic Overview met
- `go test ./...` and `make quick-check` green
- No regressions in existing session creation flow (title-required validation, etc.)
- Settings page accessible from main nav

---

## Context Preparation Guide

### Story 1 start
Files to load: `config/config.go`, `config/state.go`, `config/config_test.go`
Concepts: Go zero-value JSON unmarshal, flock pattern, `omitempty` semantics

### Story 2 start
Files to load: `proto/session/v1/session.proto`, `proto/session/v1/types.proto`,
`server/services/config_service.go`, `server/server.go` (registration block)
Concepts: ConnectRPC handler pattern, proto field numbering, `make generate-proto` workflow

### Story 3 start
Files to load: `web-app/src/components/sessions/SessionWizard.tsx`,
`web-app/src/lib/hooks/useRepositorySuggestions.ts`, generated TS client
Concepts: react-hook-form `reset()`, useEffect deps, vanilla-extract `.css.ts`

### Story 4 start
Files to load: `web-app/src/app/config/layout.tsx`, `web-app/src/app/config/page.tsx`,
`web-app/src/app/layout.tsx` (nav links)
Concepts: Next.js app router layouts, vanilla-extract `recipe()`, ConnectRPC client usage

---

## Success Criteria

- [x] All 11 atomic tasks completed and individually validated (10/11 done; Task 4.4 remaining)
- [x] `go test ./config/... -run TestResolveDefaults` — all precedence cases pass (8 table tests)
- [ ] `go test ./server/services/... -run TestDefaults` — handler tests pass (not yet written)
- [ ] `make quick-check` green (build + test + lint) — BLOCKED on Task 4.4 (missing DirectoryRulesManager)
- [ ] Settings page reachable at `/settings/defaults`; all three sections functional — BLOCKED on Task 4.4
- [x] Create-session dialog pre-fills from defaults; profile selector works; badges visible
- [x] "Save as Profile" round-trip works end-to-end
- [x] Old `config.json` loads without panic (backward compat verified)
- [ ] No regressions in existing create-session validation or session listing (needs `make restart-web` verification)
