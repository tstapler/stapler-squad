# Research: Feature Landscape — Prompt Template Library

Scope: existing UI/backend patterns this codebase already has that a prompt-template-library
feature should reuse or align with; feature-specific edge cases; unstated user needs; and what
the issue's competitor references (CodexMonitor, clideck) already establish.

## 1. Closest existing analog: `SlashCommandService` (markdown + YAML frontmatter, dual-scope)

[`server/services/slash_command_service.go`](server/services/slash_command_service.go) is
nearly a blueprint for this feature's backend. It already does:

- Two-tier directory scan: project (`<targetDir>/.claude/commands/`) then user
  (`~/.claude/commands/`), each resolved through `filepath.EvalSymlinks` before
  `filepath.WalkDir` (lines 56–85, `slash_command_service.go:61`, `:76`) — the direct precedent
  for this feature's global (`~/.stapler-squad/prompts/`) + workspace
  (`<repo-root>/.stapler-squad/prompts/`) dual-scope read.
- Precedence-with-merge via a `seen` map: project entries win over user entries win over
  built-ins (`slash_command_service.go:43-54`) — same shape as "workspace templates are
  additive to global ones," except this feature has no built-ins tier and (per requirements)
  wants **union**, not override-on-name-collision, so the merge policy differs — see §2.
- `walkCommandDir` (line 94) skips non-`.md` files and two hardcoded meta filenames
  (`CLAUDE.md`, `README.md`) — errors from `WalkDir`'s callback are swallowed (`if err != nil
  ... return nil`), i.e. a single bad entry never aborts the walk. Direct precedent for AC6
  ("malformed template file is skipped with a logged warning" — note the existing code doesn't
  even log on a walk error, it just returns nil; the new feature's spec is stricter and should
  add the `log.Warn` this existing code lacks).
- **Frontmatter parsing is hand-rolled, not `yaml.Unmarshal`**: `parseCommandFrontmatter`
  (`slash_command_service.go:135-163`) scans line-by-line for `---` delimiters and does
  `strings.CutPrefix("title:")` / `strings.CutPrefix("description:")` — it does not use
  `gopkg.in/yaml.v3` at all, and could not represent a `tags: [a, b]` list. This feature's
  frontmatter (`name`, `description`, `tags`) needs real YAML parsing since `tags` is a list —
  `gopkg.in/yaml.v3` is already a dependency, used properly (full `yaml.Unmarshal`) in
  `server/services/rules_service.go` and `session/detection/detector.go`. Model the new
  frontmatter parser on those, not on `slash_command_service.go`'s line-scanner.

## 2. Storage-path resolution: don't hand-roll `~/.stapler-squad/`

Two existing precedents for where "global, per-user" state lives, and they resolve it
differently — the new feature should pick the one that matches its intended isolation
semantics, explicitly:

- **`config.GetConfigDir()`** (`config/config.go:117-169`) is the *canonical* resolver for
  `~/.stapler-squad/` and is instance-aware: it honors `STAPLER_SQUAD_TEST_DIR`,
  `STAPLER_SQUAD_INSTANCE` (named instances get `~/.stapler-squad/instances/<name>/`), test-mode
  auto-isolation, and an opt-in per-directory workspace-isolation mode
  (`resolveDefaultConfigDir`, `config/config.go:176+`). `session_service.go`'s
  `newPromptStore()` (`server/services/session_service.go:411-417`) uses exactly this to place
  `prompts.json` at `<configDir>/prompts.json`.
- If global templates are meant to be **one true set shared across every instance** (plausible
  reading of requirements.md's `~/.stapler-squad/prompts/` — it says nothing about per-instance
  isolation), that's a deliberate deviation from `GetConfigDir()`'s default behavior and should
  be stated as such, not accidental. Using `GetConfigDir()` unmodified would silently scope
  templates per-instance/per-test-run, which likely defeats the "save once, reuse everywhere"
  intent. Flag this as a plan-time decision, not an implementation detail.
- **Workspace root resolution**: requirements.md explicitly says "workspace" means the *repo
  root*, not the worktree — `findGitRepoRoot` (`session/git/util.go:79+`) walks up from a path
  via `git.PlainOpen` until it finds the repo root, and (notably) **auto-creates and
  git-inits the directory with an initial commit if it doesn't exist** (lines 82-102) — that
  side effect is wrong for a read-only "does this workspace have templates" check; a template
  lister must NOT reuse `findGitRepoRoot` verbatim, or the mere act of listing templates could
  initialize a git repo in an arbitrary directory. A read-only variant (or one of the other
  `RepoRoot`-style helpers in `session/unfinished/scanner.go`) is the correct base, or a
  `go-git` `PlainOpenWithOptions{DetectDotGit: true}` walk that errors (not creates) when no
  repo is found, per `.claude/rules/prefer-go-git-over-subshells.md`.

## 3. Existing "@mention → pick from dynamic list" UI pattern (WorkflowDetector / AliasDetector)

`.claude/rules/session-creation-registry.md` and the omnibar detector registry already
implement the exact interaction shape a template picker needs, in two layers:

- **Detector layer** (`web-app/src/lib/omnibar/detectors/WorkflowDetector.ts`,
  `AliasDetector.ts`): both are registered dynamically at runtime (not in
  `createDefaultRegistry()`) via `OmnibarContext.tsx` effects once their backing list loads —
  `WorkflowDetector` even does its own `{{input}}` interpolation
  (`WorkflowDetector.ts:62-67`, `interpolatedPrompt.replace(/\{\{input\}\}/g, arg)`), a direct
  precedent for this feature's `{{repo}}`/`{{branch}}`/`{{issue_title}}` substitution — same
  simple `String.replace` approach, no templating engine needed.
- **In-textarea trigger layer**, which is the more relevant precedent for AC3 (populate the
  *initial prompt field*, not the top-level omnibar input): `OmnibarCreationPanel.tsx`'s "First
  Prompt" textarea (`OmnibarCreationPanel.tsx:721-757`) already has a live `/`-command
  autocomplete wired directly into it via `useSlashCommandSuggestions` (cursor-aware, not
  whole-input) + `SlashCommandDropdown` — the hook detects `/word` at the cursor
  (`web-app/src/lib/hooks/useSlashCommandSuggestions.ts:6`, `SLASH_WORD` regex), filters a
  flat list by prefix, and returns a `complete(value, cmd)` that splices the match back into
  the textarea preserving cursor position. This is the correct architectural home for a
  template picker if it's meant to trigger *from within* the prompt textarea (e.g. a `//`
  prefix, matching clideck's own `//` shortcut named in the issue) rather than only via a
  separate "From template" button/modal. Recommend building `useTemplateSuggestions` +
  `TemplateDropdown` as siblings of `useSlashCommandSuggestions`/`SlashCommandDropdown`, reusing
  the same cursor-splice mechanics, rather than inventing a new interaction model.
