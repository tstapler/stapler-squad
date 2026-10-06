# Stack Research: Prompt Library

## YAML frontmatter parsing

**Use `gopkg.in/yaml.v3 v3.0.1`** — already a direct dependency (`go.mod:48`). No frontmatter-specific
library (`adrg/frontmatter`, gohugoio parser, etc.) exists in `go.mod`, and none should be added; this
repo's ethos (ponytail / YAGNI, `.claude/rules/interface-pollution-checklist.md`) is to reuse stdlib +
existing deps over adding new ones for a solved problem.

Two existing precedents for markdown-with-frontmatter and YAML-with-Go-structs, both worth following
depending on complexity needed:

1. **Hand-rolled frontmatter scan** — `server/services/slash_command_service.go:135`
   (`parseCommandFrontmatter`). Opens the file, scans line-by-line with `bufio.Scanner`, tracks
   `inFrontmatter` between `---` delimiters, and does simple `strings.CutPrefix(line, "title:")`
   matching per key. Sufficient for flat `key: value` frontmatter (title, description) but does **not**
   handle a YAML list like `tags: [a, b]` or `tags:\n  - a\n  - b` — prompt-library's frontmatter needs
   `tags` as a list, so this exact hand-rolled approach is not directly reusable as-is.

2. **Struct + `yaml.Unmarshal`** — `server/services/rules_service.go:890-923` (`yamlRulesFile`,
   `yamlRuleEntry`, `yamlRuleExport`). Defines a plain Go struct with `yaml:"..."` tags (including
   `[]string` fields like `Programs`, `Subcommands` — the same shape `tags` needs) and calls
   `yaml.Unmarshal` directly. **This is the pattern to follow for prompt-library's frontmatter** —
   define a struct `{ Name string; Description string; Tags []string }` with yaml tags, split the file
   on the leading `---\n...\n---\n` delimiter (reuse the delimiter-scanning logic from
   `parseCommandFrontmatter` as reference, or a short `strings.SplitN` on `"---\n"`), and
   `yaml.Unmarshal` just the frontmatter block; the rest of the file is prompt body text (write as
   plain string, not further parsed as markdown/goldmark — no markdown renderer is used or needed here
   since the body is inserted as plain text into a textarea, not rendered as HTML).

No goldmark/blackfriday/markdown-render dependency exists in `go.mod` and none is needed — the prompt
body is treated as opaque text, not rendered markdown.

## Variable interpolation — existing precedent

`session/pipeline_engine.go:240-274` (`renderTemplate`, `recognizedPlaceholders`) is a **direct, deliberate
precedent** for exactly this feature's `{{repo}}`/`{{branch}}`/`{{issue_title}}` substitution:

- Uses `strings.NewReplacer` over a fixed allow-list of placeholder names — explicitly **not**
  `text/template`, with a doc comment explaining why: "no conditionals, no loops, not Turing-complete, to
  resist the templating engine rabbit hole."
- Missing/unavailable placeholder values are simply omitted from the replacer pairs, so unmatched
  `{{...}}` tokens are left in the string un-substituted by default in that implementation — for
  prompt-library, requirements.md explicitly wants undefined variables to render as **empty string**, so
  the analogous prompt-library renderer should pass `""` as the value for any unavailable placeholder
  (e.g. `issue_title` with no linked issue) rather than omitting it from the replacer pairs, which is a
  one-line deviation from `renderTemplate`'s current omission behavior, not a different mechanism.
- Also confirms `strings.ReplaceAll(cmdPart, "{{input}}", arg)` as a second, independent example of the
  same "flat placeholder substitution" idiom in `server/workflows/scheduler.go:139/147`.

**Recommendation:** implement prompt-library's interpolation as its own small `strings.NewReplacer`-based
function (3 fixed placeholders: `repo`, `branch`, `issue_title`), following the `renderTemplate` idiom —
do not reuse `pipeline_engine.go`'s function directly (different placeholder set/package), but mirror its
approach and rationale in a doc comment.

## ConnectRPC service pattern — template for a new small service

`server/services/slash_command_service.go` is the closest structural analog to the required
"template listing/read/save" service: a small, stateless-ish service (`type SlashCommandService struct{}`,
constructed via `NewSlashCommandService()`) that walks filesystem directories (global user dir +
project-local dir under a `target_directory`) for `.md` files, parses lightweight frontmatter per file,
merges results with defined precedence (project overrides user), and returns a flat `repeated` list via a
single RPC. This maps almost one-to-one onto prompt-library's `ListPromptTemplates` (global +
workspace-local dirs, merge/precedence, skip malformed files with a log rather than erroring — see
`ListSlashCommands`'s `err == nil` skip-and-continue pattern in `walkCommandDir`).

