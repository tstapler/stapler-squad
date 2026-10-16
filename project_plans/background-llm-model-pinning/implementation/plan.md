# Plan (revised 2026-10-07: delta on top of upstream #930)
1. AC9: pool `DefaultModel = config.HeadlessPoolDefaultModel` (haiku).
2. AC7: `usableModel` validation + warn log for feature/stage models and effort.
3. AC4: `GetBackgroundModels`/`UpdateBackgroundModels` RPCs (validated, persisted to config, read live), `BackgroundModelsSettings` panel at `/settings/background-models`, scanner mapping, registry regenerate.
4. AC8: document CLAUDE_CODE_SUBAGENT_MODEL evaluation (decision: not set) in docs/reference/background-model-defaults.md.
5. AC1,2,3,5,6: already satisfied upstream; add regression test that no default resolves to Opus.

## Adversarial review
- Over-strict name regex could reject a legitimate ID -> admits `[A-Za-z0-9._:\[\]-]`, covers aliases, family refs, full IDs, `[1m]`.
- Haiku pool default could degrade an unpinned quality-sensitive call -> sensitive callers (autonomous_*) are pinned sonnet; override via panel.
- Update RPC replaces all overrides: UI sends the full state it loaded, so no silent drops.