- Separately, `useAtCommandSuggestions.ts` + `AtCommandDropdown.tsx` show the same pattern
  applied to whole-input `@slug` detection (workflows) — useful if "From template" is instead
  exposed as an omnibar-level `@`-style trigger rather than in-textarea. Either precedent is a
  closer starting point than building a picker from scratch.

## 4. Existing CRUD-management-panel pattern (`AliasesManager.tsx`)

`web-app/src/components/settings/AliasesManager.tsx` is the closest precedent for the
"Save as template" form (name/description/tags/scope) and for a future template
list/edit/delete surface (see §6, "unstated needs"):
- Client via generated `SessionService` (ConnectRPC) — `listAliases({})`, load into local
  state, edit-in-place form bound to a `AliasFormData` shape, save via upsert RPC.
- Its form already handles an "advanced" collapsible section (env vars / CLI flags) — same
  shape a template's `{{repo}}`/`{{branch}}`/`{{issue_title}}` preview or raw-frontmatter view
  could use, if that's wanted later.
- No existing component in this codebase combines free-text search *and* multi-tag filtering in
  one control — `web-app/src/components/rules/TagInput.tsx` is a tag *input* (adding tags to an
  entity, used for the save form), not a tag *filter* (narrowing a list by selected tags, needed
  for the picker). `ReviewQueuePanel.tsx`, `ResumeSessionModal.tsx`, `ProfilesManager.tsx`,
  `GlobalDefaultsForm.tsx` all matched a loose "tag/filter" grep but on inspection use the term
  incidentally (session tags for grouping, not a search+tag-filter picker control) — there is no
  ready-made "searchable + tag-filterable picker" component to lift wholesale; it needs to be
  built new, composing `TagInput.tsx`'s tag-chip rendering with a plain text `<input>` filter,
  most naturally inside the `SlashCommandDropdown`-style listbox from §3.

## 5. Adjacent existing feature with a name collision risk: Prompt History

