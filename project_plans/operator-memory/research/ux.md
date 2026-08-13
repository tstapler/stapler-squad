# UX Research: Operator Memory (`stapler-squad memory show`)

Scope per requirements.md: this item ships **read-only** surfaces only — the
frozen-snapshot injection into headless prompts, and a `memory show` CLI
command. No `memory edit`/`memory add` write command exists yet.

## 1. House style for "show config/state" CLI commands

Surveyed `main.go`'s cobra command set (`resetCmd`, `debugCmd`,
`listSessionsCmd`, `versionCmd`) plus `cmd/commands/get_session.go`. Two
conventions coexist, split by whether the command talks to local state
directly or to the running server over RPC:

- **Local state/config commands → plain text, `fmt.Println`/`Printf`,
  human-readable, no JSON.** `debugCmd` prints the config path then the
  config struct as indented JSON as a *value* dump, but wraps it in a plain
  `"Config: %s\n%s\n"` line — the JSON is the payload, not the framing.
  `listSessionsCmd` is the closest analogue to `memory show`: header line
  with a count (`"Found %d sessions:\n\n"`), numbered entries, indented
  `"   Field: value"` sub-lines, blank line between entries, and an explicit
  empty-state message (`"No sessions found"`) rather than an empty list.
  `resetCmd` prints one plain confirmation line per completed step.
- **RPC-backed commands → JSON via `json.MarshalIndent`.** Only
  `cmd/commands/get_session.go` (`GetSessionCmd`) does this, because it's
  fetching a proto message from a live server over ConnectRPC — JSON is the
  wire format already, so re-serializing it is the path of least resistance.

`memory show` reads two local files directly (no RPC round-trip), so it
belongs in the first bucket: **plain text, not JSON.** Register it the same
way `commands.GetSessionCmd` is registered in `main.go`
(`rootCmd.AddCommand(commands.GetSessionCmd)`) — either add a `memory`
parent command with a `show` subcommand in `cmd/commands/`, or a
`memoryCmd`/`memoryShowCmd` pair alongside the other local-state commands in
`main.go`. Given cobra's nesting model and that a future `memory edit`/`add`
would hang off the same parent, prefer a `memoryCmd` with `AddCommand(memoryShowCmd)`
rather than a flat `memory-show` command name.

## 2. Proposed output

### Both files exist and are populated

```
Operator memory (~/.stapler-squad/memory/OPERATOR.md):

  - Team prefers small, frequent PRs over large batched ones.
  - CI flakes on TestRemoveHooksConfig; do not silently re-excuse it.

Repo memory (/home/tstapler/.stapler-squad/workspaces/<hash>/memory/REPO.md):

  - `make build` must run before `make test` — protos aren't generated otherwise.
  - Flaky: TestEnsureServerRunning_NoOp (session/tmux)
```

Render the raw markdown body under each header, not a re-parsed/re-formatted
version — the file IS the source of truth and round-tripping it (e.g.
through a markdown renderer) risks divergence between what's shown and
what's actually injected into the prompt.

### One or both empty/missing

Per AC5, empty files produce no prompt block — but `memory show` is a
diagnostic command, so it should say so explicitly rather than silently
omitting the section (silently omitting is correct for the *prompt*, wrong
for a command whose whole job is visibility):

```
Operator memory (~/.stapler-squad/memory/OPERATOR.md): empty (no content injected)

Repo memory (/home/tstapler/.stapler-squad/workspaces/<hash>/memory/REPO.md): not found (no content injected)
```

Distinguish "file missing" from "file exists but empty/whitespace-only" in
the label (`not found` vs `empty`) — both collapse to the same prompt-time
behavior (no block), but for a human debugging "why didn't the model know
X," the distinction matters (missing = never written; empty = written then
cleared, or touched but never populated).

### Run from a directory with no distinct workspace

This scenario **cannot produce an error** given the current implementation —
confirmed by reading `config.GetConfigDirForDir`/`resolveDefaultConfigDir`
(`config/config.go:121-220`). Per-directory workspace isolation is opt-in via
`STAPLER_SQUAD_WORKSPACE_MODE=true`; without it (the default), every
directory resolves to the same shared `~/.stapler-squad` config dir. So
`REPO.md` for an "unrecognized" directory silently resolves to
`~/.stapler-squad/memory/REPO.md` — functionally a second global file,
sitting right next to `OPERATOR.md`. There is no "not a workspace, can't
show repo memory" error state to design for.

This is worth surfacing to the operator rather than hiding: **always print
the resolved path**, as in the examples above, not just a relative label
like "Repo memory:". Since the path silently depends on workspace mode /
the preferred-workspace file, showing the resolved absolute path is the only
way a user running from two different directories can tell whether they're
looking at the same REPO.md or two different ones. This isn't a new
requirement — AC3 already just says "displays current contents" — but the
resolved-path line costs one `fmt.Printf` and prevents a confusing "why does
`memory show` say the same thing in every repo" support question later.

