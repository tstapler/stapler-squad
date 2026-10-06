# Plan
Order: T1 audit tooling -> T2 path-field fixes -> T3 stories -> T4 guard/CI -> T5 page sweep fixes.

1. Audit script `web-app/scripts/audit-ui.mjs`: scan for bare path inputs, raw `<button>`/`<input>` outside ui/, component usage map; emit `audit-findings.md` + `component-usage.md`. (3h, test)
2. Replace bare inputs with `RepoPathInput` in TriggerFormModal, ForkModal, AliasesManager (+ any script finds); update their tests; keep ids/data-testid for e2e. (4h, frontend)
3. Jest tests per migrated field (renders RepoPathInput, onChange). (2h, test)
4. Storybook infra: shared decorators/mocks for RPC hooks, theme; verify `build-storybook`. (3h, frontend)
5. Stories for all ui components (Button, Badge, Input, Card, Modal, Tooltip, Collapsible, RadioGroup, Skeleton, ErrorState, RepoPathInput, AutocompleteInput, FlagCombobox, ...), states per component. (6h, frontend)
6. Guard test failing on new bare path inputs (allowlist w/ justification) and on ui components lacking stories. (2h, test)
7. Page-by-page audit of 18 routes: run app on manual port block (62871), walk pages, Axe sweep; record in audit-findings.md. (4h, frontend)
8. Fix high/medium findings from sweep (empty/loading/error states, labels, focus, consistency w/ ui primitives); ticket the rest. (6h, frontend)
9. Extend e2e a11y spec to cover audited routes; add `// @feature` header. (3h, test)
10. Docs: `docs/how-to/use-component-library.md`, index in CLAUDE.md; wire build-storybook into CI/make ready optionally. (1h, docs)
11. Run `pnpm lint`, jest, `lint:duplicates`, build-storybook, affected e2e; record evidence. (2h, test)
## Adversarial review
- "Every page" unbounded -> severity triage + timebox; unfixed items ticketed, listed in findings.
- Guard heuristic false positives -> allowlist file w/ reasons (e.g. ProgramsManager command field).
- jscpd threshold: stories are repetitive -> use shared `args`/helpers; check `.jscpd.json` ignores stories.
- Don't restart live service; use separate instance.
