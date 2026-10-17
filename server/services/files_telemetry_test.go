package services

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/analytics"
)

type recordingProvider struct {
	mu     sync.Mutex
	events []analytics.Event
	err    error
	ctxErr []error // ctx.Err() as seen inside Record
	budget []time.Duration
}

func (p *recordingProvider) Record(ctx context.Context, e analytics.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, e)
	p.ctxErr = append(p.ctxErr, ctx.Err())
	if dl, ok := ctx.Deadline(); ok {
		p.budget = append(p.budget, time.Until(dl))
	}
	return p.err
}

var labelValueShape = regexp.MustCompile(`^[a-z0-9_+-]{1,24}$`)

func TestFilesTelemetryAdapter_ShouldRecordOneEventWithEnumLabels_When_WriteConflictRejected(t *testing.T) {
	p := &recordingProvider{}
	tel := NewFilesTelemetry(p)

	tel.RecordWrite(context.Background(), "sess-1", WriteOutcomeEvent{
		Outcome: WriteOutcomeConflictRejected, Reason: WriteReasonNone,
		Size: BucketSize(2048), EOL: EOLCRLF, BOM: BOMYes, Duration: 12 * time.Millisecond,
	})

	require.Len(t, p.events, 1)
	e := p.events[0]
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
	p := &recordingProvider{}
	NewFilesTelemetry(p).RecordSearch(context.Background(), "s", SearchOutcomeEvent{
		Outcome: SearchOutcomeOK, Mode: SearchModeRegex, Hits: BucketCount(250), Truncated: true, Duration: time.Second,
	})
	require.Len(t, p.events, 1)
	assert.Equal(t, "files.server.search", p.events[0].EventName)
	assert.Equal(t, map[string]string{"outcome": "ok", "mode": "regex", "hits": "100-999", "truncated": "true"}, p.events[0].Labels)
}

func TestFilesTelemetryAdapter_ShouldLogWarnNotReturnError_When_RecordFails(t *testing.T) {
	p := &recordingProvider{err: errors.New("db down")}
	var logs strings.Builder
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })

	parent, cancel := context.WithCancel(context.Background())
	cancel() // the request is already over when telemetry runs
	NewFilesTelemetry(p).RecordWrite(parent, "s", WriteOutcomeEvent{Outcome: WriteOutcomeOK})

	require.Len(t, p.events, 1)
	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "db down")
	require.NoError(t, p.ctxErr[0], "detached from the cancelled request context")
	require.Len(t, p.budget, 1, "record context carries a deadline")
	assert.LessOrEqual(t, p.budget[0], 500*time.Millisecond)
}

func TestFilesTelemetry_ShouldDefaultToNoop_When_NoProvider(t *testing.T) {
	tel := NewFilesTelemetry(nil)
	assert.NotPanics(t, func() {
		tel.RecordWrite(context.Background(), "s", WriteOutcomeEvent{})
		tel.RecordSearch(context.Background(), "s", SearchOutcomeEvent{})
	})
	assert.NotPanics(t, func() {
		NewFileOps(FilePolicy{}, osRootBackend{}) // services construct without an analytics provider
	})
}

func TestFilesTelemetryEvents_ShouldHaveNoFreeFormStringField_When_ReflectedOver(t *testing.T) {
	for _, ev := range []any{WriteOutcomeEvent{}, SearchOutcomeEvent{}} {
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
