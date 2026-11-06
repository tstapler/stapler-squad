package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/artifacts"
	"github.com/tstapler/stapler-squad/session/search"
)

const (
	minArtifactQueryLen = 2
	defaultSearchLimit  = 20
	maxSearchLimit      = 100
	// hitPoolMultiple over-fetches documents so grouping them by session still
	// fills the page.
	hitPoolMultiple = 10

	queryKindText   = "text"
	queryKindPR     = "pr"
	queryKindSHA    = "sha"
	queryKindBranch = "branch"
)

var (
	// owner/repo#12 or https://host/owner/repo/pull/12 (host defaults to github.com).
	prQueryPattern  = regexp.MustCompile(`^(?:https?://([^/\s]+)/)?([\w.-]+)/([\w.-]+)(?:#|/pull/)(\d+)/?$`)
	shaQueryPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
)

// classifiedQuery is a query reduced to its shape.
type classifiedQuery struct {
	kind string
	pr   session.PRRef // set for queryKindPR
	sha  string        // set for queryKindSHA, lowercased
}

func classifyArtifactQuery(q string) classifiedQuery {
	if m := prQueryPattern.FindStringSubmatch(q); m != nil {
		if n, err := strconv.Atoi(m[4]); err == nil {
			if ref, err := session.NewPRRef(m[1], m[2], m[3], n); err == nil {
				return classifiedQuery{kind: queryKindPR, pr: ref}
			}
		}
	}
	if shaQueryPattern.MatchString(q) {
		return classifiedQuery{kind: queryKindSHA, sha: strings.ToLower(q)}
	}
	if !strings.ContainsAny(q, " \t\n") && strings.ContainsAny(q, "/-_.") {
		return classifiedQuery{kind: queryKindBranch}
	}
	return classifiedQuery{kind: queryKindText}
}

// Bounds on a search query, so one request cannot fan out into an unbounded
// number of index lookups.
const (
	maxArtifactQueryLen   = 256
	maxArtifactQueryTerms = 16
)

// SearchSessionArtifacts finds sessions by what they produced.
func (a *ArtifactService) SearchSessionArtifacts(
	ctx context.Context,
	req *connect.Request[sessionv1.SearchSessionArtifactsRequest],
) (*connect.Response[sessionv1.SearchSessionArtifactsResponse], error) {
	if a.index == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("artifact index is not running"))
	}
	query := strings.TrimSpace(req.Msg.GetQuery())
	if len([]rune(query)) < minArtifactQueryLen {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("query must be at least %d characters", minArtifactQueryLen))
	}
	if n := len([]rune(query)); n > maxArtifactQueryLen {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("query must be at most %d characters", maxArtifactQueryLen))
	}
	if n := len(strings.Fields(query)); n > maxArtifactQueryTerms {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("query must have at most %d terms", maxArtifactQueryTerms))
	}
	prFilter, hasPRFilter, err := listPRFilter(req.Msg.GetPr())
	if err != nil {
		return nil, err
	}
	start := time.Now()
	cq := classifyArtifactQuery(query)
	vis := a.visibleSessions(req.Msg.GetExcludeAutomationSessions(), req.Msg.GetIncludeArchived())

	limit := clampSearchLimit(req.Msg.GetLimit())
	groups := newHitGroups(vis)
	for _, h := range a.index.Search(query, limit*hitPoolMultiple) {
		groups.addMatch(h.SessionID, h.Type, h.Text, h.Score)
	}
	if err := a.addExactMatches(ctx, groups, cq); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if hasPRFilter {
		allowed, err := a.sessionsForPR(ctx, prFilter, vis)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		groups.retain(allowed)
	}
	hits := groups.rankedByScore(limit)
	artifactSearchStats.observe(cq.kind, len(hits) > 0, time.Since(start))
	return connect.NewResponse(&sessionv1.SearchSessionArtifactsResponse{
		Hits:                  hits,
		UnscannedSessionCount: vis.unscanned(),
	}), nil
}

