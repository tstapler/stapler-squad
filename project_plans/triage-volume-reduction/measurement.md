# Triage volume measurement (7 days to 2026-10-05)

Source: `item_sessions` (session_role='triage', created_at >= now-7d), read-only query against
`~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db` (this host's workspace DB only).

| metric | value |
|---|---|
| triage calls | 14 (14 distinct items, 0 items triaged twice) |
| estimated cost | $3.15 total, $0.23 avg |
| completed with result / ended with failure reason | 8 / 6 (`other`) |
| calls with resolved_model set | 0 of 14 — all ran on the account default model |
| pipeline mode | 12 sdd, 2 default |
| by day | 09-30: 2 ($0.75); 10-04: 6 ($0.00, unpriced/failed); 10-05: 6 ($2.41) |

Limits: tokens are not stored per call (only `estimated_cost_usd`), so cost is the proxy. Other
hosts/instances' DBs are not included.
Takeaway: volume here is call-count-driven (1 call/item, no re-triage), every call is on the account default
model, and 6/14 failed — so the model pin and concurrency cap are the highest-value levers; skip/dedupe
protects bulk-import and re-triage paths.
