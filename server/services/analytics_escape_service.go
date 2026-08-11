package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
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
			ByteLength:   int32(e.ByteLength),
			Mangled:      e.Mangled,
			SessionSeq:   e.SessionSeq,
			WallTime:     timestamppb.New(e.WallTime),
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
		protoEvents = append(protoEvents, pe)
	}

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

	// Fetch only the fields needed for aggregation — sequence_type and mangled.
	events, err := query.Select(
		escapeevent.FieldSequenceType,
		escapeevent.FieldMangled,
	).All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	counts := make(map[string]*sessionv1.EscapeSequenceCount)
	var totalSeq, totalMangled int64
	for _, e := range events {
		c, ok := counts[e.SequenceType]
		if !ok {
			c = &sessionv1.EscapeSequenceCount{SequenceType: e.SequenceType}
			counts[e.SequenceType] = c
		}
		c.Count++
		totalSeq++
		if e.Mangled {
			c.MangledCount++
			totalMangled++
		}
	}

	histogram := make([]*sessionv1.EscapeSequenceCount, 0, len(counts))
	for _, c := range counts {
		histogram = append(histogram, c)
	}

	var mangleRate float64
	if totalSeq > 0 {
		mangleRate = float64(totalMangled) / float64(totalSeq)
	}

	return connect.NewResponse(&sessionv1.GetEscapeAnalyticsSummaryResponse{
		Histogram:      histogram,
		TotalSequences: totalSeq,
		TotalMangled:   totalMangled,
		MangleRate:     mangleRate,
	}), nil
}

// escapeMangleRate computes totalMangled/total, guarded to 0 when total is 0.
// Shared by the global rate and each per-session breakdown row below.
func escapeMangleRate(total, mangled int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(mangled) / float64(total)
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

	// Histogram: real GROUP BY sequence_type, run in SQL rather than pulling
	// every matching row into Go and folding it there.
	var histRows []escapeAggregateRow
	err := s.analyticsClient.EscapeEvent.Query().
		Where(timeFilters...).
		GroupBy(escapeevent.FieldSequenceType).
		Aggregate(
			ent.As(ent.Count(), "count"),
			ent.As(ent.Sum(escapeevent.FieldMangled), "mangled_count"),
		).
		Scan(ctx, &histRows)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	histogram := make([]*sessionv1.EscapeSequenceCount, 0, len(histRows))
	var totalSeq, totalMangled int64
	for _, row := range histRows {
		histogram = append(histogram, &sessionv1.EscapeSequenceCount{
			SequenceType: row.SequenceType,
			Count:        row.Count,
			MangledCount: row.MangledCount,
		})
		totalSeq += row.Count
		totalMangled += row.MangledCount
	}

	// Per-session breakdown: real GROUP BY session_id, same aggregate shape.
	var sessionRows []escapeAggregateRow
	err = s.analyticsClient.EscapeEvent.Query().
		Where(timeFilters...).
		GroupBy(escapeevent.FieldSessionID).
		Aggregate(
			ent.As(ent.Count(), "count"),
			ent.As(ent.Sum(escapeevent.FieldMangled), "mangled_count"),
		).
		Scan(ctx, &sessionRows)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	perSession := make([]*sessionv1.SessionEscapeSummary, 0, len(sessionRows))
	for _, row := range sessionRows {
		perSession = append(perSession, &sessionv1.SessionEscapeSummary{
			SessionId:      row.SessionID,
			TotalSequences: row.Count,
			TotalMangled:   row.MangledCount,
			MangleRate:     escapeMangleRate(row.Count, row.MangledCount),
		})
	}

	return connect.NewResponse(&sessionv1.GetEscapeAnalyticsGlobalSummaryResponse{
		Histogram:      histogram,
		TotalSequences: totalSeq,
		TotalMangled:   totalMangled,
		MangleRate:     escapeMangleRate(totalSeq, totalMangled),
		PerSession:     perSession,
	}), nil
}
