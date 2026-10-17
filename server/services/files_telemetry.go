package services

import (
	"context"
	"strconv"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/analytics"
)

// Server-side Files analytics (ADR-035). Outcome structs hold enum and bucket
// types only, so a path, query or error text cannot be recorded by construction.

const (
	filesWriteEventName  = "files.server.write"
	filesSearchEventName = "files.server.search"
	filesEventCategory   = "rpc"

	filesSearchFirstEventName = "files.server.search_first_result"
	filesPerfCategory         = "performance"

	telemetryRecordLimit = 500 * time.Millisecond
)

// FilesTelemetry is the outcome port for Files RPCs; implementations never return errors.
type FilesTelemetry interface {
	RecordWrite(ctx context.Context, sessionID string, ev WriteOutcomeEvent)
	RecordSearch(ctx context.Context, sessionID string, ev SearchOutcomeEvent)
	RecordSearchFirstResult(ctx context.Context, sessionID string, ev SearchFirstResultEvent)
}

type (
	WriteOutcome  string
	WriteReason   string
	SizeBucket    string
	EOLLabel      string
	BOMLabel      string
	SearchOutcome string
	SearchBackend string
	CountBucket   string
	ScaleBucket   string
)

const (
	WriteOutcomeOK               WriteOutcome = "ok"
	WriteOutcomeConflictRejected WriteOutcome = "conflict_rejected"
	WriteOutcomeRejected         WriteOutcome = "rejected"
	WriteOutcomeError            WriteOutcome = "error"

	WriteReasonNone            WriteReason = "none"
	WriteReasonFlagOff         WriteReason = "flag_off"
	WriteReasonHiddenSession   WriteReason = "hidden_session"
	WriteReasonArchived        WriteReason = "archived"
	WriteReasonGitInternal     WriteReason = "git_internal"
	WriteReasonSymlink         WriteReason = "symlink"
	WriteReasonBinary          WriteReason = "binary"
	WriteReasonTruncated       WriteReason = "truncated"
	WriteReasonTooLarge        WriteReason = "too_large"
	WriteReasonNonUTF8         WriteReason = "non_utf8"
	WriteReasonMixedEOL        WriteReason = "mixed_eol"
	WriteReasonWorktreeMissing WriteReason = "worktree_missing"
	WriteReasonBadToken        WriteReason = "bad_token"
	WriteReasonBadContent      WriteReason = "bad_content"

	EOLLF    EOLLabel = "lf"
	EOLCRLF  EOLLabel = "crlf"
	EOLMixed EOLLabel = "mixed"

	BOMYes BOMLabel = "yes"
	BOMNo  BOMLabel = "no"

	SearchOutcomeOK        SearchOutcome = "ok"
	SearchOutcomeTruncated SearchOutcome = "truncated"
	SearchOutcomeCancelled SearchOutcome = "cancelled"
	SearchOutcomeInvalid   SearchOutcome = "invalid"
	SearchOutcomeError     SearchOutcome = "error"
	SearchOutcomeDisabled  SearchOutcome = "disabled"
	SearchOutcomeBusy      SearchOutcome = "busy"

	SearchBackendWalker SearchBackend = "walker"
)

// WriteOutcomeEvent is the labels-only record of one WriteFile call.
type WriteOutcomeEvent struct {
	Outcome  WriteOutcome
	Reason   WriteReason
	Size     SizeBucket
	EOL      EOLLabel
	BOM      BOMLabel
	Duration time.Duration
}

// SearchOutcomeEvent is the labels-only record of one SearchContent call (ADR-035).
type SearchOutcomeEvent struct {
	Outcome  SearchOutcome
	Results  CountBucket
	Scale    ScaleBucket
	Queued   bool
	Backend  SearchBackend
	Large    bool
	Duration time.Duration
}

// SearchFirstResultEvent records the latency to the first batch of one search.
type SearchFirstResultEvent struct {
	Scale    ScaleBucket
	Duration time.Duration
}

