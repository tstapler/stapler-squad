# Build vs. Buy: backlog-already-implemented

**Date**: 2026-07-14
**Scope**: How should the reviewer gain bounded file-read/grep access to the worktree when the diff is empty/near-empty, so it can verify "already implemented" claims instead of blind-stamping UNVERIFIABLE?

---

## Summary verdict

**Do not add a library, an Anthropic SDK dependency, or a hand-rolled agentic loop.** The codebase already has the exact primitive this feature needs — `session/headless.Pool`, a subprocess wrapper around the `claude` CLI binary — and the CLI itself already supports a bounded tool-use loop (`--allowedTools`, `--permission-mode`) that is used elsewhere in this repo (`session/instance_tmux.go`) but has never been wired into the headless pool's `ProcessRunner`. The correct scope for this feature is: add `AllowedTools`/`PermissionMode` fields to `headless.CallOptions`, thread them into `ProcessRunner.Run`'s arg construction (mirroring `buildClaudeCommand`), and call the review gate with `WorkDir` set to the item's repo, `AllowedTools: "Read,Grep,Glob"`, and a non-interactive permission mode. This is a ~30-50 line plumbing change, not a new subsystem.

---

## 1. Existing OSS library/framework — is there a Go "LLM + tool loop" library already in use?

**Checked**: `go.mod`, `session/headless/`, `server/mcp/`.

- `go.mod` has **no `anthropic-sdk-go`** and no other LLM API client library. The only relevant dependency is `github.com/mark3labs/mcp-go v0.48.0`, which is used exclusively to **expose** MCP tools *to* interactive Claude Code sessions (`server/mcp/server.go`, `tools_backlog.go`, etc.) — it is a server-side tool-hosting library, not a client-side "call an LLM with tools" loop. It has no role in driving a reviewer LLM call.
- All LLM calls in this codebase — review, triage, summarize, PR description, commit message, autonomous-fix orchestration — go through one mechanism: shelling out to the `claude` CLI binary via `session/headless.Pool` (`session/headless/client.go`, `caller.go`, `pool.go`). There is no direct HTTP call to `api.anthropic.com` anywhere in the Go code.
- Conclusion: there is no existing Go library for "LLM call with tool use in an agentic loop" in the sense the Anthropic API SDK provides (Tool Runner, manual `messages.create` loop). The codebase's equivalent primitive is qualitatively different — it delegates the entire agentic loop to the `claude` CLI subprocess rather than implementing one in Go.

**Verdict: N/A — nothing to select between.** No such library is present, and none is needed (see §3/§4).

---

## 2. SaaS/managed API — does this need a bought service?

Confirmed not applicable, as anticipated. This is code-review decision logic embedded in a self-hosted Go server; there is no third-party "code review as a service" or "LLM judge as a service" that fits an in-process, repo-scoped, per-backlog-item verification step running inside this project's own review-gate pipeline. Anthropic's **Managed Agents** (server-hosted agent + sandbox) was considered and rejected for the same reason the prior triage research rejected the Claude Code SDK: it would mean sending the repo/diff to an external Anthropic-hosted container over the network for a decision that today runs as a local subprocess with the repo already on disk. That's strictly more latency, more infra (environments, vaults, sessions), and a new credential surface for zero capability gain — the codebase already has local, disk-resident, git-checked-out access to exactly the file tree the reviewer needs to inspect.

**Verdict: Not recommended — moving on.**

---

## 3. LLM-generated implementation vs. battle-tested pattern — hand-roll a new tool loop, or reuse an existing primitive?

This is the substantive question, and the codebase already answers it.

### The primitive already exists: `claude -p` itself is the bounded agentic loop

`session/headless.Pool` (`session/headless/client.go`, `caller.go`) runs `claude -p` as a subprocess. Two properties matter:

