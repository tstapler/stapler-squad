# Architecture Research: Prompt Template Library

Research Agent 3 (Architecture). No prior hotspot/architecture-review analysis exists for
this area (checked `project_plans/*/research/architecture.md` for "template" — no hits).
Fresh research below, grounded in direct repo reads.

## EventStorming table: not applicable

This feature is fundamentally CRUD-ish (read markdown files from disk, parse frontmatter,
interpolate strings, write a file on save) with a single actor (the interactive user) and no
multi-step business process, saga, or cross-actor workflow. There is no Event/Command/Policy
table in this document — it would document nothing beyond "user clicks button, file is
written." Skip it per the requirements doc's own note (`requirements.md` line 82-85).

---

## 1. Where the new service/package lives

### Business logic package: `promptlibrary/` (new, repo root, parallel to `session/`, `config/`, `github/`)

Follow the same split the codebase already uses for GitHub: `github/` holds all
filesystem/network/parsing logic as concrete types with plain constructors, and
`server/services/github_user_service.go` is a thin ConnectRPC adapter that wraps a
`github/` type (`githubpkg.UserPRCache`) and translates to/from proto messages. Confirmed by
reading `server/services/github_user_service.go:1-46` — the service struct embeds a
`*githubpkg.UserPRCache` and just fans calls out to it inside `+api:`-annotated handler
methods.

Apply the same shape:

- `promptlibrary/template.go` — `Template` struct (Name, Description, Tags, Body, Scope,
  Path), `Parse(path string) (*Template, error)` that splits YAML frontmatter (delimited by
  `---`) from the markdown body using `gopkg.in/yaml.v3` (already a **direct** dependency —
  confirmed in `go.mod:48`, no new dependency needed).
- `promptlibrary/library.go` — `List(globalDir, workspaceDir string) ([]*Template, []error)`
  (re-scans both dirs, merges, returns parse errors separately so the caller can log-and-skip
  per malformed file rather than fail the whole listing — satisfies acceptance criterion 6).
- `promptlibrary/interpolate.go` — `Interpolate(body string, vars map[string]string) string`,
  simple `{{key}}` substitution, undefined key → empty string (per requirements.md's chosen
  behavior over the leave-as-literal alternative).
- `promptlibrary/save.go` — `Save(dir string, tpl *Template) error`, writes a well-formed
  frontmatter+body file.

