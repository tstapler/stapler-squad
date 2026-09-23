# Research
Verified by reading the repo:
- `config/executor.go:13` defines `CommandExecutor` (Command/Output/LookPath) — inject for testable probe.
- `proto/session/v1/session.proto:2526` `ProgramConfigProto` (command, cli_flags); CRUD RPCs at :565-571, served by `server/services/defaults_service.go`.
- UI: `web-app/src/components/settings/ProgramsManager.tsx` (`prog-command`, `prog-flags` inputs). Session-creation program picker not yet inspected (INFERRED under `web-app/src/components/sessions/`; see session-creation-registry doc).
- `config/config.go:1267` already uses `executor.LookPath` for claude.

Design decisions:
- Server-side probe (browser can't exec). Add RPC in DefaultsService; `make proto-gen`; regenerate feature registry.
- Regex parser over help lines; tolerate failure. Reject third-party parser deps (YAGNI).
- Risk: executing arbitrary user-configured binaries with `--help` — some tools ignore `--help` and start (e.g. TUIs). Mitigate: timeout + kill process group, closed stdin, run only on explicit blur/select, and only for programs in saved config or the form being edited (single-user local app, existing threat model already runs these commands).
- Cache: sync.Map keyed by path+mtime.
