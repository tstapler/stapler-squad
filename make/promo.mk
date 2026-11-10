# Promo-materials-refresh targets, included from the top-level Makefile.

.PHONY: docs-coverage docs-coverage-baseline

docs-coverage: ## Report registry docs coverage (report-only unless docsCoverageEnforce is true)
	go run ./tools/docscoverage/cmd/docscoverage

docs-coverage-baseline: ## Write grandfathered docs-coverage sidecars for features lacking docs (idempotent)
	go run ./tools/docscoverage/cmd/docscoverage baseline