Use **concrete types, not a speculative interface** — there's exactly one implementation
(local filesystem), no second backend is planned, and the repo's own
`.claude/rules/interface-pollution-checklist.md` calls this out explicitly ("an interface
with exactly one implementation and no near-term second one" is smell #1). If
`server/services` ever needs to mock this for a test, it can construct a real
`promptlibrary.Library` pointed at a `t.TempDir()` — no interface required. If a consumer
package genuinely needs to swap implementations later, define a narrow interface *there*
(interface-pollution-checklist rule #2: define at the consumer, not next to the
implementation).

### ConnectRPC adapter: `server/services/prompt_library_service.go` (new)

Same shape as `GitHubUserService`: a struct wrapping a `*promptlibrary.Library`, with
`+api:`-annotated methods `ListPromptTemplates`, `GetPromptTemplate`, `SavePromptTemplate`.
Compile-time interface check line (`var _ sessionv1connect.PromptLibraryServiceHandler = (*PromptLibraryService)(nil)`)
matches the existing convention (`github_user_service.go:20`).

---

## 2. Path resolution — reuse existing helpers, don't hand-roll home-dir expansion

### Global dir: follow the `Config.<X>DirOrDefault()` pattern

There is **no single `config.BaseDir()` helper** — I checked and confirmed every
`~/.stapler-squad/<subdir>` resolver in `config/config.go` independently calls
`os.UserHomeDir()` and joins `.stapler-squad/<name>`. Examples read directly:

```go
// config/config.go:539-545
func (c *Config) TriageArtifactDirOrDefault() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand home dir: %w", err)
	}
	return filepath.Join(home, ".stapler-squad", "triage-artifacts"), nil
}

// config/config.go:551-557 — same shape
func (c *Config) BacklogAttachmentDirOrDefault() (string, error) { ... }
```

Add `PromptsDirOrDefault() (string, error)` to `config/config.go` following this exact
template (`filepath.Join(home, ".stapler-squad", "prompts")`), rather than resolving
`os.UserHomeDir()` inline in `promptlibrary/`. This keeps the "how do we find
`~/.stapler-squad/*`" logic centralized in `config/` as the codebase already does, and gives
future config-driven overrides (like `CheckpointDir`/`NewProjectBaseDir` already support) a
single place to add an override field later if needed — YAGNI says don't add the override
field now, just match the method shape so it's a trivial follow-up.

### Workspace-local dir: reuse `git.PlainOpenWithOptions(..., DetectDotGit: true)` — do NOT walk manually

`session/git/util.go:79` (`findGitRepoRoot`) and every function in `session/git/ops.go` (8
call sites) resolve a repo root via go-git's `git.PlainOpenWithOptions(path,
&git.PlainOpenOptions{DetectDotGit: true})`, per `.claude/rules/prefer-go-git-over-subshells.md`.
`DetectDotGit: true` walks up parent directories looking for `.git`, which is exactly the
"repo root of the working directory a session is created against" behavior the requirements
call for (requirements.md's non-functional note: "workspace" means repo root, not the
specific worktree).

Recommendation: add a small helper (either exported from `session/git` or a thin wrapper in
`promptlibrary/`) — `WorkspacePromptsDir(startPath string) (string, error)` — that opens the
repo via `PlainOpenWithOptions(startPath, DetectDotGit: true)`, gets the worktree root via
`repo.Worktree()` (or equivalently the resolved `.git` directory's parent), then joins
`.stapler-squad/prompts`. Do not hand-roll a manual `filepath.Dir` walk-up loop — go-git
already does this and it's the pattern this repo has standardized on. Note the requirements'
git-worktree caveat again: because `DetectDotGit` walks to the *main* repo's `.git` (worktrees
share the parent repo's git dir metadata), this naturally resolves to the main repo root
regardless of which worktree/branch the new session targets — matching the desired behavior
without extra logic.

---

## 3. Proto / RPC integration

### No new field on `CreateSessionRequest` — template selection is client-side prefill only

Read `proto/session/v1/session.proto:472-531`. `CreateSessionRequest` already has:
- `string path = 2` (workspace root — usable to derive `{{repo}}` client-side, e.g. basename)
- `string branch = 4` (usable directly for `{{branch}}`)
- `string initial_prompt = 15` — "prompt typed into the tmux pane as simulated keystrokes
  once the session reaches Ready state" — this is the field the interpolated template body
  should populate, alongside the existing `string prompt = 7` (CLI-arg variant) depending on
  which creation path is active. Both already exist and are wired end-to-end.

Recommendation (YAGNI, per the requirements doc's own framing of this as an open question):
**no new `CreateSessionRequest` field is needed.** Template selection happens entirely in the
frontend — the picker fills the existing prompt textarea with the already-interpolated text,
and the existing `CreateSession` RPC call is unchanged from that point on. This also means
`.claude/rules/session-creation-registry.md`'s 7-touchpoint registry is **not** triggered
(confirmed correct per requirements.md acceptance criterion 9 — this is a content change to
an existing field, not a new `SessionType`).

### New proto file: `proto/session/v1/prompt_library.proto`

Follow `proto/session/v1/github_user.proto` (169 lines, self-contained request/response
messages + one service) as the template rather than adding RPCs into the already-large
`session.proto`. Confirmed convention: `backlog.proto`, `insights.proto`, `headless.proto`,
`unfinished.proto`, `github_user.proto` are all separate files sharing the same `session.v1`
package and all compiling into the *same* generated Go package
`gen/proto/go/session/v1/sessionv1connect` (verified: `ls
gen/proto/go/session/v1/sessionv1connect/` shows one `*.connect.go` per source `.proto` file,
e.g. `github_user.connect.go` from `github_user.proto`). A new `prompt_library.proto` would
generate `prompt_library.connect.go` into that same package — no new Go package/import path
to wire up beyond the existing `sessionv1connect` import already used everywhere.

Proposed service:

```protobuf
service PromptLibraryService {
  rpc ListPromptTemplates(ListPromptTemplatesRequest) returns (ListPromptTemplatesResponse);
  rpc GetPromptTemplate(GetPromptTemplateRequest) returns (GetPromptTemplateResponse);
  rpc SavePromptTemplate(SavePromptTemplateRequest) returns (SavePromptTemplateResponse);
}
```

`ListPromptTemplatesRequest` should carry the workspace `path` (same semantics as
`CreateSessionRequest.path`) so the backend can resolve the workspace-local dir per-call —
no session/workspace ID indirection needed, this mirrors how `path` is already the source of
truth for workspace identity elsewhere in the proto.

Run `make proto-gen` after adding the file — this regenerates both
`gen/proto/go/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` per the existing
"New API Endpoints" convention in `CLAUDE.md`. Per the memory note on proto-gen (`web-app/src/gen`
is tracked despite `.gitignore`), the generated TS output must be committed alongside the
proto change, not left for CI to regenerate.

### `server/server.go` registration

Follow the plain (non-feature-flagged) registration pattern used for `GitHubUserService`
(`server/server.go:390`):

```go
ghPath, ghHandler := sessionv1connect.NewGitHubUserServiceHandler(deps.GitHubUserService, ConnectOptions(deps.ErrorRegistry)...)
srv.mux.Handle(ghPath, ghHandler)
```

`PromptLibraryService` has no reason to be feature-flag-gated (unlike `BacklogService`,
which is wrapped in a `connect.WithInterceptors(interceptors.NewFeatureFlagInterceptor(...))`
at `server/server.go:403` for its beta rollout) — this is a straightforward, low-risk
read/write-file feature with no external side effects, so register it unconditionally like
`GitHubUserService`/`InsightsService`/`SessionSummaryService`.

### CLI subcommand

`main.go` registers cobra commands via `rootCmd.AddCommand(...)` (confirmed 7 existing
top-level commands at `main.go:711-717`, e.g. `rootCmd.AddCommand(commands.GetSessionCmd)`
which is itself defined in a separate `commands` package rather than inline in `main.go`).
Follow that: a new `prompts` command (list/show templates) belongs in its own
`commands/prompts.go`-style file, registered the same way, calling into `promptlibrary/`
directly (no RPC round-trip needed for a local CLI process — same reasoning `GetSessionCmd`
already applies for direct package access vs. going over the network).

---

## 4. Data flow

```
Global dir (~/.stapler-squad/prompts/, via config.PromptsDirOrDefault())
Workspace dir (<repo-root>/.stapler-squad/prompts/, via go-git DetectDotGit)
        │                           │
        └──────────── merged in promptlibrary.List() ──────────┘
                              │
                    PromptLibraryService.ListPromptTemplates (ConnectRPC)
                              │
                    Picker UI (OmnibarCreationPanel.tsx: searchable, tag-filterable)
                              │
                    user selects a template
                              │
        client-side interpolation of {{repo}} / {{branch}} / {{issue_title}}
        from current Omnibar form state (path→repo, branch field, GitHubRef if present)
                              │
                    prefill initial_prompt / prompt textarea (still user-editable)
                              │
              existing CreateSession RPC — UNCHANGED from this point on
