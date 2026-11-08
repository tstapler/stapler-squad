package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	pkganalytics "github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/server/analytics/escapedb"
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

	if s.escapeEventsClient == nil {
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

	totalCount, err := s.escapeEventsClient.EscapeEvent.Query().Where(filters...).Count(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	query := s.escapeEventsClient.EscapeEvent.Query().
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

	if s.escapeEventsClient == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("escape analytics not available"))
	}

	q := escapedb.AggregateQuery{SessionID: req.Msg.SessionId}
	if req.Msg.StartTime != nil {
		start := req.Msg.StartTime.AsTime()
		q.Start = &start
	}
	if req.Msg.EndTime != nil {
		end := req.Msg.EndTime.AsTime()
		q.End = &end
	}
	events, err := s.escapeAggregateReader().Aggregate(ctx, q)
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
			c.Count += e.Count
			totalSeq += e.Count
		}
		if isEscapeOutcomeStage(e.Stage) {
			outcomes += e.Count
			if e.Mangled {
				c.MangledCount += e.Count
				totalMangled += e.Count
			}
			if e.MangleType == "stripped" {
				stripped += e.Count
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

// GetEscapeAnalyticsGlobalSummary returns aggregate escape sequence statistics
// across all sessions, plus a per-session breakdown to spot outliers.
// +api: analytics:get-escape-global-summary
func (s *SessionService) GetEscapeAnalyticsGlobalSummary(
	ctx context.Context,
	req *connect.Request[sessionv1.GetEscapeAnalyticsGlobalSummaryRequest],
) (*connect.Response[sessionv1.GetEscapeAnalyticsGlobalSummaryResponse], error) {
	if s.escapeEventsClient == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("escape analytics not available"))
	}

	var q escapedb.AggregateQuery
	if req.Msg.StartTime != nil {
		start := req.Msg.StartTime.AsTime()
		q.Start = &start
	}
	if req.Msg.EndTime != nil {
		end := req.Msg.EndTime.AsTime()
		q.End = &end
	}
	// Rollups answer old ranges and raw rows the partial hours at the ends, so the number
	// of rows is bounded by distinct dimension combinations, not event volume.
	events, err := s.escapeAggregateReader().Aggregate(ctx, q)
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
				a.source += e.Count
			}
			if isEscapeOutcomeStage(e.Stage) {
				a.outcomes += e.Count
				if e.Mangled {
					a.mangled += e.Count
				}
				if e.MangleType == "stripped" {
					a.stripped += e.Count
				}
			}
		}
		if isEscapeSourceStage(e.Stage) {
			h.Count += e.Count
		}
		if isEscapeOutcomeStage(e.Stage) && e.Mangled {
			h.MangledCount += e.Count
		}
	}
	histogram := make([]*sessionv1.EscapeSequenceCount, 0, len(hist))
	for _, h := range hist {
		histogram = append(histogram, h)
	}
	sort.Slice(histogram, func(i, j int) bool {
		return histogram[i].SequenceType < histogram[j].SequenceType
	})

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
	sort.Slice(perSession, func(i, j int) bool {
		return perSession[i].SessionId < perSession[j].SessionId
	})
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
	sort.Slice(perProject, func(i, j int) bool {
		return perProject[i].ProjectPath < perProject[j].ProjectPath
	})
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
