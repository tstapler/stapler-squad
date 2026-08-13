# Stack Research: AliasesManager UI + UpsertAlias/DeleteAlias RPCs

## 1. Go Backend: Concurrent Config Writes

### Pattern in use
`config.LoadConfig()` and `config.SaveConfig()` are the only I/O entry points. Every service handler (e.g. `UpsertProfile`, `DeleteProfile`, `UpsertDirectoryRule`) follows the same stateless read-modify-write cycle:

```go
cfg := config.LoadConfig()           // fresh read from disk
// mutate cfg.SessionDefaults.*
if err := config.SaveConfig(cfg); err != nil { ... }
```

`SaveConfig` writes atomically via `os.WriteFile(tmpPath, ...)` → `os.Rename(tmpPath, configPath)`. The rename is atomic on Linux/macOS.

### No locking, no live reload
There is **no mutex or file lock** around concurrent calls. The config is loaded fresh on every RPC call and is not kept in memory between calls. This means:
- Concurrent `UpsertAlias` + `DeleteAlias` calls can race (last-write wins).
- There is no live reload mechanism — changes take effect only on the next `LoadConfig()` call.
- This matches the existing pattern for `UpsertProfile`/`DeleteProfile` exactly. The project has accepted this tradeoff; `AliasesManager` should not introduce a new locking scheme.

### AliasConfig data structure
`config.AliasConfig` (in `config/config.go`) is a slice (`cfg.SessionDefaults.Aliases []AliasConfig`), not a map (unlike profiles which use `map[string]ProfileDefaults`). Upsert must iterate the slice to find an existing entry by `Name`, replace it or append; delete must filter by `Name`. The pattern used by `UpsertDirectoryRule`/`DeleteDirectoryRule` in `defaults_service.go` is the exact model to follow.

### AliasConfig.Name validation
`AliasConfig` doc comment states: `Name must match ^[\w-]+$`. Enforce this in the handler with a regex check (same as profile name non-empty check) and return `connect.CodeInvalidArgument`.

### Key files
- `config/config.go` — `AliasConfig` struct, `SessionDefaults`, `LoadConfig`, `SaveConfig`, `saveConfig` (atomic write)
- `server/services/defaults_service.go` — `UpsertProfile`, `DeleteProfile`, `UpsertDirectoryRule`, `DeleteDirectoryRule`, `ListAliases`, `aliasConfigToProto` (all reference implementations)

---

## 2. Proto: What Exists vs. What Is Needed

### Already exists (no changes needed)
- `AliasProto` message — `proto/session/v1/session.proto` lines 1773–1785, with all fields (`name`, `group`, `path`, `description`, `profile`, `program`, `auto_yes`, `tags`, `env_vars`, `cli_flags`)
- `ListAliasesRequest` / `ListAliasesResponse` messages
- `rpc ListAliases` — already in service definition and fully registered in the connect handler
- Go and TypeScript generated bindings for all of the above

### Still needed
1. `rpc UpsertAlias` + `UpsertAliasRequest` / `UpsertAliasResponse` messages
2. `rpc DeleteAlias` + `DeleteAliasRequest` / `DeleteAliasResponse` messages

**Pattern to follow** (from same file, lines 1739–1753):
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

After adding messages, run `make generate-proto`. The generated Go and TS bindings will be updated automatically. `web-app/src/gen/` is tracked in git despite `.gitignore`; commit both `gen/` and `web-app/src/gen/` changes together.

---

## 3. React: Key-Value Pair Editing (`env_vars` map)

### Established codebase pattern
`GlobalDefaultsForm.tsx` is the canonical reference for editing `map<string, string>` env vars. The pattern:

```typescript
// State: flat array of {key, value} objects — NOT a JS Map or plain object
const [envVars, setEnvVars] = useState<{ key: string; value: string }[]>([]);

// Load: Object.entries(map) → array
const vars = Object.entries(defaults.envVars).map(([key, value]) => ({ key, value }));
setEnvVars(vars);

// Edit: immutable index update
const handleEnvVarChange = (index: number, field: "key" | "value", val: string) => {
  const updated = [...envVars];
  updated[index] = { ...updated[index], [field]: val };
  setEnvVars(updated);
};

// Delete: filter by index
const handleRemoveEnvVar = (index: number) => {
  setEnvVars(envVars.filter((_, i) => i !== index));
};

// Add: append empty row
const handleAddEnvVar = () => setEnvVars([...envVars, { key: "", value: "" }]);

// Save: reconstruct map, skipping blank keys
const envVarsMap: { [key: string]: string } = {};
for (const { key, value } of envVars) {
  if (key.trim()) envVarsMap[key.trim()] = value;
}
```

The UI renders two side-by-side `<input>` elements per row (KEY / value) plus a Delete button, wrapped in `envVarRow` (flexbox). See `GlobalDefaultsForm.tsx` lines 123–283 and `GlobalDefaultsForm.css.ts` for `envVarTable`, `envVarRow`, `deleteBtn` style tokens.

**Do not use a `<table>` or Radix primitives for this** — the existing pattern uses plain divs with vanilla-extract styles.

### Tags list editing
`ProfilesManager.tsx` is the reference for chip-style tag lists with an add-on-Enter input. The `tagList`, `tag`, `tagRemove`, `tagInputRow` tokens from `ProfilesManager.css.ts` can be copied verbatim into `AliasesManager.css.ts`.

