# Implementation Plan: alias-settings-manager

**Feature**: Add UpsertAlias / DeleteAlias RPCs and an AliasesManager React component to the Settings > General tab, enabling full CRUD of alias session presets without manual `config.json` editing.
**Date**: 2026-06-21
**Status**: Ready for implementation
**ADRs**: None (no new architectural decisions required; this feature follows established patterns without novel trade-offs)

---

## Step 0.5 — Creative Pass: Architecture Alternatives

Three high-level approaches were evaluated:

| Approach | Key Strength | Key Weakness | Decision |
|----------|-------------|--------------|----------|
| **A: Fork ProfilesManager (chosen)** | ~70% code already written; consistent with every other settings component; lowest review friction | Still requires adding 6 new form fields + EnvVarsEditor sub-component | **SELECTED** |
| **B: Generic settings manager HOC** | Would unify ProfilesManager, DirectoryRulesManager, and AliasesManager under one abstraction | AliasConfig has 10 fields vs. ProfileDefaults' 5; the field delta is large enough to make a HOC unwieldy; no prior art in codebase | Rejected |
| **C: Alias-specific modal (Radix Dialog)** | Modal pattern matches mental model of "creating a new thing" | Breaks Settings consistency (no other Settings component uses a modal); violates the "no Radix in settings" precedent from ProfilesManager | Rejected |

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| **Alias** | A named session preset stored in `config.SessionDefaults.Aliases []AliasConfig`; invoked via `@name` in the omnibar | One entry in the `aliases` JSON array in `config.json` |
| **AliasName** | The unique, immutable string identifier for an alias; must match `^[\w-]+$` | Natural key for all CRUD operations; disabled during edit |
| **AliasPreset** | The full bundle of config fields (`path`, `program`, `tags`, `envVars`, etc.) associated with one `AliasName` | Proto representation: `AliasProto` |
| **UpsertAlias** | Create-or-update operation matched on `AliasName`; replaces in-place if found, appends if new | Follows slice-scan pattern (not map insertion) |
| **DeleteAlias** | Remove operation matched on `AliasName`; returns `CodeNotFound` if the alias does not exist | Filters the `Aliases` slice, saves config |
| **EnvVarsEditor** | Bespoke sub-component rendering a `{key, value}[]` array as side-by-side inputs with add/remove rows | Sourced from `GlobalDefaultsForm.tsx` pattern; placed in the Advanced section |
| **AliasesManager** | The React component added to Settings > General providing list + bottom-anchored form card for alias CRUD | Forked from `ProfilesManager.tsx`; adds group, path, profile, cliFlags, envVars fields |
| **aliasConfigToProto** | Existing helper in `defaults_service.go` (line 224) that maps `config.AliasConfig` → `*sessionv1.AliasProto` | Must be verified to map `EnvVars` and `CLIFlags` fields before building the upsert handler (see Story 1.2.1c) |
| **aliasNameRE** | Package-level compiled regex `^[\w-]+$` in `defaults_service.go` used by `UpsertAlias` for server-side name validation | Prevents names with spaces/slashes from entering `config.json` |
| **slice-scan upsert** | The pattern: iterate `[]T`, replace in-place on name match, append if not found — as opposed to map-key assignment | Used by `UpsertDirectoryRule`; distinct from `UpsertProfile` which uses a Go map |
| **Advanced section** | Collapsible form area (checkbox-gated, collapsed by default) containing `envVars` + `cliFlags` | Mirrors `showOverrides` checkbox in `DirectoryRulesManager` |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Go backend CRUD | Transaction Script (PoEAA): load cfg → mutate → save cfg | `UpsertDirectoryRule` / `DeleteDirectoryRule` in `defaults_service.go` | Domain model / event sourcing | Simple CRUD; no business rules beyond name validation |
| Slice upsert | Slice-scan (iterate, replace-or-append) | `UpsertDirectoryRule` lines 186–197 | Map key assignment | `Aliases` is `[]AliasConfig`, not a map |
| React form state | Raw `useState` | `ProfilesManager.tsx`, `DirectoryRulesManager.tsx`, `GlobalDefaultsForm.tsx` | `react-hook-form` (already installed) | Consistency; react-hook-form used only in deprecated `SessionWizard` |
| `env_vars` editor | Bespoke `EnvVarsEditor` sub-component (~50 lines), `{key,value}[]` state | `GlobalDefaultsForm.tsx` lines 123–283 | KEY=VALUE textarea | Textarea parsing is error-prone (values with `=`); row-level UI is already established |
| CSS framework | vanilla-extract `style()`, `vars` from `@/styles/theme.css` | ADR-009, `ProfilesManager.css.ts` | CSS Modules, inline styles | ADR-009 mandates vanilla-extract for all new components |
| Form modal vs inline | Bottom-anchored `formCard` below the list | `ProfilesManager`, `DirectoryRulesManager` | Radix Dialog modal | Consistent with all Settings components; no modal precedent |
| Name conflict check | Client-side pre-check on save (compare against loaded aliases, case-insensitive) | — | Server-side 409 only | Immediate feedback; avoids round-trip for a predictable local check |
| Generic settings HOC | Rejected — fork `ProfilesManager` directly | — | `withSettingsManager<T>()` HOC | Field delta too large; no prior art; fork wins on review friction |
| Alias-specific modal | Rejected | — | Radix Dialog | Breaks Settings consistency |

---

## Observability Plan

- **Logs**: `log.Info("upserted alias", "name", alias.Name)` and `log.Info("deleted alias", "name", req.Msg.Name)` in the Go handlers — mirrors existing patterns in `UpsertProfile` and `DeleteProfile`.
- **Metrics**: None required. Config writes are infrequent; no SLO defined for this feature.
- **Alerts**: None. Standard request logging covers the surface.

---

## Risk Control

- **Feature flag**: Not needed. The feature is purely additive (new RPCs + new UI section); rollback = reverting the 4 changed files and regenerated bindings.
- **Rollback procedure**: Revert `proto/session/v1/session.proto`, `server/services/defaults_service.go`, `server/services/session_service.go`, `web-app/src/app/settings/page.tsx`, and the two new component files. Run `make generate-proto` to restore bindings.
- **Staged rollout**: Not applicable (single-user local service).

---

## Unresolved Questions

None. All open questions from requirements have been resolved by research:

| Question | Resolution |
|----------|-----------|
| Does saving an alias require a service restart? | **No.** Both RPC handlers and session creation call `config.LoadConfig()` per-request. Changes are immediately live. No restart banner needed. |
| `env_vars` UI: key-value editor or textarea? | **Key-value editor** (`GlobalDefaultsForm` pattern). The structured editor exists in the codebase and is preferred by UX research. |
| Concurrent write safety | Accept existing last-write-wins pattern (same as all other config CRUD); document in a code comment; file a follow-up to add a package-level mutex. |

---

## Dependency Visualization

```
proto/session/v1/session.proto
  ├── adds: UpsertAliasRequest, UpsertAliasResponse
  ├── adds: DeleteAliasRequest, DeleteAliasResponse
  ├── adds: rpc UpsertAlias, rpc DeleteAlias
  └── runs: make generate-proto
        ├── gen/proto/go/session/v1/session_pb.go  (auto-generated)
        └── web-app/src/gen/session/v1/session_pb.ts  (auto-generated)
              └── imported by: AliasesManager.tsx

server/services/defaults_service.go
  ├── reads: config.LoadConfig()
  ├── reads: config.AliasConfig (config/config.go:441)
  ├── reads: aliasConfigToProto() (same file, line 224)
  └── writes: config.SaveConfig()

server/services/session_service.go
  └── delegates: UpsertAlias → DefaultsService.UpsertAlias
  └── delegates: DeleteAlias → DefaultsService.DeleteAlias

web-app/src/components/settings/AliasesManager.tsx
  ├── imports: AliasProto, SessionService (from generated TS)
  ├── calls: listAliases, upsertAlias, deleteAlias RPCs
  └── imports: AliasesManager.css.ts

web-app/src/app/settings/page.tsx
  └── renders: <AliasesManager /> in General tab

docs/registry/features/backend/alias/
  ├── upsert.json  (new)
  ├── delete.json  (new)
  └── list.json    (new)
docs/registry/features/alias.json
  └── updated: alias:invoke entry (existing)
docs/registry/features/alias-manager.json  (new)
```

