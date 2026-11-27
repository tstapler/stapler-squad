# Use the ssq CLI

`ssq` calls the stapler-squad ConnectRPC API from a shell. Its commands are built at startup from the compiled `session.v1` service descriptors, so a new RPC or request field appears without a CLI change. Nothing is generated or committed for it beyond the existing `gen/proto/go` output.

## Build

```bash
make build-ssq     # ./ssq
make install-ssq   # ~/.local/bin/ssq
```

## Use

```bash
ssq sessions list                         # friendly aliases: list, get, restart, delete, ack, diff
ssq sessions restart my-session --preserve-output
ssq session list-sessions --category work --status paused   # <service> <rpc>, kebab-case
ssq backlog --help                        # every service is a command group
ssq session get-session --json '{"id":"my-session"}'        # raw body: JSON, @file, or - (stdin)
```

- Request fields that are strings, enums, bools, numbers or repeated strings become flags (`hide_paused` is `--hide-paused`). Enum flags accept `paused` or `SESSION_STATUS_PAUSED`. Message and map fields go through `--json`; flags override fields from `--json`.
- Output is indented JSON; `--compact` prints one line. Errors go to stderr with exit status 1.
- Streaming RPCs (`WatchSessions`, `StreamTerminal`, ...) are not exposed.

## Connection and auth

| Setting | Flag | Env | Default |
|---|---|---|---|
| Server | `--url` | `STAPLER_SQUAD_URL` | `http://localhost:8543` |
| Token | `--token` | `STAPLER_SQUAD_TOKEN` | the local token file in the config dir |
| Deadline | `--timeout` | | 15s |

The local token file is the one `ssq-hooks` uses (`pkg/localtoken`); the config dir honors `STAPLER_SQUAD_INSTANCE`, so set it when you target an isolated instance. The token file is only used for a loopback `--url`; an explicit token over plain http to a non-loopback host is refused, and redirects are not followed. The token is sent as a Bearer header; it is only checked when `require_local_auth` is on. A hung RPC fails at `--timeout` with a message naming the RPC; the same budget goes to the server as `Connect-Timeout-Ms`.

To try it against an isolated instance, follow the manual-instance recipe in `CLAUDE.md` and pass `--url http://localhost:62871`; never point state-changing commands at `:8543` by accident.