1. **`claude -p` is not a single LLM call — it's Anthropic's own Claude Code harness running non-interactively.** It already has: a turn loop, built-in tools (`Read`, `Grep`, `Glob`, `Bash`, `Edit`, `Write`, `WebFetch`, `WebSearch`), and its own stopping condition (task complete / max turns internal to the CLI). This is functionally the same thing the research task asked about ("a small bounded agentic loop with file-read tools") — Anthropic ships it as a CLI, and this codebase already depends on it for every headless LLM call.
2. **Turn/tool bounding and timeout are already established patterns in this codebase**, just not for *this* specific call:
   - **Timeout**: `pool.CallBlocking(ctx, ...)` / `CallBlockingWithOptions` take a `context.Context`; callers wrap it with `context.WithTimeout` (see `backlog-triage-autonomous`'s reference to `headless.MaxCallTimeout = 1800s`, and the review gate's own `reviewCtx`). No new timeout mechanism is needed — reuse this.
   - **Tool allowlist**: `session/instance.go` already defines `InstanceOptions.AllowedTools` / `PermissionMode`, and `session/instance_tmux.go:buildClaudeCommand` (lines ~100-133) already constructs a `claude` invocation combining `-p --output-format json` **with** `--allowedTools` and `--permission-mode` for `OneShot` sessions:
     ```go
     if i.AllowedTools != "" {
         parts = append(parts, "--allowedTools", shellQuote(i.AllowedTools))
     }
     if i.PermissionMode != "" {
         parts = append(parts, "--permission-mode", shellQuote(i.PermissionMode))
     }
     ...
     if i.OneShot {
         parts = append(parts, "-p", "--output-format", "json")
     }
     ```
     This proves `-p --output-format json` + `--allowedTools` + `--permission-mode` is an already-used, working combination in this exact codebase — just constructed for tmux-launched one-shot sessions, not for `headless.Pool`'s subprocess path.
   - **Permission mode constants already exist**: `session/instance.go:420-423` defines `PermissionModeBypassPermissions`, `PermissionModeAcceptEdits`, `PermissionModeManual`, `PermissionModeAuto`. A read-only reviewer call needs none of the write-risk modes — `--allowedTools "Read,Grep,Glob"` alone is sufficient to scope the tool surface, with `--permission-mode bypassPermissions` (or simply relying on Read/Grep/Glob being non-mutating and not requiring approval by default) so the subprocess never blocks waiting for interactive approval it can't receive.

### Why the current review-gate code says "no tool access" — and why that's a design choice, not a hard limit

`session/backlog_review.go:135-137`:
```go
// Unlike BuildReviewPrompt, it asks for JSON output instead of tool invocation
// because headless claude -p subprocesses do not have tool access.
func BuildHeadlessReviewPrompt(...) string {
```
and `session/headless/caller.go:172-173`, the first-call args built by `acquireSession`:
```go
args = []string{"-p", "--output-format", "json", "--system-prompt", systemPrompt, "--exclude-dynamic-system-prompt-sections"}
```
`ProcessRunner.Run` (`session/headless/client.go`) never adds `--allowedTools` or `--permission-mode` — those fields don't exist on `headless.CallOptions` at all today. So the comment is currently *true of this code path* — but it's true because nobody wired the flags in, not because `-p --output-format json` is structurally incompatible with tool use. `instance_tmux.go`'s own `OneShot` construction (same flags, same binary, same `-p --output-format json` pairing) is the existing counter-example inside this repo.

**This matters for the "empty diff" case specifically**: `--output-format json` on the *final* summary doesn't preclude Claude from calling `Read`/`Grep`/`Glob` mid-session before producing that JSON — those are ordinary tool calls during the turn; the JSON constraint only governs the final text. The prior research's "no MCP tool access" finding (`backlog-triage-autonomous/research/architecture.md` & `build-vs-buy.md`, Option 2b) is about a *different* concern — `submit_triage_result` is a **custom MCP tool** requiring `--mcp-config` / network reachability to the app's own MCP server, which genuinely isn't wired into headless calls. Built-in file tools (`Read`, `Grep`, `Glob`) require no MCP server at all — they're native to the `claude` binary.

### What "battle-tested" reuse looks like here

Reuse over reinvention, concretely:

| Need | Reuse | New code |
|---|---|---|
| Bounded tool-call loop | `claude -p`'s own harness (already the execution engine for every headless call) | none |
| Timeout | `context.WithTimeout` around `pool.CallBlockingWithOptions` (same pattern as review gate today) | none |
| Tool allowlist | `--allowedTools` flag, already proven in `instance_tmux.go` | plumb `AllowedTools` field through `headless.CallOptions` → `ProcessRunner.Run` args (new field + ~10 lines) |
| Non-interactive permission | `--permission-mode` flag + existing `PermissionMode*` constants in `session/instance.go` | plumb `PermissionMode` field the same way |
| Directory scoping | `CallOptions.WorkDir` (already exists, already used by triage: `pool.CallWithOptions(ctx, key, systemPrompt, userPrompt, headless.CallOptions{WorkDir: item.RepoPath})`) | none — set it on the review call |
| Prompt instructing "check the codebase" | New system-prompt text for the empty-diff path | new prompt copy only |