---

## Phase 1: Backend RPC Layer

### Epic 1.1: Proto messages and RPC declarations

**Goal**: Extend `session.proto` with `UpsertAlias`/`DeleteAlias` RPCs and regenerate bindings.

---

#### Story 1.1.1: Add UpsertAlias and DeleteAlias to session.proto

**File**: `proto/session/v1/session.proto`

**Tasks**:

**Task 1.1.1a — Add RPC declarations to SessionService (2 min)**

Insert after `rpc ListAliases` (line 382), before `rpc ArchiveSession`:

```protobuf
  // UpsertAlias creates or updates a named alias preset (matched by name).
  rpc UpsertAlias(UpsertAliasRequest) returns (UpsertAliasResponse) {}

  // DeleteAlias removes an alias preset by name.
  rpc DeleteAlias(DeleteAliasRequest) returns (DeleteAliasResponse) {}
```

**Task 1.1.1b — Add message definitions (3 min)**

Insert immediately after `ListAliasesResponse` (after line 1791), before `ListWorktreesRequest`:

```protobuf
message UpsertAliasRequest {
  AliasProto alias = 1;
}

message UpsertAliasResponse {
  AliasProto alias = 1;
}

message DeleteAliasRequest {
  string name = 1;
}

message DeleteAliasResponse {}
```

**Acceptance criteria**:

- Given `proto/session/v1/session.proto` is opened, when searching for `rpc UpsertAlias`, then it appears in the `SessionService` service block after `rpc ListAliases`.
- Given `proto/session/v1/session.proto` is opened, when searching for `message UpsertAliasRequest`, then it appears after `ListAliasesResponse` with field `AliasProto alias = 1;`.
- Given `proto/session/v1/session.proto` is opened, when searching for `message DeleteAliasRequest`, then it has `string name = 1;` as its only field.
- Given `proto/session/v1/session.proto` is opened, when searching for `message DeleteAliasResponse`, then it is an empty message body `{}`.

---

#### Story 1.1.2: Regenerate proto bindings

**Task 1.1.2a — Run proto generation (2 min)**

```bash
make generate-proto
```

**Acceptance criteria**:

- Given `make generate-proto` is run after Story 1.1.1, when `gen/proto/go/session/v1/session_pb.go` is opened, then `UpsertAliasRequest`, `UpsertAliasResponse`, `DeleteAliasRequest`, `DeleteAliasResponse` structs are present.
- Given `make generate-proto` completes, when `web-app/src/gen/session/v1/session_pb.ts` is opened, then `UpsertAliasRequest`, `UpsertAliasResponse`, `DeleteAliasRequest`, `DeleteAliasResponse` message classes are exported.
- Given both generated files updated, when `go build ./...` is run, then it exits 0 with no errors.

---

### Epic 1.2: Go handler implementation

**Goal**: Implement `UpsertAlias` and `DeleteAlias` on `DefaultsService`; expose via thin wrappers on `SessionService`.

---

#### Story 1.2.1: Add aliasNameRE and UpsertAlias to DefaultsService

**File**: `server/services/defaults_service.go`

**Task 1.2.1a — Add package-level regex (2 min)**

Add immediately after the import block (before `type DefaultsService struct{}`):

```go
// aliasNameRE validates alias names: letters, digits, hyphens, underscores only.
var aliasNameRE = regexp.MustCompile(`^[\w-]+$`)
```

Add `"regexp"` to the import block.

**Task 1.2.1c — Verify aliasConfigToProto maps all fields (2 min)**

Before implementing the upsert handler, verify that the existing `aliasConfigToProto()` helper (line 224 of `defaults_service.go`) maps **both** `EnvVars` and `CLIFlags` fields from `config.AliasConfig` to the corresponding proto fields. Open the file and check the helper:

- If `EnvVars` and `CLIFlags` are already mapped: no change needed, proceed to Task 1.2.1b.
- If either field is missing: add the mapping now, before building the upsert handler. The handler (`UpsertAliasResponse`) returns `aliasConfigToProto(alias)` — a partial mapping here would silently drop fields from the response. The expected mappings are:
  ```go
  EnvVars:  alias.EnvVars,   // map[string]string
  CliFlags: alias.CLIFlags,  // string
  ```

This task is a prerequisite for Story 1.2.1b; if the helper is incomplete, fix it first so the upsert response is complete.

**Task 1.2.1b — Implement UpsertAlias method (5 min)**

Add after the `ListAliases` method (after line 237, before `aliasConfigToProto`):

```go
// UpsertAlias creates or updates a named alias preset (matched by name).
// NOTE: like all other config-write handlers, this follows the lock-free
// load-modify-save pattern. Concurrent writes are last-write-wins — this is
// the accepted project tradeoff; see DefaultsService for context.
func (d *DefaultsService) UpsertAlias(
	ctx context.Context,
	req *connect.Request[sessionv1.UpsertAliasRequest],
) (*connect.Response[sessionv1.UpsertAliasResponse], error) {
	if req.Msg.Alias == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("alias is required"))
	}
	name := strings.TrimSpace(req.Msg.Alias.Name)
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("alias name is required"))
	}
	if !aliasNameRE.MatchString(name) {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("alias name %q must match ^[\\w-]+$ (letters, digits, hyphens, underscores only)", name))
	}

	cfg := config.LoadConfig()

	alias := config.AliasConfig{
		Name:        name,
		Group:       req.Msg.Alias.Group,
		Path:        req.Msg.Alias.Path,
		Description: req.Msg.Alias.Description,
		Profile:     req.Msg.Alias.Profile,
		Program:     req.Msg.Alias.Program,
		AutoYes:     req.Msg.Alias.AutoYes,
		Tags:        req.Msg.Alias.Tags,
		EnvVars:     req.Msg.Alias.EnvVars,
		CLIFlags:    req.Msg.Alias.CliFlags,
	}
	if alias.Tags == nil {
		alias.Tags = []string{}
	}
	if alias.EnvVars == nil {
		alias.EnvVars = make(map[string]string)
	}

	// Slice-scan upsert: replace existing entry or append.
	// (Same pattern as UpsertDirectoryRule — aliases are a []AliasConfig slice, not a map.)
	// IMPORTANT: comparison is case-insensitive to enforce uniqueness across
	// "MyProj" and "myproj". New names that differ only by case from an existing
	// alias are treated as updates (overwrite-in-place), consistent with the
	// client-side uniqueness check in AliasesManager.tsx.
	found := false
	for i, a := range cfg.SessionDefaults.Aliases {
		if strings.ToLower(a.Name) == strings.ToLower(alias.Name) {
			cfg.SessionDefaults.Aliases[i] = alias
			found = true
			break
		}
	}
	if !found {
		cfg.SessionDefaults.Aliases = append(cfg.SessionDefaults.Aliases, alias)
	}

	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	log.Info("upserted alias", "name", alias.Name)
	return connect.NewResponse(&sessionv1.UpsertAliasResponse{
		Alias: aliasConfigToProto(alias),
	}), nil
}
```

Add `"strings"` to the import block if not already present.

**Acceptance criteria**:

- Given `UpsertAlias` is called with `alias.name = ""`, when the handler runs, then it returns `connect.CodeInvalidArgument` and does not write `config.json`.
- Given `UpsertAlias` is called with `alias.name = "my project"` (contains space), when the handler runs, then it returns `connect.CodeInvalidArgument` with a message referencing `^[\w-]+$`.
- Given `UpsertAlias` is called with `alias = { name: "myproj", path: "~/code/myproject", program: "claude" }` and no alias named `myproj` exists, when the handler runs, then `config.json` contains a new entry `{ "name": "myproj", "path": "~/code/myproject", "program": "claude" }` in the `aliases` array.
- Given `UpsertAlias` is called with `alias = { name: "myproj", description: "updated" }` and an alias named `myproj` already exists, when the handler runs, then the existing entry is replaced (not duplicated) and the response `alias.description` equals `"updated"`.

---

#### Story 1.2.2: Implement DeleteAlias on DefaultsService

**File**: `server/services/defaults_service.go`

**Task 1.2.2a — Add DeleteAlias method (4 min)**

