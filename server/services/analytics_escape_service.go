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

	// Build query
	query := s.analyticsClient.EscapeEvent.Query().
		Where(escapeevent.SessionID(req.Msg.SessionId)).
		Order(ent.Asc(escapeevent.FieldSessionSeq)).
		Limit(pageSize + 1) // fetch one extra to determine if there's a next page

	if req.Msg.Stage != "" {
		query = query.Where(escapeevent.Stage(req.Msg.Stage))
	}
	if req.Msg.SequenceType != "" {
		query = query.Where(escapeevent.SequenceType(req.Msg.SequenceType))
	}
	if req.Msg.MangledOnly {
		query = query.Where(escapeevent.Mangled(true))
	}
	if req.Msg.StartTime != nil {
		query = query.Where(escapeevent.WallTimeGTE(req.Msg.StartTime.AsTime()))
	}
	if req.Msg.EndTime != nil {
		query = query.Where(escapeevent.WallTimeLTE(req.Msg.EndTime.AsTime()))
	}

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
		TotalCount:    int32(len(protoEvents)),
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