// FindSessionsByPullRequest lists the sessions tied to one PR, Tracked rows first.
func (a *ArtifactService) FindSessionsByPullRequest(
	ctx context.Context,
	req *connect.Request[sessionv1.FindSessionsByPullRequestRequest],
) (*connect.Response[sessionv1.FindSessionsByPullRequestResponse], error) {
	if a.index == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("artifact index is not running"))
	}
	ref, err := prRefFromProto(req.Msg.GetPr())
	if err != nil {
		return nil, err
	}
	start := time.Now()
	vis := a.visibleSessions(req.Msg.GetExcludeAutomationSessions(), req.Msg.GetIncludeArchived())
	found, err := a.index.SessionsForPR(ctx, ref)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	hits := make([]*sessionv1.SessionArtifactHit, 0, len(found))
	for _, f := range found {
		if inst, ok := vis.byUUID[f.SessionID]; ok { // a link whose session is gone is an orphan
			hits = append(hits, newSessionHit(inst, string(f.Source)))
		}
	}
	sortPRHits(hits)
	artifactSearchStats.observe(queryKindPR, len(hits) > 0, time.Since(start))
	return connect.NewResponse(&sessionv1.FindSessionsByPullRequestResponse{
		Hits:                  hits,
		UnscannedSessionCount: vis.unscanned(),
	}), nil
}

// addExactMatches adds the sessions an exact lookup finds, which BM25 over the
// document text cannot: a PR's SessionPR links and a SHA longer than 7 characters.
func (a *ArtifactService) addExactMatches(ctx context.Context, g *hitGroups, cq classifiedQuery) error {
	switch cq.kind {
	case queryKindPR:
		found, err := a.index.SessionsForPR(ctx, cq.pr)
		if err != nil {
			return err
		}
		for _, f := range found {
			g.addMatch(f.SessionID, artifacts.KindPR, prDisplay(cq.pr), exactMatchScore)
		}
	case queryKindSHA:
		for _, id := range a.index.SessionsForSHAPrefix(cq.sha) {
			g.addMatch(id, artifacts.KindCommit, cq.sha, exactMatchScore)
		}
	}
	return nil
}

// exactMatchScore outranks any BM25 score: the identifier was typed in full.
const exactMatchScore = 1e6

func prDisplay(ref session.PRRef) string {
	return fmt.Sprintf("%s/%s#%d", ref.Owner(), ref.Repo(), ref.Number())
}

// sessionsForPR is the set of visible sessions tied to ref by either source.
func (a *ArtifactService) sessionsForPR(ctx context.Context, ref session.PRRef, vis visibleSet) (map[string]struct{}, error) {
	found, err := a.index.SessionsForPR(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(found))
	for _, f := range found {
		if _, ok := vis.byUUID[f.SessionID]; ok {
			out[f.SessionID] = struct{}{}
		}
	}
	return out, nil
}

func clampSearchLimit(n int32) int {
	switch {
	case n <= 0:
		return defaultSearchLimit
	case n > maxSearchLimit:
		return maxSearchLimit
	}
	return int(n)
}

// visibleSet is the live sessions a caller may see, keyed by UUID (the index key).
type visibleSet struct{ byUUID map[string]*session.Instance }

// visibleSessions applies the history-search hidden rule (hidden sessions are
// dropped only on request) and the list rule (archived are dropped unless asked
// for). A session without a live Instance is never visible: results are sessions.
func (a *ArtifactService) visibleSessions(excludeAutomation, includeArchived bool) visibleSet {
	var all []*session.Instance
	if a.allInstances != nil {
		all = a.allInstances()
	}
	out := make(map[string]*session.Instance, len(all))
	for _, inst := range all {
		snap := inst.Snapshot()
		switch {
		case snap.UUID == "":
		case excludeAutomation && snap.Hidden:
		case snap.ArchivedAt != nil && !includeArchived:
		default:
			out[snap.UUID] = inst
		}
	}
	return visibleSet{byUUID: out}
}