Add after `UpsertAlias`:

```go
// DeleteAlias removes a named alias preset by name.
func (d *DefaultsService) DeleteAlias(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteAliasRequest],
) (*connect.Response[sessionv1.DeleteAliasResponse], error) {
	name := strings.TrimSpace(req.Msg.Name)
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("alias name is required"))
	}

	cfg := config.LoadConfig()

	newAliases := make([]config.AliasConfig, 0, len(cfg.SessionDefaults.Aliases))
	deleted := false
	for _, a := range cfg.SessionDefaults.Aliases {
		if a.Name == name {
			deleted = true
			continue
		}
		newAliases = append(newAliases, a)
	}
	if !deleted {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("alias %q not found", name))
	}
	cfg.SessionDefaults.Aliases = newAliases

	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
	}

	log.Info("deleted alias", "name", name)
	return connect.NewResponse(&sessionv1.DeleteAliasResponse{}), nil
}
```

**Acceptance criteria**:

- Given `DeleteAlias` is called with `name = ""`, when the handler runs, then it returns `connect.CodeInvalidArgument`.
- Given `DeleteAlias` is called with `name = "nonexistent"` and no such alias exists, when the handler runs, then it returns `connect.CodeNotFound` and does not modify `config.json`.
- Given `DeleteAlias` is called with `name = "myproj"` and that alias exists among 3 aliases, when the handler runs, then `config.json` `aliases` array contains exactly 2 entries (the other two) and the response is `DeleteAliasResponse{}`.

---

#### Story 1.2.3: Add thin wrappers to SessionService

**File**: `server/services/session_service.go`

**Task 1.2.3a — Add UpsertAlias and DeleteAlias delegating wrappers (3 min)**

Find the existing `ListAliases` wrapper (around line 3034). Add immediately after it:

```go
// UpsertAlias creates or updates a named alias preset.
func (s *SessionService) UpsertAlias(ctx context.Context, req *connect.Request[sessionv1.UpsertAliasRequest]) (*connect.Response[sessionv1.UpsertAliasResponse], error) {
	return s.defaultsSvc.UpsertAlias(ctx, req)
}

// DeleteAlias removes a named alias preset by name.
func (s *SessionService) DeleteAlias(ctx context.Context, req *connect.Request[sessionv1.DeleteAliasRequest]) (*connect.Response[sessionv1.DeleteAliasResponse], error) {
	return s.defaultsSvc.DeleteAlias(ctx, req)
}
```

**Acceptance criteria**:

- Given `server/services/session_service.go` is compiled with `go build ./...`, when both wrapper methods reference the correct `DefaultsService` methods, then it exits 0.
- Given the connect handler registration in `server/server.go` already registers all `SessionService` methods via the generated handler, when `make build` runs, then the `UpsertAlias` and `DeleteAlias` RPCs are reachable at the expected ConnectRPC paths.

---

#### Story 1.2.4: Go unit tests for UpsertAlias and DeleteAlias

**File**: `server/services/defaults_service_test.go` (new file if it does not exist; otherwise extend it)

**Task 1.2.4a — Write tests (5 min)**

Write Go test functions (follow the same `TestUpsertProfile_*` naming pattern):

```go
// TestUpsertAlias_EmptyName ensures an empty name returns CodeInvalidArgument.
// TestUpsertAlias_InvalidName ensures a name with spaces returns CodeInvalidArgument.
// TestUpsertAlias_CreatesAlias verifies a new alias appears in config.json.
// TestUpsertAlias_UpdatesExistingAlias verifies replace-not-duplicate semantics.
// TestDeleteAlias_NotFound verifies CodeNotFound for a missing alias.
// TestDeleteAlias_DeletesAlias verifies the alias is removed and others remain.
```

> **Config isolation (CRITICAL):** Go tests for `DefaultsService` must set `STAPLER_SQUAD_TEST_DIR` to a temp directory so config writes are isolated and don't corrupt the developer's real `~/.stapler-squad/config.json`. `STAPLER_SQUAD_TEST_DIR` is the standard config injection mechanism in this codebase. Add `t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())` as the **first line** of every Go test that calls `UpsertAlias` or `DeleteAlias`. Example:
> ```go
> func TestUpsertAlias_CreatesAlias(t *testing.T) {
>     t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
>     // ... test body
> }
> ```

**Task 1.2.4b — Verify server-side regex coverage (note)**

The Go handler uses `aliasNameRE = regexp.MustCompile(`^[\w-]+$`)`. At least one test (`TestUpsertAlias_InvalidName`) must exercise this path with a name that contains a space or slash to confirm the regex guard works. This is the authoritative validation; the TypeScript copy in `AliasesManager.tsx` exists only for immediate UI feedback.

**Task 1.2.4c — Case-insensitive uniqueness test (note)**

The slice-scan upsert in `UpsertAlias` compares names case-insensitively (`strings.ToLower(a.Name) == strings.ToLower(alias.Name)`). Add a test `TestUpsertAlias_CaseInsensitiveDuplicate` that pre-populates config with `{name: "MyProj"}` and calls `UpsertAlias` with `{name: "myproj"}`. The expected behavior must be documented in a code comment: the upsert loop compares `strings.ToLower(a.Name) == strings.ToLower(alias.Name)` to treat `"MyProj"` and `"myproj"` as the same alias (overwrite-in-place), consistent with the client-side case-insensitive uniqueness check in `AliasesManager.tsx` (see Story 2.1.3).

**Acceptance criteria**:

- Given `go test ./server/services/...` is run after implementation, when all 6+ test functions execute, then all pass.
- Given `TestUpsertAlias_InvalidName` calls `UpsertAlias` with `name = "my project"`, then the returned error code is `connect.CodeInvalidArgument` and the error message contains `^[\w-]+$`.
- Given `TestUpsertAlias_UpdatesExistingAlias` pre-populates config with `{name: "myproj", description: "old"}`, when `UpsertAlias` is called with `{name: "myproj", description: "new"}`, then the config slice has exactly 1 entry with `description == "new"`.
- Given `TestUpsertAlias_CaseInsensitiveDuplicate` pre-populates config with `{name: "MyProj"}` and calls `UpsertAlias` with `{name: "myproj"}`, then the behavior matches the documented policy (reject-as-duplicate recommended) and the config has exactly 1 alias entry.

---

## Phase 2: Frontend Component

### Epic 2.1: AliasesManager component

**Goal**: Create `AliasesManager.tsx` + `AliasesManager.css.ts` providing full alias CRUD in the Settings General tab.

---

#### Story 2.1.1: Create AliasesManager.css.ts

**File**: `web-app/src/components/settings/AliasesManager.css.ts` (new file)

**Task 2.1.1a — Create the stylesheet (4 min)**

