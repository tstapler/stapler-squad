# Stack Research: backlog-already-implemented

## 1. What LLM client is used for the headless reviewer call?

**There is no Anthropic Go SDK / HTTP client in this repo at all.** `grep -n "claude" go.mod` and `grep -n "anthropic" go.mod` both return nothing — the module has zero Anthropic SDK dependency.

The "headless LLM call" mechanism (`session/headless/`) is a **subprocess wrapper around the `claude` CLI binary** (`claude -p ...`), invoked via `os/exec` through this repo's own `executor` package:

- `session/headless/runner.go:49` — `ProcessRunner` implements `ClaudeRunner`, calling `executor.StartProcess(ctx, r.claudeBin, args, opts...)` (`session/headless/runner.go:102`).
- `session/headless/caller.go:89` (`NewPool`) locates the `claude` binary via `exec.LookPath`/fallback dirs (`session/headless/caller.go:67-84`) — no HTTP client, no API key config beyond whatever env vars pass through `claudeAllowedEnvPrefixes` (`session/headless/runner.go:63-68`, includes `ANTHROPIC_`, `CLAUDE_`).
- The prompt is passed via **stdin** (not argv, to avoid leaking via `/proc/<pid>/cmdline` — `session/headless/caller.go:242-245`), and flags via argv.
- First call per session uses `-p --output-format json --system-prompt <prompt> --exclude-dynamic-system-prompt-sections [--model <model>]` (`session/headless/caller.go:173`); resumed calls use `-p --resume <sessionID> --exclude-dynamic-system-prompt-sections` (`session/headless/caller.go:180`).
- The reviewer call specifically goes through `Pool.CallBlockingWithCost(ctx, headless.FeatureKeyReview, headless.HeadlessReviewSystemPrompt(), headlessPrompt)` — `session/review_gate.go:252`.

**Model**: whatever `PoolConfig.DefaultModel` is configured to (not hardcoded in the files read); passed via `--model` only on the first call of a session.

## 2. Does the headless reviewer call have tool use / function calling today?

**No.** This is explicit in both code and comments:

- `session/review_gate.go:246-247`: `// Headless path: call LLM directly without spawning a tmux session. // Use JSON-output prompts because headless claude -p has no tool access.`
- The args built in `acquireSession` (`session/headless/caller.go:134-184`) never include `--allowedTools`, `--permission-mode`, `--mcp-config`, or `--dangerously-skip-permissions` — only `-p`, `--output-format json` / `--resume`, `--system-prompt`, `--exclude-dynamic-system-prompt-sections`, `--model`.
- Because of this, `headlessReviewSystemPrompt` (`session/headless/features.go:84-87`) instructs the model to emit a raw JSON verdict object instead of calling a tool — there is no `submit_review_verdict` tool wired into the headless path (that tool only exists for the **legacy/tmux** review path, described below).
- The **triage** headless call (`headless.FeatureKeyTriage`, `server/services/backlog_service_triage.go:671-676`) also goes through `Pool.CallBlockingWithOptions(..., headless.CallOptions{WorkDir: itemRepoPath})` — `CallOptions` (`session/headless/caller.go:18-25`) has only `WorkDir`, `Model`, `TimeoutSecs`, no `AllowedTools`/`PermissionMode` fields. Its system prompt claims "You have full filesystem write access to the artifact directory" (`session/headless/features.go:99`) but nothing in the Go call actually grants that beyond setting the subprocess `cwd` to the repo path — this looks like an existing latent gap/aspirational prompt text, not a working capability. Worth flagging but out of scope for this feature.

