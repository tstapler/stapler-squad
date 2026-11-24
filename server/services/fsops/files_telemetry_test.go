package fsops

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services/fsops/fsopstest"
)

var labelValueShape = regexp.MustCompile(`^[a-z0-9_+-]{1,24}$`)

func TestFilesTelemetryAdapter_ShouldRecordOneEventWithEnumLabels_When_WriteConflictRejected(t *testing.T) {
	p := &fsopstest.RecordingProvider{}
	tel := NewFilesTelemetry(p)

	tel.RecordWrite(context.Background(), "sess-1", WriteOutcomeEvent{
		Outcome: WriteOutcomeConflictRejected, Reason: WriteReasonNone,
		Size: BucketSize(2048), EOL: EOLCRLF, BOM: BOMYes, Duration: 12 * time.Millisecond,
	})

	require.Len(t, p.Events, 1)
	e := p.Events[0]
	assert.Equal(t, "files.server.write", e.EventName)
	assert.Equal(t, "rpc", e.EventCategory)
	assert.Equal(t, "sess-1", e.SessionID)
	require.NotNil(t, e.DurationMs)
	assert.EqualValues(t, 12, *e.DurationMs)
	assert.Equal(t, map[string]string{
		"outcome": "conflict_rejected", "reason": "none", "size": "1k-10k", "eol": "crlf", "bom": "yes",
	}, e.Labels)
	for k, v := range e.Labels {
		assert.Regexp(t, labelValueShape, v, k)
	}
	assert.LessOrEqual(t, len(e.Labels), 8)
}

func TestFilesTelemetryAdapter_ShouldRecordSearchOutcomeWithBucketedHits(t *testing.T) {
	p := &fsopstest.RecordingProvider{}
	NewFilesTelemetry(p).RecordSearch(context.Background(), "s", SearchOutcomeEvent{
		Outcome: SearchOutcomeTruncated, Results: BucketCount(250), Scale: BucketScale(60000),
		Queued: true, Backend: SearchBackendWalker, Duration: time.Second,
	})
	require.Len(t, p.Events, 1)
	assert.Equal(t, "files.server.search", p.Events[0].EventName)
	assert.Equal(t, map[string]string{
		"outcome": "truncated", "results": "100-999", "scale": "50k+", "queued": "true", "backend": "walker", "large": "false",
	}, p.Events[0].Labels)
	for k, v := range p.Events[0].Labels {
		assert.Regexp(t, labelValueShape, v, k)
	}
}

func TestFilesTelemetryAdapter_ShouldRecordFirstResultAsPerformanceEvent(t *testing.T) {
	p := &fsopstest.RecordingProvider{}
	NewFilesTelemetry(p).RecordSearchFirstResult(context.Background(), "s", SearchFirstResultEvent{Scale: BucketScale(5), Duration: 40 * time.Millisecond})
	require.Len(t, p.Events, 1)
	assert.Equal(t, "files.server.search_first_result", p.Events[0].EventName)
	assert.Equal(t, "performance", p.Events[0].EventCategory)
	assert.Equal(t, map[string]string{"scale": "lt1k"}, p.Events[0].Labels)
	require.NotNil(t, p.Events[0].DurationMs)
	assert.EqualValues(t, 40, *p.Events[0].DurationMs)
}

func TestFilesTelemetryAdapter_ShouldLogWarnNotReturnError_When_RecordFails(t *testing.T) {
	p := &fsopstest.RecordingProvider{Err: errors.New("db down")}
	var logs strings.Builder
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })

	parent, cancel := context.WithCancel(context.Background())
	cancel() // the request is already over when telemetry runs
	NewFilesTelemetry(p).RecordWrite(parent, "s", WriteOutcomeEvent{Outcome: WriteOutcomeOK})

	require.Len(t, p.Events, 1)
	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "db down")
	require.NoError(t, p.CtxErr[0], "detached from the cancelled request context")
	require.Len(t, p.Budget, 1, "record context carries a deadline")
	assert.LessOrEqual(t, p.Budget[0], 500*time.Millisecond)
}