Fork `ProfilesManager.css.ts` verbatim, rename all exports to avoid collisions (they are already module-scoped, so collisions don't occur — but use the same names for IDE discoverability), and add the following new token groups:

```typescript
// envVarTable — wrapper div for the key-value pair editor rows
export const envVarTable = style({ display: "flex", flexDirection: "column", gap: vars.space["2"] });

// envVarRow — single key+value+delete row (side-by-side)
export const envVarRow = style({ display: "flex", gap: vars.space["2"], alignItems: "center" });

// envVarInput — key or value input inside an env var row
export const envVarInput = style({
  flex: 1,
  backgroundColor: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  color: vars.color.inputText,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.sm,
  ":focus": { outline: "none", borderColor: vars.color.inputFocusBorder },
});

// deleteBtn — per-row delete button (danger style)
export const deleteBtn = style({
  backgroundColor: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.sm,
  color: vars.color.errorText,
  cursor: "pointer",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
});

// advancedToggle — the "Advanced options" checkbox label row
export const advancedToggle = style({ display: "flex", alignItems: "center", gap: vars.space["2"], cursor: "pointer" });

// previewHint — the @name live preview below the name field
export const previewHint = style({ color: vars.color.textMuted, fontSize: vars.fontSize.xs, marginTop: vars.space["1"] });

// fieldError — inline validation error below a field
export const fieldError = style({ color: vars.color.error, fontSize: vars.fontSize.xs, marginTop: vars.space["1"] });

// groupHint — muted hint below the Group field
export const groupHint = style({ color: vars.color.textMuted, fontSize: vars.fontSize.xs, marginTop: vars.space["1"] });
```

All other tokens (`container`, `heading`, `headerRow`, `formCard`, `formTitle`, `formFields`, `field`, `label`, `checkboxLabel`, `input`, `select`, `tagList`, `tag`, `tagRemove`, `tagInputRow`, `formActions`, `loadingText`, `emptyText`, `profileRow`/renamed `aliasRow`, `profileInfo`/`aliasInfo`, `profileName`/`aliasName`, `profileDesc`/`aliasDesc`, `profileMeta`/`aliasMeta`, `profileActions`/`aliasActions`) are copied verbatim from `ProfilesManager.css.ts` with the `profile*` names replaced by `alias*`.

**Acceptance criteria**:

- Given `AliasesManager.css.ts` is saved, when `make build` (which includes TypeScript compilation) runs, then it exits 0 with no CSS lint errors.
- Given the file imports `vars` from `@/styles/theme.css`, when all token references are verified, then no raw hex values or `var(--*-*)` strings are used — all colors reference `vars.color.*`.

---

#### Story 2.1.2: Create AliasesManager.tsx — core structure

**File**: `web-app/src/components/settings/AliasesManager.tsx` (new file)

**Task 2.1.2a — Scaffold interface and state (3 min)**

```typescript
"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { SessionService, type AliasProto } from "@/gen/session/v1/session_pb";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { getApiBaseUrl } from "@/lib/config";
import { PROGRAMS } from "@/lib/constants/programs";
// import all tokens from AliasesManager.css

interface EnvVar { key: string; value: string; }

interface AliasFormData {
  name: string;
  description: string;
  group: string;
  path: string;
  profile: string;
  program: string;
  autoYes: boolean;
  tags: string[];
  tagInput: string;
  envVars: EnvVar[];
  cliFlags: string;
  showAdvanced: boolean;
}

const emptyForm: AliasFormData = {
  name: "", description: "", group: "", path: "",
  profile: "", program: "", autoYes: false,
  tags: [], tagInput: "", envVars: [], cliFlags: "", showAdvanced: false,
};
```

State variables:
```typescript
const [aliases, setAliases] = useState<AliasProto[]>([]);
const [loading, setLoading] = useState(true);
const [error, setError] = useState<string | null>(null);
const [nameError, setNameError] = useState<string | null>(null);  // inline field error
const [success, setSuccess] = useState<string | null>(null);
const [showForm, setShowForm] = useState(false);
const [editingName, setEditingName] = useState<string | null>(null);
const [form, setForm] = useState<AliasFormData>({ ...emptyForm });
const [saving, setSaving] = useState(false);
const nameInputRef = useRef<HTMLInputElement>(null);
const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
```

**Task 2.1.2b — Implement loadAliases, handleEdit, handleNewAlias, handleCancel (3 min)**

```typescript
const loadAliases = useCallback(async () => {
  if (!clientRef.current) return;
  try {
    setLoading(true);
    setError(null);
    const response = await clientRef.current.listAliases({});
    setAliases(response.aliases);
  } catch (err) {
    setError(`Failed to load aliases: ${err}`);
  } finally {
    setLoading(false);
  }
}, []);

useEffect(() => {
  const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
  clientRef.current = createClient(SessionService, transport);
  loadAliases();
}, [loadAliases]);

// Focus name field when form opens
useEffect(() => {
  if (showForm && !editingName) {
    nameInputRef.current?.focus();
  }
}, [showForm, editingName]);

const handleEdit = (alias: AliasProto) => {
  setEditingName(alias.name);
  setNameError(null);
  setForm({
    name: alias.name, description: alias.description, group: alias.group,
    path: alias.path, profile: alias.profile, program: alias.program,
    autoYes: alias.autoYes, tags: [...alias.tags], tagInput: "",
    envVars: Object.entries(alias.envVars).map(([key, value]) => ({ key, value })),
    cliFlags: alias.cliFlags, showAdvanced: !!(alias.envVars && Object.keys(alias.envVars).length > 0) || !!alias.cliFlags,
  });
  setShowForm(true);
};

const handleNewAlias = () => {
  setEditingName(null);
  setNameError(null);
  setForm({ ...emptyForm });
  setShowForm(true);
};

const handleCancel = () => {
  setShowForm(false);
  setEditingName(null);
  setNameError(null);
  setForm({ ...emptyForm });
};
```

**Acceptance criteria**:

- Given `AliasesManager` mounts, when `loadAliases` is called, then it calls `clientRef.current.listAliases({})` and populates the `aliases` state from `response.aliases`.
- Given `handleNewAlias` is called, when the component re-renders, then `showForm` is `true`, `editingName` is `null`, and `form` equals `emptyForm`.
- Given `handleEdit` is called with `{ name: "myproj", envVars: { FOO: "bar" } }`, when form state is set, then `form.envVars` equals `[{ key: "FOO", value: "bar" }]`.
- Given `showForm` becomes `true` and `editingName` is `null`, when the effect fires, then `nameInputRef.current.focus()` is called.

---

#### Story 2.1.3: Implement handleSave with client-side validation

**File**: `web-app/src/components/settings/AliasesManager.tsx`

**Task 2.1.3a — Implement handleSave (5 min)**

> **Regex drift note**: Do NOT re-define the name validation regex inline. Import it from `AliasDetector.ts` instead (or from a shared constants module if one is extracted). `AliasDetector.ts` already uses `/^@[\w-]+$/` for alias-name matching; the name-only form is `/^[\w-]+$/`. Using the same source-of-truth prevents the client UI and the detector from drifting out of sync. If `AliasDetector.ts` does not export a constant for this pattern, export one now:
> ```typescript
> // In AliasDetector.ts — export for reuse
> export const ALIAS_NAME_RE = /^[\w-]+$/;
> ```
> Then import it in `AliasesManager.tsx`:
> ```typescript
> import { ALIAS_NAME_RE } from "@/lib/omnibar/detectors/AliasDetector";
> ```
>
> **After exporting `ALIAS_NAME_RE` from `AliasDetector.ts`**, run the detector test suite immediately to confirm no existing tests break:
> ```bash
> cd web-app && npx jest --no-coverage --testPathPatterns="detector.test"
> ```
> This is a required safety step — `AliasDetector.ts` is a hot path in the omnibar and any export-level change could affect tree-shaking or module initialization order. The detector tests must stay green before proceeding to implement `handleSave`.

> **Proto construction note**: Before using `} as unknown as AliasProto`, check how `ProfilesManager.tsx` constructs proto objects — open the file and search for the pattern used there. Then:
> - If `ProfilesManager.tsx` uses `create(ProfileDefaultsProtoSchema, { ... })` from `@bufbuild/protobuf`: use `create(AliasProtoSchema, { name, description, path, ... })` here too. This is the type-safe approach and is preferred if it is already established.
> - If `ProfilesManager.tsx` uses `} as unknown as ProfileDefaultsProto`: use the same cast pattern for `AliasProto` to stay consistent. Do NOT mix patterns between components.
> - If the import `create` from `@bufbuild/protobuf` is not available in the web-app dependencies: use the cast and file a follow-up to adopt `create()` codebase-wide.
>
> In all cases, consistency with the existing `ProfilesManager.tsx` pattern takes precedence. Do not introduce a new pattern without updating all settings components at the same time.

```typescript
const handleSave = async () => {
  if (!clientRef.current) return;
  const trimmedName = form.name.trim();

  // Inline validation
  if (!trimmedName) {
    setNameError("Name is required.");
    return;
  }
  if (!ALIAS_NAME_RE.test(trimmedName)) {
    setNameError("Name may only contain letters, digits, hyphens, and underscores.");
    return;
  }
  // Client-side uniqueness check (create mode only) — case-insensitive, matching server behavior
  if (!editingName) {
    const collision = aliases.find(a => a.name.toLowerCase() === trimmedName.toLowerCase());
    if (collision) {
      setNameError(`An alias named "@${collision.name}" already exists. Edit it instead.`);
      return;
    }
  }
  setNameError(null);

  // Build envVars map, skipping blank keys
  const envVarsMap: Record<string, string> = {};
  for (const { key, value } of form.envVars) {
    if (key.trim()) envVarsMap[key.trim()] = value;
  }

  try {
    setSaving(true);
    setError(null);
    setSuccess(null);
    await clientRef.current.upsertAlias({
      alias: {
        name: trimmedName,
        description: form.description,
        group: form.group,
        path: form.path,
        profile: form.profile,
        program: form.program,
        autoYes: form.autoYes,
        tags: form.tags,
        envVars: envVarsMap,
        cliFlags: form.cliFlags,
      } as unknown as AliasProto,
    });
    setSuccess(`Alias "@${trimmedName}" saved.`);
    setTimeout(() => setSuccess(null), 3000);
    setShowForm(false);
    setEditingName(null);
    setForm({ ...emptyForm });
    await loadAliases();
  } catch (err) {
    setError(`Failed to save alias: ${err}`);
  } finally {
    setSaving(false);
  }
};
```

**Acceptance criteria**:

- Given `handleSave` is called with `form.name = ""`, then `nameError` is set to `"Name is required."` and `upsertAlias` is NOT called.
- Given `handleSave` is called with `form.name = "my project"` (space), then `nameError` is set to the regex constraint message and `upsertAlias` is NOT called.
- Given `handleSave` is called with `form.name = "myproj"` (create mode) and `aliases` already contains `{ name: "myproj" }`, then `nameError` is set to the collision message and `upsertAlias` is NOT called.
- Given `handleSave` is called with `form.name = "myproj"` (edit mode, `editingName = "myproj"`) and `aliases` already contains `{ name: "myproj" }`, then the uniqueness check is skipped and `upsertAlias` is called.
- Given `handleSave` succeeds, when the RPC returns, then `success` is set to `"Alias \"@myproj\" saved."` and `loadAliases` is called to refresh the list.

---

#### Story 2.1.4: Implement handleDelete, tag handlers, and EnvVarsEditor handlers

**File**: `web-app/src/components/settings/AliasesManager.tsx`

**Task 2.1.4a — Add inline delete confirmation state and handleDelete (4 min)**

> **Do NOT use `window.confirm`**. `window.confirm` is untestable in jsdom (used by Jest/RTL), inaccessible to screen readers, and blocked by some browser policies. Use inline row-level confirmation state instead: when the user first clicks "Delete", replace that button with a "Confirm delete?" button for 3 seconds, then revert to the original "Delete" button. This pattern is testable via RTL and a11y-compliant.

Add state for tracking which alias is pending deletion:
```typescript
const [pendingDeleteName, setPendingDeleteName] = useState<string | null>(null);
const pendingDeleteTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
const successBannerTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
```

> **Timer cleanup on unmount (REQUIRED):** Both the `pendingDeleteName` auto-revert timeout and the success banner auto-dismiss timeout must be cleared when the component unmounts to prevent `setState` calls on an unmounted component (which produces React warnings and can cause subtle bugs if the component remounts). Store each `setTimeout` return value in a `useRef` and clear both refs in a single `useEffect` cleanup:
> ```typescript
> useEffect(() => {
>   return () => {
>     if (pendingDeleteTimerRef.current) clearTimeout(pendingDeleteTimerRef.current);
>     if (successBannerTimerRef.current) clearTimeout(successBannerTimerRef.current);
>   };
> }, []);
> ```
> Add this cleanup effect alongside the other `useEffect` hooks in the component. Replace all bare `setTimeout(() => setSuccess(null), 3000)` calls with `successBannerTimerRef.current = setTimeout(() => setSuccess(null), 3000)` so the ref is always populated.

Add handlers:
```typescript
const handleDeleteClick = (name: string) => {
  // Clear any prior pending confirmation
  if (pendingDeleteTimerRef.current) clearTimeout(pendingDeleteTimerRef.current);
  setPendingDeleteName(name);
  // Auto-revert after 3 seconds if no confirmation
  pendingDeleteTimerRef.current = setTimeout(() => setPendingDeleteName(null), 3000);
};

const handleDeleteConfirm = async (name: string) => {
  if (pendingDeleteTimerRef.current) clearTimeout(pendingDeleteTimerRef.current);
  setPendingDeleteName(null);
  if (!clientRef.current) return;
  try {
    setError(null);
    setSuccess(null);
    await clientRef.current.deleteAlias({ name });
    setSuccess(`Alias "@${name}" deleted.`);
    setTimeout(() => setSuccess(null), 3000);
    await loadAliases();
  } catch (err) {
    setError(`Failed to delete alias: ${err}`);
  }
};
```

In the alias row `aliasActions`, render conditionally:
```tsx
{pendingDeleteName === alias.name ? (
  <button
    type="button"
    className={confirmDeleteBtn}
    onClick={() => handleDeleteConfirm(alias.name)}
    aria-label={`Confirm delete alias ${alias.name}`}
  >
    Confirm delete?
  </button>
) : (
  <button
    type="button"
    className={deleteAliasBtn}
    onClick={() => handleDeleteClick(alias.name)}
    aria-label={`Delete alias ${alias.name}`}
  >
    Delete
  </button>
)}
```

Add `confirmDeleteBtn` to `AliasesManager.css.ts` — same visual treatment as `deleteBtn` but with a bold border to indicate urgency:
```typescript
export const confirmDeleteBtn = style({
  backgroundColor: vars.color.error,
  border: `2px solid ${vars.color.error}`,
  borderRadius: vars.radii.sm,
  color: vars.color.textInverse,
  cursor: "pointer",
  fontWeight: vars.fontWeight.bold,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
});
```

const handleAddTag = () => {
  const trimmed = form.tagInput.trim();
  if (trimmed && !form.tags.includes(trimmed)) {
    setForm({ ...form, tags: [...form.tags, trimmed], tagInput: "" });
  } else {
    setForm({ ...form, tagInput: "" });
  }
};

const handleRemoveTag = (tag: string) => setForm({ ...form, tags: form.tags.filter(t => t !== tag) });
const handleTagKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
  if (e.key === "Enter") { e.preventDefault(); handleAddTag(); }
};
```

**Task 2.1.4b — Add EnvVars handlers (2 min)**

```typescript
const handleAddEnvVar = () => setForm({ ...form, envVars: [...form.envVars, { key: "", value: "" }] });