**Contrast — the non-headless (legacy) review path DOES have full tool access**, because it isn't a bare `claude -p` JSON call at all: it spawns a real interactive-mode tmux `Instance` running full Claude Code via `sessionCreator.SpawnReviewSession` (`session/review_gate.go:332`, interface at `session/backlog_lifecycle.go:30-32`, impl at `server/services/session_service.go:738`). Full `Instance`s support:
  - `Instance.AllowedTools` (`session/instance.go:234-237`) → rendered as `--allowedTools` (`session/instance_tmux.go:118`)
  - `Instance.PermissionMode` (`session/instance.go:239-241`) → rendered as `--permission-mode` (`session/instance_tmux.go:121`, with a `bypassPermissions` special-case at line 124)
  - `Instance.MCPServerURL` (`session/instance.go:225-227`) → rendered as `--mcp-config`

  These are the exact CLI flags needed to grant a headless `claude -p` call file-read tool access too — Read/Grep/Glob are **built into the Claude Code CLI itself** (not an MCP tool), so passing `--allowedTools "Read,Grep,Glob"` (or a narrower `Read(worktree/**)` pattern) plus `--permission-mode` (or `--dangerously-skip-permissions` — used nowhere in this repo currently) to a `claude -p` invocation is the CLI's native mechanism for giving a print-mode call sandboxed file access. This requires **no new Go dependency** — just extending `CallOptions`/`acquireSession` to pass these flags through, mirroring the pattern already proven for interactive `Instance`s.

  Since `pool != nil` is checked first in `review_gate.go:245`, this legacy tool-using path is effectively dead/fallback-only whenever a headless pool is configured (which appears to be the normal running configuration) — it is not something the empty-diff feature can lean on for production traffic, but it's useful precedent for the flag-plumbing pattern.

## 3. Existing internal helpers for reading files / grep scoped to a worktree (security/sandboxing)

- **`server/services/file_service.go`** is the reusable candidate:
  - `resolveAndValidatePath(base, rel string) (string, error)` (`server/services/file_service.go:106-115`) — the canonical path-traversal guard in this repo (`filepath.Join` + `filepath.Clean` + prefix check against `base+separator`). Tested in `server/services/file_service_test.go:269` (`TestResolveAndValidatePath_TraversalRejected`) and `:291` (`TestResolveAndValidatePath_ValidPath`).
  - `FileService.GetFileContent` (`server/services/file_service.go:282-423`) — reads a file scoped to a session's worktree (`ws.EffectivePath` via `WorkspaceProvider.GetWorkspace`, `server/services/workspace_service.go:27-30`), with binary detection, a 10 MB hard cap (`maxFileSize`, line 28) and 1 MB soft truncation (`truncateSize`, line 32).
  - `FileService.ListFiles` (`:118-279`) and `FileService.SearchFiles` (`:442-490`) — directory listing and **filename/path substring search** (gitignore-aware), not content grep.
  - **No content-grep helper exists anywhere in the Go backend** (confirmed via `grep -rn "func.*[Gg]rep" server/ session/`) — `SearchFiles` only matches file names/paths, not file contents. A new reviewer tool needing "grep for a string across the worktree" would need new code; it could reuse `resolveAndValidatePath` + the existing `hardSkipDirs`/gitignore machinery (`loadGitignorePatterns`, `collectAllGitignorePatterns`) as a base, or simply shell out to `git grep` scoped to the worktree.
  - This service is exposed as a **ConnectRPC service for the web UI file browser**, not as an MCP tool — it is keyed by `session_id`, not by a raw path, and returns typed proto messages (`sessionv1.GetFileContentResponse`, etc.). It is not directly callable from a headless `claude -p` subprocess; reuse would mean either (a) factoring `resolveAndValidatedPath`-based read logic into a shared package callable from both the RPC handler and a new MCP tool, or (b) just giving the headless call the CLI's native `Read`/`Grep` tools per §2 above, sandboxed to the worktree directory via `--allowedTools`/cwd, which avoids reinventing this at all.
- **`server/mcp/` (the stapler-squad-internal MCP server exposing `report_progress`, `request_review`, `get_backlog_item`, etc. to agent sessions) has no file-read or grep tool** (`server/mcp/tools_discovery.go`, `tools_terminal.go`, `tools_vcs.go` — confirmed via `grep -n "func "` across all three; nothing matches `readFile`/`grep`). This makes sense: interactive Claude Code sessions that use this MCP server already have native Read/Grep/Glob/Bash tools from the CLI itself, so the app-level MCP server was never asked to duplicate that.
- No general `ValidatePath`/`SafeJoin`/sandboxing helper exists outside `file_service.go`'s `resolveAndValidatePath` — worktree-path-adjacent hits in the earlier grep (`session/repo_path.go`, `session/instance_worktree.go`, etc.) are about *locating/resolving* worktree directories (git worktree management), not about sandboxing file reads within one.