Proto precedent: `proto/session/v1/session.proto:2505-2525` — `ListSlashCommandsRequest` /
`SlashCommandInfo` / `ListSlashCommandsResponse`, registered as an RPC on the existing `SessionService`
(`session.proto:382-385`) rather than as a new standalone proto service. Given prompt-library needs 3
operations (list, get, save) versus slash-commands' 1 (list), it's a judgment call for the planning phase
whether to add 3 new RPCs to the existing `SessionService` (consistent with how `ListSlashCommands`,
`ListWorkflows`/`RunWorkflow` etc. are all bolted onto the one big `SessionService`) or introduce a new
small proto file (`proto/session/v1/prompts.proto`) + new `PromptService` — `insights.proto` and
`headless.proto` (170 and 46 lines respectively) show that standalone small proto files with their own
service block are an established pattern in this codebase, not a departure from convention. Either fits
existing convention; recommend a new `prompts.proto` + `PromptService` given `session.proto` is already
very large (2500+ lines) and 3 new RPCs plus their messages would add meaningfully to it.

All RPC changes require `make proto-gen` (regenerates `session/gen/session/v1/*.go` and
`web-app/src/gen/session/v1/*_pb.ts`) per `CLAUDE.md`'s "New API Endpoints" section, and a
`// +api: <scope>:<action>` marker per `.claude/rules/feature-registry.md`.

## Frontend conventions

- **CSS**: new components must use vanilla-extract `.css.ts` colocated files per
  `.claude/rules/css-architecture.md` — confirmed `OmnibarCreationPanel.tsx:18` imports
  `* as styles from "./OmnibarCreationPanel.css"` (a `.css.ts` module), the pattern any new
  `TemplatePicker` component must follow (`TemplatePicker.tsx` + `TemplatePicker.css.ts`).
- **RPC client hook pattern**: `web-app/src/lib/hooks/useSessionService.ts` uses
  `createClient(SessionService, transport)` from `@connectrpc/connect` (`^2.1.1`) against a
  generated `SessionService` from `@/gen/session/v1/session_pb`, wrapped in a custom hook exposing
  typed methods. If prompt-library RPCs are added to a new `PromptService`, the natural analog is a new
  `usePromptService.ts` hook following the same `createClient(PromptService, transport)` shape (reusing
  `getApiBaseUrl`/`createAuthInterceptor` from `@/lib/config`), rather than folding it into the already
  large `useSessionService.ts`.
- **Session-creation UI integration point**: `OmnibarCreationPanel.tsx` already has a `firstPrompt`
  textarea (`omnibar-first-prompt`, ~line 723-747) with an existing slash-command autocomplete wired to
  it (`line 179` comment: "Slash command autocomplete for the firstPrompt textarea") — this is the
  established precedent for "a picker populates the same textarea," which the new "From template" picker
  should follow (populate `firstPrompt` on select, leave it user-editable, per requirements.md AC #3).
- **Session type registry**: requirements.md AC #9 is correct per
  `.claude/rules/session-creation-registry.md` — templates only change the *content* seeded into
  `firstPrompt` for existing creation modes; they do not add a `SessionType`, so the 7-touchpoint
  registry does not apply. Confirmed no existing `SessionType` maps to "template" in
  `SESSION_TYPES` (`OmnibarCreationPanel.tsx:28`).
- **React / build versions**: `react ^19.0.0`, `@connectrpc/connect ^2.1.1`, `@bufbuild/protobuf ^2.11.0`
  (`web-app/package.json`) — no new frontend deps needed for markdown/YAML parsing since templates are
  parsed server-side in Go and delivered to the frontend as already-parsed proto messages (name,
  description, tags, body) — the frontend never parses YAML/markdown itself.

## Pinned versions relevant to this feature

| Dependency | Version | Location |
|---|---|---|
| `gopkg.in/yaml.v3` | v3.0.1 | `go.mod:48` |
| `react` | ^19.0.0 | `web-app/package.json` |
| `@connectrpc/connect` | ^2.1.1 | `web-app/package.json` |
| `@bufbuild/protobuf` | ^2.11.0 | `web-app/package.json` |

No new Go or npm dependency is required for this feature.
