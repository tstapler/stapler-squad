# Build vs Buy: CLI flag discovery

Scope: parse `<binary> --help` into `{name, short, takes_value, description}` for arbitrary user-configured agent CLIs (see `../requirements.md`).

## Bottom line

Build a small bespoke parser in Go, bounded by fixtures captured from real tools. No OSS library parses arbitrary `--help` text. Reuse the repo's existing exec helpers for the sandbox. Confidence: VERIFIED for repo facts and license/maintenance (GitHub API queried 2026-09-21); library capability claims are from prior knowledge unless marked.

## 1. Existing OSS libraries

The libraries fall into two groups, and neither fits. The first group defines or generates a CLI, so it needs the tool's source. The second group ships hand-written specs for known tools. None ingests unknown `--help` text.

`go.mod` check (VERIFIED via `grep`): `spf13/cobra v1.10.2` (direct) and `spf13/pflag v1.0.10` (indirect) are present. `posener/complete`, `carapace`, `kong`, `go-flags` are not. Cobra is a builder for this repo's own CLI, not a parser of other binaries' output, so its presence gives no reuse.

| Library | License / maintenance (GitHub API) | What it does | Fit | Verdict |
|---|---|---|---|---|
| posener/complete | MIT, 956 stars, last push 2025-03 | Generates shell completion for a Go program you write | Wrong direction (emits completions, does not read help) | Not recommended |
| carapace (carapace-sh/carapace) | Apache-2.0, 1443 stars, pushed 2026-09 | Completion framework for cobra CLIs; `carapace-bin` bundles completers for many tools | Needs the tool's cobra tree or a hand-written completer. Not a help parser | Not recommended |
| kong / go-flags | MIT, active / BSD-3, last push 2024-07 | Struct-tag CLI definition for your own program | Wrong direction | Not recommended |
| jc (kellyjonbrazil/jc) | MIT, 8681 stars, active | Python; parses output of specific commands to JSON via per-command parsers | Python runtime dependency; no generic `--help` parser (INFERRED from its design, not tested) | Not recommended |
| withfig/autocomplete specs | MIT, 25k stars, last push 2025-05 (Fig sunset, quiet since) | Hand-written TS specs for ~700 CLIs | Only covers popular tools, may lag versions (claude/aider flags churn). Shipping it needs a Node/TS spec loader | Not recommended as the source. Viable only as an optional offline hint layer, out of scope |
| microsoft/inshellisense | MIT, 10.7k stars, active | Consumes Fig specs for IDE-style completion | Terminal UI tool, not a library | Not recommended |
| argcomplete (kislyuk) | Apache-2.0, active | Python argparse completion hook | Python only | Not recommended |
| help2man / `--help` parsers | n/a | help2man goes help to man page; no maintained Go parser of comparable maturity found | I did not find a maintained Go library. Absence of evidence: I did not run an exhaustive search | Not recommended |

### Viable alternative worth noting: cobra `__complete`

Any cobra-based binary answers `tool __complete <args> ""` with a machine-readable list. VERIFIED on this machine: `gh __complete pr list --` printed `--help`, `--repo`, `--app`, `--assignee`, `--author` with descriptions, tab-separated. It covers gh, kubectl, docker (cobra-based), but not claude, aider, agy, rg, uv, git. That is too narrow to be the primary path. It could be an optional enrichment behind the same sandbox, but it adds a second exec per probe and a second parser. Verdict: Viable, defer (adds scope beyond AC 4).

## 2. SaaS / managed

N/A. Flag discovery has to execute the user's local binary on the user's machine, so a hosted service cannot do it. Sending help text to an LLM API is possible but unnecessary for a deterministic, offline, fast blur-time check and would add latency, cost and a data-egress question. Verdict: Not recommended.

## 3. Bespoke parser (LLM-written regex) vs library

No library exists to compare against (section 1), so the real choice is "bespoke and fixture-bounded" vs "no parser". The correctness risk of a regex parser is real: help formats differ. Observed on this machine (VERIFIED by running `<tool> --help`):

