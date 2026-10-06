# sdd:6-verify — llm-backend-selector

Reviewers: Go idioms, React idioms, architecture (parallel, read-only), then a Layer 3 test/lint gate.

| Finding | Severity | Resolution |
|---|---|---|
| Custom gate, PR-description draft, session-pipeline autonomous driver used the raw claude pool | BLOCKER | Routed via selector client (`BacklogLifecycleListener.SetHeadlessClient`, `SessionService.autonomousDriverClient`) |
| Base URLs unvalidated | SUGGEST | `validateBaseURL` in `BackendSettings.Validate` |
| Consolette probe held the call mutex | SUGGEST | Probe runs unlocked |
| Settings UI: reload flash, 44px touch targets, load-error style | SUGGEST | Fixed |
| Capacity-probe test was a source-text check | review gap | Now records the URL the probe really requests |
| Fallback JSONL no rotation/dedupe; per-URL pool eviction; Playwright e2e for panel; `Resolve` double-logs fallback | SUGGEST/CONCERN | Follow-ups, not blocking |
| `HeadlessService` (streaming `CallWithOptions`) stays on the raw pool | by design | Not a CallBlocking feature site |

Remaining direct `CallBlocking` callers audited: approval handler (`LLMClient`), backlog intent/triage/review
(`BacklogService.headlessPool` is a `SelectingClient`), RunOneShot, unfinished_work, rules generation, gates, drafts, drivers.

Gate: `go test` config, session/headless, session, server/services all ok; `golangci-lint --new-from-rev=origin/main` 0 issues.
`make ci` cannot run here (tmux submodule not initialised; /tmp full).
