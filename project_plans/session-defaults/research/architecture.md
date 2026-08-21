# Architecture Research: Session Defaults

## Existing Patterns

### ConnectRPC Endpoint Structure

Endpoints defined in `proto/session/v1/session.proto`, implemented in `server/services/`, registered in `server/server.go:179`.

Pattern:
```go
// Handler signature
func (s SessionService) CreateSession(ctx context.Context, req connect.Request[sessionv1.CreateSessionRequest]) (connect.Response[sessionv1.CreateSessionResponse], error)

// Registration
path, handler := sessionv1connect.NewSessionServiceHandler(deps.SessionService, ConnectOptions()...)
```

Error pattern: `connect.NewError(connect.CodeXXX, err)`
Config access: `config.LoadConfig()` inline inside handlers.

### React Create-Session Flow

`web-app/src/components/sessions/SessionWizard.tsx` — multi-step form:
1. Basic Info (title, category)
2. Repository (path, branch)
3. Configuration (program, prompt, autoYes)
4. Review

Uses `react-hook-form` + Zod schema. Accepts `initialData?: Partial<SessionFormData>` — this is the hook for injecting defaults. Form merges: `{ ...defaultValues, ...initialData }`.

**Current default injection point** (`server/services/session_service.go:519-523`):
```go
program := req.Msg.Program
if program == "" {
    cfg := config.LoadConfig()
    program = cfg.DefaultProgram  // ← only one default source today
}
```

---

## Proposed Go Data Model

Add to `config/config.go`:

```go
type SessionDefaults struct {
    Global           GlobalDefaults              `json:"global"`
    Profiles         map[string]Profile          `json:"profiles"`
    DirectoryDefaults []DirectoryDefaults         `json:"directory_defaults"`
}

type GlobalDefaults struct {
    Program  string            `json:"program"`
    Tags     []string          `json:"tags"`
    Category string            `json:"category"`
    EnvVars  map[string]string `json:"env_vars"`
    CLIFlags string            `json:"cli_flags"`
    AutoYes  bool              `json:"auto_yes"`
    Prompt   string            `json:"prompt"`
}

type Profile struct {
    Name        string            `json:"name"`
    Description string            `json:"description"`
    Program     string            `json:"program"`
    Tags        []string          `json:"tags"`
    Category    string            `json:"category"`
    EnvVars     map[string]string `json:"env_vars"`
    CLIFlags    string            `json:"cli_flags"`
    AutoYes     bool              `json:"auto_yes"`
    Prompt      string            `json:"prompt"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}

type DirectoryDefaults struct {
    Path     string            `json:"path"`     // Absolute path
    Program  string            `json:"program"`
    Tags     []string          `json:"tags"`
    Category string            `json:"category"`
    EnvVars  map[string]string `json:"env_vars"`
    CLIFlags string            `json:"cli_flags"`
    AutoYes  bool              `json:"auto_yes"`
}

// Add to Config struct:
SessionDefaults SessionDefaults `json:"session_defaults,omitempty"`
```

---

## Proposed Proto / API Design

```protobuf
service SessionService {
    // Existing RPCs ...

    rpc GetSessionDefaults(GetSessionDefaultsRequest)
        returns (GetSessionDefaultsResponse) {}

    rpc UpdateGlobalDefaults(UpdateGlobalDefaultsRequest)
        returns (UpdateGlobalDefaultsResponse) {}

    rpc UpsertProfile(UpsertProfileRequest)
        returns (UpsertProfileResponse) {}

    rpc DeleteProfile(DeleteProfileRequest)
        returns (DeleteProfileResponse) {}

    rpc UpsertDirectoryDefaults(UpsertDirectoryDefaultsRequest)
        returns (UpsertDirectoryDefaultsResponse) {}

    rpc DeleteDirectoryDefaults(DeleteDirectoryDefaultsRequest)
        returns (DeleteDirectoryDefaultsResponse) {}

    // Key endpoint: called by create-session form on mount/path-change
    rpc ResolveDefaults(ResolveDefaultsRequest)
        returns (ResolveDefaultsResponse) {}
}

message ResolveDefaultsRequest {
    string working_dir = 1;
    optional string profile_name = 2;
    bool skip_directory_defaults = 3;
}