const handleEnvVarChange = (index: number, field: "key" | "value", val: string) => {
  const updated = [...form.envVars];
  updated[index] = { ...updated[index], [field]: val };
  setForm({ ...form, envVars: updated });
};

const handleRemoveEnvVar = (index: number) =>
  setForm({ ...form, envVars: form.envVars.filter((_, i) => i !== index) });
```

**Acceptance criteria**:

- Given the alias row for `"myproj"` is rendered, when the "Delete" button is clicked, then `pendingDeleteName` is set to `"myproj"` and the row renders a "Confirm delete?" button in place of the "Delete" button.
- Given the "Confirm delete?" button is visible for `"myproj"`, when 3 seconds elapse without clicking, then `pendingDeleteName` is reset to `null` and the "Delete" button reappears.
- Given the "Confirm delete?" button is visible for `"myproj"`, when it is clicked and the RPC succeeds, then `success` is set to `"Alias \"@myproj\" deleted."` and `loadAliases` is called.
- Given `handleAddTag` is called with `form.tagInput = "backend"` and `form.tags = ["frontend"]`, then `form.tags` becomes `["frontend", "backend"]` and `tagInput` is reset to `""`.
- Given `handleAddTag` is called with `form.tagInput = "frontend"` (already exists in `form.tags`), then `form.tags` is unchanged and `tagInput` is reset to `""`.
- Given `handleEnvVarChange(0, "key", "FOO")` is called with `form.envVars = [{ key: "", value: "" }]`, then `form.envVars[0].key` becomes `"FOO"`.
- Given `handleRemoveEnvVar(1)` is called with `form.envVars = [{key:"A",value:"1"},{key:"B",value:"2"}]`, then `form.envVars` becomes `[{key:"A",value:"1"}]`.

---

#### Story 2.1.5: Implement the render output (JSX)

**File**: `web-app/src/components/settings/AliasesManager.tsx`

**Task 2.1.5a — Implement the list view JSX (4 min)**

The list renders:
- A `headerRow` div with `<h2>Aliases</h2>` and a "New Alias" `<button>`. **The "New Alias" button must be `disabled` or hidden (`display: none`) while `loading === true`** to prevent alias creation before the current alias list is known. Once `loading` becomes `false` the button is enabled regardless of whether the list is empty or populated.
- Error and success alert `<div>`s (same class names as ProfilesManager: `"alert alert-error"`, `"alert alert-success"`).
- Empty state div when `aliases.length === 0 && !showForm`.
- One `aliasRow` div per alias, with `aliasInfo` (name, description, group, path, program chips) and `aliasActions` (Edit, Delete buttons).

Each alias row `aliasInfo` section shows:
```tsx
<span className={aliasName}>{alias.name}</span>
{alias.description && <span className={aliasDesc}>{alias.description}</span>}
{alias.group && <span className={aliasMeta}>Group: {alias.group}</span>}
{alias.path && <span className={aliasMeta}>{alias.path}</span>}
{alias.program && <span className={aliasMeta}>[{alias.program}]</span>}
```

**Task 2.1.5b — Implement the form card JSX (5 min)**

When `showForm`, render the `formCard` containing:

1. Form title (`formTitle`): `"New Alias"` or `"Edit Alias: myproj"` (using `editingName`).
2. `<section aria-labelledby="alias-form-title">` wrapping the form card.
3. Primary fields (always visible):
   - **Name** (`*`, `aria-required="true"`, disabled when `editingName` is set, `ref={nameInputRef}`). Below it: `previewHint` showing `@{form.name || "name"}` (live). Below that: inline `fieldError` if `nameError` is set.

   **Omnibar preview block** (below the Tags field, above "Advanced options"): renders `@{name || "name"}`, then path (if non-empty), then `[{program}]` badge (if non-empty). Empty fields are omitted — no blank chips or placeholder text for path/program. The preview updates live as the user types in Name, Path, or Program fields.
   - **Description** (plain text input)
   - **Group** with `groupHint`: "Groups aliases in the @ palette. Leave blank for ungrouped."
   - **Path** (plain text input, hint: "Supports ~ expansion. Optional.")
   - **Profile** (select, populated from loaded `profiles` from a separate `getSessionDefaults` call OR from a hardcoded empty option + whatever the user types — for simplicity, use a plain text `<input>` for `profile` since it is a reference and profile management is separate)
   - **Program** (select from `PROGRAMS` constant, same as ProfilesManager)
   - **Auto-yes** (checkbox in `checkboxLabel`)
   - **Tags** (chip input row, same pattern as ProfilesManager)

4. Advanced section (collapsed by default):
   ```tsx
   <div className={field}>
     <label className={advancedToggle}>
       <input type="checkbox" checked={form.showAdvanced}
         onChange={e => setForm({...form, showAdvanced: e.target.checked})} />
       Advanced options
     </label>
   </div>
   {form.showAdvanced && (
     <>
       {/* EnvVarsEditor */}
       <div className={field}>
         <label className={labelClass}>Environment Variables</label>
         <p className={groupHint}>Use ${"{VAR}"} to reference shell variables (expanded at session start).</p>
         <div className={envVarTable}>
           {form.envVars.map((ev, i) => (
             <div key={i} className={envVarRow}>
               <input className={envVarInput} placeholder="KEY" value={ev.key}
                 onChange={e => handleEnvVarChange(i, "key", e.target.value)}
                 aria-label={`Environment variable ${i + 1} key`} />
               <input className={envVarInput} placeholder="value" value={ev.value}
                 onChange={e => handleEnvVarChange(i, "value", e.target.value)}
                 aria-label={`Environment variable ${i + 1} value`} />
               <button type="button" className={deleteBtn}
                 onClick={() => handleRemoveEnvVar(i)}
                 aria-label={`Remove environment variable ${ev.key || i + 1}`}>
                 Remove
               </button>
             </div>
           ))}
         </div>
         <button type="button" className="btn btn-secondary" onClick={handleAddEnvVar}>
           Add Variable
         </button>
       </div>
       {/* CLI Flags */}
       <div className={field}>
         <label className={labelClass} htmlFor="alias-cliFlags">CLI Flags</label>
         <input id="alias-cliFlags" type="text" className={input}
           placeholder="e.g. --no-ansi" value={form.cliFlags}
           onChange={e => setForm({...form, cliFlags: e.target.value})} />
       </div>
     </>
   )}
   ```

5. `formActions`: Save (disabled while saving, shows "Saving…") + Cancel.

**Acceptance criteria**:

- Given `aliases` is `[]` and `showForm` is `false`, when the component renders, then an empty-state message is visible.
- Given `aliases = [{ name: "myproj", description: "My project", group: "work", path: "~/code/myproject", program: "claude" }]`, when the list renders, then a row with text `"myproj"`, `"My project"`, `"Group: work"`, `"~/code/myproject"`, `"[claude]"` is visible.
- Given `showForm = true` and `form.name = "myproj"`, when the form renders, then the `previewHint` element contains `"@myproj"`.
- Given `form.showAdvanced = false`, when the form renders, then the `envVarTable` and CLI Flags input are NOT in the DOM.
- Given the "Advanced options" checkbox is checked, when `form.showAdvanced` becomes `true`, then the env var editor and CLI flags input appear.
- Given `nameError = "Name is required."`, when the form renders, then the `fieldError` span is visible below the name input.
- Given `showForm = true` and `editingName = "myproj"`, when the form renders, then the name input has `disabled={true}`.
- Given `form.envVars = [{ key: "FOO", value: "bar" }]` and `form.showAdvanced = true`, when the form renders, then one env var row is visible with `KEY = "FOO"` and `value = "bar"`.

---

---

#### Story 2.1.6: Jest/RTL unit tests for AliasesManager

**File**: `web-app/src/components/settings/AliasesManager.test.tsx` (new file)

**Goal**: Provide fast, jsdom-based coverage for every user-facing behavior that does not require a real server. All RPC calls are mocked via `jest.mock("@connectrpc/connect")`.

**Task 2.1.6a — Scaffold test file and mock setup (3 min)**

```typescript
// AliasesManager.test.tsx
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AliasesManager } from "./AliasesManager";
import { createClient } from "@connectrpc/connect";

