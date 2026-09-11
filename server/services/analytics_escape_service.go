package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/escapeevent"
	"github.com/tstapler/stapler-squad/session/ent/predicate"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// QueryEscapeAnalytics returns paginated escape event records for a session.
// +api: escape:query
func (s *SessionService) QueryEscapeAnalytics(
	ctx context.Context,
	req *connect.Request[sessionv1.QueryEscapeAnalyticsRequest],
) (*connect.Response[sessionv1.QueryEscapeAnalyticsResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session_id is required"))
	}

	if s.analyticsClient == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("escape analytics not available"))
	}

	pageSize := int(req.Msg.PageSize)
	if pageSize <= 0 {
		pageSize = 100
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	// Filters shared between the page query and the total-count query below —
	// the count must reflect all matching rows, not just the cursor'd page.
	filters := []predicate.EscapeEvent{escapeevent.SessionID(req.Msg.SessionId)}
	if req.Msg.Stage != "" {
		filters = append(filters, escapeevent.Stage(req.Msg.Stage))
	}
	if req.Msg.SequenceType != "" {
		filters = append(filters, escapeevent.SequenceType(req.Msg.SequenceType))
	}
	if req.Msg.MangledOnly {
		filters = append(filters, escapeevent.Mangled(true))
	}
	if req.Msg.StartTime != nil {
		filters = append(filters, escapeevent.WallTimeGTE(req.Msg.StartTime.AsTime()))
	}
	if req.Msg.EndTime != nil {
		filters = append(filters, escapeevent.WallTimeLTE(req.Msg.EndTime.AsTime()))
	}

	totalCount, err := s.analyticsClient.EscapeEvent.Query().Where(filters...).Count(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	query := s.analyticsClient.EscapeEvent.Query().
		Where(filters...).
		Order(ent.Asc(escapeevent.FieldSessionSeq)).
		Limit(pageSize + 1) // fetch one extra to determine if there's a next page

	// Cursor-based pagination via session_seq
	if req.Msg.PageToken != "" {
		cursor, err := strconv.ParseInt(req.Msg.PageToken, 10, 64)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
		}
		query = query.Where(escapeevent.SessionSeqGT(cursor))
	}

	events, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var nextPageToken string
	if len(events) > pageSize {
		events = events[:pageSize]
		nextPageToken = strconv.FormatInt(events[len(events)-1].SessionSeq, 10)
	}

	protoEvents := make([]*sessionv1.EscapeEventProto, 0, len(events))
	for _, e := range events {
		pe := &sessionv1.EscapeEventProto{
			Id:           e.ID,
			SessionId:    e.SessionID,
			Stage:        e.Stage,
			SequenceType: e.SequenceType,
			// #nosec G115 -- ByteLength is the byte length of one captured
			// terminal escape-sequence chunk, far below int32 range.
			ByteLength: int32(e.ByteLength),
			Mangled:    e.Mangled,
			SessionSeq: e.SessionSeq,
			WallTime:   timestamppb.New(e.WallTime),
		}
		if e.SequenceSubtype != "" {
			pe.SequenceSubtype = e.SequenceSubtype
		}
		if e.PayloadHash != "" {
			pe.PayloadHash = e.PayloadHash
		}
		if len(e.RawBytes) > 0 {
			pe.RawBytes = e.RawBytes
		}
		if e.MangleType != "" {
			pe.MangleType = e.MangleType
		}
		pe.ProjectPath = e.ProjectPath
		pe.SequenceSignature = e.SequenceSignature
		protoEvents = append(protoEvents, pe)
	}

	// #nosec G115 -- totalCount is a local escape-analytics event count, far below int32 range.
	return connect.NewResponse(&sessionv1.QueryEscapeAnalyticsResponse{
		Events:        protoEvents,
		NextPageToken: nextPageToken,
		TotalCount:    int32(totalCount),
	}), nil
}

