# Research
Sources: direct repo inspection (grep/ls) in this worktree.

- VERIFIED: Storybook 8.6 configured (`web-app/.storybook/main.ts`, `@storybook/react-webpack5`, glob `src/**/*.stories.@(ts|tsx)`; css.ts mock loader exists). Scripts `storybook`, `build-storybook` in package.json. Only 3 stories: Kbd, KeyboardShortcutOverlay, DrawerNav, SessionDetailBar (4 files).
- VERIFIED: `components/ui/index.ts` barrel exports only Button, Badge, Input, Card, Modal; ~35 other ui components not exported.
- VERIFIED: `RepoPathInput` used in UnfinishedSourcesSettings, DirectoryRulesManager, JulesSettings, GlobalDefaultsForm, BacklogItemForm, WorkflowForm, NewShellDialog, OmnibarCreationPanel, LocalFileBrowser. NOT used (path-like fields): `sessions/TriggerFormModal.tsx:363`, `history/ForkModal.tsx:91` (#fork-path), `settings/AliasesManager.tsx:457`, `settings/ProgramsManager.tsx:352` (command/path; probably exempt), OmnibarCreationPanel:795 worktree placeholder (check).
- VERIFIED: 18 route pages: account, backlog, config, errors, files, help, history, insights, login, logs, notifications, resolve, review-queue, rules, settings, triggers, unfinished, workflows (+ root page, sessions, workflows etc.).
- Existing test infra: jest + RTL (`*.test.tsx` next to components), Playwright e2e under tests/e2e incl. accessibility.spec.ts, Axe in CI; `@storybook/addon-a11y` available.
- INFERRED: mocking hooks (usePathCompletions etc.) needed for RepoPathInput stories; follow RepoPathInput.test.tsx mocks.
## Approach options
A. Manual audit only — rejected (unverifiable).
B. Audit via script (AST/grep for bare inputs, raw buttons) + Playwright a11y sweep + manual page walk — chosen.
C. Add storybook-test-runner/interaction tests — optional; adds Playwright dependency on built storybook; defer unless cheap.
Guard: jest test scanning src for `<input` whose id/placeholder/label matches path|dir|folder, allowlist file. Cheaper and more deterministic than a custom ESLint plugin (repo has local plugins; could upgrade later).
## Risks
Storybook webpack vs vanilla-extract (.css.ts) — mock loader exists, verify with build. RepoPathInput hooks need RPC client -> story decorators/mocks. Scope creep of "every page" — timebox, severity-triage, ticket the rest.
