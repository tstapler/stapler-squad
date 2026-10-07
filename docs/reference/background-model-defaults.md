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
proxy-claude are unchanged. Unset by default. Not yet applied to headless calls.

## Setting values

Edit `~/.stapler-squad/config.json` (read fresh on each use, no restart):

```json
{"background_models": {
  "features": {"handoff-summary": "sonnet"},
  "stages": {"review": "haiku"},
  "effort": "medium"
}}
```

A settings-panel control for these is not built yet.
