package session

// ─── fakePipelineModeRepository ─────────────────────────────────────────────

var _ PipelineModeRepository = (*fakePipelineModeRepository)(nil)

// warningLogMu and swapWarningLog live in session/sync_buffer_test.go, shared
// with session/review_gate_test.go and session/backlog_lifecycle_test.go.

// ─── Story 1.3.1: interface shape ───────────────────────────────────────────

// ─── Story 1.3.2: pipelineModeCache concurrency ─────────────────────────────

// ─── Story 1.3.3: CachingPipelineEngine fail-closed resolution ─────────────

// ─── Story 1.3.3d/1.3.3e: renderTemplate + resolved-mode rendering ─────────

// ─── Story 1.7.3: isolated commit gate — real SQL-level injection ─────────

// ─── Story 2.1.1: ExecutorFor ───────────────────────────────────────────────

// ─── Story 2.1.2: ComputeExecutorHash ───────────────────────────────────────