jest.mock("@connectrpc/connect");
jest.mock("@connectrpc/connect-web");
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost" }));

const mockListAliases = jest.fn();
const mockUpsertAlias = jest.fn();
const mockDeleteAlias = jest.fn();

(createClient as jest.Mock).mockReturnValue({
  listAliases: mockListAliases,
  upsertAlias: mockUpsertAlias,
  deleteAlias: mockDeleteAlias,
});

const sampleAlias = {
  name: "myproj",
  description: "My project",
  group: "work",
  path: "~/code/myproject",
  program: "claude",
  autoYes: false,
  tags: ["backend"],
  envVars: { FOO: "bar" },
  cliFlags: "",
};

beforeEach(() => {
  jest.clearAllMocks();
  mockListAliases.mockResolvedValue({ aliases: [sampleAlias] });
  mockUpsertAlias.mockResolvedValue({ alias: sampleAlias });
  mockDeleteAlias.mockResolvedValue({});
});
```

**Task 2.1.6b — Write test cases (8 min)**

Cover the following scenarios, each in its own `it()` block:

1. **Render with aliases** — after mount, the alias row for `"myproj"` is visible with description, group chip, path, and program chip.
2. **Empty state** — when `listAliases` resolves with `{ aliases: [] }`, the empty-state message is shown and no alias rows are present.
3. **Add flow** — click "New Alias" → form appears; fill name `"newproj"` and description → click Save → `upsertAlias` called with correct payload → list refreshed.
4. **Edit flow** — click "Edit" on `"myproj"` → form pre-populates with existing values → change description → click Save → `upsertAlias` called with updated description; name field is disabled.
5. **Name validation — empty** — open new form, click Save without entering a name → inline error `"Name is required."` appears and `upsertAlias` is NOT called.
6. **Name validation — format** — enter `"my project"` (space) → inline error cites the regex constraint and `upsertAlias` is NOT called.
7. **Name validation — conflict** — enter `"myproj"` when alias list already contains `"myproj"` → inline error `"already exists"` and `upsertAlias` is NOT called.
8. **Name validation — case-insensitive conflict** — enter `"MYPROJ"` when list contains `"myproj"` → inline error (case-insensitive match) and `upsertAlias` is NOT called.
9. **Delete flow — inline confirmation** — click "Delete" on `"myproj"` → button changes to "Confirm delete?" → click confirm → `deleteAlias` called with `{ name: "myproj" }` → list refreshed.
10. **Delete flow — auto-cancel** — click "Delete" → wait 3 seconds (use `jest.useFakeTimers`) → "Delete" button reappears without calling `deleteAlias`.
11. **Env-var editor add/remove** — open form, click "Advanced options" checkbox → "Add Variable" button appears → click it → one env var row appears → fill key `"FOO"` → click "Remove" → row disappears.
12. **Advanced section expand/collapse** — open new form → `envVarTable` is NOT in the DOM → check "Advanced options" → `envVarTable` appears → uncheck → `envVarTable` disappears.
13. **Live @name preview** — open new form, type `"proj-x"` in Name field → preview hint shows `"@proj-x"`.

**Acceptance criteria**:

- Given `cd web-app && npx jest --no-coverage --testPathPatterns="AliasesManager.test"` is run, then all 13 test cases pass.
- Given the test file is committed, when `make ci` runs, then no regressions are introduced in the existing Jest suite.
- Given test cases 7 and 8 both pass, then the case-insensitive uniqueness check is confirmed working at the component level (matching the server-side behavior enforced in Story 1.2.4c).

---

### Epic 2.2: Settings page integration

**Goal**: Insert `<AliasesManager />` into the General tab, run validation, update the registry.

---

#### Story 2.2.1: Add AliasesManager to settings page

**File**: `web-app/src/app/settings/page.tsx`

**Task 2.2.1a — Add import and section (2 min)**

Add import:
```typescript
import { AliasesManager } from "@/components/settings/AliasesManager";
```

Add section after the `DirectoryRulesManager` section (after line 59), before the Help section:
```tsx
<section className={styles.section}>
  <AliasesManager />