// GetEscapeAnalyticsSummary returns aggregate escape sequence statistics for a session.
// +api: escape:summary
func (s *SessionService) GetEscapeAnalyticsSummary(
	ctx context.Context,
	req *connect.Request[sessionv1.GetEscapeAnalyticsSummaryRequest],
) (*connect.Response[sessionv1.GetEscapeAnalyticsSummaryResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session_id is required"))
	}

	if s.analyticsClient == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("escape analytics not available"))
	}

	query := s.analyticsClient.EscapeEvent.Query().
		Where(escapeevent.SessionID(req.Msg.SessionId))

	if req.Msg.StartTime != nil {
		query = query.Where(escapeevent.WallTimeGTE(req.Msg.StartTime.AsTime()))
	}
	if req.Msg.EndTime != nil {
		query = query.Where(escapeevent.WallTimeLTE(req.Msg.EndTime.AsTime()))
	}

	events, err := query.Select(
		escapeevent.FieldSequenceType,
		escapeevent.FieldStage,
		escapeevent.FieldMangled,
		escapeevent.FieldMangleType,
	).All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	counts := make(map[string]*sessionv1.EscapeSequenceCount)
	var totalSeq, totalMangled, outcomes, stripped int64
	for _, e := range events {
		c, ok := counts[e.SequenceType]
		if !ok {
			c = &sessionv1.EscapeSequenceCount{SequenceType: e.SequenceType}
			counts[e.SequenceType] = c
		}
		if isEscapeSourceStage(e.Stage) {
			c.Count++
			totalSeq++
		}
		if isEscapeOutcomeStage(e.Stage) {
			outcomes++
			if e.Mangled {
				c.MangledCount++
				totalMangled++
			}
			if e.MangleType == "stripped" {
				stripped++
			}
		}
	}

	histogram := make([]*sessionv1.EscapeSequenceCount, 0, len(counts))
	for _, c := range counts {
		histogram = append(histogram, c)
	}

	matched := outcomes - stripped
	coverage := escapeCorrelationCoverage(totalSeq, matched)
	return connect.NewResponse(&sessionv1.GetEscapeAnalyticsSummaryResponse{
		Histogram: histogram, TotalSequences: totalSeq, TotalMangled: totalMangled,
		MangleRate:          escapeMangleRate(outcomes, totalMangled),
		CorrelationOutcomes: outcomes, MatchedSequences: matched,
		StrippedSequences: stripped, CorrelationCoverage: coverage,
		CaptureHealthy: totalSeq == 0 || coverage >= 0.8,
	}), nil
}

// escapeMangleRate computes totalMangled/total, guarded to 0 when total is 0.
// Shared by the global rate and each per-session breakdown row below.
func isEscapeSourceStage(stage string) bool {
	return stage == string(pkganalytics.StagePTYRead) || stage == "pty"
}

func isEscapeOutcomeStage(stage string) bool {
	return stage == string(pkganalytics.StageTransport) || stage == "pty"
}

func escapeMangleRate(total, mangled int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(mangled) / float64(total)
}

func escapeCorrelationCoverage(source, matched int64) float64 {
	if source == 0 {
		return 0
	}
	coverage := float64(matched) / float64(source)
	if coverage > 1 {
		return 1
	}
	return coverage
}

// escapeAggregateRow is the destination shape for both GroupBy/Aggregate
// queries below: one row per group key (sequence_type or session_id), with
// a count and a summed mangled column.
type escapeAggregateRow struct {
	SequenceType string `json:"sequence_type"`
	SessionID    string `json:"session_id"`
	Count        int64  `json:"count"`
	MangledCount int64  `json:"mangled_count"`
}

