# Research: Build vs. Buy — Prompt Template Library

Agent: Research Agent 6 (Build vs. Buy)

Scope: per `requirements.md` — markdown+frontmatter storage (global + workspace scope),
`{{repo}}`/`{{branch}}`/`{{issue_title}}` interpolation, searchable/tag-filterable picker UI,
save-as-template action.

## 1. YAML Frontmatter Parsing (Go)

**Current state**: `gopkg.in/yaml.v3 v3.0.1` is already a direct dependency in
[`go.mod`](../../../go.mod) (line 48). No frontmatter-specific library exists in the repo today.

### Option A — `github.com/adrg/frontmatter`
- Pros: purpose-built, handles YAML/JSON/TOML autodetection, struct-tag unmarshal, MIT license,
  reasonably maintained (per [pkg.go.dev](https://pkg.go.dev/github.com/adrg/frontmatter) /
  [GitHub](https://github.com/adrg/frontmatter)).
- Cons: **depends on `gopkg.in/yaml.v2`**, not v3 — this repo already carries v3. Adding it
  pulls a second, older YAML codec into the dependency graph for a format the repo doesn't need
  autodetection for (only YAML frontmatter is in scope; JSON/TOML frontmatter is not a
  requirement). New external dependency for what is, structurally, a fixed-delimiter string
  split (`---\n...\n---\n<body>`) plus one `yaml.Unmarshal` call.
- Verdict: **Not recommended.** Adds a dependency (and a duplicate YAML major version) to solve
  a problem that's ~15 lines against a library already in `go.mod`.

### Option B — Hand-roll with existing `yaml.v3`
- Split file content on the `---` delimiter (first two occurrences), `yaml.Unmarshal` the header
  block into a `TemplateFrontmatter{Name, Description, Tags []string}` struct, treat the
  remainder as the body string. Malformed frontmatter (missing delimiters, invalid YAML) is
  exactly the "skip with logged warning" case in acceptance criterion 6 — a simple error return
  from this function already covers it.
- Verdict: **Recommended.** Zero new dependencies, uses the YAML major version already
  standardized on in this repo, and the parsing logic directly maps to the one failure mode
  (malformed frontmatter) the acceptance criteria call out.

## 2. Fuzzy Search / Filter (Picker UI)

**Current state**: `fuse.js@^7.3.0` is already a direct dependency in
[`web-app/package.json`](../../../web-app/package.json). It's already in active use for
session/command search — see `web-app/src/components/sessions/QuickOpenPalette.tsx` (dynamic
`require("fuse.js")`), `web-app/src/lib/hooks/useSessionSearch.ts`, and
`web-app/src/app/insights/SessionsTable.tsx`.

- Pros: zero new dependency; consistent with the existing search UX pattern in this exact class
  of "searchable picker" component (`QuickOpenPalette`); team already knows its config surface
  (weighted keys, threshold tuning) from the session-search use case.
- Cons: none identified — the "name + description" free-text search and tag filter required by
  acceptance criterion 4 is squarely inside what `fuse.js` already does elsewhere in this
  codebase (weighted-key search across a small in-memory list; template counts are expected to
  be small, not requiring server-side search).
- Verdict: **Recommended** — reuse `fuse.js`, no new dependency. Tag filtering itself (exact-set
  match on the `tags` array) doesn't need fuzzy matching at all; only the name/description
  free-text search benefits from `fuse.js`, and it's a straightforward `keys: ["name",
  "description"]` config mirroring `QuickOpenPalette`'s existing usage.

## 3. Existing "Prompt Library" OSS Package (Go/TS)

Web search for a ready-made "prompt library" (markdown+frontmatter storage, global+workspace
scope, variable interpolation, picker UI) turned up nothing that fits this shape as an
importable library:

- [microsoft/PromptKit](https://github.com/microsoft/PromptKit) — closest conceptual match
  (composable Markdown+YAML-frontmatter prompt components), but it's a **standalone prompt
  authoring/composition framework** with its own CLI and component model (personas, protocols,
  formats), not a library that plugs into an existing Go backend + React picker UI. Adopting it
  would mean restructuring the feature around PromptKit's component taxonomy rather than the
  three-field (`name`/`description`/`tags`) format this repo's requirements already specify.
  License/maturity not evaluated further since the fit itself fails.
- [adrg/frontmatter](https://github.com/adrg/frontmatter),
  [marad/frontmatter](https://github.com/marad/frontmatter),
  [rythoris/frontmatter](https://github.com/rythoris/frontmatter),
  [AdobeDocs/frontmatters](https://github.com/AdobeDocs/frontmatters) — all are generic
  frontmatter CLI/parsing utilities, not prompt-library systems (no picker UI, no
  global/workspace scoping, no variable interpolation).
- No existing TypeScript "prompt library" package surfaced that provides a picker UI matching
  this repo's ConnectRPC + vanilla-extract + ARIA-locator conventions — any such package would
  need to be re-skinned entirely to fit `.claude/rules/css-architecture.md` and
  `.claude/rules/e2e-test-conventions.md`, eliminating most of the reuse value anyway.
- Verdict: **Not recommended.** Nothing found that does the whole thing; the closest match
  (PromptKit) is an authoring framework whose shape doesn't match this repo's requirements, and
  every UI portion would need a full rewrite regardless to meet this repo's own conventions
  (vanilla-extract, ConnectRPC, `data-testid`/ARIA locators).

## 2b. SaaS / Managed Prompt Management Service

Considered: PromptLayer, Langfuse Prompt Management, and similar hosted "prompt management"
products.

- Pros: version history, collaborative editing UI, analytics on prompt usage — none of which are
  in scope per `requirements.md`'s "Out of Scope" section (no versioning/history requirement
  beyond git, no cross-user sharing beyond git commits).
- Cons: **directly conflicts with the stated requirement.** `requirements.md` explicitly
  specifies workspace templates live in `.stapler-squad/prompts/` "committed to the repo so
  templates are shareable via the repo itself" — a filesystem/git-shareable model. A hosted
  service is a different sharing mechanism (account-based, network-dependent) that doesn't
  satisfy "shareable via the repo," would require network calls on every session-creation picker
  open (violating the non-functional requirement that 0-templates state must be fast and
  error-free), and introduces a new paid dependency and auth surface for a p2
  quality-of-life feature the issue frames as competing with two local-file-based competitor
  tools (CodexMonitor, clideck).
- Verdict: **Not recommended.** Explicitly ruled out by the requirements' own model (local
  files, git-shareable, no central registry — see "Out of Scope: Template sharing across
  users/orgs beyond git-repo commit").

## 4. LLM-Generated vs. Battle-Tested: Variable Interpolation

Requirements scope this precisely: exactly three named variables (`{{repo}}`, `{{branch}}`,
`{{issue_title}}`), undefined → empty string, no arbitrary custom-variable system (explicitly
out of scope). Two viable approaches, both stdlib-only (no new dependency either way):

### Option A — Hand-rolled `strings.ReplaceAll`
```go
func interpolate(body string, vars map[string]string) string {
    for k, v := range vars {
        body = strings.ReplaceAll(body, "{{"+k+"}}", v)
    }
    return body
}
```
- Pros: ~5 lines, trivial to read and test.
- Cons: no escaping — a template body that legitimately contains a literal `{{repo}}` string
  (e.g. documenting the template syntax itself, or a prompt about templating) gets silently
  substituted with no way to opt out. Also does repeated full-string scans per variable
  (irrelevant at this scale, but the correctness gap is the real issue, not performance).

### Option B — stdlib `text/template`
```go
tmpl, err := template.New("prompt").Option("missingkey=zero").Parse(body)
// render with a struct{ Repo, Branch, IssueTitle string }
```
- Pros: **also stdlib — zero new dependency**, same "size" as Option A in dependency-cost terms.
  Handles edge cases Option A gets wrong for free: proper token boundary parsing (won't
  misfire inside a larger token), a well-defined action syntax reviewers/future maintainers
  already recognize instead of a bespoke replace-loop convention, and forward compatibility if
  the (currently out-of-scope) custom-variable system is ever added — `text/template` already
  supports arbitrary keys without new parsing logic, `ReplaceAll` would need a rewrite.
- Cons: `{{issue_title}}` template syntax must map to Go template field names (e.g.
  `{{.IssueTitle}}`), meaning the *stored file format* (issue's `{{repo}}` etc.) needs either
  (a) a translation step (regex/string replace of `{{repo}}` → `{{.Repo}}` before parsing — adds
  back a small hand-rolled component anyway), or (b) accepting a slightly different
  user-facing template syntax across the codebase (`{{.Repo}}`) than the one specified in
  `requirements.md` (`{{repo}}`). This is a real friction point, not just a stylistic one.

### Verdict
**Recommended: Option A (hand-rolled `strings.ReplaceAll`), with the literal-brace edge case
explicitly accepted as a known limitation, not silently dropped.** Rationale, applying this
repo's own "two stdlib options, same size, take the one correct on edge cases" framing:
`text/template` is *not* actually the same size once the `{{repo}}` → `{{.Repo}}` syntax
mismatch is accounted for — matching the user-facing syntax specified in `requirements.md`
(`{{repo}}`, not `{{.Repo}}`) requires either a pre-processing translation layer (which
reintroduces a hand-rolled string-replace step, only now in front of `text/template` instead of
instead of it) or diverging from the spec'd syntax. Given the requirement explicitly caps scope
at exactly three known variables with no plans for user-defined ones in this iteration, the
`text/template` forward-compatibility argument is speculative (YAGNI) against a concrete syntax
cost today. **Follow-up**: if arbitrary custom variables are added later (flagged as future work
in `requirements.md`), re-evaluate — that's precisely the point at which `text/template`'s
extensibility stops being speculative.

## 5. Fork or Adapt Existing Picker Code

Checked `web-app/src/lib/omnibar/detector.ts` and the alias/workflow `@mention` system
(`web-app/src/lib/omnibar/detectors/AliasDetector.ts`,
`web-app/src/lib/omnibar/detectors/WorkflowDetector.ts`,
`web-app/src/components/ui/AtCommandDropdown.tsx`,
`web-app/src/lib/hooks/useAtCommandSuggestions.ts`) for reuse potential.

- **What it is**: an inline `@slug`-prefix autocomplete — the user types `@` inside the omnibar
  text field and an inline dropdown (`AtCommandDropdown.tsx`, 76 lines) filters a fixed list of
  `WorkflowEntry`/alias objects by `slug.startsWith(query)` (`useAtCommandSuggestions.ts`).
  Detection is driven by the `DetectorRegistry` (`detector.ts`), which classifies free-text
  *input* into a type (URL, path, shorthand) — not a template *content* picker.
- **Why it's not a fit for "From template"**: the requirement (acceptance criterion 3/4) is a
  standalone, explicitly-invoked, searchable+tag-filterable picker ("From template" option in
  the New Session flow) that populates the initial-prompt field — structurally closer to
  `QuickOpenPalette.tsx` (a modal/palette search-and-select UI already built on `fuse.js`) than
  to the inline `@mention` completion pattern, which is prefix-match-only (`startsWith`, no
  fuzzy search) and has no concept of tags or a description field.
  `.claude/rules/session-creation-registry.md` also explicitly confirms templates are *not* a
  new creation mode/detector — they alter existing modes' prompt content — so extending the
  `DetectorRegistry` would be the wrong touchpoint entirely, not just a suboptimal one.
- Verdict: **Adapt the `QuickOpenPalette` pattern (structure/conventions), not the
  `@mention`/`DetectorRegistry` system.** Don't extend `AtCommandDropdown` or add a `Detector`
  — build a new component following `QuickOpenPalette`'s existing `fuse.js`-backed
  search-and-select shape, since that's the closest existing UI already solving "search a small
  in-memory list by text, select one, populate a field."

## Summary Table

| Option | Verdict |
|---|---|
| `adrg/frontmatter` (new dep) | Not recommended — dup YAML major version for ~15 lines of logic |
| Hand-rolled frontmatter split + existing `yaml.v3` | **Recommended** |
| New fuzzy-search dep | Not recommended — `fuse.js` already present and already used for this exact pattern |
| Reuse existing `fuse.js` | **Recommended** |
| Existing OSS "prompt library" package (Go/TS) | Not recommended — nothing fits; closest match (PromptKit) is a different shape entirely |
| Hosted prompt-management SaaS (PromptLayer, Langfuse, etc.) | Not recommended — conflicts with explicit git-shareable local-file requirement |
| `strings.ReplaceAll` interpolation (3 known vars) | **Recommended** — `text/template` isn't actually equal-cost once the `{{x}}` vs `{{.X}}` syntax mismatch is priced in |
| Extend `@mention`/`DetectorRegistry` for the picker | Not recommended — wrong touchpoint per session-creation-registry rule |
| Adapt `QuickOpenPalette` pattern for new picker component | **Recommended** |
