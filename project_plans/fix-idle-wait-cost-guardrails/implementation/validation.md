# Validation
| Requirement | Test |
|---|---|
| Cache-aware cost | monitor test: cache_heavy fixture ⇒ EstimatedCostUSD ≈ PricingTable.EstimateCost; CostBudgetUSD trips |
| Per-role caps | triage role budget lower than work role; work unaffected |
| Terminal action | after compact + second ceiling ⇒ session stopped, event published, tracker not re-armed |
| Wall-clock bound | idle run spanning > M minutes with < N turns triggers |
| Incident replay | fixture turn shape ⇒ detection |
| Ceiling actionable | live critical finding ⇒ one deduped notification |
Pre-mortem: false-positive kill of healthy long triage; mitigated by notify-first grace, config defaults off-by-role until tuned. Run `make ready` before push.
