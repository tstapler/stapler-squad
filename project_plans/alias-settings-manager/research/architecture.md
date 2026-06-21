# Architecture Research: Alias Settings Manager

## Research Questions & Findings

---

### 1. How does the Go config layer load and persist `config.json`?

**Loading**: `config.LoadConfig()` reads `config.json` from the path returned by `GetConfigDir()` (workspace-isolated by default). It calls `LoadConfigFromPath()` which does a plain `json.Unmarshal` into a `Config` struct. There is **no hot-reload** — every RPC handler calls `config.LoadConfig()` fresh on each request, re-reading from disk each time.

**Persisting**: `config.SaveConfig(cfg)` (exported wrapper for `saveConfig()`) writes via an atomic temp-file rename: it serializes to JSON, writes to `<configPath>.tmp`, then `os.Rename` to the final path. This prevents partial writes.

**Mutex**: There is **no mutex** in the config package. The existing pattern in `defaults_service.go` is load-modify-save without any locking: all four existing CRUD methods (`UpsertProfile`, `DeleteProfile`, `UpsertDirectoryRule`, `DeleteDirectoryRule`) follow the same lock-free pattern. Concurrent writes are safe at the filesystem level due to the atomic rename, but there is a read-modify-write race if two requests arrive simultaneously. This is an accepted existing limitation — alias CRUD should follow the same pattern as the existing handlers rather than introducing a new locking scheme.

---

### 2. Proto message structure for `UpsertAliasRequest` / `UpsertAliasResponse` / `DeleteAliasRequest` / `DeleteAliasResponse`

The `AliasProto` message already exists at `proto/session/v1/session.proto` lines 1773–1785:

```protobuf
message AliasProto {
  string name = 1;
  string group = 2;
  string path = 3;
  string description = 4;
  string profile = 5;
  string program = 6;
  bool auto_yes = 7;
  repeated string tags = 8;
  map<string, string> env_vars = 9;
  string cli_flags = 10;
}
```

The new messages should follow the exact same pattern as `UpsertDirectoryRule` / `DeleteDirectoryRule` (lines 1755–1771):

```protobuf
// UpsertAlias

message UpsertAliasRequest {
  AliasProto alias = 1;
}

message UpsertAliasResponse {
  AliasProto alias = 1;
}

// DeleteAlias

message DeleteAliasRequest {
  string name = 1;
}

message DeleteAliasResponse {}
```

**Placement**: These messages should be appended immediately after `ListAliasesResponse` (after line 1791). `AliasProto` and `ListAliasesRequest`/`ListAliasesResponse` already exist; `UpsertAlias` and `DeleteAlias` messages are the only additions needed.

The `rpc` declarations go into the `SessionService` service definition (alongside `rpc ListAliases` at line 382), following the same comment+rpc pattern as `UpsertDirectoryRule` / `DeleteDirectoryRule`:

```protobuf
  // UpsertAlias creates or updates a named alias preset (matched by name).
  rpc UpsertAlias(UpsertAliasRequest) returns (UpsertAliasResponse) {}

  // DeleteAlias removes an alias preset by name.
  rpc DeleteAlias(DeleteAliasRequest) returns (DeleteAliasResponse) {}
```

---

### 3. What field numbers are available in `session.proto` for new messages?

The proto file ends at line 2469. The last message is `DeleteWorkflowFailedSessionsResponse`. **No top-level field numbers are exhausted** — these are all independent top-level `message` blocks, each with their own field number namespace (field 1, 2, etc. scoped to that message). Adding new top-level messages requires no field-number coordination with existing ones.

The `AliasProto` message already uses fields 1–10. The new `UpsertAliasRequest`, `UpsertAliasResponse`, `DeleteAliasRequest`, `DeleteAliasResponse` each use only field 1 (the `alias` or `name` field), which is well clear of any conflict.

---

### 4. How does `UpdateClaudeConfig` save config? Can the same mechanism be reused?

`UpdateClaudeConfig` (defined at `proto/session/v1/session.proto` line 67) is a **raw file editor** that writes arbitrary text/JSON content to Claude configuration files (e.g. `CLAUDE.md`, `settings.json`). It is unrelated to the application's `config.json` and operates on a different set of files entirely.

**Conclusion**: Alias CRUD must go through typed RPCs, not `UpdateClaudeConfig`. The existing `DefaultsService` in `server/services/defaults_service.go` is the correct home. Add `UpsertAlias` and `DeleteAlias` methods there, exactly parallel to `UpsertDirectoryRule` and `DeleteDirectoryRule`. The session_service.go thin wrappers (lines 3022–3034) should be mirrored for aliases.

---

### 5. Is this simple CRUD? (Skip EventStorming table?)

