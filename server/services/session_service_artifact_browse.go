package services

import (
	"cmp"
	"encoding/base64"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"connectrpc.com/connect"
	"context"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/session/artifacts"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/search"
	"github.com/tstapler/stapler-squad/telemetry"
	"go.opentelemetry.io/otel/metric"
)

const (
	defaultBrowsePageSize = 50
	maxBrowsePageSize     = 200

	// MetricArtifactBrowse counts ListSessionArtifacts calls (observability plan, T10.1.1).
	MetricArtifactBrowse = "ssq_artifact_browse_total"
)

var (
	artifactBrowseCount atomic.Int64
	artifactBrowseCtr   = func() metric.Int64Counter {
		c, _ := telemetry.GetMeter().Int64Counter(MetricArtifactBrowse)
		return c
	}()
)

// ArtifactBrowseCount is ssq_artifact_browse_total in this process.
func ArtifactBrowseCount() int64 { return artifactBrowseCount.Load() }

// browseFilter is the validated form of ListSessionArtifactsRequest's filters.
type browseFilter struct {
	types map[string]struct{}
	repo  string // lowercase owner/repo
	state string
}

func newBrowseFilter(req *sessionv1.ListSessionArtifactsRequest) browseFilter {
	f := browseFilter{repo: strings.ToLower(strings.TrimSpace(req.GetRepo())), state: strings.ToLower(strings.TrimSpace(req.GetState()))}
	if len(req.GetTypes()) > 0 {
		f.types = make(map[string]struct{}, len(req.GetTypes()))
		for _, t := range req.GetTypes() {
			f.types[t] = struct{}{}
		}
	}
	return f
}

// recordRepo is the lowercase owner/repo a PR or issue record names, "" for other kinds.
func recordRepo(rec *sessionv1.ArtifactRecordProto) string {
	switch {
	case rec.GetPr() != nil:
		return strings.ToLower(rec.GetPr().GetOwner() + "/" + rec.GetPr().GetRepo())
	case rec.GetIssue() != nil:
		return strings.ToLower(rec.GetIssue().GetOwner() + "/" + rec.GetIssue().GetRepo())
	}
	return ""
}

// keeps reports whether a record with this resolved PR entry (nil for non-PR) passes the filters.
func (f browseFilter) keeps(rec *sessionv1.ArtifactRecordProto, pr *sessionv1.SessionPullRequest) bool {
	if f.types != nil {
		if _, ok := f.types[rec.GetKind()]; !ok {
			return false
		}
	}
	if f.repo != "" && recordRepo(rec) != f.repo {
		return false
	}
	if f.state != "" && (pr == nil || pr.GetState() != f.state) {
		return false
	}
	return true
}

// pullRequestFor resolves a PR record against the session's own PR entries; a PR the
// session only saw gets a minimal entry in state "unknown".
func pullRequestFor(inst *instance.Instance, rec *sessionv1.ArtifactRecordProto) (*sessionv1.SessionPullRequest, string) {
	r := rec.GetPr()
	ref, err := domain.NewPRRef(r.GetHost(), r.GetOwner(), r.GetRepo(), int(r.GetNumber()))
	if err == nil {
		snap := inst.Snapshot()
		for _, v := range snap.GitHub.PRs {
			if v.Ref == ref {
				return adapters.SessionPullRequests([]domain.PRView{v}, snap.Branch)[0], string(search.PRSourceSessionPR)
			}
		}
	}
	host := r.GetHost()
	if host == "" {
		host = "github.com"
	}
	return &sessionv1.SessionPullRequest{
		Host: host, Owner: r.GetOwner(), Repo: r.GetRepo(), Number: r.GetNumber(), State: "unknown",
		Url: fmt.Sprintf("https://%s/%s/%s/pull/%d", host, r.GetOwner(), r.GetRepo(), r.GetNumber()),
	}, string(search.PRSourceArtifact)
}