// GetEscapeAnalyticsGlobalSummary returns aggregate escape sequence statistics
// across all sessions, plus a per-session breakdown to spot outliers.
// +api: analytics:get-escape-global-summary
func (s *SessionService) GetEscapeAnalyticsGlobalSummary(
	ctx context.Context,
	req *connect.Request[sessionv1.GetEscapeAnalyticsGlobalSummaryRequest],
) (*connect.Response[sessionv1.GetEscapeAnalyticsGlobalSummaryResponse], error) {
	if s.analyticsClient == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("escape analytics not available"))
	}

	var timeFilters []predicate.EscapeEvent
	if req.Msg.StartTime != nil {
		timeFilters = append(timeFilters, escapeevent.WallTimeGTE(req.Msg.StartTime.AsTime()))
	}
	if req.Msg.EndTime != nil {
		timeFilters = append(timeFilters, escapeevent.WallTimeLTE(req.Msg.EndTime.AsTime()))
	}

	events, err := s.analyticsClient.EscapeEvent.Query().Where(timeFilters...).Select(
		escapeevent.FieldSessionID, escapeevent.FieldProjectPath,
		escapeevent.FieldSequenceType, escapeevent.FieldStage,
		escapeevent.FieldMangled, escapeevent.FieldMangleType,
	).All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	type aggregate struct {
		source, outcomes, mangled, stripped int64
		project                             string
	}
	hist := make(map[string]*sessionv1.EscapeSequenceCount)
	sessions := make(map[string]*aggregate)
	projects := make(map[string]*aggregate)
	global := &aggregate{}
	for _, e := range events {
		h := hist[e.SequenceType]
		if h == nil {
			h = &sessionv1.EscapeSequenceCount{SequenceType: e.SequenceType}
			hist[e.SequenceType] = h
		}
		sa := sessions[e.SessionID]
		if sa == nil {
			sa = &aggregate{project: e.ProjectPath}
			sessions[e.SessionID] = sa
		}
		if sa.project == "" {
			sa.project = e.ProjectPath
		}
		pa := projects[e.ProjectPath]
		if pa == nil {
			pa = &aggregate{project: e.ProjectPath}
			projects[e.ProjectPath] = pa
		}
		for _, a := range []*aggregate{global, sa, pa} {
			if isEscapeSourceStage(e.Stage) {
				a.source++
			}
			if isEscapeOutcomeStage(e.Stage) {
				a.outcomes++
				if e.Mangled {
					a.mangled++
				}
				if e.MangleType == "stripped" {
					a.stripped++
				}
			}
		}
		if isEscapeSourceStage(e.Stage) {
			h.Count++
		}
		if isEscapeOutcomeStage(e.Stage) && e.Mangled {
			h.MangledCount++
		}
	}
	histogram := make([]*sessionv1.EscapeSequenceCount, 0, len(hist))
	for _, h := range hist {
		histogram = append(histogram, h)
	}

	perSession := make([]*sessionv1.SessionEscapeSummary, 0, len(sessions))
	for id, a := range sessions {
		matched, coverage := a.outcomes-a.stripped, escapeCorrelationCoverage(a.source, a.outcomes-a.stripped)
		perSession = append(perSession, &sessionv1.SessionEscapeSummary{
			SessionId: id, ProjectPath: a.project, TotalSequences: a.source,
			TotalMangled: a.mangled, MangleRate: escapeMangleRate(a.outcomes, a.mangled),
			CorrelationOutcomes: a.outcomes, MatchedSequences: matched,
			StrippedSequences: a.stripped, CorrelationCoverage: coverage,
			CaptureHealthy: a.source == 0 || coverage >= 0.8,
		})
	}
	perProject := make([]*sessionv1.ProjectEscapeSummary, 0, len(projects))
	for _, a := range projects {
		matched, coverage := a.outcomes-a.stripped, escapeCorrelationCoverage(a.source, a.outcomes-a.stripped)
		perProject = append(perProject, &sessionv1.ProjectEscapeSummary{
			ProjectPath: a.project, TotalSequences: a.source, TotalMangled: a.mangled,
			MangleRate: escapeMangleRate(a.outcomes, a.mangled), CorrelationOutcomes: a.outcomes,
			MatchedSequences: matched, StrippedSequences: a.stripped,
			CorrelationCoverage: coverage, CaptureHealthy: a.source == 0 || coverage >= 0.8,
		})
	}
	matched, coverage := global.outcomes-global.stripped, escapeCorrelationCoverage(global.source, global.outcomes-global.stripped)
	return connect.NewResponse(&sessionv1.GetEscapeAnalyticsGlobalSummaryResponse{
		Histogram: histogram, TotalSequences: global.source, TotalMangled: global.mangled,
		MangleRate: escapeMangleRate(global.outcomes, global.mangled), PerSession: perSession,
		CorrelationOutcomes: global.outcomes, MatchedSequences: matched,
		StrippedSequences: global.stripped, CorrelationCoverage: coverage,
		CaptureHealthy: global.source == 0 || coverage >= 0.8,
		DroppedEvents:  pkganalytics.GetGlobalEscapeWriterDroppedCount(), PerProject: perProject,
	}), nil
}