```

### Interpolation: client-side, not server-side

Recommend **client-side interpolation** in the web-app, not a server RPC round-trip per
keystroke/selection:

- The three variables (`{{repo}}`, `{{branch}}`, `{{issue_title}}`) are all values the
  Omnibar/`OmnibarCreationPanel.tsx` form state already holds or can trivially derive at
  selection time (path basename for repo, the branch form field, and `gitHubRef` from a
  detector result if the user pasted a PR/issue link first) — no new server round-trip is
  needed to have this data available.
- **Gap worth flagging for planning**: I checked `web-app/src/lib/omnibar/types.ts:108-113`
  — `GitHubRef` currently has `owner`, `repo`, `branch?`, `prNumber?` but **no `title` field**,
  and a repo-wide grep for `issue_title`/`issueTitle` in `web-app/src` and `server/services`
  returns zero hits. There is no existing plumbing that resolves an issue/PR *title* anywhere
  in the omnibar detection flow today. This means `{{issue_title}}` will render as an empty
  string in the near-universal case until/unless a follow-up adds a title-fetch step to one of
  the GitHub detectors — which is consistent with the requirements doc's explicitly chosen
  "undefined vars render blank" behavior (requirements.md line 30-32), just noting it so the
  planning phase doesn't assume `{{issue_title}}` already has a live data source to plug into.
- Keep `promptlibrary.Interpolate()` in Go too (used by the "Save as template" preview and by
  the CLI path), but the interactive picker flow doesn't need to call it over RPC — simple
  string substitution is cheap enough to duplicate as a ~10-line TS function, and doing it
  client-side means the textarea updates instantly on selection with no network latency.

### Listing: re-scan per request, no cache

Both directories are small (a handful to a few dozen `.md` files in the expected case).
Re-scanning the filesystem on every `ListPromptTemplates` call is simplest and avoids a whole
class of cache-invalidation bugs (workspace templates are edited directly via git commits or
the "Save as template" writer — a cache would need invalidation hooks for both paths). This
matches the project's own YAGNI-leaning conventions (see `ponytail` skill's ethos referenced
in `CLAUDE.md` and the non-functional requirement that 0 templates must be a fast, non-error
empty state — a `filepath.WalkDir` over an empty or missing directory is already fast and
already handles "directory doesn't exist" as "zero templates," not an error). Add caching
later only if profiling shows it's needed — no evidence of that here.

---

## 5. Consistency / concurrency

- **Filesystem is the sole source of truth** — no database table, no ent schema change. This
  keeps workspace templates naturally git-versioned/shareable (a stated goal in
  requirements.md) and global templates naturally hand-editable.
  `.claude/rules/ent-schema-generation.md` is not relevant to this feature.
- **Malformed file handling** (acceptance criterion 6): `promptlibrary.List()` should return
  `([]*Template, []error)` or log-and-skip per-file internally (matching the pattern
  `githubpkg`/other list-then-filter code uses elsewhere in this codebase) rather than
  failing the whole RPC on one bad file — confirmed this is explicitly required, not just a
  nice-to-have.
- **Concurrent writes**: "Save as template" writing a new file is a single `os.WriteFile` to a
  fresh path (name-derived filename); no read-modify-write race exists because there's no
  existing file being mutated in place for the initial version (no edit-in-place scope per
  "Out of Scope" in requirements.md — only create-new). If two saves race on an identical
  filename, last-write-wins is an acceptable outcome for this low-stakes, single-user-editing
  feature — no locking needed.

---

## Summary of concrete file/package additions

| Layer | Path | Notes |
|---|---|---|
| Business logic | `promptlibrary/template.go`, `library.go`, `interpolate.go`, `save.go` (new package) | Concrete types, no interfaces; mirrors `github/` package split |
| Config helper | `config/config.go`: add `PromptsDirOrDefault()` | Same shape as `TriageArtifactDirOrDefault()` / `BacklogAttachmentDirOrDefault()` (config/config.go:539-557) |
| Workspace root resolution | reuse `git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})` | Matches `session/git/util.go:79`, `session/git/ops.go` (8 call sites); per `.claude/rules/prefer-go-git-over-subshells.md` |
| Proto | `proto/session/v1/prompt_library.proto` (new file) | Mirrors `github_user.proto`; generates into shared `sessionv1connect` package |
| ConnectRPC adapter | `server/services/prompt_library_service.go` (new) | Mirrors `server/services/github_user_service.go`'s thin-wrapper shape |
| Server registration | `server/server.go` | Unconditional registration, same pattern as `GitHubUserService` (server/server.go:390) — no feature flag needed |
| CLI | `commands/prompts.go` (new, alongside existing `commands` package used by `GetSessionCmd`) | Registered via `rootCmd.AddCommand(...)` in `main.go` |
| No changes needed | `proto/session/v1/session.proto` (`CreateSessionRequest`) | `initial_prompt`/`prompt`/`path`/`branch` fields already sufficient; template application is a client-side prefill, not a new RPC field |
| Not triggered | `.claude/rules/session-creation-registry.md` (7 touchpoints) | Confirmed: no new `SessionType`, existing fields cover the flow |
