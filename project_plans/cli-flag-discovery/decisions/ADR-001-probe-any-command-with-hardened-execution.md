# ADR-001: ProbeProgram accepts any command but hardens execution and guards the request

**Status**: PENDING human reviewer sign-off — implementation proceeds on coordinator decision
**Date**: 2026-09-21 (revised after adversarial and architecture review)

## Context
AC8 says probing is "only allowed for the command the user is configuring". The `:8543` listener has no auth (research/pitfalls.md section 1: `SetupAuth` is wired only for the remote server), so the server, not the UI, must enforce the boundary. The pitfalls research preferred a saved-only allow-list, which cannot cover the unsaved command typed into Program Config (AC5). research/architecture.md section (e) proposed option A (any command, hardened).

Verified in the repo (2026-09-21): the HTTP chain is `otelhttp -> Logging -> CORSWithOrigins -> Compress -> [auth] -> mux` (`server/server.go:1372-1382`). `CORSWithOrigins` only sets `Access-Control-*` headers for allow-listed origins and never rejects a request (`server/middleware/cors.go:9-40`). Nothing in `server/middleware/` checks `Host` or `Origin`. Connect-go rejects unsupported content types and serves unary GET only for `NO_SIDE_EFFECTS` procedures. Conclusion: a cross-site JSON POST is already stopped by the failed CORS preflight, but a DNS-rebinding request is not, so a guard is required. `listen_address` is configurable (`config/config.go:212-214`, default `localhost:8543` at `:848`), so the listener may be non-loopback.

## Decision
The RPC accepts a command string but reduces it to a fixed, low-privilege execution shape enforced in `config/clihelp`, and a request guard limits who may call it.

**Target rules (server-side, `Resolve` + `checkExecutable`)**
- First token only after skipping leading `NAME=value` assignments; never a shell for user text; all later tokens discarded.
- The token must be a **bare command name** (no path separator; looked up in the login-shell + server PATH) or an **absolute path** (after `~` / `~/` expansion). Any other token containing a separator (`./x`, `bin/x`, `../x`, `~user/x`) is rejected as `RELATIVE_PATH` -> `NOT_FOUND`. `exec.ErrDot` is treated as not found.
- The resolved target must be a **regular file**, executable, and **not world-writable** (`mode&0o002==0`); otherwise `NOT_FOUND`. Symlinks are evaluated before these checks and the cache key.
- Known wrappers (`env`, `sudo`, `npx`, `uv`, `uvx`, `nice`, `time`, `exec`, `nohup`, `xargs`, `command`) are existence-checked only and never executed.

**Execution shape**
- Args are exactly `--help`. Env allowlist: `PATH`, `HOME`, `TERM=dumb`, `NO_COLOR=1`, `CI=1`, `COLUMNS`, `LANG`/`LC_ALL`, `PAGER=cat`, `MANPAGER=cat`. No tokens or API keys inherited.
- cwd is an empty temp dir; stdin is /dev/null; own session and process group; group SIGKILL on the timeout (default 3s, configurable through `Limits`; see plan Flagged Choice 11), on cap overflow, and after normal exit (ADR-002).
- Global 2-slot semaphore acquired by the singleflight leader only (rejection -> `BUSY`, never cached); singleflight per cache key on a non-cancellable context (ADR-002); `NOT_FOUND`, `ERROR`, `BUSY` never cached.

**Confirm before executing scripts (added in SDD phase 4, pre-mortem failure #2)**
- Many user commands are wrapper scripts that ignore `--help` and run their body. `Prober.Probe` reads the first bytes of the resolved file: a native binary (ELF `\x7fELF`, Mach-O magic) may be run implicitly; a `#!` shebang script or any unrecognized file type returns `NEEDS_CONFIRM` **without executing**. It runs only when the request carries `confirm_execute=true`, which the UI sends only for an explicit user action (the `Check` button, or Enter in the command field, which is defined as Check). Blur never confirms.
- The session-creation picker never executes on selection: it sends `resolve_only=true`, which resolves and stat-checks the file (found or not found) and returns a cached result if one exists, else `NEEDS_CONFIRM`. Execution there needs an explicit `Check`.
- Once a (real path, mtime, size) target has been confirmed, the server remembers it for the process lifetime (bounded set), so later blur probes of that same file run without another click; a changed mtime requires a new confirmation.
- Interpretation of AC8 recorded in `requirements.md` ("Interpretation notes"). This mitigation and the AC8 reading are **not yet approved**; sign-off stays PENDING.
- Known cost: Node and Python CLIs such as claude, gemini and aider are usually shebang scripts, so the first check of each needs one click.

**Request guard (`ProbeGuard`, `server/middleware/probeguard.go`, scoped to the full path `/api/session.v1.SessionService/ProbeProgram`)**
- **Placement (iteration 2 fix).** One mux serves both listeners: `Start()` (`:8543`, `authMiddleware == nil` in `main.go`) and `StartRemote()` (`:8444`, TLS + `middleware.Auth`, `server/server.go:1374-1377` vs `:1643-1646`). The guard is installed only in `Start()`'s chain, around `inner`, when `authMiddleware == nil`; never at route registration (`:414`, shared mux, config not yet set, path already stripped) and never in `StartRemote()` (a Host check there would 403 the mobile app's LAN/Tailscale hostname; auth is that listener's boundary).
- POST only (the RPC is never marked `NO_SIDE_EFFECTS`, so Connect GET is unavailable).
- Host hostname must be `localhost`, `127.0.0.1`, `::1`, or a name published via `SetHostnames`; an `Origin` header, if present, must be loopback or in the configured origins. Origins, hostnames and the bound address are read lazily per request (`SetOrigins`/`SetHostnames` run after construction, `main.go:435-450`). A non-loopback bind with no auth refuses the procedure entirely (the UI shows "Couldn't check", not "not found").
- Tests: `server/middleware/probeguard_test.go` (table, plus one mounted in front of the real Connect handler) and `server/server_probeguard_test.go` (`Start()` chain has the guard and 403s a non-loopback Host on `:8543`; `StartRemote()` chain does not; late `SetOrigins` honored).

**Audit**: one `slog` Info line `program_probe` per probe attempt (all outcomes), with resolved path, first token, status, duration, cache hit and remote address; never env values or later args.

## Alternatives rejected
- **Saved-only allow-list**: cannot probe the unsaved form; makes AC5 useless for new programs.
- **Option B: `program_id` for saved plus `unsaved=true`**: more code for a boundary the guard and target rules already narrow. Escalation to B remains additive (add a field, keep the runner and guard).

## Consequences
- AC8 is met by shaping and guarding, not by identity: an authenticated-equivalent local caller can still cause `<any-safe-regular-file> --help` to run with an empty env in an empty dir. Residual risk: a native binary that misbehaves on `--help` (scripts now need explicit confirmation). Accepted because the same caller can already create sessions running arbitrary programs, and the guard closes the browser/rebinding vector that argument does not cover.
- The plan restates AC8 honestly as "server hardening" (plan AC8 GWT).
- Owner sign-off is **PENDING**; this ADR must not be read as approved by Tyler.
