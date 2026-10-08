# Background model and effort policy

Background LLM work (backlog work/review sessions and headless pool features) is pinned to
cheaper models instead of the account default. Settings live in `config.json` under
`model_policy` and are editable at runtime through the `GetModelPolicy`/`UpdateModelPolicy`
RPCs. They are re-read on every call, so no restart or env var is involved.

| Key | Default | Applies to |
|---|---|---|
| `work` | `family:sonnet` | Work sessions on the default pipeline (`claude --model … --effort …`) |
| `review` | `family:sonnet` | Headless review calls when the pipeline pins no model |
| `completion_narrative`, `handoff_summary`, `intent_parse`, `pr_description` | `family:haiku` | Headless pool features |
| `background_effort` | `medium` | `--effort` on work sessions and headless pool sessions |

Values: `family:<alias>`, a concrete model ID, or `none` (account default / no effort flag).
Effort levels: `low`, `medium`, `high`, `xhigh`, `max`.

- No default resolves to Opus; use `family:opus` explicitly to opt in.
- A pipeline-mode model pin always wins. Executor-hash drift detection hashes only the
  pipeline's raw values, so the policy fallback never registers as drift.
- Non-claude programs never receive a Claude model ID or `--effort`.
- An invalid configured value logs a warning and falls back to the built-in default.
- The headless pool's `DefaultModel` is Sonnet, for calls with neither a per-call model nor a
  per-feature policy (tagging and triage keep their own settings).
- Changing a pool feature's model starts a fresh pool session (resumed sessions keep their model).

## CLAUDE_CODE_SUBAGENT_MODEL evaluation

`claude --help` (2.1.293) lists `--effort <level>`, which is now used. The
`CLAUDE_CODE_SUBAGENT_MODEL` env var is documented upstream but was not verifiable from the
local binary, so it is deliberately not set: forcing Haiku on subagents would silently degrade
review and exploration subagents inside work sessions, and the work session's own `--model`
already bounds its cost. Revisit if subagent spend dominates in a quota audit.