</section>
```

**Acceptance criteria**:

- Given `web-app/src/app/settings/page.tsx` is saved, when `make build` runs, then it exits 0.
- Given the settings page renders in a browser, when the General tab is active, then the "Aliases" heading is visible below the "Directory Rules" section and above the "Help" section.

---

#### Story 2.2.2: Build validation

**Task 2.2.2a — Run make quick-check (3 min)**

```bash
make quick-check
```

This runs build + test + lint as a single gate. Fix any issues before proceeding.

**Acceptance criteria**:

- Given all Phase 1 and Phase 2 code is written, when `make quick-check` is run, then it exits 0.
- Given `cd web-app && npx jest --no-coverage` is run, then no existing tests are broken.

---

### Epic 2.3: Feature registry update

**Goal**: Register the two new backend RPCs and the new frontend component in `docs/registry/features/`.

---

#### Story 2.3.1: Create backend alias registry entries

**Files**: 
- `docs/registry/features/backend/alias/upsert.json` (new)
- `docs/registry/features/backend/alias/delete.json` (new)
- `docs/registry/features/backend/alias/list.json` (new — `ListAliases` already exists but has no registry entry)

**Task 2.3.1a — Create registry files (2 min)**

`upsert.json`:
```json
{
  "id": "alias:upsert",
  "type": "backend",
  "service": "SessionService",
  "method": "UpsertAlias",
  "protoFile": "proto/session/v1/session.proto",
  "markerFound": false,
  "tested": true,
  "testIds": [
    "TestUpsertAlias_EmptyName",
    "TestUpsertAlias_InvalidName",
    "TestUpsertAlias_CreatesAlias",
    "TestUpsertAlias_UpdatesExistingAlias"
  ],
  "lastModified": "2026-06-21T00:00:00Z"
}
```

`delete.json`:
```json
{
  "id": "alias:delete",
  "type": "backend",
  "service": "SessionService",
  "method": "DeleteAlias",
  "protoFile": "proto/session/v1/session.proto",
  "markerFound": false,
  "tested": true,
  "testIds": [
    "TestDeleteAlias_NotFound",
    "TestDeleteAlias_DeletesAlias"
  ],
  "lastModified": "2026-06-21T00:00:00Z"
}
```

`list.json`:
```json
{
  "id": "alias:list",
  "type": "backend",
  "service": "SessionService",
  "method": "ListAliases",
  "protoFile": "proto/session/v1/session.proto",
  "markerFound": false,
  "tested": false,
  "testIds": [],
  "lastModified": "2026-06-21T00:00:00Z"
}
```

> **Registry coverage note for `alias:list`**: `list.json` is created with `"tested": false` because no automated test covers `ListAliases` at the time Phase 1 is complete. Once the Playwright e2e spec in Phase 3 passes (Story 3.2.1), update `list.json` to `"tested": true` and add the e2e test ID (e.g. `"alias-settings > create alias via UI"`) to `testIds`. This ensures the `alias:list` entry does NOT contribute to the coverage gap count in `docs/registry/features/coverage-gaps.json` after Phase 3 ships.

**Acceptance criteria**:

- Given `docs/registry/features/backend/alias/` exists with 3 files, when `make registry-generate` is run, then it exits 0 without overwriting the `testIds` entries already present.
- Given the e2e spec in Story 3.2.1 is committed, when `list.json` is updated to `tested: true`, then `make registry-diff` shows no net increase in the coverage gap count.

---

#### Story 2.3.2: Create frontend alias manager registry entry and update existing alias entry

**Files**:
- `docs/registry/features/alias-manager.json` (new)
- `docs/registry/features/alias.json` (update existing)

**Task 2.3.2a — Create alias-manager.json (2 min)**

```json
{
  "id": "alias:manage",
  "type": "frontend",
  "description": "Settings UI for creating, editing, and deleting alias session presets",
  "component": "AliasesManager",
  "filePath": "web-app/src/components/settings/AliasesManager.tsx",
  "tested": false,
  "testIds": []
}
```

**Task 2.3.2b — Update alias.json (1 min)**

Update `lastModified` to `"2026-06-21T00:00:00Z"` to reflect the related feature context update.

**Task 2.3.2c — Run registry generate (2 min)**

```bash
make registry-generate
```

Commit all changed registry files together.

**Acceptance criteria**:

- Given `docs/registry/features/alias-manager.json` is created, when `make registry-generate` runs, then it exits 0.
- Given `make registry-diff` is run, then the diff shows the new entries and no unexpected deletions.

---

## Phase 3: Verification

### Epic 3.1: Playwright e2e spec

---

#### Story 3.1.1: Create alias-settings.spec.ts

**File**: `tests/e2e/alias-settings.spec.ts` (new file)

**Goal**: Automated Playwright coverage for the alias CRUD UI and `@name` omnibar detection, run against `http://localhost:8544` (the test server port).

**Task 3.1.1a — Create spec file (8 min)**

```typescript
// @feature session:create, alias:settings
/**
 * E2E tests for the Alias Settings Manager UI and @alias omnibar detection.
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 */

import { test, expect } from "@playwright/test";
import { SettingsPage } from "./pages/SettingsPage"; // reuse or create helper

const BASE_URL = process.env.BASE_URL ?? "http://localhost:8544";

test.describe("alias-settings", () => {
  let settings: SettingsPage;

  test.beforeEach(async ({ page }) => {
    settings = new SettingsPage(page);
    await settings.goto();
    await settings.selectTab("General");
  });

  test("create alias via UI and verify @name detection in omnibar", async ({ page }) => {
    // Create alias
    await settings.clickNewAlias();
    await settings.fillAliasName("e2e-test");
    await settings.fillAliasDescription("E2E test alias");
    await settings.saveAlias();
    await expect(page.getByTestId("alias-row-e2e-test")).toBeVisible();

    // Navigate to the main page and open the omnibar by clicking its trigger button
    // (Do NOT use Meta+k / Cmd+K — keyboard shortcuts are unreliable in CI on Linux)
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded", timeout: 10000 });
    await page.getByText("New Session").click();
    await page.getByRole("combobox", { name: /omnibar/i }).fill("@e2e-test ");
    // The alias detection badge or suggestion should appear
    await expect(page.getByTestId("omnibar-alias-detection")).toBeVisible();
  });

  test("edit alias", async ({ page }) => {
    // Precondition: alias "e2e-test" already exists (from prior test or setup)
    await settings.clickEditAlias("e2e-test");
    await settings.fillAliasDescription("Updated by e2e");
    await settings.saveAlias();
    await expect(page.getByTestId("alias-row-e2e-test")).toContainText("Updated by e2e");
  });

  test("delete alias via inline confirmation", async ({ page }) => {
    await page.getByTestId("alias-delete-e2e-test").click();
    // First click shows confirmation button, not deletes immediately
    await expect(page.getByTestId("alias-confirm-delete-e2e-test")).toBeVisible();
    await page.getByTestId("alias-confirm-delete-e2e-test").click();
    await expect(page.getByTestId("alias-row-e2e-test")).not.toBeVisible();
  });
});
```

