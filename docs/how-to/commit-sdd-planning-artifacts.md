# Commit SDD Planning Artifacts Before Ending a Session

`project_plans/<project>/` files produced by `/sdd:1-ideate` through `/sdd:4-validate` (`requirements.md`, `research/*.md`, `plan.md`, `validation.md`, `decisions/ADR-*.md`) are durable spec artifacts, not scratch output — commit them before ending the session that produced them, even though the general rule is "never commit unless explicitly asked."

**Wrong:**
```
# Phase writes files, session ends here
Write(project_plans/my-feature/requirements.md, ...)
Write(project_plans/my-feature/research/stack.md, ...)
# session ends — files sit uncommitted, invisible outside this worktree
```

**Right:**
```bash
git add project_plans/my-feature/
git commit -m "chore(sdd): planning artifacts for my-feature"
```

## Why

Nothing currently instructs an SDD phase to commit its own output — the phase skills (`~/dotfiles/.claude/skills/sdd/skills/{1-ideate,2-research,3-plan,4-validate}/SKILL.md`) write files and stop. Combined with the (correct, for code) default of never auto-committing without being asked, planning docs are silently left uncommitted unless a user happens to remember to ask. A 2026-07-14 worktree audit found this pattern repeatedly — e.g. a worktree with 9 uncommitted `project_plans/image-upload/*` docs that would have been permanently lost to a routine worktree cleanup had they not been individually quarantined first. Planning artifacts are the one category of output where the SDD workflow's own value depends on them surviving past the worktree that created them — see the precedent commit `af5b37a2` (`chore(sdd): planning artifacts for backlog-stuck-item-visibility`), which is the exception this rule generalizes.

Note: this rule only covers this repo's project-local convention. The same gap exists in the shared SDD skill definition itself (`~/dotfiles/.claude/skills/sdd/`), which is used across other repos (stelekit, personal-wiki, etc.) — fixing it there is a separate, cross-project change, not something this rule enforces.