The only genuinely new code is: (a) two new fields on `CallOptions`/`ProcessRunner`, (b) the arg-construction lines in `session/headless/client.go` mirroring `buildClaudeCommand`, and (c) prompt/parsing changes for the empty-diff branch. Everything else — the loop, the bounding, the timeout, the directory scoping — already exists and is exercised by other features today (review gate, triage).

**Verdict: Reuse — extend `headless.Pool`'s existing subprocess primitive. Do not hand-roll a new agentic loop, and do not reach for the Anthropic API / Tool Runner (which would require adding `anthropic-sdk-go`, a new auth/credential path, and would duplicate what `claude -p` already does for every other headless feature in this repo).**

---

## 4. Fork or adapt — is there an existing "give an LLM read-only tool access to a directory" pattern to adapt?

Two candidate sources were checked:

1. **`server/mcp/` (interactive Claude Code sessions' tool exposure)**: `server/mcp/tools_backlog.go`, `tools_discovery.go`, `tools_vcs.go`, `tools_goal.go`, `tools_terminal.go`, `tools_github.go` — these are all **domain-specific** tools (`report_progress`, `request_review`, `submit_review_verdict`, git/vcs helpers, terminal control). None of them define a file-read/grep tool, because interactive Claude Code sessions already get `Read`/`Grep`/`Glob` for free as **built-in** tools — the MCP layer only adds things Claude Code doesn't already have (backlog domain actions, session control). **There is nothing to fork here** — the "give an LLM read-only file access" capability isn't implemented as an MCP tool anywhere in this repo; it's implemented by the `claude` binary itself and merely needs to be *unlocked* for the headless subprocess path via CLI flags (see §3).
2. **`session/instance_tmux.go` (how interactive/oneShot sessions get scoped tool access)**: this **is** the pattern to adapt — `AllowedTools`/`PermissionMode` on `InstanceOptions`, translated to `--allowedTools`/`--permission-mode` CLI flags in `buildClaudeCommand`. The adaptation is: replicate the same two fields and the same flag-construction logic on `headless.CallOptions`/`ProcessRunner`, scoped to a read-only tool set (`Read,Grep,Glob`, explicitly excluding `Bash`, `Edit`, `Write` since the reviewer should never mutate the worktree it's grading) with a permission mode that doesn't block on interactive approval.

**Verdict: Adapt — port `AllowedTools`/`PermissionMode` from `session/instance.go` + `instance_tmux.go`'s tmux-launch path into `session/headless`'s subprocess-launch path.** This is the same flag pair, same binary, same semantics — just a second call site that has never needed it before now.

---

## Net recommendation for the plan phase

1. Add `AllowedTools string` and `PermissionMode string` to `headless.CallOptions` (mirroring `session.InstanceOptions`).
2. In `session/headless/client.go` (`ProcessRunner.Run`) or `caller.go` (`acquireSession`'s arg-building), append `--allowedTools <value>` / `--permission-mode <value>` when set — same pattern as `instance_tmux.go:buildClaudeCommand` lines ~117-124.
3. For the empty/near-empty-diff review path: call `pool.CallBlockingWithOptions(ctx, headless.FeatureKeyReview, newSystemPrompt, prompt, headless.CallOptions{WorkDir: item.RepoPath, AllowedTools: "Read,Grep,Glob", PermissionMode: session.PermissionModeBypassPermissions})` (or a mode that permits Read/Grep/Glob without hanging on approval — needs a one-time manual check of current `claude` CLI default behavior for these three tools, since they may already not require approval and the flag may be belt-and-suspenders).
4. Update `BuildHeadlessReviewPrompt` (or a new `BuildHeadlessReviewPromptWithFileAccess`) to drop the "do NOT call tools" instruction for the empty-diff branch and instead instruct the model to use `Read`/`Grep`/`Glob` to check whether each acceptance criterion is already satisfied in the current tree, while still requiring final JSON verdict output.
5. Keep the existing non-empty-diff path untouched (per requirements' out-of-scope note) — this only changes behavior when the diff is empty/near-empty.
6. No new Go dependency, no new subprocess model, no Anthropic API client. The `session/headless` package's public surface grows by two struct fields and their plumbing.

This keeps the feature inside the existing "headless subprocess + `claude -p`" architecture that every other AI feature in this repo already uses (review, triage, summarize, PR description, commit message, autonomous-fix), rather than introducing a second, parallel way of calling an LLM.
