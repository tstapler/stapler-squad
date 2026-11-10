# Add docs coverage for a registry feature

Every user-facing registry feature needs a sidecar under `docs/registry/docs-coverage/` that links to its docs, or an `exempt` reason. The `docscoverage` checker verifies the links resolve. Design: ADR-005 (`project_plans/promo-materials-refresh/decisions/ADR-005-docs-coverage-gate-sidecar-ratchet.md`).

## Check where you stand

```bash
make docs-coverage            # report only; same output CI prints to the job summary
```

The report shows covered, exempt, grandfathered and gap counts side by side, then one line per problem, for example `foo-panel: missing help, website`.

## Give a feature docs

1. Find the sidecar name: the feature `id` with `%`, `:` and `/` percent-encoded. `alias:invoke` is `alias%3Ainvoke.json`; an id with none of those is `<id>.json`.
2. Create or edit the file:

   ```json
   {
     "id": "backlog-board",
     "help": "backlog#views",
     "website": "help/getting-started#install"
   }
   ```

   - `help` is a slug listed in `DOC_FILES` in `web-app/src/lib/docs/docLoader.ts`, optionally with `#anchor`. The page is `web-app/public/docs/<slug>.md`. A slug that is not in `DOC_FILES` fails.
   - `website` is a docs-collection id under `website/src/content/docs/` (`help/getting-started`, `guides/...`), optionally with `#anchor`. Anchors are checked against the page's headings with the same slug rules Starlight uses (github-slugger).
   - `tour` is a tour id; that tour's `tour.features` must list this feature id. A tour page is also a website page, so `tour` plus `website: tours/<id>` counts.
3. Remove `"status": "grandfathered"` once `help` and `website` resolve. The checker fails if you leave it.
4. Run `make docs-coverage`.

A feature is **covered** when `website` resolves together with `help` or `tour`. Many features may point at the same page (many-to-one is fine); do not write a page per feature.

## Exempt a feature

Use `exempt` only when no doc makes sense (an internal affordance, say):

```json
{ "id": "foo-debug-toggle", "exempt": "Developer-only toggle, not part of the product surface", "approver": "tstapler" }
```

`exempt` excludes `help`, `website` and `tour`, needs a non-empty reason and an `approver` (a GitHub login). `docs/registry/docs-coverage/` has a CODEOWNERS entry, so the owner reviews every exemption. The report prints the exempt count; `tools/docscoverage/budgets.json` caps the exempt share of in-scope features and a PR may add at most 5 exemptions.

## Grandfathered features and the ratchet

`make docs-coverage-baseline` writes `status: "grandfathered"` sidecars for existing features with no docs; running it twice changes nothing. The baseline can only shrink:

- `grandfathered` is valid only for ids that exist on the base branch. A new feature can never be grandfathered.
- A grandfathered feature whose `name`, `description` or `userFacing` changes passes and is listed as "changed, still grandfathered".
- A PR that raises the grandfathered count fails.

The gate fires on new ids and on changes to `name`, `description` or `userFacing`. Scanner-owned fields (`tested`, `testIds`, `lastModified`, `markerLine`, `handlerFile`) and moves (`component`, `path`, `filePath`) never count. Without a resolvable base (workflow_dispatch, merge queue, a zero `before`) the checker prints a notice and reports only.

## Point at a sync-generated website page

Some `website/src/content/docs/help/*.md` pages are generated from `docs/how-to/*.md` through `website/scripts/content-manifest.json`. Do not edit the generated page.

1. Edit the source, for example `docs/how-to/add-user-themes.md`.
2. Run `node website/scripts/sync-content.mjs`.
3. Point the sidecar at the generated id: `"website": "help/add-user-themes"`.

A missing anchor on such a page is reported against the source file to edit.

## Rollout

CI runs the checker report-only (`build.yml`, step "Docs coverage (report-only)"). It starts blocking for new and changed features when `docsCoverageEnforce` is `true` in `.github/promo-flags.json`; flipping it back to `false` is the rollback.
