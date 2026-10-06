# UX Research: Hook Classification Fast Path

## Surface assessment

This is primarily infrastructure, not a new interactive UI. Its user-facing surfaces are nevertheless consequential: invisible tool latency, terminal hook errors, installer output, manual-instance diagnostics, and operational dashboards. The desired normal UX is absence: tool calls proceed without noticeable hook delay and without success chatter.

## Mental model and comparable patterns

Users expect a local permission guard to behave like an IDE language service or local daemon:

- resident and fast when healthy;
- clearly bound to the current project/instance;
- safely degraded rather than hanging;
- diagnosable with one health command;
- no surprise interaction with production while testing manually.

The existing named-instance convention is the strongest product model. Socket and cache behavior should inherit `STAPLER_SQUAD_INSTANCE`/test-directory identity rather than introduce a separate user-managed naming system.

## Operational flow

### Healthy

Claude invokes one canonical hook; the primary answers silently. No terminal output beyond the required hook JSON. Dashboard shows latency and primary-path rate.

### Cached fallback

The hook returns the cached decision without noisy stderr on every event. Increment metrics and rate-limit a concise diagnostic in debug mode. A health command should say that primary is unavailable, identify the intended instance class and cache age/version, and suggest how to restore it.

### Defer fallback

Return empty stdout exactly as Claude expects. If diagnostics are enabled, explain that no valid primary/cache decision was available and Claude's permission mode now controls behavior. Never print malformed JSON that Claude interprets as hook failure.

### Protocol/identity mismatch

Treat as a correctness failure, not a generic connection error. Do not connect elsewhere. Health output should show expected versus received fingerprints in shortened non-sensitive form and give restart/upgrade guidance.

### Install/upgrade

Installer should report:

- canonical hook installed;
- number of duplicate obsolete entries removed;
- unrelated hooks preserved;
- detected server/CLI protocol compatibility;
- rollback/repair command.

No raw settings, tokens, or tool inputs should be printed.

## Manual test instance UX

A manual instance launched with `STAPLER_SQUAD_INSTANCE=claude-manual-test` should automatically publish and inject its own endpoint. Diagnostics should visibly label it as isolated and refuse production fallback. Documentation should offer a copy-paste smoke test that starts a unique instance, runs a classification probe, verifies the fingerprint/state directory, and tears it down.

Multiple simultaneous manual instances should have distinguishable but bounded-cardinality logs/metrics. The dashboard can filter by instance class and a controlled instance identifier; production alerts should exclude deliberate test-instance failures.

## Accessibility

No new interactive web surface is required. If dashboard/status UI is added:

- do not encode healthy/degraded/error solely by color;
- expose text labels and accessible table headings;
- preserve keyboard navigation and focus behavior;
- use concise status copy with remediation;
- respect reduced motion (no new animation is needed).

Terminal output must be plain text, readable without color, and actionable.

## Jobs to be done

- **Functional:** classify safely without delaying every tool; preserve automatic workflows; isolate manual instances; diagnose failures quickly.
- **Emotional:** restore trust that enabling Stapler Squad will not make Claude feel frozen or unpredictably prompt.
- **Social/operational:** let developers demonstrate and test changes without threatening the deployed instance or creating hidden cleanup work.

## UX acceptance criteria

1. Healthy hook adds no success noise and meets latency SLO.
2. Every degraded path terminates within a bounded time and has a deterministic exit: cached decision or defer.
3. Isolated-instance mismatch never silently reroutes.
4. `ssq-hooks doctor` (or equivalent) reports resolved endpoint, compatibility, cache freshness, and queue health without sensitive payloads.
5. Installer normalization clearly reports duplicates removed and preserves unrelated hooks.
6. Dashboard distinguishes primary/cache/defer and production/manual classes, with textual status and accessible labels.
