# Implementation Plan: Persistent Operator Memory

Source: `project_plans/operator-memory/requirements.md` +
`project_plans/operator-memory/research/{stack,features,architecture,pitfalls,ux,build-vs-buy}.md`.

This item ships the **storage + injection layer only** (read-side): two
file-backed memory stores loaded fresh on every headless triage/review call
and appended as a system-prompt tail block, plus a read-only
`stapler-squad memory show` CLI command. No automated writer ships here —
`OPERATOR.md`/`REPO.md` are hand-edited until the companion background-writer
item lands (requirements.md, "Scope for this item").

No Migration Plan section — this feature has no database schema; it is two
flat Markdown files under `~/.stapler-squad/`.

---

## Step 0.5 — Creative pass: how memory reaches the system prompt string

Three shapes were considered for wiring an assembled memory snapshot into the
existing stable system-prompt constants in `session/headless/features.go`:

**(a) Mutate the exported System-Prompt functions to accept a snapshot
parameter and concatenate it as a tail block.**
`HeadlessTriageSystemPrompt(memorySnapshot string) string`,
`HeadlessReviewSystemPrompt(memorySnapshot string) string`,
`HeadlessReviewSystemPromptWithCodebaseAccess(memorySnapshot string) string`.
- *Strength:* the signature change is compile-enforced — every one of the 3
  production call sites (and their unit tests) fails to build until updated,
  so it is structurally impossible to add a new headless call site that
  forgets memory assembly. It also keeps the "single point of decision"
  property `BuildReviewCallOptions` already documents for itself.
- *Weakness:* touches 3 function signatures + all their call sites and tests
  in one change, even though only the plain-diff and codebase-read review
  paths plus triage need it (mechanical, not risky, but a wider diff).

**(b) Load memory in the prompt-builder layer (`BuildHeadlessTriagePrompt`,
`BuildHeadlessReviewPrompt` in `session/backlog_triage.go` /
`session/backlog_review.go`) and thread it as a new argument down to
`Pool.CallBlocking`'s `systemPrompt` argument, without changing the
`HeadlessXSystemPrompt()` functions themselves.**
- *Strength:* keeps `features.go`'s exported functions untouched, closer to
  "don't touch the stable const property" read literally.
- *Weakness:* false economy — `BuildHeadlessTriagePrompt`/
  `BuildHeadlessReviewPrompt` build the **user** prompt (item/diff payload),
  not the **system** prompt; the system prompt for triage and for the
  plain-diff/codebase-read review paths is selected directly at the
  `CallBlocking`/`BuildReviewCallOptions` call sites, one level down from
  these builders (confirmed by reading `backlog_service_triage.go:2330-2348`
  and `backlog_review.go:405-424` — the builder functions and the
  system-prompt selection are already separate call sites for review, not
  connected the way this option assumes). Bolting memory onto the wrong
  layer would require re-deriving the same repo-path context a second time
  at the builder call site for no benefit.

**(c) A package-level loader that `TriggerTriage`/`TriggerReReview` call
directly, splicing memory into the raw prompt string at the call site,
leaving `features.go` untouched entirely.**
- *Strength:* smallest possible diff to `session/headless`.
- *Weakness:* no compiler enforcement — a future third headless call site (or
  a refactor of an existing one) can silently omit the splice, and two
  independent call sites (triage, review) would each hand-assemble the same
  "stable prompt + tail" concatenation instead of it living in one place.
  This is the failure mode requirement 3's "new suffix, not an edit" language
  is trying to make impossible to get wrong.