> **`data-testid` attributes required**: The above spec depends on `data-testid` attributes that must be added to `AliasesManager.tsx` alias rows and buttons:
> - `data-testid={`alias-row-${alias.name}`}` on each alias row div
> - `data-testid={`alias-delete-${alias.name}`}` on the "Delete" button
> - `data-testid={`alias-confirm-delete-${alias.name}`}` on the "Confirm delete?" button
> - `data-testid="omnibar-alias-detection"` on the alias detection badge in `Omnibar.tsx` (check if this already exists; if not, add it)
>
> Add these `data-testid` attributes as part of Story 2.1.5 before implementing this spec.

> **Test isolation**: Each `test` block must not rely on state left by a prior test. Either use `test.beforeEach` to create the alias from scratch (via `settings.clickNewAlias()` + fill + save), or configure a test-specific `STAPLER_SQUAD_INSTANCE` and seed the config. The preferred approach is `beforeEach` create + `afterEach` delete via the UI, matching the existing `session-lifecycle.spec.ts` pattern.

**Task 3.1.1b — Add SettingsPage helper methods (3 min)**

If `tests/e2e/pages/SettingsPage.ts` does not exist, create it. Otherwise add:
```typescript
async clickNewAlias() {
  await this.page.getByRole("button", { name: "New Alias" }).click();
}
async fillAliasName(name: string) {
  await this.page.getByLabel("Name").fill(name);
}
async fillAliasDescription(description: string) {
  await this.page.getByLabel("Description").fill(description);
}
async saveAlias() {
  await this.page.getByRole("button", { name: "Save" }).click();
  // Wait for success banner
  await expect(this.page.getByText(/saved/i)).toBeVisible();
}
async clickEditAlias(name: string) {
  await this.page.getByTestId(`alias-row-${name}`).getByRole("button", { name: "Edit" }).click();
}
```

**Task 3.1.1c — Update registry after e2e passes (1 min)**

Once the spec passes in CI, update `docs/registry/features/backend/alias/list.json`:
- Set `"tested": true`
- Add `"alias-settings > create alias via UI and verify @name detection in omnibar"` to `testIds`

Also update `docs/registry/features/alias-manager.json`:
- Set `"tested": true`
- Add the three test IDs to `testIds`

**Acceptance criteria**:

- Given the test server is running (`STAPLER_SQUAD_INSTANCE=e2e-local`), when `cd tests/e2e && npx playwright test alias-settings.spec.ts` is run, then all 3 tests pass.
- Given the spec file starts with `// @feature session:create, alias:settings`, when CI runs the e2e spec conventions check, then it exits 0 (no missing `@feature` annotation).
- Given the "create alias" test passes, then omnibar `@e2e-test ` (with trailing space) triggers `InputType.Alias` detection — confirmed by the `omnibar-alias-detection` element being visible.
- Given the "delete alias" test passes, then the "Confirm delete?" button is required before deletion — confirming the inline confirmation pattern works end-to-end.

---

### Epic 3.2: Manual validation

---

#### Story 3.2.1: Manual smoke test

**Task 3.1.1a — Smoke test the full CRUD flow (5 min)**

```bash
make install-service
```

Open `http://localhost:8543/settings?tab=general` in a browser.

Steps:
1. Scroll to the "Aliases" section — verify it appears below "Directory Rules".
2. Click "New Alias".
3. Enter name `smoke-test`, description `Smoke test alias`, group `testing`, path `~/code`, program `claude`. Click Save.
4. Verify the success banner appears and the alias row `smoke-test` is visible in the list.
5. Click "Edit" on `smoke-test`. Verify the form pre-populates with all fields. Change description to `Updated description`. Click Save.
6. Verify the row shows the updated description.
7. Click "Delete" on `smoke-test`. The button changes to "Confirm delete?". Click it. Verify the row disappears and the list is empty again.
8. Open `~/.stapler-squad/config.json`. Verify the `aliases` array no longer contains `smoke-test`.

**Acceptance criteria**:

- Given a user completes step 3, when they save, then `config.json` `aliases` array contains `{ "name": "smoke-test", "description": "Smoke test alias", "group": "testing", "path": "~/code", "program": "claude" }`.
- Given a user completes step 5 (edit), when they save, then `config.json` `aliases[0].description` equals `"Updated description"`.
- Given a user completes step 7 (delete), when confirmed, then `config.json` `aliases` array is empty.
- Given the user opens the omnibar and types `@smoke`, when the alias exists, then the alias appears in the `AliasPalette`. (Confirms live-reload with no restart needed.)

---

#### Story 3.2.2: Run full CI gate

**Task 3.1.2a — Run make ci (3 min)**

```bash
make ci
```

**Acceptance criteria**:

- Given `make ci` is run on the feature branch, then it exits 0.
- Given `make ci` completes, then the coverage gap count in `docs/registry/features/` has not grown (net zero or negative new untested entries).

---

## Summary

| Phase | Epic | Stories | Tasks | Estimated Time |
|-------|------|---------|-------|----------------|
| 1 — Backend | 1.1 Proto | 2 | 3 | ~7 min |
| 1 — Backend | 1.2 Go handlers | 4 | 7 | ~22 min |
| 2 — Frontend | 2.1 Component | 6 | 12 | ~38 min |
| 2 — Frontend | 2.2 Integration | 2 | 2 | ~5 min |
| 2 — Frontend | 2.3 Registry | 2 | 4 | ~7 min |
| 3 — Verification | 3.1 Playwright e2e | 1 | 3 | ~12 min |
| 3 — Verification | 3.2 Manual + CI | 2 | 2 | ~8 min |
| **Total** | | **19** | **33** | **~99 min** |

**Files touched**:

| File | Operation |
|------|-----------|
| `proto/session/v1/session.proto` | Edit — add 2 RPCs + 4 messages |
| `gen/proto/go/session/v1/session_pb.go` | Auto-generated by `make generate-proto` |
| `web-app/src/gen/session/v1/session_pb.ts` | Auto-generated by `make generate-proto` |
| `server/services/defaults_service.go` | Edit — add `aliasNameRE`, `UpsertAlias`, `DeleteAlias` (case-insensitive upsert) |
| `server/services/session_service.go` | Edit — add 2 thin wrappers |
| `server/services/defaults_service_test.go` | Edit/Create — add 7+ test functions (incl. case-insensitive test) |
| `web-app/src/lib/omnibar/detectors/AliasDetector.ts` | Edit — export `ALIAS_NAME_RE` constant |
| `web-app/src/components/settings/AliasesManager.tsx` | Create — inline delete confirmation, imports `ALIAS_NAME_RE` |
| `web-app/src/components/settings/AliasesManager.css.ts` | Create — incl. `confirmDeleteBtn` token |
| `web-app/src/components/settings/AliasesManager.test.tsx` | Create — 13 Jest/RTL test cases |
| `web-app/src/app/settings/page.tsx` | Edit — add import + section |
| `tests/e2e/alias-settings.spec.ts` | Create — 3 Playwright e2e tests |
| `tests/e2e/pages/SettingsPage.ts` | Edit/Create — alias CRUD helper methods |
| `docs/registry/features/backend/alias/upsert.json` | Create |
| `docs/registry/features/backend/alias/delete.json` | Create |
| `docs/registry/features/backend/alias/list.json` | Create — updated to `tested: true` after e2e ships |
| `docs/registry/features/alias-manager.json` | Create — updated to `tested: true` after e2e ships |
| `docs/registry/features/alias.json` | Edit — update lastModified |
