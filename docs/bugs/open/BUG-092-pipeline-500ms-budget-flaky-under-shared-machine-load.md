# BUG-092: `TestBackgroundResolutionPipeline_should_CompleteWithoutNetworkIO_When_PlainDirectorySession` flaky under shared-machine load

## Symptom

`server/services/session_creation_pipeline_test.go:191` asserts a plain-directory
session's Background Resolution Pipeline completes in under 500ms (no network I/O
should be involved). On this dev machine, the test fails consistently (3/3 runs in
isolation, plus repeatedly inside the broader `server/services` suite), with observed
durations of 0.56s–1.58s — over the 500ms budget but still fast in absolute terms.

## Root cause

Confirmed via `uptime`: this machine's load average is 96.7 on an 18-core box (5.4x
oversubscribed) — dozens of concurrent Claude Code sessions (interactive + background
agents) are sharing this checkout and machine. The pipeline itself does no network I/O
(confirmed correct) — the 500ms budget is simply too tight to survive CPU scheduling
contention at this load level. Also observed: transient `failed to save default
config... no such file or directory` warnings in the same test run, consistent with
filesystem/tmpdir contention under the same load.

This is NOT a regression introduced by the `async-session-creation` review/repair work
(commits `365699d5e`, `cb5f7e645`, `1193107`, `c8b187a`) — none of those touched
`session_creation_pipeline.go`'s runtime logic, only tests, doc comments, and frontend
files. Confirmed the pipeline still completes correctly and quickly in absolute terms;
only the hardcoded assertion threshold is sensitive to host contention.

## Recommended fix

Either:
1. Loosen the budget (e.g. 2-3s) to tolerate realistic CI/shared-dev-machine
   contention while still catching a genuine network-I/O regression (which would push
   duration into the 10s+ range from a real DNS/TLS attempt), or
2. Make the assertion relative (e.g. compare against a sibling GitHub-URL test's
   duration) rather than an absolute wall-clock number.

## Status

Fixed — loosened the budget from 500ms to 3s in
`server/services/session_creation_pipeline_test.go` (option 1 above), per
`fix-flaky-tests-dont-defer.md`. The `resolverCalled.Load()` assertion (the real
no-network-I/O guarantee) is unchanged; the timing check remains as a secondary
guard wide enough to survive shared-machine contention but still catch a genuine
network-I/O regression (10s+).