| Tool | Available | Style seen |
|---|---|---|
| claude | yes | commander.js: `--flag <value>  Description`, wrapped continuation lines indented, variadic `<tools...>`, long names split onto own line with description on next line (`--allowedTools, --allowed-tools <tools...>` then indented text) |
| aider | yes | argparse usage block only in the first lines: `[--model MODEL]`, `[--verify-ssl \| --no-verify-ssl]`. Option table follows |
| git | yes | `--help` shows a usage synopsis and command list, not an options table (per-subcommand pages are man pages). Expect zero flags (subcommands are out of scope) |
| gh (+ `gh pr list`) | yes | cobra: `FLAGS` section, `  -a, --assignee string   Filter by assignee` |
| gemini | yes | yargs: `-d, --debug  desc  [boolean] [default: false]`. Prints a stderr config error first, so capture stdout and stderr together |
| opencode | yes | yargs |
| pi | yes | hand-rolled: `--provider <name>  desc`, `--print, -p  desc` (short after long) |
| agy | yes | Go `flag` package: `-c`, `--continue  desc`, no value hints (`--add-dir  desc (default [])`), takes_value not detectable from the line |
| rg, uv, cargo | yes | clap, useful for a clap fixture (`-e, --regexp <PATTERN>`) |
| codex | MISSING | cannot capture |
| tmux | yes | rejects `--help`, prints usage on stderr: treat as unparseable, empty flags |

Suggested bounding:
1. Golden fixtures: commit real `--help` output as `testdata/help/<tool>-<version>.txt` for claude, aider, gh, gh-pr-list, gemini, agy, pi, rg, uv, plus git and tmux as "expect empty" negatives. Table-driven test asserts an exact expected flag list (name, short, takes_value) per fixture. Strip version-volatile lines rather than regenerating, and note the tool version in the filename.
2. Never-fail contract (AC 4): fuzz/`testing.F` that any input returns no panic and a bounded result. Also assert every returned flag name matches `^--?[A-Za-z0-9][\w-]*$` so garbage never reaches the UI.
3. Precision over recall: only accept a line as a flag when it starts (after indentation) with `-`. Use continuation-line joining only for indented lines with no leading `-`. A missed flag costs one false "unknown flag" warning, and warnings are non-blocking (AC 6), so under-parsing is the safe failure.
4. Regeneration script (`go test -run TestFixturesFresh -update` or a Make target, gated off CI) documents provenance. Do not run real tools in CI, since they are not installed there.
5. Treat AC 10's "claude/aider/git-style" as claude/aider/gh-cobra-style. Git's top-level help does not contain flags, so a git fixture is a negative case.

Verdict: Bespoke parser, Recommended, with the fixtures above. Regex should be written by hand against fixtures, not accepted from an LLM without those tests passing.

## 4. Fork / adapt: what the repo already has

Reusable (VERIFIED by reading files):
- `executor/safeexec/safeexec_pg.go`: `CommandContextPG` sets `Setpgid: true`, `WaitDelay`, and a `cmd.Cancel` that SIGTERMs the whole process group. This is the exact primitive that AC 3/8 need ("kill process group"). Prefer it over raw `exec.CommandContext`.
- `executor/safeexec/safeexec_pdeathsig_linux.go`: `EnsurePdeathsig` so a probe child dies if the server dies.
- Lint gate: `tools/lint/norawexec` fails any direct `exec.Command/CommandContext` outside `executor/` and `executor/safeexec`. The probe must go through safeexec (or take `//nolint:norawexec`), so this choice is forced, not optional.
- `config/executor.go:13` `CommandExecutor` (Command/Output/LookPath) for injection in tests. Note its `Output` uses the timeout executor's `OutputWithPipes`; check whether it caps output size. I did not verify a size cap, and none was found in the grep for `LimitReader`/`LimitedReader` (see gap below).
- `executor/timeout_executor.go` (`TimeoutExecutor.OutputWithPipes`): existing timeout capture. Whether it suits a 256KB cap is UNVERIFIED.

Not present: a help-text parser, or any output size limiter. The `grep` for `LimitedReader|io.LimitReader` returned hits only in `executor/managed_process*`, `executor/shortlived*` and `executor/safeexec/*` files listed as containing `Setpgid` or limits. I did not open those to confirm which pattern matched. Implementation should read `executor/shortlived.go` before writing its own bounded-capture code, in case a size-limited runner already exists.

Verdict: reuse `safeexec.CommandContextPG` and the `CommandExecutor` seam, Recommended. Write only the parser and a small size-capped writer (about 20 lines).

## Recommendation summary

| Option | Verdict |
|---|---|
| Bespoke fixture-tested Go parser | Recommended |
| Reuse `safeexec.CommandContextPG` and `CommandExecutor` seam | Recommended |
| cobra `__complete` enrichment | Viable, defer |
| withfig specs as hints | Viable, out of scope |
| posener/complete, carapace, kong, go-flags, jc, inshellisense, argcomplete | Not recommended |
| SaaS / LLM-at-runtime parsing | Not recommended |

Gaps: `findings.md` says "reject third-party parser deps"; this research supports that conclusion, with the reasoning being "no library does this job" rather than YAGNI alone. Library capability descriptions in the table are from prior knowledge, not re-verified against source; only license, star count and push date were queried.