// unscanned counts visible sessions whose artifacts are not scanned yet
// (scan_state not_linked or scanning, or no blob at all).
func (v visibleSet) unscanned() int32 {
	var n int32
	for _, inst := range v.byUUID {
		blob := inst.Snapshot().Artifacts
		if blob == nil {
			n++
			continue
		}
		if st := blob.State(); st == artifacts.ScanStateNotLinked || st == artifacts.ScanStateScanning {
			n++
		}
	}
	return n
}

func sessionLastActivity(snap *session.InstanceSnapshot) time.Time {
	if !snap.LastMeaningfulChangeAt.IsZero() {
		return snap.LastMeaningfulChangeAt
	}
	return snap.UpdatedAt
}

func newSessionHit(inst *session.Instance, source string) *sessionv1.SessionArtifactHit {
	snap := inst.Snapshot()
	hit := &sessionv1.SessionArtifactHit{SessionId: inst.GetStableID(), Title: snap.Title, Source: source}
	if t := sessionLastActivity(snap); !t.IsZero() {
		hit.LastActivity = timestamppb.New(t)
	}
	return hit
}

// sortPRHits orders rows Tracked first, then Seen in transcript; inside each
// group the newest activity first, then the title.
func sortPRHits(hits []*sessionv1.SessionArtifactHit) {
	rank := func(h *sessionv1.SessionArtifactHit) int {
		if h.Source == string(search.PRSourceSessionPR) {
			return 0
		}
		return 1
	}
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		if ta, tb := a.LastActivity.AsTime(), b.LastActivity.AsTime(); !ta.Equal(tb) {
			return ta.After(tb)
		}
		return a.Title < b.Title
	})
}

// hitGroups collects matching artifacts per visible session.
type hitGroups struct {
	vis   visibleSet
	hits  map[string]*sessionv1.SessionArtifactHit
	score map[string]float64
}

func newHitGroups(vis visibleSet) *hitGroups {
	return &hitGroups{vis: vis, hits: map[string]*sessionv1.SessionArtifactHit{}, score: map[string]float64{}}
}

func (g *hitGroups) addMatch(uuid string, kind artifacts.RecordKind, text string, score float64) {
	inst, ok := g.vis.byUUID[uuid]
	if !ok {
		return
	}
	h := g.hits[uuid]
	if h == nil {
		h = newSessionHit(inst, string(search.PRSourceArtifact))
		g.hits[uuid] = h
	}
	for _, m := range h.Matches {
		if m.Type == string(kind) && m.Text == text {
			g.score[uuid] = max(g.score[uuid], score)
			return
		}
	}
	h.Matches = append(h.Matches, &sessionv1.ArtifactMatch{Type: string(kind), Text: text, Score: float32(score)})
	g.score[uuid] = max(g.score[uuid], score)
}

func (g *hitGroups) retain(allowed map[string]struct{}) {
	for id := range g.hits {
		if _, ok := allowed[id]; !ok {
			delete(g.hits, id)
			delete(g.score, id)
		}
	}
}

// rankedByScore returns at most limit sessions, best score first, then newest
// activity, then title. Each session's matches are best first.
func (g *hitGroups) rankedByScore(limit int) []*sessionv1.SessionArtifactHit {
	out := make([]*sessionv1.SessionArtifactHit, 0, len(g.hits))
	for _, h := range g.hits {
		sort.SliceStable(h.Matches, func(i, j int) bool { return h.Matches[i].Score > h.Matches[j].Score })
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if sa, sb := g.score[a.SessionId], g.score[b.SessionId]; sa != sb {
			return sa > sb
		}
		if ta, tb := a.LastActivity.AsTime(), b.LastActivity.AsTime(); !ta.Equal(tb) {
			return ta.After(tb)
		}
		return a.Title < b.Title
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
