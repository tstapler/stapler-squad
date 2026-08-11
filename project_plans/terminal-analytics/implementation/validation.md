# Terminal Escape Code Analytics — Validation Plan

**Version**: 1.0  
**Date**: 2026-05-14  
**Status**: Ready for implementation  
**References**: requirements.md (AC-1 through AC-8), plan.md (Epics 1–7)

---

## Summary

| Test Type | Count |
|-----------|-------|
| Unit tests (T-UNIT-NNN) | 28 |
| Integration tests (T-INT-NNN) | 8 |
| Performance benchmarks (T-BENCH-NNN) | 4 |
| ConnectRPC API tests (T-API-NNN) | 8 |
| Frontend Jest tests (T-FE-NNN) | 7 |
| **Total** | **55** |

**Requirements coverage**: 8/8 acceptance criteria covered (AC-1 through AC-8).

---

## 1. Unit Tests

### 1.1 EscapeParser (`pkg/analytics/`)

| Test ID | Test Name | AC | File | Description |
|---------|-----------|-----|------|-------------|
| T-UNIT-001 | `TestEscapeParser_ParsesCSISequences` | AC-1 | `pkg/analytics/escape_code_parser_test.go` | Table-driven test with corpus of valid CSI sequences (SGR colors, cursor movement, erase, alternate screen). Verifies `SequenceType="CSI"` and correct `SequenceSubtype` for each. |
| T-UNIT-002 | `TestEscapeParser_ParsesOSCSequences` | AC-1 | `pkg/analytics/escape_code_parser_test.go` | OSC sequences terminated by both BEL and ST (`ESC \`). Verifies OSC 0 (title), OSC 7 (CWD), OSC 52 (clipboard) are each parsed and typed correctly. |
| T-UNIT-003 | `TestEscapeParser_ParsesDCSSequences` | AC-1 | `pkg/analytics/escape_code_parser_test.go` | DCS sequences including tmux passthrough (`ESC P tmux ;`). Verifies `SequenceSubtype="tmux-passthrough"` is set for tmux DCS. |
| T-UNIT-004 | `TestEscapeParser_ParsesSS2SS3AndBareESC` | AC-1 | `pkg/analytics/escape_code_parser_test.go` | SS2, SS3 and bare ESC + single-char sequences are parsed without error. |
| T-UNIT-005 | `TestEscapeParser_SplitSequenceAcrossReads` | AC-8 | `pkg/analytics/escape_code_parser_test.go` | Simulate a CSI sequence split across two `Parse()` calls (first call delivers `ESC [`, second delivers `31m`). Verifies exactly one `EscapeEventRecord` is emitted (no duplicate, no drop). |
| T-UNIT-006 | `TestEscapeParser_SplitOSCSequenceAcrossReads` | AC-8 | `pkg/analytics/escape_code_parser_test.go` | OSC sequence split at the payload boundary. Second call delivers the ST terminator. Verifies one event emitted with correct `ByteLen`. |
| T-UNIT-007 | `TestEscapeParser_MalformedCSI_Recovered` | AC-1 | `pkg/analytics/escape_code_parser_test.go` | Inject malformed CSI (no final byte before next ESC). Verifies parser resets gracefully: subsequent valid sequence is still parsed, and malformed input does not panic. |
| T-UNIT-008 | `TestEscapeParser_PartialBufferCap` | AC-8 | `pkg/analytics/escape_code_parser_test.go` | Feed a partial sequence that grows `partialBuffer` beyond 4096 bytes. Verifies the cap triggers a reset, subsequent valid sequences are still parsed, and no panic occurs. |
| T-UNIT-009 | `TestEscapeParser_OSCPayloadLengthCap` | AC-1 | `pkg/analytics/escape_code_parser_test.go` | Feed an unterminated OSC with 70 KB of payload data across multiple calls. Verifies sequence is recorded with `mangle_type="truncated"` after the 65536-byte cap is hit, and no memory blowup. |
| T-UNIT-010 | `TestEscapeParser_OSCRedaction_Enabled` | AC-1 | `pkg/analytics/escape_code_parser_test.go` | With `redactOSCPayloads=true`: OSC 52, OSC 0, OSC 7 payloads produce records with `RawBytes=nil` and `PayloadHash=""`. Sequence type and byte length are preserved. |
| T-UNIT-011 | `TestEscapeParser_OSCRedaction_Disabled` | AC-1 | `pkg/analytics/escape_code_parser_test.go` | With `redactOSCPayloads=false`: OSC 52 payload is included in `PayloadHash` when `capture_level=full`. |

### 1.2 MangleCorrelator (`pkg/analytics/`)

| Test ID | Test Name | AC | File | Description |
|---------|-----------|-----|------|-------------|
| T-UNIT-012 | `TestMangleCorrelator_HashMatch_NotMangled` | AC-3 | `pkg/analytics/mangle_correlator_test.go` | RecordStage1 then CheckStage2 with identical hash and byte length. Verifies `(false, "")` returned and entry evicted. |
| T-UNIT-013 | `TestMangleCorrelator_ByteLengthDiffers_Truncated` | AC-3 | `pkg/analytics/mangle_correlator_test.go` | Stage 2 byte length shorter than Stage 1. Verifies `(true, "truncated")`. |
| T-UNIT-014 | `TestMangleCorrelator_SameLength_DifferentHash_Mutated` | AC-3 | `pkg/analytics/mangle_correlator_test.go` | Same byte length, different hash. Verifies `(true, "mutated")`. |
| T-UNIT-015 | `TestMangleCorrelator_TTLExpiry_EmitsStripped` | AC-3 | `pkg/analytics/mangle_correlator_test.go` | RecordStage1 then advance mock clock past TTL. Call `EvictExpired`. Verifies a `mangled=true, mangle_type="stripped"` event is written to the spy `EscapeEventWriter`. |
| T-UNIT-016 | `TestMangleCorrelator_BoundedSize_DropsOldest` | AC-3 | `pkg/analytics/mangle_correlator_test.go` | Insert entries up to `maxSize+1`. Verifies oldest entry is evicted (dropped, not emitted as stripped) before the new one is inserted, and map size stays within `maxSize`. |
| T-UNIT-017 | `TestMangleCorrelator_NoMatchAtStage2_NotMangled` | AC-3 | `pkg/analytics/mangle_correlator_test.go` | CheckStage2 for a key that was never recorded at Stage 1. Verifies `(false, "")` — absence alone is not a mangle until TTL expires. |

### 1.3 CircularBuffer `totalBytesWritten` (`session/`)

| Test ID | Test Name | AC | File | Description |
|---------|-----------|-----|------|-------------|
| T-UNIT-018 | `TestCircularBuffer_TotalBytesWritten_MonotonicIncrease` | AC-2 | `session/circular_buffer_test.go` | Write data in multiple calls. Verify `TotalBytesWritten()` increases by exactly `len(data)` on each write. |
| T-UNIT-019 | `TestCircularBuffer_TotalBytesWritten_AfterWrap` | AC-2 | `session/circular_buffer_test.go` | Write data that causes the circular buffer to wrap multiple times. Verify `TotalBytesWritten()` is still strictly monotonically increasing and equals the sum of all written bytes, never the current occupancy. |
| T-UNIT-020 | `TestCircularBuffer_TotalBytesWritten_ZeroInitial` | AC-2 | `session/circular_buffer_test.go` | Fresh buffer returns 0 before any writes. |

### 1.4 SQLite Batch Writer (`server/analytics/`)

| Test ID | Test Name | AC | File | Description |
|---------|-----------|-----|------|-------------|
| T-UNIT-021 | `TestEscapeEventBatchWriter_FlushAt100Rows` | AC-2 | `server/analytics/escape_event_batch_writer_test.go` | Send exactly 100 records without ticker firing. Verify all 100 are flushed to the in-memory test DB as a batch. |
| T-UNIT-022 | `TestEscapeEventBatchWriter_FlushOnTicker` | AC-2 | `server/analytics/escape_event_batch_writer_test.go` | Send 10 records and advance mock clock past 500ms ticker. Verify all 10 are flushed before the 100-row threshold. |
| T-UNIT-023 | `TestEscapeEventBatchWriter_BackpressureDrop` | AC-2 | `server/analytics/escape_event_batch_writer_test.go` | Fill the internal channel to capacity (1000). Call `Write()` one more time. Verify it returns immediately (no block) and the dropped counter increments by 1. |
| T-UNIT-024 | `TestEscapeEventBatchWriter_PerSessionRowCap` | AC-2 | `server/analytics/escape_event_batch_writer_test.go` | Send `maxRowsPerSession+50` records for the same session. Verify exactly `maxRowsPerSession` rows are written to DB and the excess 50 are silently dropped. |
| T-UNIT-025 | `TestEscapeEventBatchWriter_DrainOnShutdown` | AC-2 | `server/analytics/escape_event_batch_writer_test.go` | Send 40 records, then cancel the context before the ticker fires. Verify all 40 records are flushed to DB during the drain-on-shutdown path. |

### 1.5 Config Parsing (`config/`)

| Test ID | Test Name | AC | File | Description |
|---------|-----------|-----|------|-------------|
| T-UNIT-026 | `TestConfig_EscapeAnalyticsDefaults` | AC-6 | `config/config_test.go` | Load config from empty JSON `{}`. Verify: `CaptureLevel="summary"`, `SamplingRate=1.0`, `MaxRowsPerSession=10000`, `RedactOSCPayloads=true`, `RetentionDays=7`. |
| T-UNIT-027 | `TestConfig_EscapeAnalyticsCaptureLevel_Validation` | AC-6 | `config/config_test.go` | Test all three valid values (`"full"`, `"summary"`, `"off"`) load without error. Test an invalid value (e.g., `"verbose"`) returns a validation error. |
| T-UNIT-028 | `TestConfig_EscapeAnalyticsSamplingRate_Validation` | AC-7 | `config/config_test.go` | Valid boundary values `0.0` and `1.0` are accepted. Values `-0.1` and `1.1` return a validation error. |

---

## 2. Integration Tests

All integration tests use a real SQLite in-memory or temp-file ent client, and a running server with a test PTY session where noted.

| Test ID | Test Name | AC | File | Description |
|---------|-----------|-----|------|-------------|
| T-INT-001 | `TestStage1Capture_EscapeEventsInSQLite` | AC-2 | `server/services/analytics_integration_test.go` | Start a test session with `capture_level=summary`. Write a sequence of known ANSI escape codes into the PTY (CSI SGR, OSC title, cursor movement). After the write, wait for the batch flush. Query the `escape_event` table and verify: correct `session_id`, correct `sequence_type` values, non-zero `byte_length`, `stage="pty_read"`. Verifies AC-2. |
| T-INT-002 | `TestMangleDetection_StrippedOSCFlagged` | AC-3 | `server/services/analytics_integration_test.go` | Inject a Stage 1 observation for an OSC sequence into the `MangleCorrelator`. Then invoke the Stage 2 observer with the same session_id and session_seq but with the OSC payload removed (byte length = 0, hash differs). Verify the resulting `escape_event` row has `mangled=true` and `mangle_type="truncated"`. Verifies AC-3. |
| T-INT-003 | `TestCaptureLevelOff_ZeroRowsWritten` | AC-6 | `server/services/analytics_integration_test.go` | Start a server with `capture_level=off`. Write escape sequences to a test PTY session. After the session, count `escape_event` rows in SQLite. Assert count is 0. Verifies AC-6. |
| T-INT-004 | `TestSamplingRate_ZeroWritesNoRows` | AC-7 | `server/services/analytics_integration_test.go` | Set `sampling_rate=0.0`. Write escape sequences to PTY. Verify zero `escape_event` rows appear. |
| T-INT-005 | `TestSamplingRate_FullWritesRows` | AC-7 | `server/services/analytics_integration_test.go` | Set `sampling_rate=1.0`. Write N known escape sequences. Verify the row count is equal to N (no sampling drops at rate=1.0). |
| T-INT-006 | `TestSamplingDecision_DeterministicAcrossStages` | AC-7 | `pkg/analytics/escape_code_parser_test.go` | For the same sessionID and chunkSeqNumber, verify Stage 1 and Stage 2 parsers make the same sampling decision (both include or both exclude) when called with identical inputs. Uses the deterministic hash function directly. |
| T-INT-007 | `TestBatchWriter_FlushBeforeSessionClose` | AC-2 | `server/analytics/escape_event_batch_writer_test.go` | Simulate session close by cancelling context while records are queued. Verify all queued records are written to the SQLite test DB before the goroutine exits. Tests the shutdown drain path end-to-end with real ent client. |
| T-INT-008 | `TestMangleCorrelator_EvictionEmitsEvent_Integration` | AC-3 | `pkg/analytics/mangle_correlator_test.go` | Full integration: RecordStage1 for a sequence. Wait for TTL to expire (using `time.Sleep` + short TTL for test). Call `EvictExpired`. Verify the `EscapeEventBatchWriter` spy received a `mangled=true, mangle_type="stripped"` write call. |

---

## 3. Performance Benchmarks

All benchmarks must be run with `&` per CLAUDE.md conventions. Run from the Go module root.

| Test ID | Test Name | AC | File | Pass Threshold | Description |
|---------|-----------|-----|------|----------------|-------------|
| T-BENCH-001 | `BenchmarkEscapeParser4KB` | AC-7 | `pkg/analytics/escape_code_parser_test.go` | < 50µs per operation | Feed a 4 KB buffer containing a realistic mix of CSI, OSC, and plain text sequences to `Parse()` with `NoopEscapeEventWriter`. Measure wall time per `Parse` call. Must stay under 50µs to satisfy NFR-1 / AC-7. |
| T-BENCH-002 | `BenchmarkEscapeParser4KB_WithWriter` | AC-7 | `pkg/analytics/escape_code_parser_test.go` | < 60µs per operation | Same as T-BENCH-001 but with a real non-blocking channel writer. Validates that the channel send overhead does not push total time past a reasonable bound. |
| T-BENCH-003 | `BenchmarkBatchWriterThroughput` | AC-2 | `server/analytics/escape_event_batch_writer_test.go` | > 10,000 rows/sec | Sustained load test: spin up N goroutines each calling `Write()` with pre-built records for 5 seconds. Measure total committed rows / elapsed time. Baseline for regression tracking. |
| T-BENCH-004 | `BenchmarkMangleCorrelatorRecordAndCheck` | AC-3 | `pkg/analytics/mangle_correlator_test.go` | < 1µs per round trip | Benchmark a single `RecordStage1` + `CheckStage2` round trip under lock. Validates that correlator adds negligible latency to Stage 2 path. |

### Running the benchmarks

```bash
# All escape analytics benchmarks (run in background per CLAUDE.md)
go test ./pkg/analytics/... -bench=BenchmarkEscapeParser -benchtime=5s -run='^$' &
go test ./server/analytics/... -bench=BenchmarkBatchWriter -benchtime=10s -run='^$' &
go test ./pkg/analytics/... -bench=BenchmarkMangleCorrelator -benchtime=5s -run='^$' &
```

---

## 4. ConnectRPC API Tests

All API tests use a `connecttest` in-process server backed by an in-memory SQLite ent client pre-seeded with test data.

| Test ID | Test Name | AC | File | Description |
|---------|-----------|-----|------|-------------|
| T-API-001 | `TestQueryEscapeAnalytics_FilterBySessionID` | AC-4 | `server/services/analytics_service_test.go` | Seed DB with events for session A and session B. Query with `session_id=A`. Verify only session A events are returned and count matches. Verifies AC-4. |
| T-API-002 | `TestQueryEscapeAnalytics_FilterByMangledTrue` | AC-4 | `server/services/analytics_service_test.go` | Seed DB with mixed mangled/clean events for one session. Query with `mangled_only=true`. Verify all returned events have `mangled=true`. |
| T-API-003 | `TestQueryEscapeAnalytics_FilterByTimeRange` | AC-4 | `server/services/analytics_service_test.go` | Seed events at times T-10, T, T+10 minutes. Query with start=T-5, end=T+5. Verify only the event at T is returned. |
| T-API-004 | `TestQueryEscapeAnalytics_PaginationCursorAdvances` | AC-4 | `server/services/analytics_service_test.go` | Seed 250 events. Query with `page_size=100`. Verify first response has 100 events and a non-empty `next_page_token`. Second query with that token returns the next 100. Third returns remaining 50 with empty `next_page_token`. |
| T-API-005 | `TestQueryEscapeAnalytics_EmptyResultForUnknownSession` | AC-4 | `server/services/analytics_service_test.go` | Query with `session_id="does-not-exist"`. Verify empty `events` list and `total_count=0` with no error. |
| T-API-006 | `TestGetEscapeAnalyticsSummary_HistogramCorrectness` | AC-4, AC-5 | `server/services/analytics_service_test.go` | Seed: 10 CSI events (2 mangled), 5 OSC events (0 mangled) for one session. Call `GetEscapeAnalyticsSummary`. Verify histogram has exactly 2 entries; CSI entry has `count=10, mangled_count=2`; OSC entry has `count=5, mangled_count=0`; `mangle_rate=2/15≈0.133`. |
| T-API-007 | `TestGetEscapeAnalyticsSummary_ZeroForNoEvents` | AC-4 | `server/services/analytics_service_test.go` | Call summary for session with zero events. Verify `total_sequences=0`, `total_mangled=0`, `mangle_rate=0.0`, empty histogram. |
| T-API-008 | `TestQueryEscapeAnalytics_FilterByStageAndSequenceType` | AC-4 | `server/services/analytics_service_test.go` | Seed events across stages `pty_read` and `transport`. Query filtered to `stage="transport"` and `sequence_type="CSI"`. Verify only transport-stage CSI events returned. |

---

## 5. Frontend Tests (Jest)

All frontend tests use `@testing-library/react` with mocked ConnectRPC client via Jest module mocks.

| Test ID | Test Name | AC | File | Description |
|---------|-----------|-----|------|-------------|
| T-FE-001 | `EscapeAnalyticsPage_renders_histogram_with_mock_data` | AC-5 | `web-app/src/components/analytics/EscapeAnalyticsPage.test.tsx` | Mock `useEscapeAnalyticsSummary` to return histogram with 3 sequence types. Render `EscapeAnalyticsPage`. Assert 3 histogram bars render with the correct labels. Verifies AC-5. |
| T-FE-002 | `SequenceHistogram_shows_mangled_counts_in_contrasting_color` | AC-5 | `web-app/src/components/analytics/EscapeAnalyticsPage.test.tsx` | Render `SequenceHistogram` with a mock histogram entry that has `mangled_count > 0`. Assert a DOM element with the mangled-indicator class is present. |
| T-FE-003 | `MangleRateIndicator_renders_correct_color_tier` | AC-5 | `web-app/src/components/analytics/EscapeAnalyticsPage.test.tsx` | Render `MangleRateIndicator` three times with rates 0.005, 0.03, 0.08. Assert classes/text corresponding to green/yellow/red are applied per tier boundaries (< 1%, 1–5%, > 5%). |
| T-FE-004 | `EscapeEventTable_renders_event_rows` | AC-5 | `web-app/src/components/analytics/EscapeAnalyticsPage.test.tsx` | Mock `useEscapeEvents` to return 5 events. Render `EscapeEventTable`. Assert 5 rows are present in the table. |
| T-FE-005 | `EscapeEventTable_mangledOnly_filter_updates_query_params` | AC-5 | `web-app/src/components/analytics/EscapeAnalyticsPage.test.tsx` | Render `EscapeEventTable`. Click the "mangled only" checkbox. Assert `useEscapeEvents` is re-called with `mangled_only=true` in the updated params (via spy on the mock hook). |
| T-FE-006 | `EscapeEventTable_rawBytes_rendered_as_hex_not_terminal` | AC-5 | `web-app/src/components/analytics/EscapeAnalyticsPage.test.tsx` | Mock event with `raw_bytes = Uint8Array([0x1b, 0x5b, 0x33, 0x31, 0x6d])`. Render table. Assert the cell text contains the hex string `"1b 5b 31 6d"` or similar and does NOT contain the raw escape character `\x1b` rendered as a literal character (security check). |
| T-FE-007 | `EscapeEventTable_loadMore_fetchesNextPage` | AC-5 | `web-app/src/components/analytics/EscapeAnalyticsPage.test.tsx` | Mock `useEscapeEvents` with `nextPageToken="tok123"`. Render table. Click "Load more" button. Assert `fetchNextPage` was called exactly once. |

---

## 6. Requirement-to-Test Traceability Matrix

| AC | Criterion Summary | Covered By |
|----|-------------------|------------|
| AC-1 | EscapeParser identifies all CSI, OSC, DCS sequences from known corpus | T-UNIT-001 through T-UNIT-011 |
| AC-2 | `escape_event` rows appear in SQLite after vim session with `capture_level=summary` | T-UNIT-018 through T-UNIT-025, T-INT-001, T-INT-007 |
| AC-3 | Mangle detection flags stripped OSC sequence in integration test | T-UNIT-012 through T-UNIT-017, T-INT-002, T-INT-008, T-BENCH-004 |
| AC-4 | `QueryEscapeAnalytics` returns correct paginated results filtered by session_id | T-API-001 through T-API-008 |
| AC-5 | Web UI escape analytics page renders sequence histogram for a test session | T-FE-001 through T-FE-007, T-API-006 |
| AC-6 | `capture_level=off` → zero `escape_event` rows written | T-UNIT-026, T-UNIT-027, T-INT-003 |
| AC-7 | Stage 1 overhead < 50µs on 4KB chunks | T-BENCH-001, T-BENCH-002, T-UNIT-028, T-INT-004, T-INT-005, T-INT-006 |
| AC-8 | Parser handles sequences split across two read() calls without drop or duplicate | T-UNIT-005, T-UNIT-006, T-UNIT-007, T-UNIT-008 |

**Coverage**: 8/8 acceptance criteria covered.

---

## 7. Test File Locations Summary

### New test files to create

| File | Tests |
|------|-------|
| `pkg/analytics/escape_code_parser_test.go` | T-UNIT-001–011, T-UNIT-027(partial), T-INT-006, T-BENCH-001, T-BENCH-002 |
| `pkg/analytics/mangle_correlator_test.go` | T-UNIT-012–017, T-INT-008, T-BENCH-004 |
| `session/circular_buffer_test.go` | T-UNIT-018–020 (extend if already exists) |
| `server/analytics/escape_event_batch_writer_test.go` | T-UNIT-021–025, T-INT-007, T-BENCH-003 |
| `config/config_test.go` | T-UNIT-026–028 (extend if already exists) |
| `server/services/analytics_integration_test.go` | T-INT-001–005 |
| `server/services/analytics_service_test.go` | T-API-001–008 |
| `web-app/src/components/analytics/EscapeAnalyticsPage.test.tsx` | T-FE-001–007 |

### Existing test files to extend

| File | Tests to Add |
|------|--------------|
| `session/circular_buffer_test.go` | T-UNIT-018–020 (if file exists) |
| `config/config_test.go` | T-UNIT-026–028 (if file exists) |
| `pkg/analytics/escape_code_parser_test.go` | T-UNIT-010–011 (OSC redaction, extend existing) |

---

## 8. CI Integration

All tests are covered by `make ci`. Specific make targets:

```bash
make build && make test      # Runs all Go tests including T-UNIT and T-INT
make test-coverage           # Coverage report — escape analytics packages should target > 80%
cd web-app && npx jest --no-coverage  # Runs T-FE-001 through T-FE-007

# Benchmarks (run separately, not in make ci)
go test ./pkg/analytics/... -bench=BenchmarkEscapeParser -benchtime=5s -run='^$' &
go test ./server/analytics/... -bench=BenchmarkBatchWriter -benchtime=10s -run='^$' &
```

Benchmark baselines should be committed to `.claude/docs/benchmarks.md` after the first passing run per existing project conventions.

---

## 9. Test Implementation Notes

### EscapeParser test corpus (T-UNIT-001)

The test corpus should include at minimum:

```go
var csiCorpus = []struct {
    input    []byte
    wantType string
    wantSub  string
}{
    {[]byte("\x1b[31m"), "CSI", "SGR"},             // foreground red
    {[]byte("\x1b[0m"), "CSI", "SGR"},              // reset
    {[]byte("\x1b[2J"), "CSI", "erase-display"},    // clear screen
    {[]byte("\x1b[H"), "CSI", "cursor-position"},   // cursor home
    {[]byte("\x1b[?1049h"), "CSI", "alternate-screen-on"},
    {[]byte("\x1b[?1049l"), "CSI", "alternate-screen-off"},
    {[]byte("\x1b[A"), "CSI", "cursor-up"},
    {[]byte("\x1b[1;32m"), "CSI", "SGR"},           // bold green, multiple params
}
```

### Split-sequence test pattern (T-UNIT-005, T-UNIT-006)

```go
func TestEscapeParser_SplitSequenceAcrossReads(t *testing.T) {
    spy := &spyEscapeEventWriter{}
    parser := newTestParser(spy)
    // First half: ESC + [
    parser.Parse([]byte("\x1b["), 0)
    assert.Equal(t, 0, len(spy.events), "no event before sequence complete")
    // Second half: 31m (SGR red)
    parser.Parse([]byte("31m"), 2)
    assert.Equal(t, 1, len(spy.events), "exactly one event after sequence completes")
    assert.Equal(t, "CSI", spy.events[0].SequenceType)
}
```

### Backpressure drop test pattern (T-UNIT-023)

```go
func TestEscapeEventBatchWriter_BackpressureDrop(t *testing.T) {
    w := newTestBatchWriter(WithChannelCap(1000))
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    w.Start(ctx)
    // Pause the consumer goroutine by blocking the flush path
    // Fill channel to capacity
    for i := 0; i < 1000; i++ {
        w.Write(makeTestRecord("sess-a"))
    }
    // This call must not block
    done := make(chan struct{})
    go func() {
        w.Write(makeTestRecord("sess-a")) // should drop
        close(done)
    }()
    select {
    case <-done:
        // pass — returned immediately
    case <-time.After(50 * time.Millisecond):
        t.Fatal("Write() blocked when channel full")
    }
    assert.Equal(t, int64(1), w.DroppedCount())
}
```
