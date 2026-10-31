# ESLint rules formerly supplied by `next/core-web-vitals`

`web-app/eslint.config.mjs` (flat config) lists these rules explicitly. `eslint-config-next` is still
installed (rollback path) but nothing extends it.

Inventory taken with `eslint --print-config src/app/page.tsx` before and after the change: the
effective rule sets are identical except for the 21 `@next/next/*` rules, which no longer exist.

| Source plugin | Rule | Severity |
|---|---|---|
| `eslint-plugin-react-hooks` 5.2.0 | `rules-of-hooks` | error |
| | `exhaustive-deps` | warn |
| `eslint-plugin-jsx-a11y` 6.10.2 | `alt-text` (`img`, `Image`) | warn |
| | `aria-props`, `aria-proptypes`, `aria-unsupported-elements` | warn |
| | `role-has-required-aria-props`, `role-supports-aria-props` | warn |
| `eslint-plugin-import` 2.32.0 | `import/no-anonymous-default-export` | warn |
| `eslint-plugin-react` 7.37.5 (`recommended`) | `display-name`, `jsx-key`, `jsx-no-comment-textnodes`, `jsx-no-duplicate-props`, `jsx-no-undef`, `jsx-uses-react`, `jsx-uses-vars`, `no-children-prop`, `no-danger-with-children`, `no-deprecated`, `no-direct-mutation-state`, `no-find-dom-node`, `no-is-mounted`, `no-render-return-value`, `no-string-refs`, `no-unescaped-entities`, `require-render-return` | error |
| | `no-unknown-property`, `react-in-jsx-scope`, `prop-types`, `jsx-no-target-blank` | off |

Dropped: all 21 `@next/next/*` rules (for example `no-img-element`). Six `eslint-disable` comments in
`src/` still name `@next/next/no-img-element`, and a disable comment for an unknown rule is reported as
an error, so the config registers a no-op stub of that one rule. Delete the six comments and the stub
together in a follow-up; editing those files now would put their pre-existing warnings under the
changed-files gate.

## How the lint runs

| Command | Scope | Fails on |
|---|---|---|
| `bun run lint:eslint` (`eslint .`) | `src/**` | errors only; the pre-existing warnings are tolerated |
| `bash scripts/eslint-gate.sh <files>` | the given files | any warning or error (`--max-warnings 0`) |
| `bun run lint:affected` | files changed vs `origin/main` | any warning or error, via the gate |

CI (`.github/workflows/lint.yml`, the `lint` job) runs the gate over files changed by the push or PR.
Files outside `src/**` are skipped silently, as `next lint` covered only `src/`.

## Rollback

Revert the PR that introduced `eslint.config.mjs`. `eslint-config-next` is still installed, so the
`.eslintrc.json` path and `next lint` return whole.

## Verifying a config change

`scripts/eslint-report-diff.mjs` compares two `eslint -f json` reports as (file, rule id, severity)
multisets. The flat-config conversion produced an empty diff over 1,855 findings:

```bash
cd web-app
ESLINT_USE_FLAT_CONFIG=false eslint src -f json -o before.json   # on the pre-conversion commit
eslint . -f json -o after.json
node scripts/eslint-report-diff.mjs before.json after.json
```
