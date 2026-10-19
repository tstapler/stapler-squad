# CLI Subcommands Architecture Research

## 1. Shared Client Construction

**Where it lives:** `cmd/commands/` package, a new `client.go` file with a package-level `NewSessionClient(serverURL string) sessionv1connect.SessionServiceClient` helper. The `--server`/`SSQ_SERVER` flag belongs on the root cobra command as a `PersistentFlag` so every subcommand inherits it without re-declaring it.

**Current state:** `get_session.go` hard-codes `http://localhost:8080/api` inline. The fix is a single `serverURL` string var bound to `rootCmd.PersistentFlags().StringVar(...)` in `main.go`'s `init()`, with `os.Getenv("SSQ_SERVER")` as the default (falling back to `http://localhost:8543/api` if empty). Each `cmd/commands/*.go` command receives the resolved URL — either via a package-level var exported from `main` (not ideal, creates coupling) or via a constructor function (`NewGetSessionCmd(serverURL *string)`) that closes over the pointer populated before `Execute()` runs.

**Recommendation:** Use a constructor approach: `func NewGetSessionCmd(serverURL *string) *cobra.Command` mirroring `newIntegrationCredentialCmd()` and `newContractBundleCmd()` in `main.go` (lines 932-933 show this is already the established pattern for commands needing runtime state). The client is created inside `RunE`, not at command construction time, so it picks up the parsed flag value.

## 2. ConnectRPC Error Surfacing

**Pattern:** `connectrpc.com/connect` wraps RPC errors as `*connect.Error`. The `connect.CodeOf(err)` function extracts the code from any error (returns `connect.CodeUnknown` for non-connect errors). Human-readable status messages come from `(*connect.Error).Message()`.

**Recommended pattern for CLI:**

```go
if err != nil {
    var connectErr *connect.Error
    if errors.As(err, &connectErr) {
        return fmt.Errorf("%s: %s", connectErr.Code(), connectErr.Message())
    }
    return fmt.Errorf("request failed: %w", err)
}
```

This surfaces errors like `not_found: session abc123 does not exist` instead of the raw gRPC wire format. No existing command in `cmd/commands/` handles connect errors explicitly — `get_session.go` just `fmt.Errorf("could not get session: %w", err)`.

## 3. Output Contract: --json and serialization library

**Existing precedent:** `get_session.go` (line 37) uses `encoding/json` (`json.MarshalIndent(resp.Msg.Session, "", "  ")`). The generated proto structs have `protojson` tags (field name casing follows proto JSON mapping), but `encoding/json` also works and produces camelCase from the struct tags. The two differ in:
- `protojson` preserves proto3 field naming conventions and handles `oneof`/`optional` semantics correctly; omits zero-value fields by default.
- `encoding/json` uses Go struct tags; for generated proto types these are the same camelCase names, but zero-value fields (empty string, 0) are omitted only with `omitempty`.

**Recommendation:** Use `google.golang.org/protobuf/encoding/protojson` for `--json` output on proto-typed responses (already a transitive dep since the project uses Connect). For plain string/int output (e.g. diff stats), `fmt.Printf` is sufficient and `--json` can use `encoding/json` with a simple struct. `DiffStats` (returned by `GetSessionDiff`) is a proto message so `protojson` is the right choice there.

## 4. Existing `listSessionsCmd` and Refactor Path

**Current implementation** (main.go lines 729-776): `listSessionsCmd` is a `var` in `main.go` that directly instantiates `session.NewEntRepository()` and `session.NewStorageWithRepository(repo)` — it reads the local SQLite DB, bypassing the running server entirely. This is a different code path from the ConnectRPC `ListSessions` RPC.

**The refactored version** should be a new `cmd/commands/list_sessions.go` file that calls `SessionService.ListSessions` via ConnectRPC, following the same client-constructor pattern. The old `listSessionsCmd` in `main.go` can remain as-is (it accesses local state and does not need a running server), or can be deprecated once the RPC-backed version lands. The new command should be registered from `init()` alongside `commands.GetSessionCmd` (main.go line 935).

**`ListSessionsRequest` fields** (from proto): supports `status_filter` (repeated `SessionStatus`), `category_filter` (string), `tags_filter` (repeated string), and `search_query`. A minimal `ssq list` with no flags calls with an empty request.

## 5. Command Registration Integration Point

New commands are registered in `main.go`'s `init()` function (lines 864-937). The pattern for commands needing runtime state (like `serverURL`) is:

```go
// In main.go init():
rootCmd.PersistentFlags().StringVar(&serverURL, "server",
    envOrDefault("SSQ_SERVER", "http://localhost:8543/api"),
    "stapler-squad server base URL")

rootCmd.AddCommand(commands.NewGetSessionCmd(&serverURL))
rootCmd.AddCommand(commands.NewListSessionsCmd(&serverURL))
rootCmd.AddCommand(commands.NewCreateSessionCmd(&serverURL))
rootCmd.AddCommand(commands.NewSteerCmd(&serverURL))
rootCmd.AddCommand(commands.NewGetDiffCmd(&serverURL))
```

The existing `commands.GetSessionCmd` (a `var`, not a constructor) will need to be converted to `NewGetSessionCmd(*string)` as part of the port-fix.

## 6. Persistent Flags: Root vs Per-Command

**Current pattern:** All flags in `main.go`'s `init()` use `rootCmd.Flags()` (local to `rootCmd`) or per-command `.Flags()`. There are **no** `rootCmd.PersistentFlags()` calls today — every flag is either on `rootCmd` itself (only applies when running without a subcommand, i.e. the web server mode) or on a specific subcommand.

**Implication:** Adding `--server` as a `PersistentFlag` on `rootCmd` is net-new infrastructure. This is the right call — it avoids duplicating the flag on every subcommand — but it does mean the web server mode will also parse (and ignore) `--server`, which is harmless.

**Alternative:** Put `--server` on each subcommand's `Flags()` individually. This is noisier but avoids the persistent-flag-on-root pattern. Given the requirement for `SSQ_SERVER` env var fallback shared across all commands, the persistent root flag is the cleaner design.

## Key Bug Fix

`get_session.go` line 25: `"http://localhost:8080/api"` should be `"http://localhost:8543/api"`. The correct mount point is confirmed by main.go line 320 (`address = "localhost:8543"`) and CLAUDE.md ("Go web server on `localhost:8543`").

## Proto Message Quick Reference

| Command | RPC | Key request fields |
|---|---|---|
| `create-session` | `CreateSession` | `title` (required), `path` (required), `branch`, `program`, `category`, `prompt` |
| `list` | `ListSessions` | `status_filter[]`, `category_filter`, `tags_filter[]`, `search_query` |
| `steer` | `UpdateSession` | `id` (required), `steer_message` (optional string, field 11) |
| `get-diff` | `GetSessionDiff` | `id` (required) → returns `DiffStats` |
