# Stack Research: Session Defaults

## Config Package Overview

**Files:**
- `config/config.go` — main Config struct, load/save, path resolution
- `config/state.go` — UI/app state (separate from Config, different file)

**File path:** `~/.stapler-squad/<instance>/config.json`

**Load/save mechanism:**
- `config.LoadConfig()` reads JSON from disk; returns `DefaultConfig()` on any error (no migration needed — missing fields zero-value to Go defaults)
- `config.SaveConfig()` writes atomically: write `.tmp` → rename
- Serialization: `json.MarshalIndent` with 2-space indent
- State file uses file locking (`flock`) with read/write lock separation

**Config path resolution priority (highest → lowest):**
1. `STAPLER_SQUAD_TEST_DIR` env var override
2. `STAPLER_SQUAD_INSTANCE` explicit named instance
3. Preferred workspace (from file)
4. Test mode auto-detection (binary name contains `.test`)
5. Workspace-based isolation per git repo (default)
6. Global shared state (fallback)

---

## Existing Config Schema

```go
type Config struct {
    ListenAddress            string
    PasskeyRPID              string
    PasskeyEnabled           bool
    DefaultProgram           string            // "proxy-claude" by default
    AutoYes                  bool
    DaemonPollInterval       int
    BranchPrefix             string            // "username/"
    DetectNewSessions        bool
    SessionDetectionInterval int
    StateRefreshInterval     int
    LogsEnabled              bool
    LogsDir                  string
    LogMaxSize/MaxFiles/MaxAge int
    LogCompress              bool
    UseSessionLogs           bool
    TmuxSessionPrefix        string            // "staplersquad_"
    PerformBackgroundHealthChecks bool
    KeyCategories            map[string]string
    TerminalStreamingMode    string            // "raw", "state", "hybrid"
    VCSPreference            string            // "auto", "jj", "git"
    AvailablePrograms        []string
}
```

**Notable:** `DefaultProgram` already exists — session defaults extends this concept.

---

## Session Instance Fields Relevant to Defaults

From `session/instance.go` (Instance struct + InstanceOptions):

| Field | Type | Default-worthy? |
|---|---|---|
| `Program` | string | ✅ Primary field |
| `AutoYes` | bool | ✅ |
| `Tags` | []string | ✅ Pre-applied tags |
| `Category` | string | legacy, migrates to tags |
| `TmuxPrefix` | string | maybe |
| `AutonomousMode` | bool | maybe |
| `Prompt` | string | ❌ too session-specific |
| `GitHubPRNumber` | int | ❌ |

**Current default injection point** (`server/services/session_service.go:519-523`):
```go
program := req.Msg.Program
if program == "" {
    cfg := config.LoadConfig()
    program = cfg.DefaultProgram  // ← only one default source today
}
```

---

## Extension Strategy

### Proposed Go types (add to `config/config.go`)

```go
type SessionDefaults struct {
    Program        string                    `json:"program,omitempty"`
    AutoYes        bool                      `json:"auto_yes,omitempty"`
    Tags           []string                  `json:"tags,omitempty"`
    Profiles       map[string]ProfileDefaults `json:"profiles,omitempty"`
    DirectoryRules []DirectoryRule            `json:"directory_rules,omitempty"`
}

type ProfileDefaults struct {
    Program  string   `json:"program,omitempty"`
    AutoYes  bool     `json:"auto_yes,omitempty"`
    Tags     []string `json:"tags,omitempty"`
}

type DirectoryRule struct {
    PathPattern string         `json:"path_pattern"` // glob or prefix
    MatchMode   string         `json:"match_mode"`   // "exact", "prefix", "glob"
    Profile     string         `json:"profile,omitempty"`
    Overrides   ProfileDefaults `json:"overrides,omitempty"`
}
```

Add to `Config`:
```go
SessionDefaults SessionDefaults `json:"session_defaults,omitempty"`
```

### Migration safety
- All new fields are `omitempty` — old config.json loads without error
- Go zero-values apply for missing fields (empty string, false, nil slice)
- `DefaultConfig()` initializes `SessionDefaults{}` (empty, no-op)
- No version field needed — additive-only change

---

## Proto / API Surface

**Current `CreateSessionRequest` fields** (proto/session/v1/session.proto):
```proto
string title = 1;         // Required
string path = 2;          // Required
string working_dir = 3;
string branch = 4;
string program = 5;
string category = 6;
string prompt = 7;
bool   auto_yes = 8;
string existing_worktree = 9;
string resume_id = 10;
```

**Additions needed:**
```proto
string profile = 11;      // Apply named profile defaults
bool skip_defaults = 12;  // Bypass all defaults (explicit override)
```

**New RPC endpoints:**
```proto
rpc GetSessionDefaults(GetSessionDefaultsRequest)
    returns (GetSessionDefaultsResponse) {}

rpc UpdateSessionDefaults(UpdateSessionDefaultsRequest)
    returns (UpdateSessionDefaultsResponse) {}
```

---

## Precedence Resolution (highest → lowest)

```
1. Explicit request fields (non-empty Program, Tags, etc.)
2. DirectoryRule.Overrides (directory-matched rule)
3. Named Profile (request.profile or directory rule's profile)
4. SessionDefaults global fields
5. Config.DefaultProgram (legacy fallback — preserved for backward compat)
```
