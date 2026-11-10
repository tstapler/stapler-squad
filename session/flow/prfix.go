package flow

import (
	"context"
	"slices"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// MarkerKind names a family of server-written flow markers.
type MarkerKind string

// MarkerKind values. Only server code writes them; they are never parsed from note text.
const (
	MarkerPRFixOutcome MarkerKind = "pr_fix_outcome"
	MarkerPRFixTrigger MarkerKind = "pr_fix_trigger"
	MarkerManualNudge  MarkerKind = "manual_nudge"
)

// PR-fix outcome values.
const (
	OutcomeSteered       = "steered"
	OutcomeRespawned     = "respawned"
	OutcomeSkippedActive = "skipped_active"
)

// Estimated codes emitted by the capture-dependent PR-fix metrics.
const (
	EstimatedPRFixOutcomeSinceCapture = "pr_fix_outcome:since_capture"
	EstimatedManualNudgeSinceCapture  = "manual_nudge:since_capture"
	EstimatedPRFixCauseSinceCapture   = "pr_fix_cause:since_capture"
	EstimatedPRFixCauseUnavailable    = "pr_fix_cause:unavailable"
	EstimatedPRFixDwellSplit          = "pr_fix_dwell_split:since_capture"
	EstimatedLeadTimeInWindowOnly     = "lead_time:in_window_only"
	EstimatedFPYInWindowOnly          = "fpy:in_window_only"
	EstimatedDwellSeedMixedOffset     = "dwell_seed:mixed_offset"
)

// TriggerJoinWindow is how far before a reopen event its trigger marker may
// sit: the writer's 30m dedupe interval plus 5m of slack, so a constant-trigger
// loop always has a recent record.
const TriggerJoinWindow = 35 * time.Minute

// ResolvedWithin is how soon after an outcome a merge counts as resolved by it.
const ResolvedWithin = 24 * time.Hour

// Marker is one server-written marker row.
type Marker struct {
	Item     ItemID
	Kind     MarkerKind
	Value    string
	PRNumber int
	At       time.Time
}

// MarkerCapture is when each marker kind first appeared; zero when never.
type MarkerCapture struct {
	FirstOutcome time.Time
	FirstTrigger time.Time
	FirstNudge   time.Time
}

// MarkerSource reads the system-written marker table only, never note text,
// so agent-posted text cannot forge a marker. Markers returns From <= At < To.
type MarkerSource interface {
	Markers(ctx context.Context, from, to time.Time) ([]Marker, error)
	MarkerCapture(ctx context.Context) (MarkerCapture, error)
}

// TriggerJoin attributes PR-fix reopen events to the trigger marker that
// preceded them, for events at or after Capture only. A nil join, or one with
// a zero Capture, attributes nothing.
type TriggerJoin struct {
	capture time.Time
	byItem  map[ItemID][]Marker
}

// NewTriggerJoin indexes the trigger markers in markers (other kinds are
// ignored). capture is the earliest capture time; zero disables the join.
func NewTriggerJoin(markers []Marker, capture time.Time) *TriggerJoin {
	j := &TriggerJoin{capture: capture.UTC(), byItem: map[ItemID][]Marker{}}
	for _, m := range markers {
		if m.Kind == MarkerPRFixTrigger {
			j.byItem[m.Item] = append(j.byItem[m.Item], m)
		}
	}
	for _, ms := range j.byItem {
		slices.SortStableFunc(ms, func(a, b Marker) int { return a.At.Compare(b.At) })
	}
	return j
}

// Active reports whether capture exists, so the split labels apply.
func (j *TriggerJoin) Active() bool { return j != nil && !j.capture.IsZero() }

// After reports whether at is on or after the capture date.
func (j *TriggerJoin) After(at time.Time) bool { return j.Active() && !at.Before(j.capture) }

// TriggerFor returns the value of the latest trigger marker for item at or
// before at and within TriggerJoinWindow of it; unknown otherwise, including
// for every event before capture.
func (j *TriggerJoin) TriggerFor(item ItemID, at time.Time) PRFixTrigger {
	if !j.After(at) {
		return PRFixTriggerUnknown
	}
	var m Marker
	found := false
	for _, cand := range j.byItem[item] { // sorted by At; linear is fine, markers per item are few
		if cand.At.After(at) {
			break
		}
		m, found = cand, true
	}
	if !found {
		return PRFixTriggerUnknown
	}
	if at.Sub(m.At) > TriggerJoinWindow {
		return PRFixTriggerUnknown
	}
	if t := PRFixTrigger(m.Value); ValidPRFixTrigger(t) {
		return t
	}
	return PRFixTriggerUnknown
}

// PRFixOutcomes counts PR-fix nudge outcomes in a window. Recording is false
// when no outcome was ever captured, in which case the counts are meaningless
// and must be shown as "not yet recording", not as zeros.
type PRFixOutcomes struct {
	Recording        bool
	Steered          int
	Respawned        int
	SkippedActive    int
	ResolvedWithin24 int
}

// PRFixOutcomeInput is the data PRFixOutcomeCounts reads.
type PRFixOutcomeInput struct {
	Window FlowWindow
	// Spans are the window's collapsed spans (real spans carry TriggeredBy).
	Spans []Span
	// Markers are the system-written outcome markers (other kinds are ignored).
	Markers []Marker
	// Since is when outcome capture began; zero means not recording.
	Since time.Time
	// RespawnSince is the overall capture start for derived respawns, which
	// exist before the first outcome marker; zero falls back to Since.
	RespawnSince time.Time
}

type outcomeEvent struct {
	item ItemID
	at   time.Time
}

// PRFixOutcomeCounts counts steered and skipped_active from system-written
// markers, and respawned from real pr_pending>in_progress status events whose
// TriggeredBy is "system" and whose note is the PR-fix reopen note, counted
// from the overall capture start (RespawnSince, else Since). Respawned is derived rather than stored, and the RPC and
// MCP transition paths cannot write TriggeredBy "system", so agent text and
// agent transitions cannot inflate it.
//
// ResolvedWithin24 counts those outcomes after which the item has a real
// transition into done within 24h. It only sees merges inside the window, so
// outcomes in the last 24h of the window can undercount.
func PRFixOutcomeCounts(in PRFixOutcomeInput) PRFixOutcomes {
	out := PRFixOutcomes{Recording: !in.Since.IsZero()}
	if !out.Recording {
		return out
	}
	respawnSince := in.RespawnSince
	if respawnSince.IsZero() || in.Since.Before(respawnSince) {
		respawnSince = in.Since
	}
	events := markerOutcomes(in, &out)
	merges := map[ItemID][]time.Time{}
	for _, s := range in.Spans {
		rs, ok := s.(RealSpan)
		if !ok {
			continue
		}
		if rs.To() == domain.BacklogStatusDone {
			merges[rs.Item()] = append(merges[rs.Item()], rs.At())
		}
		if isSystemPRFixReopen(rs) && !rs.At().Before(respawnSince) {
			out.Respawned++
			events = append(events, outcomeEvent{rs.Item(), rs.At()})
		}
	}
	for _, e := range events {
		if slices.ContainsFunc(merges[e.item], func(at time.Time) bool {
			return at.After(e.at) && at.Sub(e.at) <= ResolvedWithin
		}) {
			out.ResolvedWithin24++
		}
	}
	return out
}

// markerOutcomes tallies steered and skipped_active markers inside the window
// and from Since, returning them as outcome events.
func markerOutcomes(in PRFixOutcomeInput, out *PRFixOutcomes) []outcomeEvent {
	var events []outcomeEvent
	for _, m := range in.Markers {
		if m.Kind != MarkerPRFixOutcome || !in.Window.Contains(m.At) || m.At.Before(in.Since) {
			continue
		}
		switch m.Value {
		case OutcomeSteered:
			out.Steered++
		case OutcomeSkippedActive:
			out.SkippedActive++
		default:
			continue
		}
		events = append(events, outcomeEvent{m.Item, m.At})
	}
	return events
}

// isSystemPRFixReopen is a real pr_pending>in_progress written by the system
// with the PR-fix reopen note.
func isSystemPRFixReopen(rs RealSpan) bool {
	return rs.From() == domain.BacklogStatusPRPending && rs.To() == domain.BacklogStatusInProgress &&
		rs.TriggeredBy() == "system" && ClassifyNote(rs.Note()) == CausePRFixReopen
}

// ManualNudgeCounts tallies manual_nudge markers inside the window, bucketed
// by ISO week (every week in the window present, quiet ones with N 0). Not
// recording (first is zero) yields no counts and no buckets.
func ManualNudgeCounts(w FlowWindow, markers []Marker, first time.Time) ManualNudges {
	out := ManualNudges{Recording: !first.IsZero(), CaptureSince: first, Drill: DrillManualNudge}
	if !out.Recording {
		return out
	}
	last := isoWeekStart(w.End().Add(-time.Nanosecond))
	var weeks []time.Time
	for ws := isoWeekStart(w.Start()); !ws.After(last); ws = ws.AddDate(0, 0, 7) {
		weeks = append(weeks, ws)
	}
	if len(weeks) > maxTrendWeeks {
		weeks = weeks[len(weeks)-maxTrendWeeks:]
	}
	out.Weekly = make([]ThroughputBucket, len(weeks))
	for i, ws := range weeks {
		out.Weekly[i] = ThroughputBucket{WeekStart: ws, Drill: DrillManualNudge}
	}
	for _, m := range markers {
		if m.Kind != MarkerManualNudge || !w.Contains(m.At) || m.At.Before(first) {
			continue
		}
		if i, ok := weekIndex(weeks, m.At); ok {
			out.Weekly[i].N++
			out.Total++
		}
	}
	return out
}