message ResolveDefaultsResponse {
    GlobalDefaults resolved = 1;
    bool used_global = 2;
    bool used_directory = 3;
    bool used_profile = 4;
    optional string matched_directory = 5;
}
```

`ResolveDefaults` is the primary RPC for the create-session flow — call it with `working_dir` and optional `profile_name`, get back pre-merged defaults ready to inject into the form.

---

## Precedence Resolution Algorithm

```
ResolveDefaults(workingDir, profileName):
  result = copy(GlobalDefaults)         // layer 1: global
  meta = {used_global: true}

  dirDefault = findClosestMatch(workingDir, DirectoryDefaults)
  if dirDefault != nil:
    merge(result, dirDefault)            // layer 2: directory (overrides global)
    meta.used_directory = true

  if profileName != "":
    profile = Profiles[profileName]
    if profile exists:
      merge(result, profile)             // layer 3: profile (overrides all)
      meta.used_profile = true

  return result, meta

findClosestMatch(workingDir, dirs):
  // Return entry with longest matching path prefix
  // e.g., /home/user/projects/foo wins over /home/user/projects
  best = nil
  for dir in dirs:
    if workingDir == dir.path OR strings.HasPrefix(workingDir, dir.path + "/"):
      if best == nil OR len(dir.path) > len(best.path):
        best = dir
  return best

merge(target, source):
  // Non-zero source fields win; zero/empty source fields do NOT overwrite
  if source.program != "": target.program = source.program
  if len(source.tags) > 0: target.tags = source.tags
  if source.category != "": target.category = source.category
  for k, v in source.env_vars: target.env_vars[k] = v
  if source.cli_flags != "": target.cli_flags = source.cli_flags
  if source.auto_yes: target.auto_yes = true
  if source.prompt != "": target.prompt = source.prompt
```

Precedence order (lowest → highest): **global → directory → profile**

---

## React Integration Design

### New hook: `useSessionDefaults`

```typescript
interface UseSessionDefaultsResult {
    defaults: GlobalDefaults | null;
    usedGlobal: boolean;
    usedDirectory: boolean;
    usedProfile: boolean;
    matchedDirectory: string | null;
    loading: boolean;
    error: string | null;
    resolve: (workingDir: string, profileName?: string) => Promise<void>;
}
```

### SessionWizard integration

```typescript
// SessionWizard receives workingDir prop
const { defaults } = useSessionDefaults(workingDir, selectedProfile);

// Re-resolve when profile or workingDir changes
useEffect(() => { resolve(workingDir, selectedProfile); }, [selectedProfile, workingDir]);

// Inject into form (initialData wins over defaults)
useEffect(() => {
    if (defaults) reset({ ...defaultValues, ...defaults, ...initialData });
}, [defaults]);
```

Profile selector added in Configuration step (step 2).
"Save as Profile" + "Save as Directory Default" buttons in Review step (step 3).

### CreateSessionRequest additions

```protobuf
string profile = 11;      // Apply named profile
bool skip_defaults = 12;  // Bypass all defaults
```

---

## Settings Page Structure

New route: `/settings` → `web-app/src/app/settings/`

```
SettingsPage
├── SettingsNav (sidebar)
└── DefaultsSettings
    ├── GlobalDefaults section
    │   ├── ProgramSelector
    │   ├── TagsInput
    │   ├── EnvVarsEditor (key/value table)
    │   └── CLIFlagsInput
    ├── ProfilesManager section
    │   ├── ProfilesList (name, description, edit/delete)
    │   └── ProfileForm (inline create/edit)
    └── DirectoryDefaultsManager section
        ├── DirectoriesList (path → profile/overrides)
        └── DirectoryForm (path picker + defaults)
```

State: `getSessionDefaults()` on mount; individual `updateGlobal / upsertProfile / upsertDirectory` RPCs on save. Toast feedback on success/error.

---

## Key Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Persistence | Extend `config.json` | No new file, atomic writes already implemented |
| Directory matching | Longest-prefix registration (not walk-up) | Deterministic, no FS traversal, user controls what's registered |
| Precedence | global → directory → profile (fixed) | Predictable; profile is most intentional so it wins |
| Migration | `DefaultProgram` → `SessionDefaults.Global.Program` fallback | Backward compat via zero-value check in `ResolveDefaults` |
| Settings entry point | `/settings` route + "Save as default" in create dialog | Matches UX requirement for both |
| Proto scope | Program, tags, category, env_vars, cli_flags, auto_yes, prompt | Full set of session creation fields |