// ListSessionArtifacts browses the artifact records of every visible session, newest first.
func (a *ArtifactService) ListSessionArtifacts(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSessionArtifactsRequest],
) (*connect.Response[sessionv1.ListSessionArtifactsResponse], error) {
	offset, err := decodePageToken(req.Msg.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	vis := a.visibleSessions(req.Msg.GetExcludeAutomationSessions(), req.Msg.GetIncludeArchived())
	if prFilter, ok, err := listPRFilter(req.Msg.GetPr()); err != nil {
		return nil, err
	} else if ok {
		if a.index == nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("artifact index is not running"))
		}
		allowed, err := a.sessionsForPR(ctx, prFilter, vis)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		for id := range vis.byUUID {
			if _, ok := allowed[id]; !ok {
				delete(vis.byUUID, id)
			}
		}
	}
	rows := collectBrowseRows(vis, newBrowseFilter(req.Msg))
	sortBrowseRows(rows)

	total := len(rows)
	size := clampBrowsePageSize(req.Msg.GetPageSize())
	start := min(offset, total)
	end := min(start+size, total)
	next := ""
	if end < total {
		next = encodePageToken(end)
	}
	artifactBrowseCount.Add(1)
	if artifactBrowseCtr != nil {
		artifactBrowseCtr.Add(ctx, 1)
	}
	return connect.NewResponse(&sessionv1.ListSessionArtifactsResponse{
		Rows:                  rows[start:end],
		NextPageToken:         next,
		TotalCount:            int32(total), //nolint:gosec // bounded by the in-memory record count
		UnscannedSessionCount: vis.unscanned(),
	}), nil
}

func collectBrowseRows(vis visibleSet, f browseFilter) []*sessionv1.SessionArtifactRow {
	var rows []*sessionv1.SessionArtifactRow
	for _, inst := range vis.byUUID {
		snap := inst.Snapshot()
		if snap.Artifacts == nil {
			continue
		}
		// Redacted again here so blobs stored before redaction existed never leave the server.
		for _, r := range artifacts.RedactedRecords(snap.Artifacts.Records) {
			rec := adapters.ArtifactRecordToProto(r)
			if rec == nil {
				continue
			}
			row := &sessionv1.SessionArtifactRow{
				SessionId: inst.GetStableID(), SessionTitle: snap.Title,
				SessionArchived: snap.ArchivedAt != nil, Record: rec, Source: string(search.PRSourceArtifact),
			}
			if rec.GetPr() != nil {
				row.PullRequest, row.Source = pullRequestFor(inst, rec)
			}
			if !f.keeps(rec, row.PullRequest) {
				continue
			}
			if t := sessionLastActivity(snap); !t.IsZero() {
				row.SessionLastActivity = timestamppb.New(t)
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// sortBrowseRows orders newest record first; ties break on session then record text so pages are stable.
func sortBrowseRows(rows []*sessionv1.SessionArtifactRow) {
	slices.SortStableFunc(rows, func(a, b *sessionv1.SessionArtifactRow) int {
		if c := cmp.Compare(b.GetRecord().GetFirstSeenAt().AsTime().UnixNano(), a.GetRecord().GetFirstSeenAt().AsTime().UnixNano()); c != 0 {
			return c
		}
		if c := cmp.Compare(a.GetSessionId(), b.GetSessionId()); c != 0 {
			return c
		}
		return cmp.Compare(a.GetRecord().String(), b.GetRecord().String())
	})
}

func clampBrowsePageSize(n int32) int {
	switch {
	case n <= 0:
		return defaultBrowsePageSize
	case n > maxBrowsePageSize:
		return maxBrowsePageSize
	}
	return int(n)
}

func encodePageToken(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func decodePageToken(tok string) (int, error) {
	if tok == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return 0, fmt.Errorf("invalid page_token")
	}
	n, err := strconv.Atoi(string(raw))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid page_token")
	}
	return n, nil
}
