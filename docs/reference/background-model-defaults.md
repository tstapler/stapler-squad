# Background model defaults

Unattended LLM work used to inherit the account default model, usually the most
expensive. Opus costs several times Sonnet, and Sonnet more than Haiku, against the
subscription quota ([models and usage limits](https://support.claude.com/en/articles/14552983-models-usage-and-limits-in-claude-code));
thinking tokens bill as output ([costs](https://code.claude.com/docs/en/costs)). So
background work is pinned. Code: `config/background_models.go`.

## Headless features (`--model` on the headless pool)

Precedence per call: explicit `CallOptions.Model` > `background_models.features[key]`
> built-in default below > pool `DefaultModel` (none).

| Feature key | Default | Why |
|---|---|---|
| `session-completion-summary`, `handoff-summary` | haiku | Short summarisation of a transcript |
| `backlog-intent-parse` | haiku | Free text to a small JSON draft |
| `pr-description`, `commit-message`, `summarize`, `acceptance-criteria` | haiku | Single-shot text from a diff |
| `session-tagging` | haiku | Own setting: `tagging_classifier.model` |
| `autonomous_approval` | sonnet | A wrong APPROVE is unsafe; Haiku is too weak for a security judgement |
| `autonomous_fix` | sonnet | Code-writing; a bad fix costs a rework loop |
| `review`, `triage`, `custom`, `rules-generation`, `instance-resume` | sonnet | Judgement-heavy or open-ended; pinned so the haiku pool default does not apply to them |
| `unfinished-work-summary` | haiku | Short summarisation |

The pool only launches `claude`, so these pins never reach agy/gemini/proxy-claude.
A reused session keeps its model until it rotates (25 calls or errors).

## Backlog stages

Order: pipeline-mode stage executor (per item) > `background_models.stages[role]` > default.

| Stage | Default | Notes |
|---|---|---|
| work | sonnet | Spawned as `claude --model <id>`; skipped when the item's executor program or the resolved default program is not bare `claude` (a pin would replace flags or a wrapper such as proxy-claude) |
| review | sonnet | Headless; only when the executor program is empty or `claude` |

Triage is not a stage pin: it uses [`headless_triage_model`](../../config/config.go)
(default `family:sonnet`, `none` = account default), and a pipeline-mode triage executor
overrides it. `background_models.stages` has no triage entry.

The chosen model is stored on the item session as `resolved_model`. The executor drift
hash still covers only the raw pipeline-mode pair, so pinning does not flag drift.

## Effort

`claude --help` (2.1.290) lists `--effort <level>` with `low, medium, high, xhigh, max`.
`background_models.effort` is appended to work-session programs as `--effort <level>`,
only on a `claude --model ...` program and only for those five values; agy, gemini and
proxy-claude are unchanged. Defaults to `medium` when unset or invalid (`config.DefaultBackgroundEffort`); set `"off"` to leave the CLI default. Not applied to headless calls: the pool
launches `claude -p`, and effort there is left to the CLI default.

## Setting values

Edit `~/.stapler-squad/config.json` (read fresh on each use, no restart):

```json
{"background_models": {
  "features": {"handoff-summary": "sonnet"},
  "stages": {"review": "haiku"},
  "effort": "medium"
}}
```

Or use **Settings → Background Models** (`/settings/background-models`,
`GetBackgroundModels`/`UpdateBackgroundModels`): blank fields use the built-in default,
saves apply on the next call, no restart.

An override that is not a plausible model name (whitespace, leading `-`, over 128
characters), or an `effort` that is not one of the five levels, is ignored with a
`warn` log and the built-in default applies (`usableModel` in `config/background_models.go`).

## Pool-wide default

The headless pool is built with `DefaultModel: haiku` (`config.HeadlessPoolDefaultModel`),
so a call that names neither a model nor a pinned feature key is never sent on the account
default.

## `CLAUDE_CODE_SUBAGENT_MODEL` evaluation (2026-10-07, claude 2.1.293)

Per the [model-config docs](https://code.claude.com/docs/en/model-config), the env var sets
the model for subagents, agent-team teammates and workflow agents that are not given one
another way; a definition's `model` field wins unless `CLAUDE_CODE_SUBAGENT_MODEL_FORCE` is
set. Decision: **not set by the app.** Subagent work in work sessions ranges from file
search (Haiku-suitable) to code review (not), so one blanket value would trade quality for
cost without evidence, and it can only be injected as an env var, which this feature avoids
as a control surface. Revisit with measured per-subagent cost data; the work-session
`--model` pin and `--effort` already bound the main thread, which is the dominant spend.