**Chosen: (a).** Recorded as the Pattern Decision below ("System-prompt
function signature") with (b) and (c) as the rejected alternatives. Loading
itself happens at the call site (the seam identified in (a)'s weakness is
accepted, not avoided — see Pattern Decision "Snapshot loading site").

---

## Domain Glossary

| Term | Definition |
|---|---|
| **`session/opmemory`** | New Go package housing all memory-store logic: path resolution, capped file reads, snapshot assembly, and the write-time injection scanner. Deliberately not named `session/memory` — that path is already an unrelated package (RAM measurement for the hibernation sweeper; `session/memory/reader.go`). |
| **Operator memory** | Fleet-level knowledge file, `OPERATOR.md`, resolved at `filepath.Join(config.GetConfigDir(), "memory", "OPERATOR.md")`. One file, not per-workspace. |
| **Repo memory** | Workspace-scoped knowledge file, `REPO.md`, resolved at `filepath.Join(cfgDir, "memory", "REPO.md")` where `cfgDir, _ = config.GetConfigDirForDir(repoPath)`. |
| **Memory snapshot** | The single assembled string produced by `opmemory.LoadSnapshot(repoPath)` — the delimited, byte-capped concatenation of operator memory + repo memory, or `""` when both are empty/missing/whitespace-only. This is the "frozen snapshot" from the requirements: read fresh at the top of each headless call, never cached across calls. |
| **`opmemory.LoadSnapshot(repoPath string) string`** | Top-level loader. Never returns an error and never blocks a caller — any failure (missing file, permission error, `config.GetConfigDirForDir` erroring on `os.UserHomeDir()` failure, oversized content) degrades to omitting that file's content, never to failing the call. |
| **`opmemory.OperatorMemoryPath() (string, error)`** | Resolves `OPERATOR.md`'s absolute path via `config.GetConfigDir()`. |
| **`opmemory.RepoMemoryPath(repoPath string) (string, error)`** | Resolves `REPO.md`'s absolute path via `config.GetConfigDirForDir(repoPath)`. |
| **`opmemory.readCappedFile(path string, maxBytes int) (content string, existed bool)`** (unexported) | Reads one file, trims whitespace, applies the byte cap with a `[truncated]` marker + a single warning log line on truncation. Any `os.ReadFile` error (including `os.ErrNotExist`) returns `("", false)` with no error propagated — `os.ErrNotExist` is not logged (expected steady state before any file is ever created); other read errors (permission denied, etc.) are logged once at `log.WarningLog` level. |
| **`maxOperatorMemoryBytes`, `maxRepoMemoryBytes`** | Byte-cap constants (20,000 bytes each — same order of magnitude as `MaxDiffSizeReview`/`maxDiffSizePR` in `session/headless/features.go`), defined in `session/opmemory/opmemory.go`. |
| **`operatorMemoryTag`, `repoMemoryTag`** | The XML-style delimiter tag names (`operator_memory`, `repo_memory`) each file's content is wrapped in before being appended to the prompt, mirroring `server/services/rule_prompt_builder.go:169`'s `<command>...</command>` precedent. |
| **`opmemory.ScanForPromptInjection(content string) error`** | Deterministic denylist/heuristic scan over operator-authored content. Returns a non-nil error naming the matched pattern when content should be rejected before a write. Exported for the companion auto-writer item and for this item's own required "write-scan rejection" unit test — **not wired into any write call site in this item**, since this item ships no write command (see Unresolved Questions). |
| **`MemoryCmd`, `memoryShowCmd`** | New Cobra commands in `cmd/commands/memory.go`: `MemoryCmd` is the `memory` parent command; `memoryShowCmd` is its `show` subcommand, wired via `MemoryCmd.AddCommand(memoryShowCmd)` and registered in `main.go` via `rootCmd.AddCommand(commands.MemoryCmd)`. |
| **Frozen snapshot** | The requirement's own term (from the Hermes `memory_tool.py` reference design) for "loaded once, doesn't reflect concurrent mid-call writes." In this codebase's one-shot-subprocess-per-call architecture (triage, codebase-read review) this collapses to "read fresh at the top of the function, right before the one `CallBlocking` call" — there is no longer-lived session object to freeze the snapshot *against*. See ADR-001 for the one path (plain-diff review) where a longer-lived claude-CLI session does exist and the snapshot's freshness guarantee is weaker. |

---

## Pattern Decisions

| Decision | Chosen approach | Alternative rejected | Reason |
|---|---|---|---|
| Package location | New `session/opmemory` package | `config/memory` (co-located with `GetConfigDirForDir`) | `config` is a generic, foundational leaf package (log-level, JSON config persistence); adding prompt-injection-scan and system-prompt-assembly-adjacent logic there breaks its single responsibility and would make `config` depend conceptually on prompt-safety concerns it has no business knowing about. |
| Package location | New `session/opmemory` package | `session/headless/operatormemory` | `session/headless` is scoped to LLM feature-call plumbing (`Pool`, `CallOptions`, session reuse) — file storage and CLI-facing path resolution is a different concern, and the CLI (`cmd/commands/memory.go`) needs to read the files without depending on `Pool` machinery at all. |
| Abstraction shape | Concrete functions (`LoadSnapshot`, `OperatorMemoryPath`, `RepoMemoryPath`, `ScanForPromptInjection`) — no interface | A `MemoryStore` interface with a filesystem implementation | Exactly one implementation exists and none is imminent (build-vs-buy.md ruled out both an OSS memory/RAG library and a SaaS memory API for this scope) — a single-implementation interface is exactly the "speculative interface" smell `.claude/rules/interface-pollution-checklist.md` warns against. Two flat-file reads plus one denylist scan is a `func`, not a `Strategy`. |
| System-prompt function signature | `HeadlessTriageSystemPrompt(memorySnapshot string) string` and same shape for the two headless review functions — memory concatenated as a tail block at the function boundary | (b) load memory in the prompt-builder layer and thread a new arg to `Pool.CallBlocking` | See Step 0.5 — the builder functions (`BuildHeadlessTriagePrompt` etc.) construct the *user* prompt, not the *system* prompt; bolting memory on there requires re-deriving repo-path context a second time one layer removed from where the system prompt is actually selected. |
| System-prompt function signature | (as above) | (c) a package-level loader called directly by `TriggerTriage`/`TriggerReReview`, splicing memory into the raw prompt string, `features.go` untouched | No compiler enforcement — nothing stops a future or refactored call site from omitting the splice, and it duplicates the "stable prompt + tail" concatenation logic at two independent call sites instead of centralizing it in one function each caller must go through. |
| Interpolation shape | Append `stablePromptConst + "\n\n" + memorySnapshot` (or return the const unchanged when `memorySnapshot == ""`) — never edit the const string itself | Interpolate memory into the middle of the const via `fmt.Sprintf` with a `%s` hole | `features.go`/`pool.go` both document prefix-caching as load-bearing ("Stable prompts enable prefix-caching across repeated calls," `features.go:69`). Interpolating into the middle would invalidate the cached prefix on every memory content change; strict tail-append preserves it (pitfalls.md #2) — a unit test asserts the stable-prefix bytes are unchanged regardless of memory content. |
| Snapshot loading site | Fresh `os.ReadFile`-based read at the call site, immediately before each `CallBlocking`/`BuildReviewCallOptions` invocation; no caching | Load `OPERATOR.md` once at `Pool`/`BacklogService` construction (fleet-level, so structurally cacheable) | Reintroduces the exact staleness problem the frozen-snapshot design exists to avoid: a hand-edit to `OPERATOR.md` mid-process-uptime would never be picked up without a restart, since this item ships no writer/reload signal. A file read is a few KB against a call that already costs a `claude -p` subprocess spin-up (hundreds of ms–seconds) — negligible overhead, and it avoids inventing cache-invalidation machinery this item has no use for. |
| Snapshot loading site | (as above) | An in-process cache (`sync.Map` or similar) with TTL-based invalidation | No concurrent-writer exists yet in this item's scope (read-only), so there is nothing to invalidate against except operator hand-edits — the same staleness problem as the previous row, just deferred behind a TTL instead of solved. If a future writer needs a cache, follow the `sync.Map`/no-lock-across-I/O precedent from `docs/bugs/fixed/BUG-020`–`024`, not `map + mutex` — noted for that item, not built here. |
| Injection scan | Deterministic phrase/pattern denylist (`ScanForPromptInjection`) | A second LLM call to classify content as safe/unsafe | Non-deterministic — directly conflicts with the required "write-scan rejection" unit test wanting a stable, non-flaky assertion; adds latency/cost/a new failure mode to a synchronous local file write; a layering smell (using an LLM to guard content that will itself be fed to an LLM doesn't remove the trust boundary). |
| Injection scan | (as above) | A dedicated Go prompt-injection-detection library | No mature, widely-used Go library exists for this (the space is Python-tooling-dominated: `rebuff`, `llm-guard`, no first-class Go port) — pulling in an immature dependency for one denylist check is the same over-engineering problem as an OSS memory framework, just smaller. |
| Read-side defense-in-depth | **Not implemented** — only the write-time injection-pattern scanner is built; no secret scan on the read path | Reuse `server/services.ScanForSecrets` on the read path before injecting memory into a prompt (pitfalls.md #1's second recommendation) | Import-cycle constraint discovered during planning, not present in the research: `server/services` already imports `session` (`backlog_github_forward_sync.go`, `approval_handler.go`, etc.), and `session/opmemory` must be importable *from* `session` (`session/backlog_review.go`'s `BuildReviewCallOptions`) — so `session/opmemory` cannot import back into `server/services` where `ScanForSecrets` lives. Duplicating `ScanForSecrets`'s logic into `opmemory` was considered and rejected as scope creep beyond req 6's literal ask (an *injection* scan, not a *secret* scan); flagged as a candidate follow-up if the companion writer item wants secret-scanning too — likely requires relocating `ScanForSecrets` to a lower-level shared package first. |
| Untrusted-content delimiting | Wrap each file's content in `<operator_memory>...</operator_memory>` / `<repo_memory>...</repo_memory>` tags | `SanitizeDiff`-style backtick-fence escaping (`session/backlog_review.go:614`) | `SanitizeDiff` solves a narrower problem (preserve a diff's exact bytes while preventing markdown-fence breakout); memory content isn't fenced in a code block in this design, so escaping backticks is the wrong tool. The closer precedent is `rule_prompt_builder.go:169`'s XML-delimiter wrap around untrusted command text, "to prevent prompt injection from command content" — same shape, reused here. |
| Byte cap | Two named constants, `maxOperatorMemoryBytes` and `maxRepoMemoryBytes` (20,000 bytes each), in `session/opmemory/opmemory.go`, following `features.go`'s `MaxDiffSizeReview`/`maxDiffSizePR`/`maxDiffSizeCommit` convention (three separately-named consts of the same order of magnitude, not one shared constant) | A single shared `maxMemoryFileBytes` constant applied to both files | Matches the existing repo convention exactly (per-purpose named consts, even when currently equal in value) and leaves room to tune the two independently later without a signature change, at zero extra cost now. |
| CLI data path | `memory show` reads both files directly, in-process, via `opmemory.OperatorMemoryPath()`/`RepoMemoryPath(cwd)` — no RPC call | RPC round-trip through the running server (the `cmd/commands/get_session.go` pattern) | `memory show` is local file state, not server state — it must work even when the server isn't running, and reading two small files directly is strictly simpler than adding a new ConnectRPC endpoint, a proto message, and `make proto-gen` for a command with zero server-side logic. |

---

## Observability Plan

- **Truncation.** `readCappedFile` logs one `log.WarningLog.Printf("[opmemory] %s exceeded %d byte cap, truncated", path, maxBytes)` line each time a file is truncated on read — surfaces growth before it becomes a BUG-017/BUG-018-style surprise (pitfalls.md #4), without duplicating file content into logs.
- **Read failures.** Any `os.ReadFile` error other than `os.ErrNotExist` (permission denied, etc.) logs one `log.WarningLog.Printf("[opmemory] failed to read %s: %v — treating as empty", path, err)` line and degrades to empty content. `os.ErrNotExist` is not logged — it is the expected steady state before either file is ever created.
- **Path resolution failures.** If `config.GetConfigDir()`/`GetConfigDirForDir()` itself errors (e.g. `os.UserHomeDir()` failure), `LoadSnapshot` logs one `log.WarningLog.Printf("[opmemory] failed to resolve memory path for repo=%s: %v", repoPath, err)` line and degrades to empty content — never propagates the error to the triage/review call.
- **No per-call "memory loaded" log line.** `LoadSnapshot` runs on every headless triage/review call; logging on the happy path (file found, under cap) would add log volume proportional to call volume for no diagnostic value. Only the exception paths above are logged.
- **Not built in this item (flagged as a follow-up, not an AC):** a debuggability breadcrumb correlating a specific past triage/review call's log line with the exact byte counts of the snapshot it saw (ux.md §5's suggestion, e.g. `memory: 412 bytes from OPERATOR.md, 0 bytes from REPO.md` emitted into the same log line that records prompt construction). Useful for reconstructing "what did call X actually see," but touches prompt-assembly logging call sites this item doesn't otherwise need to change — out of scope, noted below in Unresolved Questions.

---

## Risk Control

| Risk | Mitigation | Where enforced |
|---|---|---|
| A memory read failure blocks or fails a triage/review call | `LoadSnapshot` never returns an error; every internal failure mode collapses to `""` | `opmemory.LoadSnapshot`, unit-tested with a permission-denied fixture and a `STAPLER_SQUAD_TEST_DIR` pointed at a nonexistent parent |
| Memory content silently invalidates the prefix-cache-optimized stable system prompt | Tail-append only, never `Sprintf`-interpolated into the const body | `session/headless/features_test.go`: assert stable-prefix bytes identical across different `memorySnapshot` inputs |
| Unbounded memory-file growth inflates every headless call's token cost indefinitely | Byte cap (20,000/file) with truncation marker + warning log | `session/opmemory/opmemory_test.go`: oversized-file fixture asserts truncation + marker |
| A future headless call site is added without memory assembly | Function signature change is compile-enforced (Step 0.5, option (a)) | Go compiler — no runtime test needed for this one |
| `REPO.md`/`OPERATOR.md` collapse to the same directory in default (non-workspace) config mode, silently sharing state across repos | Accepted, documented consequence of reusing `config.GetConfigDirForDir`'s existing convention (architecture.md Q4) — not treated as a bug to fix in this item | Called out explicitly here and in `memory show`'s output (always prints the resolved absolute path, per ux.md §2) |
| Plain-diff review path's claude-CLI session reuse silently drops the system prompt (and therefore memory) on resumed calls | Accepted as a documented, pre-existing limitation — not introduced or worsened by this item | ADR-001 |
| Operator pastes a credential into `OPERATOR.md`/`REPO.md`, which then flows into every future prompt | **Not mitigated in this item** (see Pattern Decisions — import-cycle constraint) | Flagged as a follow-up for the companion writer item |
| Injection-pattern denylist misses novel phrasing | Documented as an accepted limitation of a heuristic approach (build-vs-buy.md Option 3) — not a gap unique to this item's implementation | N/A — inherent to the chosen approach, revisit only if it proves insufficient in practice |

---

## Unresolved Questions

1. **`memory edit`/`memory add` discrepancy (ux.md's flagged open question).** The originating issue's "Proposed Work" describes a CLI to "view/edit" the stores; requirements.md's functional requirements and ACs list only `stapler-squad memory show` (view). **Resolution for this plan: (a)** — no write CLI command ships in this item. `OPERATOR.md`/`REPO.md` remain hand-edited with a text editor until the companion background-reviewer writer item lands (requirements.md's own "Scope for this item" section states this explicitly). `opmemory.ScanForPromptInjection` is still built and unit-tested per AC's "write-scan rejection" requirement, but it has no call site in this item — it exists ready for the companion writer to call.
2. **`REPO.md` isolation granularity.** In the default (non-workspace-mode) configuration, `REPO.md` is not isolated per literal filesystem repo path — it is isolated per config-dir *instance* (opt-in `STAPLER_SQUAD_WORKSPACE_MODE=true`, `STAPLER_SQUAD_INSTANCE`, or `STAPLER_SQUAD_TEST_DIR`), the same granularity every other piece of "workspace" state in this codebase already gets (architecture.md Q4). A multi-repo backlog running with default config will share one `REPO.md` across all repos unless the operator opts into per-workspace isolation. This is accepted as a known, existing-convention consequence, not a new gap this item introduces or must fix.
3. **Plain-diff review path memory freshness.** See ADR-001 — accepted as a documented limitation (Option A), not fixed in this item. A follow-up item ("key headless `Pool` sessions by repo, or rotate on system-prompt-hash change") is the correct place to close it.
4. **Snapshot audit trail / debuggability breadcrumb.** No mechanism persists which memory snapshot (content hash, byte length) a specific historical triage/review call actually saw — `memory show` only reflects current file state (features.md §7, ux.md §5). Flagged as a follow-up; would touch prompt-assembly logging call sites this item doesn't otherwise need to change.
5. **Web UI visibility.** No requirement asks for a web-app panel mirroring `memory show`; CLI-only is explicitly this item's scope (features.md §7). Flagged as a plausible follow-up, not assumed wanted.
6. **Read-side secret scanning.** Pitfalls.md's suggestion to reuse `ScanForSecrets` as read-side defense-in-depth against an operator accidentally pasting a credential into `OPERATOR.md` is not implemented in this item due to the import-cycle constraint discovered during planning (see Pattern Decisions). Flagged as a follow-up for the companion writer item, which may need to relocate `ScanForSecrets` to a shared leaf package first.

---

## Dependency Visualization

```
config.GetConfigDir() / GetConfigDirForDir(repoPath)   (existing, config/config.go)
                    │
                    ▼
        session/opmemory  (NEW package)
        ├─ OperatorMemoryPath() / RepoMemoryPath(repoPath)
        ├─ readCappedFile(path, maxBytes)      — unexported
        ├─ LoadSnapshot(repoPath) string        — never errors, never blocks caller
        └─ ScanForPromptInjection(content) error — exported, no call site in this item
                    │
        ┌───────────┼──────────────────────────────┐
        ▼                                           ▼
session/headless/features.go                cmd/commands/memory.go (NEW)
  HeadlessTriageSystemPrompt(snapshot)         MemoryCmd → memoryShowCmd
  HeadlessReviewSystemPrompt(snapshot)              │
  HeadlessReviewSystemPromptWithCodebaseAccess       ▼
  (snapshot)                                  main.go: rootCmd.AddCommand(commands.MemoryCmd)
        │
        ├── called from server/services/backlog_service_triage.go:2332 (TriggerTriage)
        └── called from session/backlog_review.go:405 (BuildReviewCallOptions),
            itself called from server/services/backlog_service_triage.go:2698 (TriggerReReview)
```

No new third-party dependency. No proto/RPC changes. No `session/ent` schema
changes.

---

## Phase / Epic / Story / Task Breakdown

### Epic 1 — `session/opmemory`: path resolution and capped file reads

#### Story 1.1 — Package skeleton and path resolution

- **Task 1.1.1** (3 min): Create `session/opmemory/opmemory.go` with package doc comment explaining the fleet/repo two-tier model and explicitly noting the `session/memory` naming collision this package avoids. Add `OperatorMemoryPath() (string, error)` (`filepath.Join(config.GetConfigDir(), "memory", "OPERATOR.md")`) and `RepoMemoryPath(repoPath string) (string, error)` (`filepath.Join(cfgDir, "memory", "REPO.md")` via `config.GetConfigDirForDir(repoPath)`).
- **Task 1.1.2** (2 min): Add `maxOperatorMemoryBytes = 20_000` and `maxRepoMemoryBytes = 20_000` constants to the same file, each with a one-line comment cross-referencing `session/headless/features.go`'s `MaxDiffSizeReview` convention.
- **Task 1.1.3** (4 min): Unit test `TestOperatorMemoryPath_should_ResolveUnderConfigDir` and `TestRepoMemoryPath_should_ResolveUnderConfigDirForDir_When_GivenARepoPath` in `session/opmemory/opmemory_test.go`, using `STAPLER_SQUAD_TEST_DIR=t.TempDir()` (the standard pattern from `config/config_test.go:877-888`) to assert the resolved path is `<tempdir>/memory/OPERATOR.md` / `<tempdir>/memory/REPO.md`.

#### Story 1.2 — Capped, degrade-on-failure file reads

- **Task 1.2.1** (5 min): Implement `readCappedFile(path string, maxBytes int, label string) string` (unexported) in `session/opmemory/opmemory.go`: `os.ReadFile`, `os.ErrNotExist` → `""` no log; other error → `""` + `log.WarningLog.Printf("[opmemory] failed to read %s: %v — treating as empty", path, err)`; on success, `strings.TrimSpace`; if resulting length `> maxBytes`, truncate to `maxBytes` bytes, append `"\n[truncated]"`, and log `log.WarningLog.Printf("[opmemory] %s exceeded %d byte cap, truncated", path, maxBytes)`.
- **Task 1.2.2** (4 min): Unit test `TestReadCappedFile_should_ReturnEmpty_When_FileMissing` — no file at path, assert `""` returned and no panic.
- **Task 1.2.3** (3 min): Unit test `TestReadCappedFile_should_ReturnEmpty_When_FileWhitespaceOnly` — write `"   \n\t\n"` to a temp file, assert `""` returned (AC5's trim-before-check requirement).
- **Task 1.2.4** (4 min): Unit test `TestReadCappedFile_should_TruncateWithMarker_When_ContentExceedsCap` — write a 25,000-byte file with `maxBytes=20_000`, assert the returned string is exactly 20,000 bytes of content followed by `"\n[truncated]"`.
- **Task 1.2.5** (3 min): Unit test `TestReadCappedFile_should_ReturnEmpty_When_PermissionDenied` — `os.Chmod(path, 0000)` on a temp file (skip on platforms where this doesn't apply / running as root), assert `""` returned, not a panic or propagated error.

### Epic 2 — Snapshot assembly and the injection-scan function

#### Story 2.1 — `LoadSnapshot`

- **Task 2.1.1** (5 min): Implement `LoadSnapshot(repoPath string) string` in `session/opmemory/opmemory.go`: resolve both paths (degrade to `""` content on path-resolution error, logged per the Observability Plan), read both via `readCappedFile`, and if both are empty return `""`. Otherwise build:
  ```
  ## Operator Memory

  <operator_memory>
  {operator content, only if non-empty}
  </operator_memory>

  <repo_memory>
  {repo content, only if non-empty}
  </repo_memory>
  ```
  omitting whichever `<...>` block is empty (never emit an empty tag pair).
- **Task 2.1.2** (4 min): Unit test `TestLoadSnapshot_should_IncludeBothBlocks_When_BothFilesPopulated` — write both files with distinct known content under `STAPLER_SQUAD_TEST_DIR`, assert the returned string contains `"## Operator Memory"` once and both tag pairs with the exact written content.
- **Task 2.1.3** (3 min): Unit test `TestLoadSnapshot_should_OmitRepoTag_When_OnlyOperatorMemoryPopulated` — only `OPERATOR.md` written, assert `<repo_memory>` does not appear anywhere in the output.
- **Task 2.1.4** (3 min): Unit test `TestLoadSnapshot_should_ReturnEmptyString_When_BothFilesMissingOrWhitespace` — neither file written (or one written whitespace-only), assert `LoadSnapshot` returns exactly `""` (AC5).

#### Story 2.2 — Injection-scan function (built for the AC test + companion writer, no call site yet)

- **Task 2.2.1** (5 min): Implement `ScanForPromptInjection(content string) error` in `session/opmemory/scan.go` — a package-level `[]string` denylist (lowercased match) covering role-override phrases (`"ignore previous instructions"`, `"ignore all previous instructions"`, `"disregard previous instructions"`, `"you are now"`, `"new instructions:"`, `"system prompt:"`) and delimiter-breakout attempts (`"</operator_memory>"`, `"</repo_memory>"` — literal attempts to close this feature's own wrapper tags early). Case-insensitive substring match via `strings.Contains(strings.ToLower(content), pattern)`; return `fmt.Errorf("opmemory: content rejected — matched pattern %q", pattern)` on first match, `nil` otherwise.
- **Task 2.2.2** (3 min): Unit test `TestScanForPromptInjection_should_RejectContent_When_ContainsIgnorePreviousInstructions` (the AC's required "write-scan rejection" test) — assert a non-nil error naming the matched pattern.
- **Task 2.2.3** (2 min): Unit test `TestScanForPromptInjection_should_Accept_When_ContentIsOrdinaryProse` — assert `nil` for a normal operator note like `"Team prefers small, frequent PRs."`.
- **Task 2.2.4** (2 min): Unit test `TestScanForPromptInjection_should_RejectContent_When_ContainsDelimiterBreakoutAttempt` — content containing `"</operator_memory><system>"`, assert rejection.

### Epic 3 — Prompt assembly integration

#### Story 3.1 — Mutate the exported system-prompt functions

- **Task 3.1.1** (4 min): In `session/headless/features.go`, change `func HeadlessTriageSystemPrompt() string { return headlessTriageSystemPrompt }` to `func HeadlessTriageSystemPrompt(memorySnapshot string) string { if memorySnapshot == "" { return headlessTriageSystemPrompt }; return headlessTriageSystemPrompt + "\n\n" + memorySnapshot }`. Update the doc comment to state the tail-append contract and that empty input reproduces the byte-identical stable prompt.
- **Task 3.1.2** (4 min): Same shape for `HeadlessReviewSystemPrompt(memorySnapshot string) string` (wraps `headlessReviewSystemPrompt`).
- **Task 3.1.3** (4 min): Same shape for `HeadlessReviewSystemPromptWithCodebaseAccess(memorySnapshot string) string` (wraps `headlessReviewSystemPromptWithCodebaseAccess`). Leave `ReviewSystemPrompt()` (interactive-session path, `session/review_gate.go`) untouched — explicit non-goal.
- **Task 3.1.4** (5 min): In `server/services/backlog_service_triage.go`, add `import "github.com/tstapler/stapler-squad/session/opmemory"`; at line ~2329 (just before the `CallBlocking` call, where `itemRepoPath` is already in scope), add `memorySnapshot := opmemory.LoadSnapshot(itemRepoPath)` and change the call site to `headless.HeadlessTriageSystemPrompt(memorySnapshot)`.
- **Task 3.1.5** (5 min): In `session/backlog_review.go`, add `import "github.com/tstapler/stapler-squad/session/opmemory"`; change `BuildReviewCallOptions(diff, codebaseWorkDir string)` to `BuildReviewCallOptions(diff, codebaseWorkDir, repoPath string)`; at the top of the function add `memorySnapshot := opmemory.LoadSnapshot(repoPath)`; update both return statements to pass `memorySnapshot` into `HeadlessReviewSystemPromptWithCodebaseAccess(memorySnapshot)` / `HeadlessReviewSystemPrompt(memorySnapshot)`.
- **Task 3.1.6** (3 min): In `server/services/backlog_service_triage.go` line ~2698, update the call to `session.BuildReviewCallOptions(workSessionDiff, codebaseWorkDir, item.RepoPath)`.
- **Task 3.1.7** (2 min): While editing `BuildReviewCallOptions`'s doc comment (Task 3.1.5), correct the stale claim "both `ReviewGateRunner.Run` and `TriggerReReview` must call this" — confirmed by reading `session/review_gate.go:326-346` that `ReviewGateRunner.Run` calls `reviewPromptFor`/`BuildReviewPrompt` (the interactive tool-using path) and never calls `BuildReviewCallOptions` at all; only `TriggerReReview` does. Fix the comment to name the one real call site, found while touching this exact function (collateral debt, cheap to fix in the same diff).
- **Task 3.1.8** (5 min): Update the 4 test files that reference the changed signatures (`session/backlog_review_test.go`, `server/services/backlog_service_test.go`, `session/pipeline_mode_seed_test.go`, `session/headless/features_test.go`) to pass `""` (or a fixture snapshot where the test is specifically about memory) as the new argument — mechanical, compiler-driven.

#### Story 3.2 — Prefix-stability and empty-snapshot regression tests

- **Task 3.2.1** (4 min): In `session/headless/features_test.go`, add `TestHeadlessTriageSystemPrompt_should_ReturnStableConstUnchanged_When_MemorySnapshotEmpty` — assert `HeadlessTriageSystemPrompt("") == headlessTriageSystemPrompt` byte-for-byte (package-internal test, same package as the const).
- **Task 3.2.2** (4 min): Add `TestHeadlessTriageSystemPrompt_should_PreserveStablePrefix_When_MemorySnapshotNonEmpty` — assert `strings.HasPrefix(HeadlessTriageSystemPrompt("some memory"), headlessTriageSystemPrompt)` is true, and that the prefix bytes are identical regardless of what `memorySnapshot` string is passed (loop over 2-3 different snapshot fixtures, assert the first `len(headlessTriageSystemPrompt)` bytes never change) — the pitfalls.md #2 prefix-cache regression guard.
- **Task 3.2.3** (3 min): Same two tests (stable-when-empty, stable-prefix-when-populated) for `HeadlessReviewSystemPrompt` and `HeadlessReviewSystemPromptWithCodebaseAccess`.

### Epic 4 — `stapler-squad memory show` CLI

#### Story 4.1 — Command implementation

- **Task 4.1.1** (5 min): Create `cmd/commands/memory.go`, package `commands`, following `get_session.go`'s file-per-command convention. Define `var MemoryCmd = &cobra.Command{Use: "memory", Short: "Inspect operator/repo memory stores"}`.
- **Task 4.1.2** (5 min): Define `var memoryShowCmd = &cobra.Command{Use: "show", Short: "Display current operator and repo memory contents", RunE: ...}` in the same file. `RunE` resolves `os.Getwd()`, calls `opmemory.OperatorMemoryPath()` and `opmemory.RepoMemoryPath(cwd)`, reads each file directly with `os.ReadFile` (not `readCappedFile` — `memory show` displays the true on-disk content uncapped, since it's a diagnostic view, not a prompt payload; note this distinction in a comment), and prints per the ux.md §2 format: resolved absolute path on the header line, then either the raw content or an explicit `empty (no content injected)` / `not found (no content injected)` label distinguishing missing-file from empty-file. Call `MemoryCmd.AddCommand(memoryShowCmd)` in an `init()` or at package `var` init.
- **Task 4.1.3** (2 min): In `main.go`, add `rootCmd.AddCommand(commands.MemoryCmd)` alongside the existing `AddCommand` calls at lines 711-717.
- **Task 4.1.4** (4 min): Manual verification (not e2e — this is a CLI, not a web-app feature per `.claude/rules/e2e-test-conventions.md`'s web-app scope): `go build -o /tmp/ssq-memory-test .`, then `STAPLER_SQUAD_TEST_DIR=$(mktemp -d) /tmp/ssq-memory-test memory show` with no files present (expect both `not found` lines), then write both files and re-run (expect both contents printed with resolved paths).

### Epic 5 — Documentation

#### Story 5.1 — ADR

- **Task 5.1.1** (done as part of this planning pass): `project_plans/operator-memory/decisions/ADR-001-plain-diff-review-memory-scope.md` — documents the Option A decision from `research/architecture.md`'s "Summary of the key finding" table.

---

## Acceptance Criteria → Given-When-Then

**AC1 — `OPERATOR.md` and `REPO.md` loaded at session start, injected into system prompt as a frozen snapshot.**
> Given `~/.stapler-squad/memory/OPERATOR.md` contains `"Team prefers small, frequent PRs."` and `<cfgDir>/memory/REPO.md` (where `cfgDir, _ = config.GetConfigDirForDir(item.RepoPath)`) contains `"make build before make test — protos aren't generated otherwise."`,
> When `TriggerTriage`'s goroutine calls `headless.HeadlessTriageSystemPrompt(opmemory.LoadSnapshot(itemRepoPath))` immediately before `CallBlocking` (`server/services/backlog_service_triage.go:2330-2348`),
> Then the returned string equals `headlessTriageSystemPrompt + "\n\n## Operator Memory\n\n<operator_memory>\nTeam prefers small, frequent PRs.\n</operator_memory>\n\n<repo_memory>\nmake build before make test — protos aren't generated otherwise.\n</repo_memory>\n"` (exact block shape per Task 2.1.1), with the original constant's bytes appearing unchanged as the prefix.

**AC2 — Injection scan runs before any write to either file.**
> Given this item ships no write CLI command (Unresolved Question 1, resolution (a)),
> When a future writer (out of scope — companion backlog item) calls `opmemory.ScanForPromptInjection(content)` before persisting to `OPERATOR.md`/`REPO.md`,
> Then content containing `"ignore previous instructions"` returns a non-nil error naming the matched pattern, and ordinary operator prose returns `nil` — verified directly by Task 2.2.2's unit test today, with no production call site wiring it in until the companion writer item exists.

**AC3 — `stapler-squad memory show` displays current contents.**
> Given `~/.stapler-squad/memory/OPERATOR.md` contains `"X"` and, from `/home/tstapler/Programming/stapler-squad`, the resolved `REPO.md` (under `config.GetConfigDirForDir("/home/tstapler/Programming/stapler-squad")`) contains `"Y"`,
> When the operator runs `stapler-squad memory show` from that directory,
> Then stdout prints the resolved absolute path of each file followed by its raw content — `Operator memory (~/.stapler-squad/memory/OPERATOR.md):\n\n  X\n\nRepo memory (<resolved path>/memory/REPO.md):\n\n  Y` (exact framing per Task 4.1.2 / ux.md §2).

**AC4 — Empty memory files produce no system prompt noise.**
> Given `OPERATOR.md` does not exist and `REPO.md` exists but contains only `"   \n\t\n"`,
> When `opmemory.LoadSnapshot(repoPath)` is called,
> Then it returns `""`, and `headless.HeadlessTriageSystemPrompt("")` returns exactly `headlessTriageSystemPrompt` with no `"## Operator Memory"` substring anywhere in the output (Task 2.1.4 + Task 3.2.1).

**AC5 — Unit tests: prompt assembly with populated memory, prompt assembly with empty memory, write-scan rejection.**
> Given the test suite in `session/opmemory/opmemory_test.go`, `session/opmemory/scan_test.go`, and `session/headless/features_test.go`,
> When `go test ./session/opmemory/... ./session/headless/...` runs,
> Then `TestLoadSnapshot_should_IncludeBothBlocks_When_BothFilesPopulated` (populated), `TestLoadSnapshot_should_ReturnEmptyString_When_BothFilesMissingOrWhitespace` + `TestHeadlessTriageSystemPrompt_should_ReturnStableConstUnchanged_When_MemorySnapshotEmpty` (empty), and `TestScanForPromptInjection_should_RejectContent_When_ContainsIgnorePreviousInstructions` (write-scan rejection) all pass — satisfying the three named test categories directly, by name.

---

## Summary of touched files (no placeholders)

**New:**
- `session/opmemory/opmemory.go`
- `session/opmemory/opmemory_test.go`
- `session/opmemory/scan.go`
- `session/opmemory/scan_test.go`
- `cmd/commands/memory.go`
- `project_plans/operator-memory/decisions/ADR-001-plain-diff-review-memory-scope.md`

**Modified:**
- `session/headless/features.go` — 3 function signatures
- `session/headless/features_test.go` — call-site updates + 4 new stability tests
- `session/backlog_review.go` — `BuildReviewCallOptions` signature + doc-comment fix
- `session/backlog_review_test.go` — call-site update
- `server/services/backlog_service_triage.go` — 2 call sites (triage, `TriggerReReview`)
- `server/services/backlog_service_test.go` — call-site update
- `session/pipeline_mode_seed_test.go` — call-site update
- `main.go` — `rootCmd.AddCommand(commands.MemoryCmd)`