---

## 4. Vanilla-Extract: Patterns to Follow

### Import convention (universal across all settings components)
```typescript
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";  // NOT theme-contract.css — always theme.css
```

`vars` is exported from `theme.css` (which applies the theme contract). Never import from `theme-contract.css.ts` directly in component stylesheets.

### Available token namespaces (from `theme-contract.css.ts`)
- `vars.color.*` — text, surfaces, borders, inputs, status, accents
- `vars.space.*` — `"0"` through `"16"` (string keys)
- `vars.radii.*` — `sm`, `md`, `lg`, `full`
- `vars.fontSize.*` — `xs`, `sm`, `base`, `lg`, `xl`
- `vars.fontWeight.*` — `normal`, `medium`, `semibold`, `bold`
- `vars.shadow.*`, `vars.transition.*`

### Key token usage in settings components
| CSS purpose | Token |
|---|---|
| Card / form container bg | `vars.color.cardBackground` |
| Input bg | `vars.color.inputBackground` |
| Input border | `vars.color.inputBorder` |
| Input focus border | `vars.color.inputFocusBorder` |
| Input text | `vars.color.inputText` |
| Section border | `vars.color.borderColor` |
| Muted / subtle border | `vars.color.borderSubtle` |
| Delete button bg | `vars.color.errorBg` |
| Delete button border | `vars.color.error` |
| Delete button text | `vars.color.errorText` |
| Tag chip bg | `vars.color.accentBg` |
| Primary text | `vars.color.textPrimary` |
| Secondary / label text | `vars.color.textSecondary` |
| Muted / meta text | `vars.color.textMuted` |

### Inline styles and z-index
- Never use inline `style={{ flexDirection: ... }}` or hardcoded `zIndex` numbers.
- Settings modals/overlays must use `createPortal` + a named slot from `zIndex` in `theme-contract.css.ts`.
- The `AliasesManager` form is inline (not a modal), so z-index is not relevant.

### Recipe / variants
`@vanilla-extract/recipes` v0.5.7 is installed. Use `recipe()` only when a component needs multiple intent/size variants. For a settings manager with a single visual style, `style()` is sufficient (same as `ProfilesManager.css.ts`).

---

## 5. Dependency Versions and Gotchas

| Package | Version | Notes |
|---|---|---|
| `@connectrpc/connect` | ^2.1.1 | Use `createClient(SessionService, transport)` pattern — matches existing service hooks |
| `@connectrpc/connect-web` | ^2.1.1 | `createConnectTransport({ baseUrl: getApiBaseUrl() })` |
| `@vanilla-extract/css` | ^1.20.1 | `style()` API stable; no breaking changes expected |
| `@vanilla-extract/recipes` | ^0.5.7 | Available but not needed for single-variant component |
| `@radix-ui/react-dialog` | ^1.1.15 | Not used in ProfilesManager; don't introduce unless truly modal |
| `react` | ^19.0.0 | Use `useRef` for client (avoids re-creating transport on every render) |
| `next` | 15.3.2 | `"use client"` directive required — all settings components are client components |

### Known CI gotchas
- `web-app/src/gen/` is tracked in git despite `.gitignore`. After `make generate-proto`, stage and commit changes in both `gen/proto/go/` and `web-app/src/gen/`.
- `buf` setup in CI has had rate-limit issues. If proto generation fails in CI, check `buf` action logs first.
- CSS lint (`lint:css`) will fail on any undefined `var(--token)` referenced from `.module.css` files. Since we're using vanilla-extract `.css.ts`, this doesn't apply — but don't introduce any `.module.css` files.
- `make quick-check` (build + test + lint) is the fastest pre-push check; `make ci` is the definitive gate.

### ConnectRPC client pattern
All settings components use a `useRef` for the ConnectRPC client to avoid transport recreation:
```typescript
const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
useEffect(() => {
  const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
  clientRef.current = createClient(SessionService, transport);
  loadAliases();
}, [loadAliases]);
```

### `as unknown as AliasProto` cast
`ProfilesManager.tsx` uses `as unknown as ProfileDefaultsProto` when constructing the proto message literal. This is expected — the generated TS types include readonly fields and metadata that can't be satisfied by plain object literals. Use the same cast in `AliasesManager`.

---

## 6. Summary of What Already Exists for Aliases

| Artifact | Status |
|---|---|
| `config.AliasConfig` struct | Exists |
| `config.SessionDefaults.Aliases []AliasConfig` | Exists |
| `AliasProto` protobuf message | Exists |
| `ListAliasesRequest/Response` protobuf messages | Exists |
| `rpc ListAliases` in service definition | Exists |
| Go handler `ListAliases` | Exists |
| `aliasConfigToProto` helper | Exists |
| Go generated bindings | Exists |
| TS generated bindings (`AliasProto`, `ListAliases*`) | Exists |
| `rpc UpsertAlias` | **Missing — needs to be added** |
| `rpc DeleteAlias` | **Missing — needs to be added** |
| Go handlers `UpsertAlias`, `DeleteAlias` | **Missing** |
| `AliasesManager.tsx` component | **Missing** |
| `AliasesManager.css.ts` styles | **Missing** |
