# CLI Subcommands — Technology Stack Research

Sources: `main.go` (lines 700–937), `cmd/commands/get_session.go`,
`cmd/commands/ensure_ports_free.go`, `go.mod`.

---

## Module and Go version

- **Module path**: `github.com/tstapler/stapler-squad`
- **Go version**: `go 1.26.6` (`go.mod` line 3)
- **Key deps**: `connectrpc.com/connect v1.20.0`, `github.com/spf13/cobra` (transitive)

---

## 1. Cobra patterns

Two registration styles coexist:

| Style | Where | Example |
|---|---|---|
| Package-level exported var | `cmd/commands/*.go` → registered in `main.go`'s `init()` | `commands.GetSessionCmd`, `commands.EnsurePortsFreeCmd` |
| File-level unexported var | Inline in `main.go` | `listSessionsCmd`, `printQRCodesCmd`, `debugCmd` |

New session-oriented subcommands (`create-session`, `list`, `steer`, `get-diff`) should use
the **`cmd/commands/` package** style — it keeps `main.go` from growing further and mirrors
the existing `GetSessionCmd` precedent.

**Structural conventions** (verified across both files):

- Use `RunE` (returns `error`), never bare `Run`.
- Positional args validated with `cobra.ExactArgs(N)` / `cobra.MinimumNArgs(N)`.
- Flags declared in the command's own `init()` via `cmd.Flags().StringVar(...)` etc.;
  read at runtime with `cmd.Flags().GetString(...)`.
- Hidden internal commands use `Hidden: true` on the `Command` struct.
- All commands registered via `rootCmd.AddCommand(...)` in `main.go`'s `init()`.
- Error messages follow `fmt.Errorf("short context: %w", err)` wrapping.

---

## 2. Output formatting — no `--json` flag convention

The existing `get-session` command **always emits JSON** via
`json.MarshalIndent(resp.Msg.Session, "", "  ")` then `fmt.Println(...)`. There is no
`--json` flag pattern; output format is baked in per command. The `list` command in
`main.go` uses plain human-readable text (`fmt.Printf`). New commands should pick one
mode appropriate to their audience and be consistent — for machine-usable commands
(`get-diff`, `list` when piped) always-JSON is the existing model; a `--json` flag can
be added later if mixed output is needed.

---

## 3. ConnectRPC client setup

Pattern from `cmd/commands/get_session.go` (the only ConnectRPC client in the repo):

```go
client := sessionv1connect.NewSessionServiceClient(
    http.DefaultClient,
    "http://localhost:8080/api",  // ← BUG: should be :8543 (main.go line 320 default)
)
req := &connect.Request[sessionv1.GetSessionRequest]{
    Msg: &sessionv1.GetSessionRequest{Id: args[0]},
}
resp, err := client.GetSession(context.Background(), req)
```

Key findings for new commands:

- **Base URL**: use `http://localhost:8543/api` (server default from `main.go:320`); expose
  as a persistent flag (`--server`, defaulting to `"http://localhost:8543/api"`) or read
  from `PORT` env var to match the server's own `PORT` env override. The existing
  `get-session` command hard-codes `:8080` — that is a bug to fix when touching it.
- **Auth**: no auth headers on `localhost` traffic; the server applies auth only to
  remote-access routes (`:8444`). `http.DefaultClient` is sufficient for loopback calls.
- **Timeout**: current code uses `context.Background()` with no timeout. New commands
  should use `context.WithTimeout(cmd.Context(), 30*time.Second)` at minimum; the
  cobra `cmd.Context()` already carries OS signal cancellation.
- **ConnectRPC options**: no `connect.WithInterceptors()` or retry options are set today.
  Not required for CLI use — ConnectRPC Connect-protocol calls are idempotent GET/POST
  over HTTP/1.1 to localhost; a single timeout context is enough.

---

## 4. ConnectRPC client type to use

```go
// Typed client for SessionService — import path verified in go.mod and get_session.go
sessionv1connect.NewSessionServiceClient(httpClient, baseURL)

// Request wrapper
connect.Request[sessionv1.SomeRequestType]{Msg: &sessionv1.SomeRequestType{...}}

// Response access
resp.Msg.SomeField
```

The `sessionv1connect` package lives at
`github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect` and is
generated from `proto/session/v1/session.proto` via `make proto-gen` (not committed —
`gen/`-prefixed output is in `.gitignore`; always run `make build` before editing these
files).

---

## Summary

1. New commands go in `cmd/commands/` as exported `var Cmd = &cobra.Command{...}` with
   `RunE`, registered in `main.go`'s `init()` via `rootCmd.AddCommand(commands.XxxCmd)`.
2. No `--json` flag convention exists; always-JSON is the established model for
   machine-readable commands (`get-session` precedent).
3. The existing `get-session` client uses `http.DefaultClient`, `context.Background()`,
   and hard-codes `:8080` (a bug — should be `:8543`). New commands should use
   `cmd.Context()` + a timeout, and default the base URL to `http://localhost:8543/api`
   with a `--server` flag override.