func TestFilesTelemetryEvents_ShouldHaveNoFreeFormStringField_When_ReflectedOver(t *testing.T) {
	for _, ev := range []any{WriteOutcomeEvent{}, SearchOutcomeEvent{}, PatchOutcomeEvent{}} {
		typ := reflect.TypeOf(ev)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Type.Kind() == reflect.String {
				assert.NotEqual(t, "string", f.Type.Name(), "%s.%s must be a named enum type, not a free-form string", typ.Name(), f.Name)
			}
			assert.NotEqual(t, reflect.Interface, f.Type.Kind(), "%s.%s", typ.Name(), f.Name)
			assert.NotEqual(t, reflect.Map, f.Type.Kind(), "%s.%s", typ.Name(), f.Name)
			assert.NotEqual(t, reflect.Slice, f.Type.Kind(), "%s.%s", typ.Name(), f.Name)
		}
	}
}

func TestBuckets_ShouldMapBoundaries(t *testing.T) {
	for n, want := range map[int64]SizeBucket{0: "lt1k", 1023: "lt1k", 1024: "1k-10k", 10239: "1k-10k", 10240: "10k-100k",
		102400: "100k-1m", 1 << 20: "ge1m"} {
		assert.Equal(t, want, BucketSize(n), n)
	}
	for n, want := range map[int]CountBucket{0: "0", 1: "1-9", 9: "1-9", 10: "10-99", 999: "100-999", 1000: "1000+"} {
		assert.Equal(t, want, BucketCount(n), n)
	}
}

func TestFilesTelemetry_ShouldRecordOnePatchOutcomePerCall_WithEnumLabelsOnly_And_NotAlterResponseOnError(t *testing.T) {
	outcomes := []PatchOutcome{
		PatchOutcomeOK, PatchOutcomeConflict, PatchOutcomeOffsetMismatch, PatchOutcomeResultMismatch,
		PatchOutcomeOutOfOrder, PatchOutcomeAnalysisPending, PatchOutcomeTooLarge, PatchOutcomeRejected,
		PatchOutcomeDisabled, PatchOutcomeError,
	}
	p := &fsopstest.RecordingProvider{}
	tel := NewFilesTelemetry(p)
	for _, o := range outcomes {
		tel.RecordPatch(context.Background(), "sess-1", PatchOutcomeEvent{
			Outcome: o, Tier: PatchTierT1, Transport: TransportConnectUnary,
			Payload: BucketSize(2048), Ops: BucketOps(12), Duration: 7 * time.Millisecond,
		})
	}

	require.Len(t, p.Events, len(outcomes), "exactly one event per call")
	for i, e := range p.Events {
		assert.Equal(t, "files.server.patch", e.EventName)
		assert.Equal(t, "rpc", e.EventCategory)
		assert.Equal(t, "sess-1", e.SessionID)
		require.NotNil(t, e.DurationMs)
		assert.EqualValues(t, 7, *e.DurationMs)
		assert.Equal(t, map[string]string{
			"outcome": string(outcomes[i]), "tier": "t1", "transport": "connect_unary",
			"payload": "1k-10k", "ops": "10-99",
		}, e.Labels)
		for k, v := range e.Labels {
			assert.Regexp(t, labelValueShape, v, k)
		}
	}

	t.Run("recorder error is logged at Warn and never returned", func(t *testing.T) {
		failing := &fsopstest.RecordingProvider{Err: errors.New("db down")}
		var logs strings.Builder
		prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
		t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })
		NewFilesTelemetry(failing).RecordPatch(context.Background(), "s", PatchOutcomeEvent{Outcome: PatchOutcomeOK})
		require.Len(t, failing.Events, 1)
		assert.Contains(t, logs.String(), "level=WARN")
	})
	t.Run("no-op default does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			NewFilesTelemetry(nil).RecordPatch(context.Background(), "s", PatchOutcomeEvent{})
		})
	})
}

func TestBucketOps_ShouldMapBoundaries(t *testing.T) {
	for n, want := range map[int]OpsBucket{0: "1", 1: "1", 2: "2-9", 9: "2-9", 10: "10-99", 99: "10-99", 100: "100-999", 999: "100-999", 1000: "1000+"} {
		assert.Equal(t, want, BucketOps(n), n)
	}
}