`session/prompts/store.go` (`PromptStore`, wired in `server/services/session_service.go:412`
`newPromptStore()`) is a **different, already-shipped** feature: `ListPromptHistory` /
`DeletePromptHistory` RPCs (`session_service.go:3446`, `:3469`) auto-record the raw text of
recently-typed initial prompts (dedup by content hash, `entryID`, with a `UsedCount` /
`LastUsed`) to a single JSON file at `<configDir>/prompts.json` — surfaced in
`SessionWizard.tsx` (`listPromptHistory({ limit: 10 })`, line ~139) as a "recent prompts"
suggestion list. This is **not** the template library (no name/description/tags, no
global-vs-workspace split, no markdown files, auto-populated not user-curated) but:
- The storage path is adjacent-but-distinct: `~/.stapler-squad/prompts.json` (existing, a
  *file*) vs. `~/.stapler-squad/prompts/` (proposed, a *directory*) — not a literal collision,
  but close enough in name that implementers and future readers will conflate "prompt history"
  and "prompt templates" unless the docs/UI copy clearly distinguish them (e.g. don't call the
  new feature's directory or RPC service just "Prompts").
- Product question worth surfacing to plan.md: should the template picker and the recent-prompt
  history dropdown in `SessionWizard.tsx` be presented as one unified "prompt" affordance
  (recent + saved templates in one list) or kept as two separate UI entry points? Not answered
  by requirements.md; flagging as a scope question rather than assuming.

## 6. Feature-format precedent: Go CLI subcommand registration

`main.go` registers subcommands via `rootCmd.AddCommand(...)` (lines 711-717) — flat list of
`*cobra.Command` vars (`resetCmd`, `versionCmd`, `listSessionsCmd`, etc.), and
`commands.GetSessionCmd` (line 717) shows the pattern for a subcommand defined in its own
package (`commands` pkg) rather than inline in `main.go`. A new `templatesCmd` (or
`prompt-template` subcommand per requirements.md's guidance to map onto "whatever the actual
session-creation CLI surface is") should follow the `commands.GetSessionCmd` external-package
pattern, not add another 40-line inline `cobra.Command{}` literal to `main.go`.

## 7. Edge cases and failure modes (feature-specific)

- **Malformed frontmatter / invalid YAML** — precedent: AC6 requires skip + logged warning.
  `walkCommandDir` in `slash_command_service.go` silently skips (no log). The new
  implementation should log at `Warn` with the file path, matching this repo's general
  `log.Warn("[Component] message", "err", err)` convention seen throughout `session/`,
  `server/services/`.
- **Duplicate template names across global/workspace scope** — requirements.md's AC2 implies
  both are visible simultaneously ("selectable... alongside global templates"), so name
  collisions are not resolved by hiding one; the picker must disambiguate visually (e.g. a scope
  badge/tag next to the name), analogous to how `AtCommandDropdown` labels each result with
  `@slug` + name — a plain flat merged list (as `slash_command_service.go`'s `seen` map does,
  which *drops* the losing entry) would be the wrong behavior here and silently hides a
  same-named workspace template.
- **Template references a variable with no value in context** (e.g. `{{issue_title}}` with no
  linked issue) — requirements.md already resolves this: render as empty string. Precedent:
  `WorkflowDetector.ts`'s `{{input}}` replace is unconditional/simple string substitution;
  a similar `strings.NewReplacer` or sequential `strings.ReplaceAll` in Go (or `.replace()`
  chain in TS, if interpolation happens client-side after the RPC returns raw body + resolved
  context) is sufficient — no need for a templating library.
- **Very large template files** — no existing size guard in `slash_command_service.go`'s walk
  (reads whole file via `bufio.Scanner` line-by-line, which is fine for large files since it
  doesn't buffer the whole content, but a full read-into-string approach for the *body* would
  need a size cap). Not addressed anywhere in this codebase's existing file-parsing code;
  flag as a genuinely new concern to size a limit for (e.g. reject/warn above some KB threshold)
  rather than something to copy from precedent.
- **Symlinks / traversal in the prompts directory** — `slash_command_service.go` resolves the
  *directory* itself through `filepath.EvalSymlinks` before walking (so a symlinked top-level
  dir is followed once) but does **not** guard against symlinks *within* the walked tree
  escaping outside the intended root — `filepath.WalkDir` follows a symlinked dir into
  arbitrary parts of the filesystem if one exists inside `~/.stapler-squad/prompts/` or
  `<repo>/.stapler-squad/prompts/`. This is an existing, unaddressed gap in the precedent code,
  not something to blindly copy — worth an explicit decision (WalkDir with `d.Type() &
  fs.ModeSymlink` skip, or accept the same posture as `slash_command_service.go` and document
  it as an accepted risk parity call).
- **Concurrent edits to the same template file** — `writeSettingsAtomic`
  (`server/services/mcp_injector.go:129`) is this repo's established pattern: unique
  `os.CreateTemp` name (not a fixed `.tmp` suffix) + `os.Rename` for atomicity, specifically
  because a fixed tmp-filename caused a real, previously-shipped corruption bug (see
  `TestWriteSettingsAtomic_ConcurrentWritesToSameSettingsPath_NeverProduceCorruptJSON`,
  `server/services/hook_injector_test.go:415-447`, and its doc comment). "Save as template"
  writes must use the same unique-temp-file-then-rename approach, not a naive `os.WriteFile`.
- **Workspace templates when the working directory isn't a git repo at all** — `findGitRepoRoot`
  (§2 above) would *create* a git repo as a side effect if reused naively; a correct
  implementation must treat "not a git repo" as "zero workspace templates," not as an error and
  not as a trigger to `git init` a directory just because someone opened the template picker.
- **Templates directory not existing yet (first use)** — precedent:
  `slash_command_service.go`'s `filepath.WalkDir` on a nonexistent dir hits the `err != nil`
  branch of the callback and returns nil per-entry, but actually — `WalkDir` on a root that
  doesn't exist invokes the callback once with the root path and a non-nil `err` (from `Lstat`),
  which this code already handles as a no-op (`if err != nil || d.IsDir() { return nil }`).
  Confirms non-functional requirement ("0 templates = empty, fast-loading picker, not an error
  state") is achievable with the exact same `WalkDir` idiom already in use — no special-casing
  needed for the "not created yet" directory.

## 8. Unstated user needs (beyond explicit requirements)

- **Edit/delete templates from the UI, not just create** — requirements.md's "Save as template"
  (item 4) only covers *creation*; nothing in scope covers editing a template's body/tags after
  the fact or deleting one from the UI (only "drop a `.md` file" / presumably manually delete
  it). `AliasesManager.tsx` (§4) demonstrates this codebase's users already expect full
  list/edit/delete CRUD for comparable "named, reusable, tagged" entities (aliases, workflows,
  approval rules all have full CRUD RPCs: `ListWorkflows`, `UpdateWorkflow`, `DeleteWorkflow` in
  `workflow_service.go`; `UpsertApprovalRule`, `DeleteApprovalRule` in `rules_service.go`). A
  template library with create-and-manually-delete-via-filesystem-only is a notably weaker
  experience than every other named/tagged entity in this app already gets, and is likely to be
  raised as a gap in review even though it's explicitly out of initial scope per requirements.md
  ("Rich template editor UI" is out of scope, but *basic* edit/delete via RPC is a different,
  smaller ask than a rich editor and isn't explicitly excluded).
- **Preview interpolated output before applying** — requirements.md's AC3 says selecting a
  template "populates... the initial prompt field... which remains user-editable before
  submit," which *is* the preview (the interpolated text sits in an editable field the user sees
  before hitting submit) — so this need is implicitly met by the described flow, not a gap. Flag
  as already covered rather than a new requirement.
- **Keyboard-only access** (`//` shortcut, per clideck) — requirements.md's item 3 only commits
  to "a 'From template' option in the New Session flow... with a searchable, tag-filterable
  picker," not a keyboard trigger syntax. Given this codebase already has two live
  keyboard-triggered pickers in the exact same textarea (`/` for slash commands, implicitly
  `@` for workflows/aliases at the omnibar level), a `//`-triggered (or similar) in-textarea
  template picker is a natural, low-cost extension of existing infrastructure (§3) and is very
  likely an implicit expectation given the issue explicitly cites clideck's `//` UX as
  competitive inspiration — worth raising as a "should this be in scope" question for plan.md
  even though requirements.md doesn't commit to it, since the two competing UI patterns (button
  in a form vs. inline trigger) have different implementation costs and the issue's own
  competitive framing leans toward the inline-trigger UX.

## 9. Competitor references (as already documented in requirements.md — not re-fetched)

requirements.md itself only names the two competitor tools in its "Priority Signal" section
(`CodexMonitor`, `clideck`) without describing their UX in the requirements body — the `$CODEX_HOME/prompts`
path convention and the `//` shortcut mechanic are referenced in this research task's own
prompt (i.e., from the original GitHub issue text, not from requirements.md), not from any file
in this repo. No further detail on either tool exists in this codebase to cross-reference beyond
what's already summarized above (§8) — the `//`-shortcut comparison is the one actionable
takeaway, already covered.
