# Promo-materials-refresh targets, included from the top-level Makefile.

.PHONY: docs-coverage docs-coverage-baseline termcheck promo-guards

# Base commit the PR guards diff against; CI uses HEAD^1 (the pull_request merge ref's first parent).
PROMO_BASE ?= origin/main

docs-coverage: ## Report registry docs coverage (report-only unless docsCoverageEnforce is true)
	go run ./tools/docscoverage/cmd/docscoverage

docs-coverage-baseline: ## Write grandfathered docs-coverage sidecars for features lacking docs (idempotent)
	go run ./tools/docscoverage/cmd/docscoverage baseline

termcheck: ## Scan tracked source for banned trademark terms (allow-list read from PROMO_BASE, default origin/main)
	go run ./tools/promo/cmd/promoguard termcheck -mode=source -base "$(PROMO_BASE)"

promo-guards: termcheck ## Run the PR guards locally: termcheck plus mediaguard against PROMO_BASE (default origin/main)
	go run ./tools/promo/cmd/promoguard mediaguard -base "$(PROMO_BASE)"
