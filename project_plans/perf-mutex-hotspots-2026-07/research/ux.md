# UX Research: perf-mutex-hotspots-2026-07

N/A — pure infrastructure change. No user-facing surface.

The three fixes (singleflight for GoGitVCSReader, sync.RWMutex for CircularBuffer,
TTL cache for IsDirty) have no UI, no API changes, and no user-visible behavior changes.
The only user-observable effect is reduced terminal output jank and faster unfinished-work
tab refresh — both are improvements with no new interaction patterns.