## 3. Accessibility / keyboard

Not applicable — plain-text CLI stdout output, no interactive/keyboard
surface. Skipping per task instructions.

## 4. Display-time escaping for hand-edited files

Requirements scope the injection scanner strictly to the write path ("scan
before any write"), and this item ships no write command — files are hand-
edited with a text editor. Two distinct questions follow:

- **Does `memory show` need read-time sanitization/escaping?** No new
  scanning logic is needed. `memory show` writes the file's raw bytes to
  the operator's own terminal (stdout) — the same person who just hand-
  edited the file is the only audience. There's no trust boundary crossed
  by echoing a file back to the human who owns it; a "prompt injection
  attempt" is only a threat at the point it reaches the *model's* context
  (headless prompt assembly), not at the point it reaches the operator's own
  screen. Do not build a display-time scanner for this item — it would be
  unused defense with no threat model, the same anti-pattern the
  requirements doc explicitly warns against for the write scanner
  ("state that explicitly rather than building an unused scanner").
- **What `memory show` genuinely should do:** treat the file as inert text.
  Print it via `fmt.Println`/`Printf` (or `os.Stdout.Write`), not through
  any templating/interpolation step — the only real "escaping" risk here is
  accidental format-string interpretation if the file content is ever passed
  as a `Printf` format argument rather than a `%s` value. Standard Go string
  handling avoids this as long as file content is always an argument, never
  the format string itself.
- **Terminal-escape-sequence caution (adjacent, not injection):** since the
  file is hand-edited and could theoretically contain raw ANSI escape
  sequences (accidentally pasted, or a copy-paste artifact), a defensive-but-
  optional nicety is stripping/neutralizing control characters before
  writing to the terminal. This is a terminal-safety concern, not a prompt-
  injection concern, and is not required by any AC — flagging as a low-
  priority nice-to-have only, not a scope item.

The write-time injection scanner (the one AC2 actually requires, per the
"companion writer" item's future `memory edit`/`add` path this item
explicitly does not build) is out of scope for `memory show` entirely.

## 5. Job-to-be-done

**Job:** "After a headless triage/review call made an unexpected decision, let
me quickly confirm what institutional knowledge it had available — was it
working from stale/wrong operator notes, or did it just make a bad call with
correct information?" `memory show` answers "what would the model have seen"
without needing to grep raw prompt logs or reconstruct the snapshot logic by
hand. Secondary job: "let me sanity-check what I just hand-edited into
OPERATOR.md/REPO.md actually saved correctly (right file, right content, no
typo in the path)" — closer to a `cat`-with-context than a full viewer, but
the count/path framing (§2) is what elevates it above a bare `cat` for the
first job.

**Suggestion (not a requirement, do not fold into ACs):** a one-line
debuggability breadcrumb — e.g. `memory: 412 bytes from OPERATOR.md, 0 bytes
from REPO.md` — emitted into the same structured log line/span that already
records headless prompt assembly (wherever `BuildHeadlessTriagePrompt` /
`BuildHeadlessReviewPrompt` currently logs prompt construction, if it does;
research/stack.md or the code-path research agent should confirm whether
such a log line exists today). This closes the loop between "what `memory
show` says is on disk right now" and "what a *specific past call* actually
saw" — `memory show` only reflects current file state, not what a call from
an hour ago loaded before someone hand-edited the file again. Byte counts
are cheap, low-noise (one line, no content duplication into logs — avoids
leaking hand-authored notes into log aggregators), and directly serve the
stated problem ("why did it make an unexpected choice"). Flagging as a
plan.md open question / nice-to-have, not scoping it into this item's ACs
since it touches prompt-assembly logging, not the CLI or storage layer this
item covers.

## Open question for plan.md

The originating issue's "Proposed Work" section (per the task prompt)
describes a CLI to **"view/edit"** the stores, but requirements.md's
functional requirements and ACs list only `stapler-squad memory show`
(view). There is no `memory edit`/`memory add` write command in this item's
scope — confirmed by requirements.md itself, which explicitly defers the
scanner "to whatever write path this issue actually introduces" and says to
"state that explicitly rather than building an unused scanner" if no write
command ships. **Flag this discrepancy explicitly in plan.md**: either (a)
confirm the write command is intentionally deferred to the companion
background-reviewer item (requirements.md's "Scope for this item" section
implies this — "until that lands, OPERATOR.md/REPO.md are operator-edited by
hand"), or (b) if a manual `memory edit`/`add` command was actually intended
for *this* item (distinct from the automatic background writer), scope it
in explicitly along with its own injection-scan integration test. Given
requirements.md's explicit language, (a) reads as the intended scope, but
the discrepancy with the original issue text should be called out rather
than silently resolved.