// BucketScale maps the number of files a search scanned to its scale bucket.
func BucketScale(files int) ScaleBucket {
	switch {
	case files < 1000:
		return "lt1k"
	case files < 10000:
		return "1k-10k"
	case files < 50000:
		return "10k-50k"
	default:
		return "50k+"
	}
}

// BucketSize maps a byte count to its size bucket.
func BucketSize(n int64) SizeBucket {
	switch {
	case n < 1<<10:
		return "lt1k"
	case n < 10<<10:
		return "1k-10k"
	case n < 100<<10:
		return "10k-100k"
	case n < 1<<20:
		return "100k-1m"
	default:
		return "ge1m"
	}
}

// BucketCount maps a hit count to its bucket.
func BucketCount(n int) CountBucket {
	switch {
	case n <= 0:
		return "0"
	case n < 10:
		return "1-9"
	case n < 100:
		return "10-99"
	case n < 1000:
		return "100-999"
	default:
		return "1000+"
	}
}

type noopFilesTelemetry struct{}

func (noopFilesTelemetry) RecordWrite(context.Context, string, WriteOutcomeEvent)   {}
func (noopFilesTelemetry) RecordSearch(context.Context, string, SearchOutcomeEvent) {}
func (noopFilesTelemetry) RecordSearchFirstResult(context.Context, string, SearchFirstResultEvent) {
}

// NewFilesTelemetry adapts an AnalyticsProvider; nil gives the no-op default.
func NewFilesTelemetry(provider analytics.AnalyticsProvider) FilesTelemetry {
	if provider == nil {
		return noopFilesTelemetry{}
	}
	return &analyticsFilesTelemetry{provider: provider}
}

type analyticsFilesTelemetry struct{ provider analytics.AnalyticsProvider }

func (a *analyticsFilesTelemetry) RecordWrite(ctx context.Context, sessionID string, ev WriteOutcomeEvent) {
	a.record(ctx, analytics.Event{
		EventName: filesWriteEventName, EventCategory: filesEventCategory, SessionID: sessionID,
		DurationMs: durationMs(ev.Duration),
		Labels: map[string]string{
			"outcome": string(ev.Outcome), "reason": string(ev.Reason), "size": string(ev.Size),
			"eol": string(ev.EOL), "bom": string(ev.BOM),
		},
	})
}

func (a *analyticsFilesTelemetry) RecordSearch(ctx context.Context, sessionID string, ev SearchOutcomeEvent) {
	a.record(ctx, analytics.Event{
		EventName: filesSearchEventName, EventCategory: filesEventCategory, SessionID: sessionID,
		DurationMs: durationMs(ev.Duration),
		Labels: map[string]string{
			"outcome": string(ev.Outcome), "results": string(ev.Results), "scale": string(ev.Scale),
			"queued": strconv.FormatBool(ev.Queued), "backend": string(ev.Backend),
			"large": strconv.FormatBool(ev.Large),
		},
	})
}

func (a *analyticsFilesTelemetry) RecordSearchFirstResult(ctx context.Context, sessionID string, ev SearchFirstResultEvent) {
	a.record(ctx, analytics.Event{
		EventName: filesSearchFirstEventName, EventCategory: filesPerfCategory, SessionID: sessionID,
		DurationMs: durationMs(ev.Duration),
		Labels:     map[string]string{"scale": string(ev.Scale)},
	})
}

// record runs after the RPC's response is decided: a detached 500 ms context keeps
// telemetry alive past a cancelled request, and a failure is logged, never returned.
func (a *analyticsFilesTelemetry) record(ctx context.Context, e analytics.Event) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), telemetryRecordLimit)
	defer cancel()
	if err := a.provider.Record(ctx, e); err != nil {
		log.Warn("files telemetry record failed", "event", e.EventName, "err", err)
	}
}

func durationMs(d time.Duration) *int64 {
	ms := d.Milliseconds()
	return &ms
}
