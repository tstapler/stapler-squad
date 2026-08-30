# Implementation Plan: prompt-library

**Feature**: Prompt template library for reusable session-kickoff prompts — markdown+YAML-frontmatter files stored globally (`~/.stapler-squad/prompts/`) and per-workspace (`<workspace>/.stapler-squad/prompts/`), selectable from a searchable picker in the New Session flow, with variable interpolation and a "Save as template" action.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: ADR-001 (shared-not-instance-scoped global prompts dir), ADR-002 (new `prompt_library.proto` service, not an extension of `session.proto` or `PromptStore`)

---

## Step 0.5 — Creative Pass (Alternatives Considered)

Three distinct high-level approaches were brainstormed before committing to an architecture:

**A. Client-side interpolation + thin ConnectRPC CRUD service** (list/get/save raw template files; the three placeholder variables — `{{repo}}`, `{{branch}}`, `{{issue_title}}` — are already present in the Omnibar's local form state, so substitution happens in the browser at selection time, no round-trip). *Strength*: zero added latency on template selection (it's a local string replace against data the form already has), and the simplest possible RPC contract (3 methods, no per-keystroke or per-selection network dependency). *Weakness*: the substitution logic exists in two places (Go, for CLI/tests/save-time validation; TypeScript, for the picker) that must be kept in sync if the variable set ever grows.

**B. Server-side interpolation via a dedicated `ApplyTemplate` RPC** (client sends template ID + context, server returns the fully rendered body). *Strength*: single source of truth for interpolation logic, easier to reason about and unit-test in one place (Go only). *Weakness*: adds a network round-trip (with its own error/timeout handling) for what is, today, a pure local string substitution over three already-client-side-available variables — real cost for no correctness benefit at this scope.

**C. Reuse/extend the existing `session/prompts.PromptStore`** ("recent prompts" auto-history) by adding `Name`/`Description`/`Tags`/`Scope` fields to it. *Strength*: reuses an already-shipped, already-tested component and avoids a new package. *Weakness*: `PromptStore` is architecturally a single flat global JSON file with no scope concept — it cannot represent requirements.md's explicit git-shareable-per-workspace markdown file format without either splitting into two incompatible schemas or bolting an ad hoc scope tag onto an unrelated auto-log feature; `research/features.md` also flags "recent prompts" and "prompt templates" as conceptually distinct product surfaces that must stay visually and structurally separate.

**Chosen: Approach A.** See the Pattern Decisions table's last row and ADR-002 for the full rejection rationale for B and C.

---

## Domain Glossary