## 4. Versions pinned in go.mod (relevant deps)

From `go.mod` (module `github.com/tstapler/stapler-squad`, `go 1.26.3`):

| Dependency | Version | Relevance |
|---|---|---|
| `github.com/mark3labs/mcp-go` | `v0.48.0` | Powers `server/mcp/` (the stapler-squad MCP server exposing `report_progress`, `request_review`, etc. to agent sessions) — **not** used by the headless reviewer call itself. |
| `connectrpc.com/connect` | `v1.19.0` | Backs `FileService` and all other RPC services (`server/services/*.go`). |
| `connectrpc.com/otelconnect` | `v0.8.0` | OTel instrumentation for ConnectRPC. |
| `github.com/go-git/go-git/v5` | `v5.14.0` | Used for gitignore pattern matching in `file_service.go` (`go-git/plumbing/format/gitignore`), and elsewhere for git operations. |
| `github.com/go-git/go-billy/v5` | `v5.6.2` | go-git filesystem abstraction (indirect via go-git). |
| — Anthropic Go SDK | **absent** | No `github.com/anthropics/anthropic-sdk-go` or similar in go.mod — confirms the headless path is 100% CLI-subprocess-based, not API-client-based. |

No `claude` CLI version pin was found in `package.json`, `Makefile`, or `scripts/*.sh` — the CLI is expected to already be installed/discoverable via `PATH` or well-known install dirs (`session/headless/caller.go:39-47`). The code already relies on relatively current CLI flags (`--exclude-dynamic-system-prompt-sections`, `--resume`, `--output-format json`), so whatever CLI version is currently required in production already supports `--allowedTools`/`--permission-mode` (used elsewhere in this same codebase for interactive sessions) — no CLI upgrade should be needed to add tool access to the headless path.

## Implications for planning (Phase 3)

1. **No new external dependency is needed.** The minimal-dependency path to giving the reviewer file-read capability is to extend `headless.CallOptions` (`session/headless/caller.go:18-25`) with `AllowedTools`/`PermissionMode`-equivalent fields, thread them through `acquireSession`'s arg-building (`session/headless/caller.go:134-184`) and `CallWithOptions` (`:391-435`), and pass `WorkDir` (already supported) set to the worktree path so the CLI's built-in Read/Grep/Glob tools are sandboxed to that directory. This directly mirrors the existing `Instance.AllowedTools`/`PermissionMode` → `--allowedTools`/`--permission-mode` pattern already proven in `session/instance_tmux.go:118-124`.
2. **Prompting changes are required regardless of tool access** — the headless system prompt (`headlessReviewSystemPrompt`, `session/headless/features.go:84-87`) currently asks for a bare JSON verdict; if tool use is enabled, `claude -p` needs a compatible output contract (either still-JSON-after-tool-use, which `-p` supports, or a switch to the `submit_review_verdict`-tool pattern used by the interactive path's `reviewSystemPrompt`, `session/headless/features.go:79-80`).
3. **`report_progress`'s per-criterion `note` field** (mentioned in requirements as currently dropped) is a separate, purely prompt/data-plumbing fix — `BuildHeadlessReviewPrompt` (`session/backlog_review.go:135-137`) would need to include it; no stack/dependency implications there.
4. **If a content-grep capability is wanted** (beyond CLI-native `Grep`, e.g. an app-level MCP tool), there is no existing helper to reuse for content search — only filename/path search (`SearchFiles`) exists. Cheapest option is likely to lean entirely on the CLI's native `Grep` tool via `--allowedTools` rather than adding a new Go-side grep implementation.