**Yes — skip EventStorming.** This is straightforward CRUD over an in-memory slice in `config.SessionDefaults.Aliases []AliasConfig`. The alias list is already loaded/saved by the same `LoadConfig`/`SaveConfig` path as profiles and directory rules. The key difference from profiles is that aliases are stored as a `[]AliasConfig` slice (matched by `Name`), not a `map[string]ProfileDefaults` — the upsert logic should mirror `UpsertDirectoryRule` (scan slice, replace in-place or append), not `UpsertProfile` (map key assignment).

---

## Implementation Plan (ordered)

### Step 1: Proto messages + RPC declarations

**File**: `proto/session/v1/session.proto`

1. Add `rpc UpsertAlias` and `rpc DeleteAlias` to the `SessionService` service (after `rpc ListAliases`, around line 383).
2. Append `UpsertAliasRequest`, `UpsertAliasResponse`, `DeleteAliasRequest`, `DeleteAliasResponse` message definitions after `ListAliasesResponse` (after line 1791).
3. Run `make generate-proto` to regenerate Go and TypeScript bindings.

### Step 2: Go backend — `defaults_service.go`

**File**: `server/services/defaults_service.go`

Add two methods to `DefaultsService`:

```go
// UpsertAlias creates or updates a named alias preset (matched by name).
func (d *DefaultsService) UpsertAlias(
    ctx context.Context,
    req *connect.Request[sessionv1.UpsertAliasRequest],
) (*connect.Response[sessionv1.UpsertAliasResponse], error) {
    if req.Msg.Alias == nil {
        return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("alias is required"))
    }
    if req.Msg.Alias.Name == "" {
        return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("alias name is required"))
    }

    cfg := config.LoadConfig()

    alias := config.AliasConfig{
        Name:        req.Msg.Alias.Name,
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

    // Replace existing alias with same name or append (same pattern as DirectoryRule).
    found := false
    for i, a := range cfg.SessionDefaults.Aliases {
        if a.Name == alias.Name {
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

// DeleteAlias removes a named alias preset by name.
func (d *DefaultsService) DeleteAlias(
    ctx context.Context,
    req *connect.Request[sessionv1.DeleteAliasRequest],
) (*connect.Response[sessionv1.DeleteAliasResponse], error) {
    if req.Msg.Name == "" {
        return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("alias name is required"))
    }

    cfg := config.LoadConfig()

    aliases := cfg.SessionDefaults.Aliases
    newAliases := make([]config.AliasConfig, 0, len(aliases))
    deleted := false
    for _, a := range aliases {
        if a.Name == req.Msg.Name {
            deleted = true
            continue
        }
        newAliases = append(newAliases, a)
    }
    if !deleted {
        return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("alias %q not found", req.Msg.Name))
    }
    cfg.SessionDefaults.Aliases = newAliases

    if err := config.SaveConfig(cfg); err != nil {
        return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save config: %w", err))
    }

    log.Info("deleted alias", "name", req.Msg.Name)
    return connect.NewResponse(&sessionv1.DeleteAliasResponse{}), nil
}
```

Note: `aliasConfigToProto` already exists in this file (lines 224–237) — no new helper needed.

### Step 3: Thin wrappers in `session_service.go`

**File**: `server/services/session_service.go`

Add after the `ListAliases` wrapper (~line 3034):

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

### Step 4: `AliasesManager` React component

**Files to create**:
- `web-app/src/components/settings/AliasesManager.tsx`
- `web-app/src/components/settings/AliasesManager.css.ts`

**Pattern**: Mirror `ProfilesManager.tsx` and `DirectoryRulesManager.tsx`. Key differences from `ProfilesManager`:

- Data source: call `client.listAliases({})` directly (there is a `ListAliases` RPC); aliases come as a flat `AliasProto[]` array rather than `map<string, ProfileDefaultsProto>`.
- Save: call `client.upsertAlias({ alias: { name, group, path, ... } })`.
- Delete: call `client.deleteAlias({ name })`.
- Form fields: `name` (unique key — disabled on edit), `group`, `path`, `description`, `profile`, `program`, `autoYes`, `tags`.
- Validation: `name` required, must match `^[\w-]+$` (per `AliasConfig` doc comment in config.go line 441).

**File**: `web-app/src/app/settings/page.tsx`

Add `AliasesManager` as a new `<section>` in the General tab, after `DirectoryRulesManager` (line 59), following the same `<section className={styles.section}>` wrapper pattern.

---

## Key Findings Summary

- **No mutex needed**: the existing pattern is lock-free load-modify-save; alias CRUD follows the same convention. The atomic file rename in `saveConfig` is the only safety mechanism, which is already in place.
- **`AliasProto` already exists**: only 4 new messages (`UpsertAliasRequest/Response`, `DeleteAliasRequest/Response`) and 2 new `rpc` declarations need to be added to the proto file. `ListAliasesRequest`/`ListAliasesResponse`/`AliasProto` are already present.
- **Slice-based upsert, not map**: `Aliases []AliasConfig` uses name-matched slice replacement (same as `DirectoryRules`), not map keying (unlike `Profiles`). The `UpsertDirectoryRule` implementation is the correct reference, not `UpsertProfile`.