*(Ubiquitous language — every domain term that appears as a type, method, or variable name. These exact names must be used consistently in code, tests, and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `Template` | Parsed representation of one prompt template file: `Name`, `Description string`, `Tags []string`, `Body string`, `Scope TemplateScope`, `Path string` (absolute source file path) | `promptlibrary/template.go`; concrete struct, not an interface |
| `TemplateScope` | Sum type (`type TemplateScope int`, consts `ScopeGlobal`/`ScopeWorkspace`) marking which directory tier a `Template` was loaded from | Drives the picker's scope badge and the Save form's scope selector. Converted to/from the proto `TemplateScope` enum only via `scopeFromProto`/`scopeToProto` in `server/services/prompt_library_service.go` (Task 2.2.1b) — proto `TEMPLATE_SCOPE_GLOBAL=1`/`TEMPLATE_SCOPE_WORKSPACE=2` are NOT ordinally aligned with domain `ScopeGlobal=0`/`ScopeWorkspace=1`; a direct int cast silently swaps them, so it is never used |
| `TemplateSlug` | `type TemplateSlug string` with smart constructor `NewTemplateSlug(name string) TemplateSlug`, producing the filesystem-safe `[a-z0-9-]+` string derived from a template's `Name`, used as the `.md` filename stem on Save and the sole slug type accepted anywhere a slug (not an arbitrary name) is expected | `promptlibrary/save.go`; the primary path-traversal defense (name never used as a raw filename component); the single slugification entry point — `Save` and the CLI's `show <slug>` lookup (Task 3.1.1b) both derive from this one function, never a second reimplementation |
| `Interpolate` | Pure function `Interpolate(body string, vars map[string]string) string` substituting the fixed 3-key placeholder allow-list (`repo`, `branch`, `issue_title`) via `strings.NewReplacer`; unrecognized `{{token}}`s are left literal, recognized-but-absent vars render as `""` | `promptlibrary/interpolate.go`; mirrors `session/pipeline_engine.go`'s `renderTemplate`, deliberately a separate function (different placeholder set/package) |
| `RecognizedPlaceholder` | One of the three allow-listed interpolation tokens: `repo`, `branch`, `issue_title` | Fixed set for v1; no user-defined variables (out of scope per requirements.md) |
| `List` | `promptlibrary.List(globalDir, workspaceDir string) (templates []*Template, parseErrors []ParseError)` — unions global and workspace templates (never override-on-collision), returns malformed-file errors separately for log-and-skip handling | `promptlibrary/library.go` |
| `ParseError` | A non-fatal per-file error value returned by `List` when a file's frontmatter is missing or invalid YAML; logged via `log.Warn`, never raised as a Go `error` that aborts the whole listing | Carries `Path string` and `Reason string` |
| `Save` | `promptlibrary.Save(dir string, tpl *Template, overwrite bool) error` — atomic temp-file-then-rename write of a new template file; returns the sentinel `ErrTemplateExists` (checked via `errors.Is`) when `overwrite=false` and a file already exists at the target slug path | `promptlibrary/save.go`; mirrors `writeSettingsAtomic` (`server/services/mcp_injector.go:129`) |
| `PromptsDirOrDefault` | `Config` method resolving the global template directory (`~/.stapler-squad/prompts/`); the one deliberate exception to `GetConfigDir()`'s per-instance scoping (see ADR-001) | `config/config.go` |
| `WorkspacePromptsDir` | `promptlibrary.WorkspacePromptsDir(startPath string) (string, error)` — resolves `<repo-root>/.stapler-squad/prompts/` via read-only `git.PlainOpenWithOptions(startPath, &git.PlainOpenOptions{DetectDotGit:true})`; returns an error (never creates a repo) when `startPath` is not inside a git repo | `promptlibrary/library.go`; explicitly does NOT reuse `session/git/util.go`'s `findGitRepoRoot`, which auto-`git init`s a missing directory as a side effect — wrong for a read-only list call |
| `PromptLibraryService` | The ConnectRPC adapter (`server/services/prompt_library_service.go`) translating proto requests to `promptlibrary/` calls; a thin Gateway (PoEAA), not a Repository — it adds no caching or business logic of its own | Compile-time check: `var _ sessionv1connect.PromptLibraryServiceHandler = (*PromptLibraryService)(nil)` |
| `PromptTemplateProto` | The wire-format message (proto `PromptTemplate`), the RPC boundary's parse/serialize point — distinct from the Go domain `Template` | `proto/session/v1/prompt_library.proto` |
| `TemplatePicker` | Frontend searchable-palette component (`web-app/src/components/sessions/TemplatePicker.tsx`) presenting templates for selection, adapting `QuickOpenPalette.tsx`/`web-app/src/components/ui/AliasPalette.tsx`'s combobox pattern | Never auto-submits session creation — selection only prefills `firstPrompt` |
| `usePromptService` | Frontend hook (`web-app/src/lib/hooks/usePromptService.ts`) wrapping the `PromptLibraryService` ConnectRPC client for List/Get/Save calls, mirroring `useSessionService.ts`'s shape | Deliberately its own hook, not folded into the already-large `useSessionService.ts` |
| `SaveAsTemplateModal` | Frontend form component (`web-app/src/components/sessions/SaveAsTemplateModal.tsx`) for the "Save as template" action: name/description/tags/scope fields | Built on the existing `Modal.tsx` (Radix Dialog), per UX research — not a bespoke portal |
| `PromptStore` (existing, distinct) | Pre-existing, already-shipped auto-recorded "recent prompts" history component (`session/prompts/store.go`) backing `~/.stapler-squad/prompts.json` | Explicitly NOT this feature's template library; left unmodified (ADR-002) — never referred to as just "Prompts" in new code/UI copy to avoid conflation |
| `FirstPrompt` | The existing Omnibar form field (`OmnibarFormState.firstPrompt` in `web-app/src/components/sessions/Omnibar.tsx`) that template selection populates | No new field is introduced for this — confirms AC9 |
| `SkipCount` | The picker's non-blocking UI counter, computed client-side in `usePromptService.ts` as `ListPromptTemplatesResponse.skippedPaths.length` (no separate wire field — `skipped_count` was removed as redundant, Task 2.1.1b), surfacing "N templates couldn't be loaded" | `role="status" aria-live="polite"`, per UX research |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `promptlibrary/` package | Concrete types, no interfaces | Interface-pollution checklist (`.claude/rules/interface-pollution-checklist.md`) | `TemplateStore` interface with one fs-backed implementer | Only one implementation exists or is planned; an interface here adds indirection with no near-term second implementer (checklist smell #1) |
| `TemplateScope` | Sum type (typed const enum) | type-driven-design | Raw string `"global"`/`"workspace"` | Compiler-checked exhaustive `switch` in `List`/`Save`; a typo'd scope string becomes a compile error instead of a silent runtime bug |
| Template name → filename (Save) | Parse-don't-validate at the RPC boundary (slugify + `resolveAndValidatePath` before domain logic runs) | type-driven-design | Validate the raw name string deep inside `Save()` only | Slugification + `resolveAndValidatePath` (`server/services/file_service.go:106-115`) both run in the RPC handler before `promptlibrary.Save` ever sees a "maybe-malicious" string — defense-in-depth at the boundary, not buried in business logic |
| `PromptLibraryService` | Thin Adapter / Gateway (PoEAA) | Fowler (PoEAA) | Repository pattern with an in-memory cache of listed templates | The data source already is the filesystem (small, dozens-not-thousands file count); a cache layer reintroduces exactly the stale-read race class `.claude/rules/go-double-checked-locking.md` warns about, for an unmeasured performance problem (pitfalls.md is explicit: no cache for v1) |
| Interpolation (both Go and TS) | Hand-rolled `strings.NewReplacer` / equivalent fixed-map substitution (Transaction-Script-simple) | PoEAA / `research/build-vs-buy.md` | Go `text/template` | Fixed 3-key allow-list, no loops/conditionals ever needed; `text/template`'s `{{.Field}}` syntax doesn't match the issue's required `{{repo}}` UX, so a translation layer would be needed either way — not actually cheaper, and this repo already rejected `text/template` for the same reason in `session/pipeline_engine.go` |
| `List()` on a missing prompts directory | Idiom: `os.IsNotExist(err)` → empty result, `nil` error | Go idiom, `WalkDir` precedent | Return an error when the directory doesn't exist | Zero templates on first run is the *common* case per the non-functional requirement ("0 templates = fast, not an error"), not an edge case |
| Overall architecture | Client-side interpolation + thin ConnectRPC CRUD service (Approach A, Step 0.5) | This plan's creative pass | (B) server-side `ApplyTemplate` RPC; (C) extend `session/prompts.PromptStore` | (B) adds a network round-trip for a local substitution the client already has the data for; (C) conflates two structurally and conceptually distinct features and cannot represent per-workspace git-shareable markdown storage in a single flat JSON file (see ADR-002) |

---

## Migration Plan

Omitted — no schema or database changes. This feature is entirely new filesystem-backed state (`~/.stapler-squad/prompts/*.md`, `<workspace>/.stapler-squad/prompts/*.md`) with no migration of existing data; `session/prompts.PromptStore`'s existing `~/.stapler-squad/prompts.json` is untouched (ADR-002).

## Observability Plan

- **Logs**: `promptlibrary.List` logs one `log.Warn("skipping malformed prompt template", "path", p.Path, "reason", parseErr.Reason)` line per skipped file (AC6). `PromptLibraryService`'s three handlers log at entry (`log.Debug("ListPromptTemplates", "path", req.Path)` etc.) and on any non-`ParseError` failure (e.g. `resolveAndValidatePath` rejection) log at `Warn` with the offending path.
- **Metrics**: none required for v1 — filesystem scans of "dozens, not thousands" of small files are not expected to cross this app's existing >100ms instrumentation threshold; if a future audit shows otherwise, add an RPC-timing metric via the existing `createRpcTimingInterceptor` (already applied to all ConnectRPC clients, `useSessionService.ts:24`) rather than inventing a new one.
- **Alerts**: none required — this is a local, single-user, filesystem-backed feature with no external dependency or SLA to page on.

## Risk Control

- **Feature flag**: not gated — `PromptLibraryService` registers unconditionally in `server/server.go` (matches `GitHubUserService`'s registration, per ADR-002), and the "From template" button/Save-as-template action render unconditionally in the Omnibar/session UI once merged.
- **Rollback procedure**: standard revert via PR close + revert commit. No data migration to unwind — reverting removes the RPC handlers, UI entry points, and the two new Go packages/proto file; any `.md` template files a user already created on disk are inert (plain markdown) and harmless to leave behind.
- **Staged rollout**: full rollout on merge (no cohort/percentage gating — this is a client+local-server feature with no shared-state blast radius across users).

## Unresolved Questions

Both open questions raised in requirements.md/research are resolved below, not left dangling:

1. **Overwrite-vs-empty-only on template selection (UX open question).** **Resolved**: if the `firstPrompt` field is empty, selecting a template applies immediately (single action — click a row, or Enter). If `firstPrompt` already has user-typed content, the picker enters a "pending replace" state on selection (the row is highlighted, a footer control reads "Replace current draft") requiring one explicit confirming action (click "Replace" or press Enter a second time) before the textarea is overwritten; Escape or clicking elsewhere cancels the pending replace with no change. *Rationale*: preserves single-action speed for the common "empty field, apply template" path while adding exactly one extra confirming step only when a destructive overwrite is actually possible — this is small enough it doesn't need further sign-off.
2. **Typo'd token (`{{repoo}}`) literal-vs-blank (pitfalls.md open question).** **Resolved**: literal — matches the existing unrecognized-token behavior of `session/pipeline_engine.go`'s `renderTemplate` precedent, and keeps `Interpolate`'s behavior fully described by one rule ("only the 3 allow-listed keys are ever substituted; everything else in the body passes through unchanged"), rather than needing a second "did the author mean to type a real token" heuristic. To catch typos at authoring time instead of leaving silent dead text, `SavePromptTemplate` performs a **non-blocking** validation pass (Story 6.1.2 / AC5): any `{{token}}` in the body not in `{repo, branch, issue_title}` produces a warning surfaced in the Save-as-template form ("`{{repoo}}` won't be replaced — did you mean `{{repo}}`?") but does not block Save. *Rationale*: rejecting the save outright (mirroring `rules_service.go`'s stricter `validateYAMLEntry`-style hard validation) would also block legitimate templates that intentionally contain literal `{{...}}` text (e.g. a template teaching someone else's templating syntax); a warning satisfies "catch typos at authoring time" without that false-positive cost.

No remaining items require a human decision before implementation starts.

## Dependency Visualization

```
Phase 1: Backend Domain & Storage
  1.1 Template model+parsing ──┐
  1.2 Library listing (union)  ├──> 1.3 Interpolate ──┐
  1.5 Config dir helpers ───────┘                     │
  1.4 Save (depends on 1.1, TemplateSlug) ─────────────┼──> Phase 2
                                                        │
Phase 2: ConnectRPC API                                │
  2.1 proto file + proto-gen  <──────────────────────────┘
    └──> 2.2 Service adapter (wraps 1.2/1.3/1.4) ──> 2.3 server.go registration
                    │
                    ├──> Phase 3: CLI (cmd/commands/prompts.go, calls promptlibrary/ directly, no RPC)
                    │
                    └──> Phase 4: Frontend data layer
                            4.1 usePromptService (ConnectRPC client)
                            4.2 Client-side interpolateTemplate.ts
                                    │
                                    ├──> Phase 5: TemplatePicker UI ──> 5.2 Omnibar integration
                                    │
                                    └──> Phase 6: Save-as-template UI
                                              │
                                              v
                                    Phase 7: Registry + Tests (spans all prior phases;
                                              individual test tasks depend on their
                                              corresponding implementation task, not
                                              on Phase 7 as a monolithic block)
```

---

## Phase 1: Backend Domain & Storage

### Epic 1.1: Template Domain Model & Parsing
**Goal**: Define the `Template`/`TemplateScope` types and a YAML-frontmatter parser that safely rejects malformed input without crashing.

#### Story 1.1.1: `Template` struct and frontmatter `Parse`
**As a** backend developer, **I want** a `Template` struct and a `Parse(path string) (*Template, error)` function, **so that** every other component (List, Save, CLI) shares one parsing implementation.
**Acceptance Criteria**:
- Parsing a well-formed file with `name`, `description`, `tags: [a, b]` frontmatter and a body produces a `Template` with all fields populated and `Scope` left unset (caller sets it — `Parse` doesn't know its own scope).
  - *Given* a file at `/tmp/t/dependency-audit.md` containing:
    ```
    ---
    name: "Dependency Audit"
    description: "Audit package.json / go.mod for outdated or vulnerable deps"
    tags: [maintenance, security]
    ---
    Run a full dependency audit on {{repo}} and file findings as backlog items.
    ```
    *When* `promptlibrary.Parse("/tmp/t/dependency-audit.md")` is called, *Then* it returns `&Template{Name: "Dependency Audit", Description: "Audit package.json / go.mod for outdated or vulnerable deps", Tags: []string{"maintenance", "security"}, Body: "Run a full dependency audit on {{repo}} and file findings as backlog items.", Path: "/tmp/t/dependency-audit.md"}` and a `nil` error.
- A file with no `---` frontmatter delimiters at all, or invalid YAML between them, returns a non-nil error (a `ParseError`) rather than panicking (AC6).
  - *Given* `/tmp/t/broken.md` containing only `This is not a template.` (no `---` markers), *When* `Parse` is called, *Then* it returns `(nil, ParseError{Path: "/tmp/t/broken.md", Reason: "missing YAML frontmatter delimiters"})`.
- Frontmatter is capped at a fixed size (e.g. 8 KiB) read before YAML parsing, as defense-in-depth against a maliciously huge frontmatter block (pitfalls.md).

**Files**: `promptlibrary/template.go`, `promptlibrary/template_test.go`

##### Task 1.1.1a: Define `Template` and `TemplateScope` types (~3 min)
- In `promptlibrary/template.go`: `type TemplateScope int` with `const (ScopeGlobal TemplateScope = iota; ScopeWorkspace)`; `type Template struct { Name, Description, Body, Path string; Tags []string; Scope TemplateScope }`.
- Files: `promptlibrary/template.go`

##### Task 1.1.1b: Hand-roll `---`-split frontmatter extraction (~5 min)
- Add unexported `splitFrontmatter(raw []byte) (frontmatter, body []byte, err error)`: requires the file to start with a line that is exactly `---`, find the second `---` line, return the bytes between as frontmatter and everything after as body (trimmed of one leading newline). Return an error if either delimiter is missing.
- Files: `promptlibrary/template.go`

##### Task 1.1.1c: YAML-unmarshal frontmatter into a concrete struct (~4 min)
- Add unexported `type templateFrontmatter struct { Name string `yaml:"name"`; Description string `yaml:"description"`; Tags []string `yaml:"tags"` }` (mirrors `server/services/rules_service.go`'s `yamlRuleEntry` pattern — concrete struct, never `interface{}`/`map[string]interface{}`, per pitfalls.md's YAML-safety note). Use `gopkg.in/yaml.v3`.
- Files: `promptlibrary/template.go`

##### Task 1.1.1d: Implement `Parse(path string) (*Template, error)` with size cap (~5 min)
- Read file via `os.ReadFile`; if `len(raw) > maxTemplateFileBytes` (define `const maxTemplateFileBytes = 64 * 1024`), return a `ParseError`. Call `splitFrontmatter`, cap frontmatter bytes at 8 KiB before `yaml.Unmarshal`, populate `Template{Path: path, Name: fm.Name, Description: fm.Description, Tags: fm.Tags, Body: string(body)}`. Reject (return `ParseError`) when `Name` is empty after trim. This constant is defined once, here, and reused by `Save` (Task 1.4.1c) to reject oversized bodies before writing — never duplicated.
- Files: `promptlibrary/template.go`

##### Task 1.1.1e: Define `ParseError` type (~2 min)
- `type ParseError struct { Path, Reason string }` with `func (e ParseError) Error() string { return fmt.Sprintf("%s: %s", e.Path, e.Reason) }`.
- Files: `promptlibrary/template.go`

##### Task 1.1.1f: Unit tests for `Parse` (~5 min)
- `TestParse_should_ReturnPopulatedTemplate_When_FrontmatterWellFormed` (the "Dependency Audit" example above), `TestParse_should_ReturnParseError_When_FrontmatterDelimitersMissing`, `TestParse_should_ReturnParseError_When_YAMLInvalid` (e.g. `tags: [a, b` unclosed), `TestParse_should_ReturnParseError_When_NameEmpty`, `TestParse_should_ReturnParseError_When_FileExceedsSizeCap`.
- Files: `promptlibrary/template_test.go`

---

### Epic 1.2: Library Listing (Union of Global + Workspace)
**Goal**: `List` scans both directory tiers, unions results (no override-on-collision), skips malformed files with a logged warning, and never errors on a missing directory.

#### Story 1.2.1: `List(globalDir, workspaceDir string) ([]*Template, []ParseError)`
**As a** picker/CLI caller, **I want** one function that returns every valid template from both scopes plus a separate list of skip reasons, **so that** the UI can render both the templates and a non-blocking "N couldn't load" notice.
**Acceptance Criteria**:
- Global and workspace templates both appear in the result — a name collision between scopes does not drop either one (AC1, AC2).
  - *Given* `~/.stapler-squad/prompts/review.md` (name "Code Review") and `<workspace>/.stapler-squad/prompts/review.md` (also name "Code Review", different body) both exist, *When* `List(globalDir, workspaceDir)` is called, *Then* the returned slice contains two `*Template` entries, one with `Scope: ScopeGlobal` and one with `Scope: ScopeWorkspace`, both named "Code Review" (the UI, not `List`, disambiguates via scope badge).
- A malformed file is excluded from the template slice and appears in the `[]ParseError` return instead (AC6).
  - *Given* `~/.stapler-squad/prompts/broken.md` has no frontmatter and `~/.stapler-squad/prompts/dependency-audit.md` is well-formed, *When* `List` is called, *Then* the returned `[]*Template` has exactly one entry ("Dependency Audit") and the returned `[]ParseError` has exactly one entry for `broken.md`.
- A missing directory (global, workspace, or both) produces zero templates and zero errors for that tier, not a Go `error` (non-functional: 0 templates = fast, not an error state).
  - *Given* `workspaceDir` does not exist on disk (non-git working directory, or first run before anyone created `.stapler-squad/prompts/`), *When* `List` is called, *Then* it returns whatever global templates exist plus an empty contribution from the workspace tier, with no panic and no error.

**Files**: `promptlibrary/library.go`, `promptlibrary/library_test.go`

##### Task 1.2.1a: `scanDir(ctx context.Context, dir string, scope TemplateScope) ([]*Template, []ParseError)` helper (~6 min)
- `os.ReadDir(dir)`; if `os.IsNotExist(err)`, return `(nil, nil)`. Before processing each entry, check `ctx.Err()` — if non-nil (deadline exceeded), stop scanning and return whatever templates/errors have been collected so far (partial results, not a hard failure — see Task 1.2.1b's caller-side deadline). For each `*.md` entry (skip symlinks explicitly — `d.Type()&os.ModeSymlink != 0` → skip, per pitfalls.md's flagged symlink-traversal decision, documented inline as "skip, don't follow, to avoid an unbounded walk outside the prompts dir"), call `Parse`, set `.Scope = scope` on success, append to the templates or errors slice accordingly.
- Files: `promptlibrary/library.go`

##### Task 1.2.1b: `List(globalDir, workspaceDir string) ([]*Template, []ParseError)` (~5 min)
- Thread a `context.Context` with a short deadline appropriate for a local filesystem scan: `ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second); defer cancel()`. Call `scanDir(ctx, globalDir, ScopeGlobal)` and `scanDir(ctx, workspaceDir, ScopeWorkspace)`, concatenate both templates and both error slices, return — a hung network-mounted home directory can time out partially without blocking session creation indefinitely (non-functional requirement: "must not block or slow down session creation"). `List`'s exported signature stays `(globalDir, workspaceDir string) ([]*Template, []ParseError)` — no `context.Context` parameter forced onto callers, since every current caller (the `ListPromptTemplates` handler, the CLI) wants the same fixed local-filesystem bound; the deadline is internal.
- Files: `promptlibrary/library.go`

##### Task 1.2.1c: Log skipped files (~2 min)
- After computing the combined `[]ParseError`, loop and call `log.Warn("skipping malformed prompt template", "path", pe.Path, "reason", pe.Reason)` for each — this satisfies AC6's "logged warning" requirement at the point closest to detection.
- Files: `promptlibrary/library.go`

##### Task 1.2.1d: Unit tests for `List` (~6 min)
- `TestList_should_UnionGlobalAndWorkspace_When_BothDirsHaveTemplates`, `TestList_should_KeepBothEntries_When_NamesCollideAcrossScopes`, `TestList_should_ExcludeAndReportMalformed_When_OneFileBroken`, `TestList_should_ReturnEmpty_When_BothDirsMissing`, `TestList_should_SkipSymlinks_When_SymlinkPresentInPromptsDir`, `TestList_should_ReturnPartialResults_When_ContextDeadlineExceeded` (call `scanDir` directly with an already-expired context, or override the deadline via a test-only short constant, and assert the function returns whatever was already collected rather than blocking or panicking).
- Files: `promptlibrary/library_test.go`

#### Story 1.2.2: Read-only workspace root resolution
**As a** backend developer, **I want** `WorkspacePromptsDir(startPath string) (string, error)` to resolve the workspace-local prompts directory without ever creating a git repo as a side effect, **so that** listing templates for a session-creation flow never mutates the filesystem.
**Acceptance Criteria**:
- Resolves to `<repo-root>/.stapler-squad/prompts/` starting from any path inside the repo, including a worktree path (worktrees share the parent repo's `.git` metadata via `DetectDotGit`).
  - *Given* `/home/tstapler/code/myrepo` is a git repo with a worktree checked out at `/home/tstapler/.stapler-squad/worktrees/myrepo-feature-x`, *When* `WorkspacePromptsDir("/home/tstapler/.stapler-squad/worktrees/myrepo-feature-x")` is called, *Then* it returns `/home/tstapler/code/myrepo/.stapler-squad/prompts` (the main repo root, not the worktree path) — templates committed on `main` are visible from any worktree.
- A non-git directory produces a clear "not a git repo" error, not a filesystem mutation.
  - *Given* `/tmp/scratch` is a plain directory with no `.git` anywhere in its ancestry, *When* `WorkspacePromptsDir("/tmp/scratch")` is called, *Then* it returns an error and `/tmp/scratch` is left completely unmodified (in particular, no `.git` directory is created — the opposite of `session/git/util.go`'s `findGitRepoRoot`, which is explicitly NOT reused here for this reason).

**Files**: `promptlibrary/library.go`, `promptlibrary/library_test.go`

##### Task 1.2.2a: Implement `WorkspacePromptsDir` via `go-git` `DetectDotGit` (~4 min)
- `repo, err := git.PlainOpenWithOptions(startPath, &git.PlainOpenOptions{DetectDotGit: true})`; on error, wrap and return (`fmt.Errorf("resolve workspace root: %w", err)`); on success, get the worktree root via `repo.Worktree()` → `wt.Filesystem.Root()`, join with `.stapler-squad/prompts`.
- Files: `promptlibrary/library.go`

##### Task 1.2.2b: Unit tests for `WorkspacePromptsDir` (~5 min)
- `TestWorkspacePromptsDir_should_ResolveMainRepoRoot_When_CalledFromWorktree` (init a temp repo + `git worktree add`, assert resolution), `TestWorkspacePromptsDir_should_Error_When_PathNotInGitRepo`, `TestWorkspacePromptsDir_should_NotCreateGitRepo_When_PathMissing` (assert no `.git` appears after the error).
- Files: `promptlibrary/library_test.go`

---

### Epic 1.3: Interpolation
**Goal**: Fixed-allow-list variable substitution with the literal/blank distinction resolved per the Unresolved Questions section above.

#### Story 1.3.1: `Interpolate(body string, vars map[string]string) string`
**As a** picker/CLI/save-validation caller, **I want** a single substitution function for the 3 allow-listed variables, **so that** interpolation behavior is identical everywhere it's invoked.
**Acceptance Criteria**:
- Recognized-but-unavailable variables render as empty string (AC3, requirements.md §2).
  - *Given* `body = "Please review branch {{branch}} of {{repo}} and summarize {{issue_title}}"` and `vars = map[string]string{"repo": "stapler-squad", "branch": "fix-flaky-test"}` (no `issue_title` key), *When* `Interpolate(body, vars)` is called, *Then* it returns `"Please review branch fix-flaky-test of stapler-squad and summarize "`.
- Unrecognized tokens (typos, or non-allow-listed names) are left as literal text (Unresolved Questions §2).
  - *Given* `body = "Audit {{repoo}} now"` and `vars = map[string]string{"repo": "stapler-squad"}`, *When* `Interpolate(body, vars)` is called, *Then* it returns `"Audit {{repoo}} now"` unchanged — `{{repoo}}` is not one of the 3 allow-listed keys, so it's never a substitution candidate.
- A value that itself contains `{{` is inserted verbatim with no second-round substitution (pitfalls.md).
  - *Given* `body = "{{repo}}"` and `vars = map[string]string{"repo": "{{branch}}"}`, *When* `Interpolate(body, vars)` is called, *Then* it returns the literal string `"{{branch}}"` (not further substituted) — `strings.NewReplacer` performs exactly one pass.

**Files**: `promptlibrary/interpolate.go`, `promptlibrary/interpolate_test.go`

##### Task 1.3.1a: Implement `Interpolate` via `strings.NewReplacer` over a fixed allow-list (~4 min)
- `var recognizedPlaceholders = []string{"repo", "branch", "issue_title"}`. Build replacement pairs only for keys present in `vars` (build `{{key}}` → `vars[key]`, using `""` when the key is in `recognizedPlaceholders` but absent from `vars` — iterate `recognizedPlaceholders`, not `vars`, so every allow-listed key always gets a pair even when its value is `""`). Return `strings.NewReplacer(pairs...).Replace(body)`.
- Files: `promptlibrary/interpolate.go`

##### Task 1.3.1b: Unit tests for `Interpolate` (~4 min)
- `TestInterpolate_should_RenderEmptyString_When_VariableUnavailable`, `TestInterpolate_should_LeaveUnrecognizedTokenLiteral_When_TokenIsTypo`, `TestInterpolate_should_NotDoubleSubstitute_When_ValueContainsBraces`, `TestInterpolate_should_SubstituteAllThree_When_AllVarsPresent`.
- Files: `promptlibrary/interpolate_test.go`

##### Task 1.3.1c: Save-time typo-warning helper (non-blocking) (~4 min)
- Add `promptlibrary.FindUnrecognizedTokens(body string) []string`: regex-scan for `\{\{([a-zA-Z0-9_]+)\}\}`, return any captured names not in `recognizedPlaceholders` (deduped). Used by `SavePromptTemplate`'s handler (Story 2.2.1) to build the non-blocking warning from Unresolved Questions §2 — does not reject the save.
- Files: `promptlibrary/interpolate.go`

##### Task 1.3.1d: Unit tests for `FindUnrecognizedTokens` (~3 min)
- `TestFindUnrecognizedTokens_should_ReturnTypo_When_BodyContainsRepoo`, `TestFindUnrecognizedTokens_should_ReturnEmpty_When_OnlyRecognizedTokensPresent`.
- Files: `promptlibrary/interpolate_test.go`

---

### Epic 1.4: Save (Atomic Write with Path-Traversal Defense)
**Goal**: Writing a new template file is atomic, slugified, size-bounded, defended against path traversal at two independent layers, and never silently overwrites an existing template.

#### Story 1.4.1: `Save(dir string, tpl *Template, overwrite bool) error`
**As a** backend developer, **I want** an atomic, slug-safe, collision-protected, size-bounded file write, **so that** a concurrent read never observes a partial file, a malicious template name can never escape the prompts directory, an oversized body can never be written unreadable, and two same-named templates can never silently clobber each other.
**Acceptance Criteria**:
- A well-formed template is written as valid markdown+frontmatter to `<dir>/<slug>.md` (AC5).
  - *Given* `dir = "~/.stapler-squad/prompts"` and `tpl = &Template{Name: "Dependency Audit", Description: "Audit deps for vulnerabilities", Tags: []string{"maintenance", "security"}, Body: "Run a full dependency audit on {{repo}}..."}`, *When* `Save(dir, tpl, false)` is called, *Then* `~/.stapler-squad/prompts/dependency-audit.md` exists afterward containing well-formed YAML frontmatter (`name: Dependency Audit`, `description: ...`, `tags: [maintenance, security]`) followed by `---` and the body, and re-`Parse`-ing that file returns an equivalent `*Template`.
- A malicious name cannot escape the target directory.
  - *Given* `tpl.Name = "../../../.ssh/authorized_keys"`, *When* `Save(dir, tpl, false)` is called, *Then* the slugified filename component is `.ssh-authorized_keys` (or similarly sanitized to `[a-z0-9-]+`, non-empty), and the write lands inside `dir` — it never writes outside it.
- An empty-after-slugify name is rejected before any file I/O.
  - *Given* `tpl.Name = "???"` (slugifies to empty string), *When* `Save(dir, tpl, false)` is called, *Then* it returns an error and no file is created.
- A slug collision is rejected unless the caller explicitly opts into overwrite.
  - *Given* `<dir>/dependency-audit.md` already exists (from a prior `Save`), *When* `Save(dir, &Template{Name: "Dependency Audit", ...}, false)` is called, *Then* it returns the sentinel `ErrTemplateExists` (checkable via `errors.Is`) and the existing file's bytes are unchanged; *When* the same call is retried with `overwrite=true`, *Then* it succeeds and the file's content is replaced with the new template's.
- An oversized body is rejected before any file I/O.
  - *Given* `tpl.Body` is large enough that the rendered markdown exceeds `maxTemplateFileBytes` (64 KiB, Task 1.1.1d), *When* `Save(dir, tpl, false)` is called, *Then* it returns an error and no file is created — mirrors `Parse`'s read-side cap so a save can never produce a file that immediately becomes unloadable.

**Files**: `promptlibrary/save.go`, `promptlibrary/save_test.go`

##### Task 1.4.1a: `TemplateSlug` type + `NewTemplateSlug(name string) TemplateSlug` (~4 min)
- Define `type TemplateSlug string` and smart constructor `func NewTemplateSlug(name string) TemplateSlug`: lowercase, replace whitespace runs with `-`, strip everything outside `[a-z0-9-]`, collapse repeated `-`, trim leading/trailing `-` (mirrors `session/git/util.go`'s `sanitizeBranchName` shape but for filenames, not branch names — a new small function, not a reused one, since the allowed character set and collapsing rules differ slightly). `TemplateSlug` is the *only* slugification entry point in the codebase — `Save` (Task 1.4.1c) and the CLI's `show <slug>` lookup (Task 3.1.1b) both derive from this one function's rules; `SaveAsTemplateModal.tsx`'s client-side preview (Task 6.1.2a) is a documented, explicitly non-authoritative TS duplicate.
- Files: `promptlibrary/save.go`

##### Task 1.4.1b: Serialize `Template` back to frontmatter+body markdown (~4 min)
- `func renderMarkdown(tpl *Template) ([]byte, error)`: `yaml.Marshal(templateFrontmatter{Name: tpl.Name, Description: tpl.Description, Tags: tpl.Tags})`, wrap with `---\n`...`---\n\n` + body.
- Files: `promptlibrary/save.go`

##### Task 1.4.1c: Implement `Save` with slug validation, size cap, collision check + atomic write (~7 min)
- Signature: `func Save(dir string, tpl *Template, overwrite bool) error` (the `overwrite` parameter, not a field on `Template`, keeps the collision decision out of the serialized domain object — `renderMarkdown`, Task 1.4.1b, never marshals it). `slug := NewTemplateSlug(tpl.Name)`; if `slug == ""`, return `fmt.Errorf("template name %q has no valid characters for a filename", tpl.Name)`. `content, err := renderMarkdown(tpl)`; if `len(content) > maxTemplateFileBytes` (the shared constant from Task 1.1.1d), return `fmt.Errorf("template body exceeds %d byte limit", maxTemplateFileBytes)` before any file I/O. `os.MkdirAll(dir, 0o755)`. `targetPath := filepath.Join(dir, string(slug)+".md")`; if `!overwrite`, `if _, statErr := os.Stat(targetPath); statErr == nil { return ErrTemplateExists }` (sentinel `var ErrTemplateExists = errors.New("template already exists")` defined in `promptlibrary/save.go`, checked via `errors.Is` by callers). Atomic write mirroring `writeSettingsAtomic` (`server/services/mcp_injector.go:129-149`): `os.CreateTemp(dir, string(slug)+"-*.tmp")` (unique name, not a fixed suffix), write, `os.Rename(tmpPath, targetPath)`, `defer os.Remove(tmpPath)` for cleanup-on-failure. The `os.Stat`-then-`os.Rename` collision check has a benign TOCTOU window under concurrent writers — acceptable here because the rename itself stays atomic (worst case is a missed overwrite-warning, never a torn/corrupt file; see the concurrent-write test in Task 1.4.1d for the corruption-safety property this check does not need to provide).
- Files: `promptlibrary/save.go`

##### Task 1.4.1d: Unit tests for `Save` (~9 min)
- `TestSave_should_WriteWellFormedFile_When_TemplateValid` (round-trips through `Parse`), `TestSave_should_SlugifyTraversalAttempt_When_NameContainsDotDot` (asserts the written file's path is inside `dir`, never resolves above it), `TestSave_should_ReturnError_When_SlugEmpty`, `TestSave_should_NeverLeaveTempFile_When_WriteSucceeds` (glob for `*.tmp` after a successful save, assert none remain), `TestSave_should_ReturnErrTemplateExists_When_SlugCollidesAndOverwriteFalse` (write once, call `Save` again with the same name and `overwrite: false`, assert `errors.Is(err, ErrTemplateExists)` and the original file's content is unchanged), `TestSave_should_Overwrite_When_OverwriteTrueAndSlugCollides` (same setup, `overwrite: true`, assert the file's content is replaced), `TestSave_should_RejectOversizedBody_When_ExceedsCap` (a `Body` whose rendered markdown exceeds `maxTemplateFileBytes`, assert an error and no file created), `TestSave_should_NeverProduceCorruptFile_When_ConcurrentSavesRace` (modeled on `TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON`, `server/services/hook_injector_test.go:424` — spin N goroutines all calling `Save(dir, tpl, true)` with different bodies but the same `Name`/slug, then assert the resulting file at `<dir>/<slug>.md` is one of the N valid complete writes, never a torn/partial mix of two writers' bytes).
- Files: `promptlibrary/save_test.go`

---

### Epic 1.5: Config Directory Helpers
**Goal**: Resolve the global prompts directory per ADR-001 (shared-not-instance-scoped, still test-dir-isolated).

#### Story 1.5.1: `PromptsDirOrDefault()` on `Config`
**As a** backend developer, **I want** a config helper matching `TriageArtifactDirOrDefault()`'s shape, **so that** callers get the correct global prompts path without knowing ADR-001's exception logic themselves.
**Acceptance Criteria**:
- Without `STAPLER_SQUAD_TEST_DIR` set, resolves to `~/.stapler-squad/prompts` regardless of `STAPLER_SQUAD_INSTANCE`.
  - *Given* `STAPLER_SQUAD_INSTANCE=claude-manual-test` is set and `STAPLER_SQUAD_TEST_DIR` is unset, *When* `config.PromptsDirOrDefault()` is called, *Then* it returns `<home>/.stapler-squad/prompts` — NOT `<home>/.stapler-squad/instances/claude-manual-test/prompts`.
- With `STAPLER_SQUAD_TEST_DIR` set, resolves under the test directory (e2e isolation).
  - *Given* `STAPLER_SQUAD_TEST_DIR=/tmp/stapler-squad-test-12345`, *When* `config.PromptsDirOrDefault()` is called, *Then* it returns `/tmp/stapler-squad-test-12345/prompts`.

**Files**: `config/config.go`, `config/config_test.go`

##### Task 1.5.1a: Implement `PromptsDirOrDefault()` (~4 min)
- Add the function exactly as specified in ADR-001 (including the inline "do not fix this into instance-scoping" comment), placed near `TriageArtifactDirOrDefault()`/`BacklogAttachmentDirOrDefault()` (`config/config.go:539-557`).
- Files: `config/config.go`

##### Task 1.5.1b: Unit tests for `PromptsDirOrDefault` (~4 min)
- `TestPromptsDirOrDefault_should_IgnoreInstanceScoping_When_InstanceSet` (set `STAPLER_SQUAD_INSTANCE`, assert path has no `instances/` segment), `TestPromptsDirOrDefault_should_UseTestDir_When_TestDirSet`.
- Files: `config/config_test.go`

---

## Phase 2: ConnectRPC API

### Epic 2.1: Proto Definition
**Goal**: Define the wire contract per ADR-002.

#### Story 2.1.1: `prompt_library.proto`
**As a** frontend/backend integrator, **I want** a small standalone proto file for this feature, **so that** it's independently reviewable and doesn't grow `session.proto` further.
**Acceptance Criteria**:
- `make proto-gen` succeeds and produces Go + TS bindings for `PromptLibraryService`.
  - *Given* `proto/session/v1/prompt_library.proto` defines `PromptLibraryService` with 2 RPCs (`ListPromptTemplates`, `SavePromptTemplate` — `GetPromptTemplate` was cut for v1, see Task 2.1.1c), *When* `make proto-gen` runs, *Then* `session/gen/session/v1/prompt_library_pb.go`, `session/gen/session/v1/prompt_library_grpc.pb.go` (or connect-equivalent), and `web-app/src/gen/session/v1/prompt_library_pb.ts` all exist and `go build ./...` / `cd web-app && npx tsc --noEmit` both succeed.

**Files**: `proto/session/v1/prompt_library.proto`

##### Task 2.1.1a: Define `PromptTemplate`, `TemplateScope` proto messages (~4 min)
- `message PromptTemplate { string name = 1; string description = 2; repeated string tags = 3; string body = 4; TemplateScope scope = 5; string slug = 6; }`; `enum TemplateScope { TEMPLATE_SCOPE_UNSPECIFIED = 0; TEMPLATE_SCOPE_GLOBAL = 1; TEMPLATE_SCOPE_WORKSPACE = 2; }`. Note: these proto ordinals (`GLOBAL=1`, `WORKSPACE=2`) are NOT the same as the Go domain `TemplateScope` ordinals (`ScopeGlobal=0`, `ScopeWorkspace=1`, Task 1.1.1a) — every conversion between them MUST go through `scopeFromProto`/`scopeToProto` (Task 2.2.1b), never a direct int cast.
- Files: `proto/session/v1/prompt_library.proto`

##### Task 2.1.1b: Define `ListPromptTemplatesRequest`/`Response` (~3 min)
- `message ListPromptTemplatesRequest { string path = 1; }` (workspace root resolution input, same semantics as `CreateSessionRequest.path`). `message ListPromptTemplatesResponse { repeated PromptTemplate templates = 1; repeated string skipped_paths = 2; }` — no separate `skipped_count` field; it's fully derivable from `len(skipped_paths)` / `skippedPaths.length` on both the Go and TS sides, so a redundant wire field is omitted.
- Files: `proto/session/v1/prompt_library.proto`

##### Task 2.1.1c: Define `SavePromptTemplateRequest`/`Response` (~4 min)
- `GetPromptTemplateRequest`/`Response` are cut for v1 — `GetPromptTemplate` has no consumer anywhere in this plan (`TemplatePicker` already gets full bodies from `ListPromptTemplates`, and the CLI's `show` command reads via `promptlibrary.List` in-process, never over RPC — Task 3.1.1b). `message SavePromptTemplateRequest { PromptTemplate template = 1; string path = 2; bool overwrite = 3; }` (the `overwrite` field lets a second Save attempt on a colliding slug succeed explicitly, since the client otherwise has no way to say "yes, replace it" — Task 2.2.1d) / `message SavePromptTemplateResponse { PromptTemplate saved = 1; repeated string unrecognized_tokens = 2; }` (carries the non-blocking typo warning from Unresolved Questions §2).
- Files: `proto/session/v1/prompt_library.proto`

##### Task 2.1.1d: Define the `PromptLibraryService` service block (~2 min)
- `service PromptLibraryService { rpc ListPromptTemplates(ListPromptTemplatesRequest) returns (ListPromptTemplatesResponse); rpc SavePromptTemplate(SavePromptTemplateRequest) returns (SavePromptTemplateResponse); }` — 2 RPCs, matching ADR-002's Decision section with `GetPromptTemplate` removed (cut for v1, Task 2.1.1c); if ADR-002's text still lists 3 RPCs, this plan supersedes it on this point.
- Files: `proto/session/v1/prompt_library.proto`

##### Task 2.1.1e: Run `make proto-gen`, commit generated output (~3 min)
- Run `make proto-gen`; verify `session/gen/session/v1/prompt_library_*.go` and `web-app/src/gen/session/v1/prompt_library_pb.ts` are produced; `go build ./...` and `cd web-app && npx tsc --noEmit` both pass.
- Files: `session/gen/session/v1/prompt_library_pb.go`, `session/gen/session/v1/prompt_library_*connect.go`, `web-app/src/gen/session/v1/prompt_library_pb.ts` (all generated — see `.claude/CLAUDE.md`'s note that `web-app/src/gen` is tracked despite `.gitignore`)

---

### Epic 2.2: Service Adapter
**Goal**: Thin ConnectRPC handler wrapping `promptlibrary/`, with path validation and scope-safe proto↔domain conversion at the boundary.

#### Story 2.2.1: `PromptLibraryService` handler
**As a** frontend caller, **I want** RPC methods that call straight through to `promptlibrary/` with request/response translation only, **so that** the service stays a Gateway, not a second business-logic layer.
**Acceptance Criteria**:
- `ListPromptTemplates` returns the union of global+workspace templates translated to proto, with skip info populated (AC1, AC2, AC6).
  - *Given* the same "Dependency Audit" (global) + "broken.md" (malformed) fixture from Story 1.2.1, *When* `ListPromptTemplates(&ListPromptTemplatesRequest{Path: "/home/tstapler/code/myrepo"})` is called, *Then* the response has `templates: [{name: "Dependency Audit", scope: TEMPLATE_SCOPE_GLOBAL, ...}]` (scope set via `scopeToProto`, Task 2.2.1b), `skipped_paths: ["<prompts-dir>/broken.md"]` (no `skipped_count` field — derived client-side as `skippedPaths.length`).
- `SavePromptTemplate` rejects a path-traversal attempt with `CodeInvalidArgument` before touching `promptlibrary.Save`, converts scope correctly, and returns unrecognized-token warnings for typo'd placeholders.
  - *Given* `SavePromptTemplateRequest{template: {name: "../../evil", scope: TEMPLATE_SCOPE_GLOBAL, body: "test {{repoo}}"}}`, *When* `SavePromptTemplate` is called, *Then* it returns a `connect.CodeInvalidArgument` error (slug validation fails before any file write) — this specific case never reaches the unrecognized-token check since it fails earlier; a *separately* well-named request with `scope: TEMPLATE_SCOPE_WORKSPACE, body: "test {{repoo}}"` instead succeeds, is written into the workspace directory (never the global one — `scopeFromProto`, Task 2.2.1b, is what makes this correct instead of an int-cast coin-flip), and returns `unrecognized_tokens: ["repoo"]` in the response.
- `SavePromptTemplate` returns `CodeAlreadyExists` on a slug collision unless the client explicitly opts into overwrite.
  - *Given* a template named "Dependency Audit" already exists at `<globalDir>/dependency-audit.md`, *When* `SavePromptTemplate(&SavePromptTemplateRequest{template: {name: "Dependency Audit", scope: TEMPLATE_SCOPE_GLOBAL, ...}, overwrite: false})` is called, *Then* it returns a `connect.CodeAlreadyExists` error and the existing file is untouched; *When* the same request is retried with `overwrite: true`, *Then* it succeeds and the file's content is replaced.

**Files**: `server/services/prompt_library_service.go`, `server/services/prompt_library_service_test.go`

##### Task 2.2.1a: Struct + constructor + compile-time interface check (~3 min)
- `type PromptLibraryService struct { cfg *config.Config }`; `func NewPromptLibraryService(cfg *config.Config) *PromptLibraryService`; `var _ sessionv1connect.PromptLibraryServiceHandler = (*PromptLibraryService)(nil)`.
- Files: `server/services/prompt_library_service.go`

##### Task 2.2.1b: `ListPromptTemplates` handler (~5 min)
- `// +api: prompts:list`. Resolve `globalDir, _ := s.cfg.PromptsDirOrDefault()`; resolve `workspaceDir` via `promptlibrary.WorkspacePromptsDir(req.Msg.Path)`, treating a resolution error as "no workspace templates" (log at Debug, not Warn — a non-git working directory is expected, not a problem) rather than failing the whole request. Call `promptlibrary.List`, translate `[]*Template` → `[]*sessionv1.PromptTemplate` and `[]ParseError` → `skipped_paths`/`skipped_count`.
- Files: `server/services/prompt_library_service.go`

##### Task 2.2.1c: `GetPromptTemplate` handler (~4 min)
- `// +api: prompts:get`. Resolve the correct directory from `req.Msg.Scope`, call `promptlibrary.List` (or a targeted single-file `Parse` by reconstructing `<dir>/<slug>.md`), return `CodeNotFound` if absent.
- Files: `server/services/prompt_library_service.go`

##### Task 2.2.1d: `SavePromptTemplate` handler with path-traversal + typo-warning wiring (~5 min)
- `// +api: prompts:save`. Slugify+`resolveAndValidatePath` the incoming `template.name` against the resolved target dir (global via `PromptsDirOrDefault`, or workspace via `WorkspacePromptsDir(req.Msg.Path)`, per `template.scope`) before calling `promptlibrary.Save`; on traversal failure, `connect.NewError(connect.CodeInvalidArgument, ...)`. After a successful save, call `promptlibrary.FindUnrecognizedTokens(template.body)` and populate `unrecognized_tokens` in the response (non-blocking, per Unresolved Questions §2).
- Files: `server/services/prompt_library_service.go`

##### Task 2.2.1e: Go tests for all 3 handlers (~5 min)
- `TestListPromptTemplates_should_ReturnUnionWithSkipInfo_When_OneFileMalformed`, `TestSavePromptTemplate_should_RejectTraversal_When_NameContainsDotDot`, `TestSavePromptTemplate_should_ReturnUnrecognizedTokens_When_BodyHasTypo`, `TestGetPromptTemplate_should_ReturnNotFound_When_SlugMissing`.
- Files: `server/services/prompt_library_service_test.go`

### Epic 2.3: Server Registration
**Goal**: Wire the handler into the ConnectRPC mux, unconditionally.

#### Story 2.3.1: `server/server.go` registration
**As an** operator, **I want** `PromptLibraryService` reachable at its ConnectRPC path with no feature flag, **so that** the feature ships without a rollout toggle (per Risk Control).
**Acceptance Criteria**:
- The service is registered and reachable.
  - *Given* the server is running (`make install-service` or a manual test instance per `CLAUDE.md`), *When* a ConnectRPC client calls `ListPromptTemplates`, *Then* it receives a `200`-equivalent successful response (empty `templates: []` on a fresh install, not a 404/`Unimplemented`).

**Files**: `server/server.go`

##### Task 2.3.1a: Register `PromptLibraryServiceHandler` (~3 min)
- Mirror `GitHubUserService`'s registration (`server/server.go:388-393`): construct `services.NewPromptLibraryService(deps.Config)`, `path, handler := sessionv1connect.NewPromptLibraryServiceHandler(promptLibService, ConnectOptions(deps.ErrorRegistry)...)`, `mux.Handle(path, handler)`, `log.Info("Registered PromptLibraryService handler", "path", path)`.
- Files: `server/server.go`

---

## Phase 3: CLI

### Epic 3.1: `prompts` Subcommand
**Goal**: A cobra subcommand for listing/showing templates without an RPC round-trip (same process, calls `promptlibrary/` directly — per `research/stack.md`'s `GetSessionCmd` precedent).

#### Story 3.1.1: `stapler-squad prompts list` / `stapler-squad prompts show <slug>`
**As a** terminal user, **I want** to list and inspect templates from the CLI, **so that** I can check what's available without opening the web UI.
**Acceptance Criteria**:
- `list` prints name, scope, and description for every discoverable template.
  - *Given* the "Dependency Audit" global template fixture exists and the CLI is run from `/home/tstapler/code/myrepo` (a git repo with no workspace templates), *When* `stapler-squad prompts list` runs, *Then* stdout includes a line containing `Dependency Audit`, `global`, and `Audit package.json / go.mod for outdated or vulnerable deps`.
- `show <slug>` prints the raw (non-interpolated) body.
  - *Given* the same fixture, *When* `stapler-squad prompts show dependency-audit` runs, *Then* stdout is exactly the template's body text, unmodified (no interpolation — the CLI has no session-creation context to interpolate against).

**Files**: `cmd/commands/prompts.go`, `main.go`

##### Task 3.1.1a: `PromptsListCmd` (~4 min)
- `var PromptsListCmd = &cobra.Command{Use: "list", RunE: func(...) error { ... }}` — resolve cwd via `os.Getwd()`, call `config.PromptsDirOrDefault()` + `promptlibrary.WorkspacePromptsDir(cwd)` (tolerate its error as "no workspace templates"), call `promptlibrary.List`, print a simple table (`name | scope | description`).
- Files: `cmd/commands/prompts.go`

##### Task 3.1.1b: `PromptsShowCmd` and parent `PromptsCmd` (~4 min)
- `var PromptsShowCmd = &cobra.Command{Use: "show <slug>", Args: cobra.ExactArgs(1), RunE: ...}` — reuse the same `List` call, find the matching template by slug (compute slug the same way `Save` does, or match on filename stem), print `.Body`. `var PromptsCmd = &cobra.Command{Use: "prompts"}`; `PromptsCmd.AddCommand(PromptsListCmd, PromptsShowCmd)`.
- Files: `cmd/commands/prompts.go`

##### Task 3.1.1c: Register in `main.go` (~2 min)
- `rootCmd.AddCommand(commands.PromptsCmd)` alongside the existing `rootCmd.AddCommand(commands.GetSessionCmd)`-style registrations (`main.go:711-714` area).
- Files: `main.go`

##### Task 3.1.1d: CLI integration tests (~4 min)
- `TestPromptsListCmd_should_PrintFixtureTemplate_When_GlobalDirPopulated`, `TestPromptsShowCmd_should_PrintRawBody_When_SlugExists`, using `STAPLER_SQUAD_TEST_DIR` to isolate fixture directories.
- Files: `cmd/commands/prompts_test.go`

---

## Phase 4: Frontend Data Layer

### Epic 4.1: `usePromptService` Hook
**Goal**: A dedicated ConnectRPC client hook, kept separate from `useSessionService.ts`.

#### Story 4.1.1: `usePromptService.ts`
**As a** frontend developer, **I want** `listTemplates`, `getTemplate`, `saveTemplate` functions backed by the generated `PromptLibraryService` client, **so that** `TemplatePicker`/`SaveAsTemplateModal` don't each hand-roll ConnectRPC calls.
**Acceptance Criteria**:
- `listTemplates(path)` resolves to the RPC response's `templates` array plus skip info.
  - *Given* the backend returns `{templates: [{name: "Dependency Audit", ...}], skippedCount: 0}` for `path="/home/tstapler/code/myrepo"`, *When* `usePromptService().listTemplates("/home/tstapler/code/myrepo")` resolves, *Then* the hook's returned promise yields `{templates: [...], skippedCount: 0}` with proto field names converted to camelCase per the existing codegen convention.

**Files**: `web-app/src/lib/hooks/usePromptService.ts`

##### Task 4.1.1a: Create the hook shell + `createClient` wiring (~4 min)
- Mirror `useSessionService.ts`'s top (`createClient(PromptLibraryService, transport)`, reusing `getApiBaseUrl`/`createAuthInterceptor` from `@/lib/config`), export `function usePromptService(options?: { baseUrl?: string })`.
- Files: `web-app/src/lib/hooks/usePromptService.ts`

##### Task 4.1.1b: `listTemplates`/`getTemplate`/`saveTemplate` methods (~5 min)
- Three `useCallback`-wrapped async functions calling the generated client methods, returning typed results; errors surfaced via thrown `ConnectError` (caller's responsibility to catch, matching `useSessionService.ts`'s existing error-handling convention).
- Files: `web-app/src/lib/hooks/usePromptService.ts`

##### Task 4.1.1c: Jest tests for the hook (~5 min)
- `usePromptService_should_ReturnTemplates_When_ListSucceeds`, `usePromptService_should_ThrowConnectError_When_SaveRejected`, using a mocked transport (matching `useSessionService.test.ts`'s existing mock pattern if present, else a minimal `createRouterTransport` fixture).
- Files: `web-app/src/lib/hooks/usePromptService.test.ts`

### Epic 4.2: Client-Side Interpolation
**Goal**: The TS-side counterpart of `Interpolate`, since interpolation happens client-side (Approach A) to avoid a round-trip per selection.

#### Story 4.2.1: `interpolateTemplate.ts`
**As a** picker component, **I want** a pure TS function mirroring the Go `Interpolate` semantics, **so that** selecting a template substitutes variables instantly with no network call.
**Acceptance Criteria**:
- Same literal/blank behavior as the Go implementation (AC3, Unresolved Questions §2).
  - *Given* `body = "Review {{branch}} of {{repo}}: {{issue_title}}"` and `vars = {repo: "stapler-squad", branch: "fix-flaky-test"}` (no `issue_title`), *When* `interpolateTemplate(body, vars)` runs, *Then* it returns `"Review fix-flaky-test of stapler-squad: "`.

**Files**: `web-app/src/lib/omnibar/interpolateTemplate.ts`

##### Task 4.2.1a: Implement `interpolateTemplate(body: string, vars: Partial<Record<"repo"|"branch"|"issue_title", string>>): string` (~4 min)
- `const RECOGNIZED = ["repo", "branch", "issue_title"] as const;` build a regex-replace pass over exactly those 3 tokens, substituting `vars[key] ?? ""`, leaving anything else untouched.
- Files: `web-app/src/lib/omnibar/interpolateTemplate.ts`

##### Task 4.2.1b: Jest tests (~4 min)
- `interpolateTemplate_should_RenderEmptyString_When_VariableUnavailable`, `interpolateTemplate_should_LeaveUnrecognizedTokenLiteral_When_TokenIsTypo`, `interpolateTemplate_should_MatchGoImplementation_When_GivenSameFixture` (a shared fixture table asserting parity with the Go test fixtures from Task 1.3.1b, documented as "known-limitation: {{issue_title}} is near-always blank in v1 since no omnibar detector currently resolves an issue/PR title — see `research/stack.md`").
- Files: `web-app/src/lib/omnibar/interpolateTemplate.test.ts`

---

## Phase 5: Template Picker UI

### Epic 5.1: `TemplatePicker` Component
**Goal**: A searchable, tag-filterable, accessible palette adapting `QuickOpenPalette.tsx`/`AliasPalette.tsx`.

#### Story 5.1.1: Base combobox structure
**As a** user, **I want** a keyboard- and screen-reader-navigable list of templates, **so that** I can find and select one without a mouse.
**Acceptance Criteria**:
- ARIA roles match the `AliasPalette.tsx` combobox variant (input keeps DOM focus, `aria-activedescendant`), per UX research.
  - *Given* the picker is open with 3 templates loaded, *When* the user presses `ArrowDown` twice then `Enter`, *Then* the 2nd row's `onSelect` fires (verified via `aria-activedescendant` pointing at the 2nd row's `id` before `Enter`, matching `AliasPalette.tsx:63,103`'s pattern), and `scrollIntoView({block: "nearest"})` is called on the active row.

**Files**: `web-app/src/components/sessions/TemplatePicker.tsx`, `web-app/src/components/sessions/TemplatePicker.css.ts`

##### Task 5.1.1a: `TemplatePicker.css.ts` — vanilla-extract styles (~4 min)
- `style()`/`recipe()` for the container, search input, listbox, row (default/active/selected states), scope badge, tag chip — import tokens from `web-app/src/styles/theme.css.ts`'s `vars`, per `.claude/rules/css-architecture.md`. No hardcoded hex values, no `var(--undefined-token)`.
- Files: `web-app/src/components/sessions/TemplatePicker.css.ts`

##### Task 5.1.1b: Component shell + props (~4 min)
- `interface TemplatePickerProps { templates: PromptTemplate[]; skippedCount: number; onSelect: (tpl: PromptTemplate) => void; onClose: () => void; }`; `function TemplatePicker(props: TemplatePickerProps)` — local `query`, `activeIndex`, `selectedTags` state.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.1c: Search input + listbox markup with ARIA wiring (~5 min)
- `<input aria-label="Search templates" autoComplete="off" aria-activedescendant={...} />` (`QuickOpenPalette.tsx:225-236` precedent for the label/autocomplete attrs; `AliasPalette.tsx:63,103` precedent for `aria-activedescendant`); `<div role="listbox" aria-label="Templates">`; each row `<div role="option" aria-selected={...} id={rowId(i)}>`.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.1d: Keyboard handling (~5 min)
- `ArrowUp`/`ArrowDown` move `activeIndex` (clamped, wrap disabled), `Enter` commits, `Escape` calls `onClose()`. `useEffect` scrolling the active row into view via `scrollIntoView({block: "nearest"})` on `activeIndex` change.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.1e: Row selection via `onMouseDown` + `preventDefault` (~2 min)
- Per `SlashCommandDropdown.tsx:67-70`'s precedent — prevents the textarea's blur from closing the popover before the click registers.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

#### Story 5.1.2: Search (fuse.js) + tag filter
**As a** user, **I want** free-text search and tag chips, **so that** I can narrow a large template list quickly (AC4).
**Acceptance Criteria**:
- Free-text search matches `name`/`description`.
  - *Given* templates `["Dependency Audit" (tags: maintenance, security), "PR Review Pass" (tags: review), "Test Generator" (tags: testing)]`, *When* the user types `"audit"` into the search box, *Then* only "Dependency Audit" remains visible in the listbox.
- Tag chips filter by exact-set match and toggle.
  - *Given* the same 3 templates, *When* the user clicks the `"security"` tag chip, *Then* only "Dependency Audit" remains visible; *When* the user clicks `"security"` again, *Then* all 3 templates are visible again.

**Files**: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.2a: Wire `fuse.js` over `name`+`description` (~4 min)
- `new Fuse(templates, { keys: ["name", "description"], threshold: 0.4 })` (matching `useSessionSearch.ts`'s existing options shape), recompute `Fuse` results from `query` via `useMemo`.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.2b: Tag chip row (`role="group"`, `aria-pressed`) (~4 min)
- Derive the unique tag set from `templates`, render chip buttons per `LevelFilterChips.tsx:25-35`'s pattern (`role="group"`, each `<button aria-pressed={selected}>`), toggle membership in `selectedTags` (a `Set<string>`) on click.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.2c: Combine search + tag filter into the rendered row list (~3 min)
- `visibleTemplates = fuseFilteredTemplates.filter(t => selectedTags.size === 0 || [...selectedTags].every(tag => t.tags.includes(tag)))`.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.2d: Jest tests for search/filter (~5 min)
- `TemplatePicker_should_FilterByQuery_When_UserTypesAudit`, `TemplatePicker_should_FilterByTag_When_ChipClicked`, `TemplatePicker_should_RestoreFullList_When_ChipToggledOff`.
- Files: `web-app/src/components/sessions/__tests__/TemplatePicker.test.tsx`

#### Story 5.1.3: Scope badge, skip notice, empty state
**As a** user, **I want** to see which templates are global vs. workspace-scoped, and to know when templates failed to load, **so that** I trust the picker's completeness (AC1, AC2, AC6).
**Acceptance Criteria**:
- Each row shows a visible scope badge.
  - *Given* "Dependency Audit" has `scope: TEMPLATE_SCOPE_GLOBAL` and "PR Review Pass" has `scope: TEMPLATE_SCOPE_WORKSPACE`, *When* the picker renders, *Then* the "Dependency Audit" row shows a "Global" badge and the "PR Review Pass" row shows a "Workspace" badge, both visible without hovering (mobile-safe, per UX research).
- A non-zero skip count surfaces a low-key notice.
  - *Given* `skippedCount = 2`, *When* the picker renders, *Then* a `role="status" aria-live="polite"` element reads "2 templates couldn't be loaded — check ~/.stapler-squad/prompts/".
- Zero templates shows actionable empty-state copy, not a blank panel.
  - *Given* `templates = []` and `skippedCount = 0`, *When* the picker renders, *Then* a `role="status" aria-live="polite"` row reads "Drop a .md file in ~/.stapler-squad/prompts/, or save one from an existing session's prompt."

**Files**: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.3a: Scope badge rendering (~3 min)
- Small `<span className={scopeBadge({scope})}>` per row using the `.css.ts` recipe from Task 5.1.1a.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.3b: Skip-count and empty-state notices (~4 min)
- Two conditionally-rendered `role="status" aria-live="polite"` blocks per the acceptance criteria copy above.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.3c: Jest tests (~4 min)
- `TemplatePicker_should_ShowScopeBadge_When_TemplateIsWorkspaceScoped`, `TemplatePicker_should_ShowSkipNotice_When_SkippedCountNonZero`, `TemplatePicker_should_ShowEmptyStateCopy_When_NoTemplatesAndNoSkips`.
- Files: `web-app/src/components/sessions/__tests__/TemplatePicker.test.tsx`

#### Story 5.1.4: Mobile bottom-sheet variant
**As a** mobile user, **I want** the picker to render as a full-screen sheet rather than an anchored popover, **so that** the on-screen keyboard doesn't clip it (mobile+desktop UX requirement).
**Acceptance Criteria**:
- Below the mobile breakpoint, the picker renders inside `Modal.tsx` as a bottom sheet.
  - *Given* `useIsMobile()` returns `true` (viewport < breakpoint), *When* the "From template" button is clicked, *Then* `TemplatePicker` renders wrapped in a full-screen `Modal.tsx` instance rather than an anchored popover, and all tag chips/rows meet the ≥44×44px touch-target minimum.

**Files**: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.4a: Branch on `useIsMobile()` from `ViewportProvider.tsx` (~4 min)
- `const isMobile = useIsMobile();` conditionally wrap the picker's render output in `<Modal fullScreen>` vs. an anchored `<div>` popover, matching the branching style already used in `Header.tsx`/`CockpitShell.tsx`.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.1.4b: Touch-target sizing in `.css.ts` (~3 min)
- Ensure chip/row `minHeight`/`minWidth` ≥ `44px` under the mobile media-query branch in `TemplatePicker.css.ts`.
- Files: `web-app/src/components/sessions/TemplatePicker.css.ts`

---

### Epic 5.2: Omnibar Integration
**Goal**: Wire the picker into `OmnibarCreationPanel.tsx`'s First Prompt field, resolving the UX open question.

#### Story 5.2.1: "From template" button + picker open/close
**As a** user creating a session, **I want** a "From template" affordance next to the First Prompt textarea, **so that** I can open the picker without leaving the form.
**Acceptance Criteria**:
- Clicking "From template" opens the picker with data already loaded.
  - *Given* the Omnibar form has `path` resolved to `/home/tstapler/code/myrepo`, *When* the user clicks "From template" next to the First Prompt field (adjacent to the existing textarea at `OmnibarCreationPanel.tsx:721-760`), *Then* `usePromptService().listTemplates("/home/tstapler/code/myrepo")` fires and the picker renders with the returned templates.

**Files**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 5.2.1a: Add the button and picker-open state (~4 min)
- Near the existing First Prompt block (`OmnibarCreationPanel.tsx:721-760`), add a small button (`<button onClick={() => setPickerOpen(true)}>From template</button>`) and `const [pickerOpen, setPickerOpen] = useState(false)`.
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 5.2.1b: Fetch templates on open (~4 min)
- `useEffect` (or on-click handler) calling `usePromptService().listTemplates(formState.path)` when `pickerOpen` becomes `true`; store result in local state; pass to `<TemplatePicker>`.
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

#### Story 5.2.2: Select → interpolate → populate, with overwrite guard
**As a** user, **I want** selecting a template to safely populate my First Prompt field, **so that** I never lose in-progress typed content by accident, and never accidentally auto-submit a session (AC3, Unresolved Questions §1, pitfalls.md's hard "never auto-submit" rule).
**Acceptance Criteria**:
- Empty field: selection applies immediately.
  - *Given* `formState.firstPrompt === ""` and the Omnibar's resolved context is `{repo: "stapler-squad", branch: "fix-flaky-test"}` (no GitHub ref), *When* the user selects "Dependency Audit" (body `"Run a full dependency audit on {{repo}} and file findings as backlog items."`), *Then* `firstPrompt` becomes `"Run a full dependency audit on stapler-squad and file findings as backlog items."` immediately, the picker closes, and the session is NOT submitted — the Create button still requires an explicit separate click.
- Non-empty field: selection requires one explicit confirm.
  - *Given* `formState.firstPrompt === "some notes I already typed"`, *When* the user selects "Dependency Audit", *Then* the picker shows a "Replace current draft" confirm control instead of immediately overwriting; *When* the user then clicks "Replace" (or presses Enter again), *Then* `firstPrompt` is overwritten with the interpolated body.

**Files**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`, `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.2.2a: Derive interpolation vars from Omnibar form state (~4 min)
- `const vars = { repo: basename(formState.path), branch: formState.branch, issue_title: formState.gitHubRef?.title }` — since `GitHubRef` has no `title` field today (confirmed: `web-app/src/lib/omnibar/types.ts:108-113`), `issue_title` is always `undefined` for v1; document this inline as the known, accepted limitation from `research/stack.md`, not a bug.
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 5.2.2b: `handleTemplateSelect` with the overwrite guard (~5 min)
- `function handleTemplateSelect(tpl: PromptTemplate) { const interpolated = interpolateTemplate(tpl.body, vars); if (formState.firstPrompt.trim() === "") { setFormField("firstPrompt", interpolated); setPickerOpen(false); } else { setPendingTemplate({ tpl, interpolated }); } }` — a second confirm handler applies `pendingTemplate.interpolated` and clears `pendingTemplate`/closes the picker.
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 5.2.2c: "Replace current draft" confirm UI in the picker (~4 min)
- When `pendingTemplate` is set, render a footer row in `TemplatePicker` with "Replace current draft" (button) and a live-updating diff/preview hint (reusing the body text) so the user can see what would replace their draft before confirming.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 5.2.2d: Jest tests for selection behavior (~5 min)
- `OmnibarCreationPanel_should_ApplyImmediately_When_FirstPromptEmpty`, `OmnibarCreationPanel_should_RequireConfirm_When_FirstPromptHasContent`, `OmnibarCreationPanel_should_NeverAutoSubmit_When_TemplateSelected` (asserts no `createSession` call fires as a side effect of selection alone).
- Files: `web-app/src/components/sessions/__tests__/OmnibarCreationPanel.test.tsx`

---

## Phase 6: Save-as-Template UI

### Epic 6.1: `SaveAsTemplateModal`
**Goal**: A form to capture name/description/tags/scope and write a new template (AC5), following `AliasesManager.tsx`'s CRUD-form precedent and surfacing the non-blocking typo warning.

#### Story 6.1.1: Modal form structure
**As a** user with an existing session's initial prompt, **I want** a "Save as template" button that opens a form, **so that** I can turn that prompt into a reusable template.
**Acceptance Criteria**:
- Clicking "Save as template" opens `Modal.tsx` with name/description/tags/scope fields prefilled with the session's initial-prompt body.
  - *Given* an active session whose initial prompt is `"Run a full dependency audit on stapler-squad and file findings as backlog items."`, *When* the user clicks "Save as template" on that session's prompt panel, *Then* a `Modal.tsx`-based dialog opens with an empty Name field, an empty Description field, an empty Tags field, a Scope radio (default: Global), and a read-only preview of the body text (the session's initial prompt, unmodified).

**Files**: `web-app/src/components/sessions/SaveAsTemplateModal.tsx`, `web-app/src/components/sessions/SaveAsTemplateModal.css.ts`

##### Task 6.1.1a: `SaveAsTemplateModal.css.ts` (~3 min)
- vanilla-extract styles for the form fields, mobile-stacked layout below the breakpoint (per UX research), using `vars` from the shared theme contract.
- Files: `web-app/src/components/sessions/SaveAsTemplateModal.css.ts`

##### Task 6.1.1b: Component shell with `Modal.tsx` (~4 min)
- `interface SaveAsTemplateModalProps { body: string; open: boolean; onClose: () => void; onSaved: (tpl: PromptTemplate) => void; }`; wraps `<Modal open={open} onClose={onClose}>` (Radix Dialog per `.claude/rules/css-architecture.md`'s "never a bespoke portal" guidance).
- Files: `web-app/src/components/sessions/SaveAsTemplateModal.tsx`

##### Task 6.1.1c: Form fields (name, description, tags, scope) (~5 min)
- Controlled inputs for `name`/`description`, a simple comma-or-Enter-delimited tags input (matching `AliasesManager.tsx`'s existing tag-entry pattern if present, else a minimal chip-add input), a radio group for `scope: "global" | "workspace"` (disable "workspace" with a tooltip if the current session has no resolvable workspace root, e.g. a one-off session).
- Files: `web-app/src/components/sessions/SaveAsTemplateModal.tsx`

#### Story 6.1.2: Save wiring, slug preview, typo warning
**As a** user, **I want** to see the exact filename my template will be saved as and be warned about typo'd variables before I submit, **so that** I catch mistakes before they become dead text in a saved template.
**Acceptance Criteria**:
- Submitting calls `saveTemplate` and writes the file (AC5).
  - *Given* the form is filled with Name="Dependency Audit", Description="Audit deps for vulnerabilities", Tags=["maintenance", "security"], Scope=Global, *When* the user clicks "Save", *Then* `usePromptService().saveTemplate(...)` is called with a `PromptTemplate` matching those fields, and on success the modal closes and calls `onSaved(savedTemplate)`.
- A typo'd token produces a visible, non-blocking warning.
  - *Given* the body (from the session's initial prompt) contains `{{repoo}}`, *When* the user clicks "Save", *Then* the RPC still succeeds (per Unresolved Questions §2) and the modal shows a dismissible warning: "`{{repoo}}` won't be replaced — did you mean `{{repo}}`?" sourced from `SavePromptTemplateResponse.unrecognizedTokens`.

**Files**: `web-app/src/components/sessions/SaveAsTemplateModal.tsx`

##### Task 6.1.2a: Live slug preview (~3 min)
- Derive a client-side slug preview (same slugify rule as `promptlibrary.slugify` — a small duplicated pure function `slugifyPreview(name: string): string` in the component, documented as "must match `promptlibrary/save.go`'s `slugify` — server is authoritative, this is preview-only") shown under the Name field as "Will save as: `dependency-audit.md`".
- Files: `web-app/src/components/sessions/SaveAsTemplateModal.tsx`

##### Task 6.1.2b: Submit handler calling `saveTemplate` (~4 min)
- `async function handleSubmit() { const result = await savePrompt.saveTemplate({...}); if (result.unrecognizedTokens.length > 0) { setWarning(...); } else { onClose(); } onSaved(result.saved); }` — form stays open (not auto-closed) when a warning is present, closes automatically when there is none.
- Files: `web-app/src/components/sessions/SaveAsTemplateModal.tsx`

##### Task 6.1.2c: Wire the "Save as template" button into the session's prompt panel (~4 min)
- Add the trigger button wherever the existing session's initial-prompt text is displayed (locate via the session detail view that already shows the first prompt — same surface `PromptStore`'s "recent prompts" reads from, per research/features.md — labeled distinctly as "Save as template" to avoid the naming collision called out in the Domain Glossary).
- Files: `web-app/src/components/sessions/SaveAsTemplateModal.tsx`, and the session-detail component that hosts it (exact host component confirmed at implementation time by locating the current initial-prompt display — flag for the implementing subagent to `grep -rn "firstPrompt\|initialPrompt" web-app/src/components/sessions/` if not already obvious from context)

##### Task 6.1.2d: Jest tests (~5 min)
- `SaveAsTemplateModal_should_CallSaveTemplate_When_FormSubmitted`, `SaveAsTemplateModal_should_ShowSlugPreview_When_NameTyped`, `SaveAsTemplateModal_should_ShowTypoWarning_When_ResponseHasUnrecognizedTokens`, `SaveAsTemplateModal_should_DisableWorkspaceScope_When_NoWorkspaceRoot`.
- Files: `web-app/src/components/sessions/__tests__/SaveAsTemplateModal.test.tsx`

---

## Phase 7: Registry, Tests, Docs

### Epic 7.1: Feature Registry (AC7)
**Goal**: Per-feature JSON files for the 3 new RPCs and the new UI feature, per `.claude/rules/feature-registry.md`.

#### Story 7.1.1: Registry entries
**Acceptance Criteria**:
- All 4 new feature files exist and `make registry-generate` reflects them with no net increase in `coverage-gaps.json`'s count once Phase 7's test tasks land.
  - *Given* `// +api: prompts:list` / `prompts:get` / `prompts:save` markers exist in `server/services/prompt_library_service.go` (Tasks 2.2.1b-d) and a `// +feature: prompt-template-picker` marker exists in `TemplatePicker.tsx`'s first 10 lines, *When* `make registry-generate` runs, *Then* `docs/registry/features/backend/list-prompt-templates.json`, `get-prompt-template.json`, `save-prompt-template.json` (each `markerFound: true`) and `docs/registry/features/frontend/prompt-template-picker.json` all exist, and after Task 7.2.1/7.3.1's tests are added and `testIds` populated, each entry's `tested` flips to `true`.

**Files**: `docs/registry/features/backend/list-prompt-templates.json`, `docs/registry/features/backend/get-prompt-template.json`, `docs/registry/features/backend/save-prompt-template.json`, `docs/registry/features/frontend/prompt-template-picker.json`

##### Task 7.1.1a: Add `// +feature: prompt-template-picker` marker to `TemplatePicker.tsx` (~2 min)
- Add the marker comment in the file's first 10 lines, per the registry rule's requirement.
- Files: `web-app/src/components/sessions/TemplatePicker.tsx`

##### Task 7.1.1b: Run `make registry-generate`, verify diff (~3 min)
- Run `make registry-generate`; confirm the 4 new per-feature JSON files appear under `docs/registry/features/backend/` and `docs/registry/features/frontend/`, `markerFound: true` on each backend entry; commit the generated files alongside the aggregated ones.
- Files: `docs/registry/features/backend/*.json`, `docs/registry/features/frontend/*.json`, and the generated aggregate files under `docs/registry/`

##### Task 7.1.1c: Set `tested: true` + `testIds` after Phase 7.2/7.3 land (~3 min)
- Once Go and Jest tests exist (Stories 7.2.1, 7.3.1), edit each of the 4 per-feature files to set `"tested": true` and populate `"testIds"` with the actual test function names, and bump `"lastModified"`.
- Files: `docs/registry/features/backend/*.json`, `docs/registry/features/frontend/prompt-template-picker.json`

### Epic 7.2: Backend Test Sweep (AC8)
**Goal**: Confirm the full Go test suite for this feature is present and passing (individual test tasks already specified per-story above — this epic is the consolidation/verification pass).

#### Story 7.2.1: `go test` sweep
**Acceptance Criteria**:
- All `promptlibrary/` and `server/services/prompt_library_service_test.go` tests pass.
  - *Given* every unit test task from Phases 1-2 has been implemented, *When* `go test ./promptlibrary/... ./server/services/... ./cmd/commands/... ./config/...` runs, *Then* it exits 0 with no failing or skipped tests related to this feature.

**Files**: (verification only — no new files; touches all Go test files listed in Phases 1-3)

##### Task 7.2.1a: Run `make build && make test`, fix any failures (~5 min)
- Per `CLAUDE.md`'s standard workflow — build (regenerates protos) then run the full test suite; fix anything red before proceeding.
- Files: (as needed)

### Epic 7.3: Frontend Test Sweep (AC8)
#### Story 7.3.1: `jest` sweep
**Acceptance Criteria**:
- All new Jest suites pass with no coverage regression.
  - *Given* every Jest test task from Phases 4-6 has been implemented, *When* `cd web-app && npx jest --no-coverage --testPathPatterns="TemplatePicker|SaveAsTemplateModal|usePromptService|interpolateTemplate|OmnibarCreationPanel"` runs, *Then* it exits 0.

**Files**: (verification only)

##### Task 7.3.1a: Run the targeted Jest sweep, fix failures (~4 min)
- Files: (as needed)

### Epic 7.4: E2E Test (AC8)
#### Story 7.4.1: `tests/e2e/prompt-library.spec.ts`
**As a** CI pipeline, **I want** one end-to-end test covering the picker's happy path, **so that** the full stack (backend file scan → RPC → frontend render → interpolation → field population) is exercised together.
**Acceptance Criteria**:
- Dropping a fixture template into the isolated test server's prompts dir and selecting it via the UI populates the First Prompt field with interpolated content.
  - *Given* the e2e global setup (`tests/e2e/global-setup.ts`) has started an isolated server with `STAPLER_SQUAD_TEST_DIR=/tmp/stapler-squad-test-<pid>`, and the test writes a fixture file to `/tmp/stapler-squad-test-<pid>/prompts/dependency-audit.md` (frontmatter: name "Dependency Audit", description "Audit package.json / go.mod for outdated or vulnerable deps", tags [maintenance, security], body `"Run a full dependency audit on {{repo}}."`) before navigating, *When* the test opens the New Session omnibar, clicks "From template", and selects "Dependency Audit" (`page.getByRole("option", { name: /Dependency Audit/ })`), *Then* `expect(page.getByTestId("first-prompt-textarea")).toHaveValue(/Run a full dependency audit on .+\./)` passes (exact repo name depends on the test fixture's working directory, hence the regex) and no `waitForTimeout` is used anywhere in the spec.

**Files**: `tests/e2e/prompt-library.spec.ts`

##### Task 7.4.1a: Write the spec with feature annotation header (~5 min)
- `// @feature prompts:list, prompts:save, prompt-template-picker` as the first line, per `.claude/rules/e2e-test-conventions.md`. Use `data-testid`/ARIA-role locators only, no `waitForTimeout`.
- Files: `tests/e2e/prompt-library.spec.ts`

##### Task 7.4.1b: Add a `data-testid="first-prompt-textarea"` to the existing textarea if not already present (~2 min)
- Check `OmnibarCreationPanel.tsx`'s existing First Prompt `<textarea>` (`OmnibarCreationPanel.tsx:728-736`) for an existing test id; add one if missing.
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 7.4.1c: Run the spec locally, verify green (~5 min)
- `cd tests/e2e && npx playwright test prompt-library.spec.ts`; fix any locator/timing issues found.
- Files: (as needed)

##### Task 7.4.1d: Second e2e scenario — malformed template does not break the picker (AC6) (~4 min)
- Extend the same spec (or add a second `test()` block) that also drops a malformed fixture (`broken.md`, no frontmatter) alongside the valid one, opens the picker, and asserts both that "Dependency Audit" is still selectable AND that the skip notice (`page.getByRole("status")` containing "couldn't be loaded") is visible.
- Files: `tests/e2e/prompt-library.spec.ts`
