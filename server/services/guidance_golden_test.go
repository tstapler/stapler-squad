package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
)

// guidance_golden_test.go is a black-box transcript of the guidance RPCs, answer
// rules, reaper, acknowledgement detector and e2e seed route. It was recorded
// before the guidance leaf was extracted; set GUIDANCE_GOLDEN_UPDATE=1 to re-record.

const guidanceGoldenFile = "testdata/guidance.golden"

var (
	goldenUUIDRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	goldenTimeRe = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z`)
)

type guidanceTranscript struct {
	t   *testing.T
	b   strings.Builder
	ids map[string]string
}

func (g *guidanceTranscript) logf(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	s = goldenUUIDRe.ReplaceAllStringFunc(s, func(id string) string {
		if _, ok := g.ids[id]; !ok {
			g.ids[id] = fmt.Sprintf("<id%d>", len(g.ids)+1)
		}
		return g.ids[id]
	})
	g.b.WriteString(goldenTimeRe.ReplaceAllString(s, "<time>") + "\n")
}

func (g *guidanceTranscript) msg(m proto.Message) string {
	raw, err := protojson.Marshal(m)
	require.NoError(g.t, err)
	var v any
	require.NoError(g.t, json.Unmarshal(raw, &v))
	out, err := json.Marshal(v) // protojson randomizes whitespace; this is canonical
	require.NoError(g.t, err)
	return string(out)
}

func (g *guidanceTranscript) errText(err error) string {
	var ce *connect.Error
	switch {
	case err == nil:
		return "ok"
	case errors.As(err, &ce):
		return fmt.Sprintf("connect %s: %s", ce.Code(), ce.Message())
	default:
		return "error: " + err.Error()
	}
}

func (g *guidanceTranscript) rpc(label string, m proto.Message, err error) {
	if err != nil {
		g.logf("%s => %s", label, g.errText(err))
		return
	}
	g.logf("%s => %s", label, g.msg(m))
}

// events drains what was published synchronously.
func (g *guidanceTranscript) events(label string, ch <-chan *pkgevents.Event) {
	for {
		select {
		case ev := <-ch:
			meta, err := json.Marshal(ev.NotificationMetadata)
			require.NoError(g.t, err)
			g.logf("%s event type=%s session=%s context=%q ntype=%d prio=%d title=%q message=%q meta=%s", label,
				ev.Type, ev.SessionID, ev.Context, ev.NotificationType, ev.NotificationPriority, ev.NotificationTitle, ev.NotificationMessage, meta)
		default:
			return
		}
	}
}

func (g *guidanceTranscript) row(label string, s *session.Storage, id string) {
	r, err := s.GetGuidanceRequest(context.Background(), uuid.MustParse(id))
	require.NoError(g.t, err)
	g.logf("%s row status=%s answer=%q route=%q state=%q attempts=%d cancel=%q answerer=%s/%s", label,
		r.Status(), r.Answer, r.DeliveryRoute, r.DeliveryState, r.DeliveryAttempts, r.CancelReason.String(), r.AnswererKind, r.AnswererSessionUUID)
}

type goldenRespawner struct {
	calls chan string
	err   error
}

func (r *goldenRespawner) ResumeTriage(_ context.Context, itemID string) error {
	r.calls <- itemID
	return r.err
}

const (
	goldenAsker = "7c1e0000-0000-4000-8000-00000000a001"
	goldenPeer  = "7c1e0000-0000-4000-8000-00000000a002"
	goldenOther = "7c1e0000-0000-4000-8000-00000000a003"
)

func goldenRow(t *testing.T, s *session.Storage, p domain.GuidanceRequestParams) string {
	t.Helper()
	gr, err := domain.NewGuidanceRequest(p)
	require.NoError(t, err)
	row, err := s.CreateMirroredGuidanceRequest(context.Background(), gr, 50)
	require.NoError(t, err)
	return row.ID
}

func goldenAgentRow(t *testing.T, s *session.Storage, text string) string {
	t.Helper()
	return goldenRow(t, s, domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: goldenAsker, Source: domain.QuestionSourceAgentMCP,
		QuestionText: text, QuestionType: domain.QuestionTypeShortAnswer,
	})
}

func goldenPeerRow(t *testing.T, s *session.Storage, text string) string {
	t.Helper()
	return goldenRow(t, s, domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: goldenAsker, Source: domain.QuestionSourcePeer,
		AskerSessionUUID: goldenAsker, AnswererKind: domain.AnswererKindSession, AnswererSessionUUID: goldenPeer,
		QuestionText: text, QuestionType: domain.QuestionTypeShortAnswer,
	})
}

func goldenHookRow(t *testing.T, s *session.Storage, text string) string {
	t.Helper()
	payload, err := domain.NewQuestionPayload([]domain.Question{{Text: text, Header: "H", Options: []domain.Option{{Label: "Red"}, {Label: "Blue"}}}})
	require.NoError(t, err)
	return goldenRow(t, s, domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: goldenAsker, Source: domain.QuestionSourceHook,
		AskerSessionUUID: goldenAsker, Payload: payload, QuestionText: text, QuestionType: domain.QuestionTypeMultipleChoice,
		Options: `["Red","Blue"]`, HookCallID: domain.HookCallID("toolu_" + text), DeliveryRoute: domain.DeliveryRouteLiveKeystroke,
	})
}

func (g *guidanceTranscript) createAndRead() {
	t, ctx := g.t, context.Background()
	storage := svctest.CreateTestStorage(t)
	svc := goldenGuidanceService(storage, nil, nil)
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "linked"})
	require.NoError(t, err)
	_, err = storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: goldenAsker, SessionRole: "work"})
	require.NoError(t, err)
	missing, bad := "7c1e0000-0000-4000-8000-00000000ffff", "nope"

	create := func(label string, req *sessionv1.CreateGuidanceRequestRequest) {
		resp, err := svc.CreateGuidanceRequest(ctx, connect.NewRequest(req))
		if err != nil {
			g.rpc("create "+label, nil, err)
			return
		}
		g.rpc("create "+label, resp.Msg, nil)
	}
	base := func(scope, qtype, text string) *sessionv1.CreateGuidanceRequestRequest {
		return &sessionv1.CreateGuidanceRequestRequest{Scope: scope, QuestionType: qtype, QuestionText: text}
	}
	create("bad scope", base("planet", "yes-no", "q"))
	create("bad type", base("standalone", "essay", "q"))
	r := base("standalone", "yes-no", "q")
	r.ItemId = &bad
	create("bad item id", r)
	create("item scope without item", base("backlog-item", "yes-no", "q"))
	r = base("backlog-item", "yes-no", "q")
	r.ItemId, r.CallerSessionUuid = &missing, goldenAsker
	create("unknown item", r)
	r = base("backlog-item", "yes-no", "q")
	r.ItemId, r.CallerSessionUuid = &item.ID, goldenOther
	create("unlinked caller", r)
	create("session scope without session", base("session", "yes-no", "q"))
	r = base("session", "yes-no", "q")
	r.SessionUuid, r.CallerSessionUuid = goldenAsker, goldenOther
	create("session mismatch", r)
	create("empty text", base("standalone", "yes-no", ""))

	r = base("backlog-item", "multiple-choice", "merge first?")
	r.ItemId, r.CallerSessionUuid, r.Options = &item.ID, goldenAsker, []string{"yes", "no <&>"}
	create("linked item", r)
	create("linked item duplicate", r)
	r = base("session", "short-answer", "approach?")
	r.SessionUuid, r.CallerSessionUuid = goldenAsker, goldenAsker
	create("session ok", r)
	for i := range domain.DefaultGuidanceRequestPendingCap + 1 {
		create(fmt.Sprintf("standalone %d", i), base("standalone", "yes-no", fmt.Sprintf("standalone question %d", i)))
	}

	for _, id := range []string{bad, missing} {
		resp, err := svc.GetGuidanceRequest(ctx, connect.NewRequest(&sessionv1.GetGuidanceRequestRequest{Id: id}))
		if err != nil {
			g.rpc("get "+id, nil, err)
		} else {
			g.rpc("get "+id, resp.Msg, nil)
		}
		a, err := svc.AnswerGuidanceRequest(ctx, connect.NewRequest(&sessionv1.AnswerGuidanceRequestRequest{Id: id, Answer: "x"}))
		if err != nil {
			g.rpc("answer "+id, nil, err)
		} else {
			g.rpc("answer "+id, a.Msg, nil)
		}
	}
	for _, scope := range []struct{ scope, key string }{{"session", goldenAsker}, {"backlog-item", item.ID}, {"standalone", ""}, {"planet", ""}} {
		for _, answered := range []bool{false, true} {
			resp, err := svc.ListGuidanceRequests(ctx, connect.NewRequest(&sessionv1.ListGuidanceRequestsRequest{Scope: scope.scope, ScopeKey: scope.key, IncludeAnswered: answered}))
			if err != nil {
				g.rpc("list "+scope.scope, nil, err)
				continue
			}
			sort.Slice(resp.Msg.Requests, func(i, j int) bool { return resp.Msg.Requests[i].QuestionText < resp.Msg.Requests[j].QuestionText })
			g.rpc(fmt.Sprintf("list %s answered=%t", scope.scope, answered), resp.Msg, nil)
		}
	}
	all, err := svc.ListAllPendingGuidanceRequests(ctx, connect.NewRequest(&sessionv1.ListAllPendingGuidanceRequestsRequest{}))
	require.NoError(t, err)
	sort.Slice(all.Msg.Requests, func(i, j int) bool { return all.Msg.Requests[i].QuestionText < all.Msg.Requests[j].QuestionText })
	g.rpc("list all", all.Msg, nil)
}

func (g *guidanceTranscript) answerAndNotify() {
	t, ctx := g.t, context.Background()
	storage := svctest.CreateTestStorage(t)
	bus := events.NewEventBus(64)
	respawner := &goldenRespawner{calls: make(chan string, 8), err: errors.New("respawn failed")}
	svc := goldenGuidanceService(storage, bus, respawner)
	var heard []string
	svc.SetAnswerListener(func(_ context.Context, id string) { heard = append(heard, id) })
	ch, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	answer := func(label, id, text string) {
		resp, err := svc.AnswerGuidanceRequest(ctx, connect.NewRequest(&sessionv1.AnswerGuidanceRequestRequest{Id: id, Answer: text}))
		if err != nil {
			g.rpc("answer "+label, nil, err)
		} else {
			g.rpc("answer "+label, resp.Msg, nil)
		}
		g.events(label, ch)
		g.logf("%s listener=%v", label, heard)
		heard = nil
	}
	long := strings.Repeat("é", 79)
	for _, n := range []int{0, 1, 2} {
		answer(fmt.Sprintf("session truncation +%d", n), goldenAgentRow(t, storage, long+strings.Repeat("ü", n)), strings.Repeat("ß", 79+n))
	}
	first := goldenAgentRow(t, storage, "twice?")
	answer("first", first, "yes")
	answer("second", first, "no")
	answer("empty", goldenAgentRow(t, storage, "empty answer?"), "")
	answer("hook source", goldenHookRow(t, storage, "hook"), "Red")
	answer("peer source by human", goldenPeerRow(t, storage, "peer?"), "on behalf")
	answer("standalone", goldenRow(t, storage, domain.GuidanceRequestParams{
		Scope: domain.RequestScopeStandalone, Source: domain.QuestionSourceAgentMCP, QuestionText: "alone?", QuestionType: domain.QuestionTypeYesNo,
	}), "yes")
	cancelled := goldenAgentRow(t, storage, "cancelled?")
	_, err := storage.CancelGuidanceRequest(ctx, uuid.MustParse(cancelled), "test")
	require.NoError(t, err)
	answer("cancelled", cancelled, "yes")

	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "triage"})
	require.NoError(t, err)
	itemID := uuid.MustParse(item.ID)
	row, err := storage.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
		Scope: domain.RequestScopeBacklogItem, ItemID: &itemID, QuestionText: "mobile too?", QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)
	answer("backlog item", row.ID, "yes")
	select {
	case got := <-respawner.calls:
		g.logf("respawn %s", got)
	case <-time.After(10 * time.Second):
		t.Fatal("respawn not called")
	}
	notes, err := storage.ListActivityNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	for _, n := range notes {
		g.logf("note %q", n.Message)
	}
}

func (g *guidanceTranscript) authorize() {
	t, ctx := g.t, context.Background()
	storage := svctest.CreateTestStorage(t)
	svc := goldenGuidanceService(storage, nil, nil)
	agent := goldenAgentRow(t, storage, "agent?")
	peer := goldenPeerRow(t, storage, "peer one?")
	whos := []struct {
		name string
		who  guidance.Answerer
	}{{"zero", guidance.Answerer{}}, {"empty session", guidance.SessionAnswerer("")}, {"other", guidance.SessionAnswerer(goldenOther)},
		{"asker", guidance.SessionAnswerer(goldenAsker)}, {"peer", guidance.SessionAnswerer(goldenPeer)}}
	for _, id := range []string{agent, peer, "bad", "7c1e0000-0000-4000-8000-00000000ffff"} {
		for _, w := range whos {
			data, applied, err := svc.AnswerAs(ctx, id, "by "+w.name, w.who)
			g.logf("answerAs %s %s: applied=%t data=%t forbidden=%t %s", id, w.name, applied, data != nil,
				errors.Is(err, domain.ErrAnswererForbidden), g.errText(err))
		}
	}
	g.row("peer after", storage, peer)
	_, applied, err := svc.AnswerAsSession(ctx, goldenPeerRow(t, storage, "peer two?"), "yes", goldenPeer)
	g.logf("answerAsSession applied=%t %s", applied, g.errText(err))
	human := goldenPeerRow(t, storage, "peer three?")
	data, applied, err := svc.AnswerAs(ctx, human, "stand in", guidance.HumanAnswerer())
	g.logf("human on peer applied=%t answer=%q %s", applied, data.Answer, g.errText(err))
	g.row("human on peer", storage, human)
}

func (g *guidanceTranscript) savedAndDerived() {
	t, ctx := g.t, context.Background()
	storage := svctest.CreateTestStorage(t)
	svc := goldenGuidanceService(storage, nil, nil)
	var ended, archived, unconfirmed bool
	n := 0
	mk := map[string]func(*testing.T, *session.Storage, string) string{"agent": goldenAgentRow, "peer": goldenPeerRow, "hook": goldenHookRow}
	for _, probes := range []string{"none", "empty", "set"} {
		switch probes {
		case "empty":
			svc.SetDeliveryProbes(guidance.DeliveryProbes{})
		case "set":
			svc.SetDeliveryProbes(guidance.DeliveryProbes{
				Unconfirmed:     func(string) bool { return unconfirmed },
				SessionEnded:    func(id string) bool { return ended && id == goldenAsker },
				SessionArchived: func(string) bool { return archived },
			})
		}
		for _, src := range []string{"agent", "peer", "hook"} {
			for _, life := range []struct{ ended, archived bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
				n++
				ended, archived = life.ended, life.archived
				id := mk[src](t, storage, fmt.Sprintf("%s %s %d", src, probes, n))
				_, applied, err := svc.AnswerAs(ctx, id, "a", guidance.HumanAnswerer())
				g.logf("saved probes=%s src=%s ended=%t archived=%t applied=%t %s", probes, src, ended, archived, applied, g.errText(err))
				g.row("saved", storage, id)
				_, err = storage.CancelGuidanceRequest(ctx, uuid.MustParse(id), "cleanup")
				require.NoError(t, err)
			}
		}
	}

	ended, archived = false, false
	for _, src := range []string{"agent", "peer", "hook"} {
		for _, state := range []domain.DeliveryState{"", domain.DeliveryStatePending, domain.DeliveryStateSent, domain.DeliveryStateFailed, domain.DeliveryStateAcknowledged} {
			n++
			id := mk[src](t, storage, fmt.Sprintf("derived %s %s %d", src, state, n))
			for _, step := range []domain.DeliveryState{domain.DeliveryStatePending, domain.DeliveryStateSent, state} {
				if state == "" || (state == domain.DeliveryStatePending && step != domain.DeliveryStatePending) {
					break
				}
				applied, err := storage.SetGuidanceDelivery(ctx, uuid.MustParse(id), domain.DeliveryRouteLiveKeystroke, step, 1)
				g.logf("set %s applied=%t %s", step, applied, g.errText(err))
				if step == state {
					break
				}
			}
			for _, p := range []struct{ ended, archived, unconfirmed bool }{{false, false, false}, {true, false, true}, {false, true, false}, {true, true, true}} {
				ended, archived, unconfirmed = p.ended, p.archived, p.unconfirmed
				resp, err := svc.GetGuidanceRequest(ctx, connect.NewRequest(&sessionv1.GetGuidanceRequestRequest{Id: id}))
				require.NoError(t, err)
				r := resp.Msg.GetRequest()
				g.logf("derived src=%s state=%q probes=%+v => unack=%t ended=%t archived=%t", src, state, p, r.Unacknowledged, r.SessionEnded, r.SessionArchived)
			}
			ended, archived, unconfirmed = false, false, false
			_, err := storage.CancelGuidanceRequest(ctx, uuid.MustParse(id), "cleanup")
			require.NoError(t, err)
			resp, err := svc.GetGuidanceRequest(ctx, connect.NewRequest(&sessionv1.GetGuidanceRequestRequest{Id: id}))
			require.NoError(t, err)
			g.rpc("cancelled", resp.Msg, nil)
		}
	}
	saved, err := svc.ListSavedGuidanceAnswers(ctx, connect.NewRequest(&sessionv1.ListSavedGuidanceAnswersRequest{}))
	require.NoError(t, err)
	sort.Slice(saved.Msg.Requests, func(i, j int) bool { return saved.Msg.Requests[i].QuestionText < saved.Msg.Requests[j].QuestionText })
	g.rpc("list saved", saved.Msg, nil)
}

func (g *guidanceTranscript) waiverAndPeerCancel() {
	t, ctx := g.t, context.Background()
	storage := svctest.CreateTestStorage(t)
	svc := goldenGuidanceService(storage, nil, nil)
	var resumed, heard []string
	svc.SetWaiverResumer(func(_ context.Context, id string) { resumed = append(resumed, id) })
	svc.SetAnswerListener(func(_ context.Context, id string) { heard = append(heard, id) })
	raw, err := domain.SerializeAcCriteria([]domain.AcCriterion{{Index: 0, Text: "a", Status: domain.AcStatusPending}, {Index: 1, Text: "b", Status: domain.AcStatusPending}})
	require.NoError(t, err)
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "w", Status: string(domain.BacklogStatusInProgress), AcceptanceCriteria: raw})
	require.NoError(t, err)
	itemID := uuid.MustParse(item.ID)
	for i, c := range []struct {
		idx    int
		answer string
	}{{1, "Keep"}, {1, domain.OwnerActionWaiveResponse}, {1, domain.OwnerActionWaiveResponse}, {9, domain.OwnerActionWaiveResponse}} {
		row, err := storage.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
			Scope: domain.RequestScopeBacklogItem, ItemID: &itemID, QuestionType: domain.QuestionTypeMultipleChoice,
			QuestionText: domain.OwnerActionQuestionPrefix + domain.FormatOwnerActionWaiver(c.idx) + fmt.Sprintf("AC %d", i), Options: `["Waive criterion","Keep"]`,
		})
		require.NoError(t, err)
		_, applied, err := svc.AnswerAs(ctx, row.ID, c.answer, guidance.HumanAnswerer())
		got, gerr := storage.GetBacklogItem(ctx, item.ID)
		require.NoError(t, gerr)
		cs, gerr := domain.ParseAcCriteria(got.AcceptanceCriteria)
		require.NoError(t, gerr)
		g.logf("waive idx=%d answer=%q applied=%t %s ac=%s,%s resumed=%v", c.idx, c.answer, applied, g.errText(err), cs[0].Status, cs[1].Status, resumed)
		notes, gerr := storage.ListActivityNotesForItem(ctx, item.ID)
		require.NoError(t, gerr)
		lines := make([]string, 0, len(notes))
		for _, n := range notes {
			lines = append(lines, n.Message)
		}
		sort.Strings(lines)
		g.logf("waive notes=%q", lines)
		resumed = nil
	}

	heard = nil
	open := goldenPeerRow(t, storage, "open peer?")
	answered := goldenPeerRow(t, storage, "answered peer?")
	_, _, err = svc.AnswerAs(ctx, answered, "done", guidance.SessionAnswerer(goldenPeer))
	require.NoError(t, err)
	heard = nil
	for _, id := range []string{"bad", "7c1e0000-0000-4000-8000-00000000ffff", goldenAgentRow(t, storage, "not peer?"), goldenHookRow(t, storage, "nothook"), open, open, answered} {
		data, applied, err := svc.CancelPeerHelp(ctx, id)
		status := ""
		if data != nil {
			status = string(data.Status()) + " reason=" + data.CancelReason.String()
		}
		g.logf("cancelPeer %s applied=%t notPeer=%t status=%s heard=%v %s", id, applied, errors.Is(err, guidance.ErrNotPeerHelp), status, heard, g.errText(err))
		heard = nil
	}
}

type goldenEnds struct{ ended map[string]bool }

func (e goldenEnds) ResolveUUID(id string) string {
	switch id {
	case "asker-title":
		return goldenAsker
	case "unknown":
		return ""
	}
	return id
}
func (e goldenEnds) Ended(id string) bool { return e.ended[id] }

func (g *guidanceTranscript) reaper() {
	t, ctx := g.t, context.Background()
	run := func(label string, ended map[string]bool, act func(*guidance.GuidanceSessionReaper)) {
		storage := svctest.CreateTestStorage(t)
		counts := &mirrorCounts{}
		mirror := newRealStoreMirror(storage, questionkit.NewPendingQuestionStore(nil), counts)
		ids := []string{goldenHookRow(t, storage, "r-hook "+label), goldenPeerRow(t, storage, "r-peer "+label), goldenAgentRow(t, storage, "r-agent "+label)}
		reaper := guidance.NewGuidanceSessionReaper(storage, goldenEnds{ended: ended}, mirror, counts.onCleared)
		act(reaper)
		for i, id := range ids {
			g.row(fmt.Sprintf("reaper %s %d", label, i), storage, id)
		}
		g.logf("reaper %s cleared=%v", label, counts.cleared)
	}
	for _, id := range []string{goldenAsker, goldenPeer, goldenOther, "asker-title", "unknown"} {
		run("deleted "+id, nil, func(r *guidance.GuidanceSessionReaper) { r.HandleEvent(ctx, pkgevents.NewSessionDeletedEvent(id)) })
		run("archived "+id, nil, func(r *guidance.GuidanceSessionReaper) { r.HandleEvent(ctx, pkgevents.NewSessionArchivedEvent(id)) })
	}
	run("other event", nil, func(r *guidance.GuidanceSessionReaper) {
		r.HandleEvent(ctx, pkgevents.NewUserInteractionEvent(goldenAsker, "x", "y"))
		r.HandleEvent(ctx, nil)
	})
	for _, ended := range []string{"", goldenAsker, goldenPeer, goldenOther} {
		run("startup "+ended, map[string]bool{ended: ended != ""}, func(r *guidance.GuidanceSessionReaper) { r.StartupPass(ctx) })
	}
}

type goldenAckStore struct {
	mu      sync.Mutex
	lines   []string
	rows    []*domain.GuidanceRequestData
	listErr error
	applied bool
	setErr  error
}

func (s *goldenAckStore) SetGuidanceDelivery(_ context.Context, id uuid.UUID, route domain.DeliveryRoute, state domain.DeliveryState, add int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, fmt.Sprintf("set %s %s %s add=%d", id, route, state, add))
	return s.applied, s.setErr
}

func (s *goldenAckStore) ListGuidanceByDelivery(_ context.Context, route domain.DeliveryRoute, state domain.DeliveryState) ([]*domain.GuidanceRequestData, error) {
	s.lines = append(s.lines, fmt.Sprintf("list %q %q", route, state))
	return s.rows, s.listErr
}

type goldenCards struct{ store *goldenAckStore }

func (c goldenCards) ResolveQuestionNotifications(id string) {
	c.store.lines = append(c.store.lines, "resolve "+id)
}
func (c goldenCards) ReopenQuestionNotifications(id string) {
	c.store.lines = append(c.store.lines, "reopen "+id)
}

func (g *guidanceTranscript) ackDetector() {
	t, ctx := g.t, context.Background()
	idleBytes, err := os.ReadFile("../../session/testdata/dialog_presence/idle_composer.txt")
	require.NoError(t, err)
	up := strings.ReplaceAll(replyDialogFixture(t, "question_single_select_3opts.txt"), "2. Green", "2. "+replyColorLabel)
	frames := map[string]string{"own": up, "other": replyDialogFixture(t, "question_wrapped_2opts.txt"), "idle": string(idleBytes), "junk": "build output\n", "err": ""}
	const rowID = "7c1e0000-0000-4000-8000-00000000b001"

	scripts := []string{
		"t3:idle t3:idle", "t3:idle t3:own t3:idle t3:idle", "t3:junk t3:junk t3:err", "t1:idle t1:idle t1:idle",
		"t61:own", "t61:other", "t61:idle", "t61:junk", "t61:err", "t61:err t3:idle t3:idle", "t61:junk receipt",
		"receipt-other receipt-session receipt", "end t3:own", "end t61:own", "t3:err t3:idle t3:err t3:idle",
		"t59:own t1:own t3:own", "t61:junk t61:junk", "t61:junk end t3:own",
	}
	for _, script := range scripts {
		for _, mode := range []string{"ok", "indeterminate", "refused", "storeerr", "badrow", "nonotes"} {
			store := &goldenAckStore{applied: mode != "refused"}
			if mode == "storeerr" {
				store.setErr = errors.New("db down")
			}
			clock := newManualClock()
			capture, ended, leased := up, false, []bool{}
			var capErr error
			det := guidance.NewGuidanceAckDetector(store, func(_ context.Context, _ string, underLease bool) (string, error) {
				leased = append(leased, underLease)
				return capture, capErr
			}, func(string) bool { return ended }, clock.Now)
			if mode != "nonotes" {
				det.SetNotes(goldenCards{store})
			}
			id := rowID
			if mode == "badrow" {
				id = "not-a-uuid"
			}
			det.Sent(questionkit.SentDelivery{RowID: id, SessionUUID: "s1", Dialog: colorDialog, Route: domain.DeliveryRouteLiveKeystroke, Indeterminate: mode == "indeterminate"})
			g.logf("ack %q %s start unconfirmed=%t", script, mode, det.Unconfirmed(id))
			for _, step := range strings.Fields(script) {
				switch step {
				case "receipt":
					det.Receipt(ctx, "s1", goldenAckTool)
				case "receipt-other":
					det.Receipt(ctx, "s1", "Bash")
				case "receipt-session":
					det.Receipt(ctx, "s2", goldenAckTool)
				case "end":
					ended = true
				default:
					var secs int
					var frame string
					_, err := fmt.Sscanf(strings.Replace(step, ":", " ", 1), "t%d %s", &secs, &frame)
					require.NoError(t, err)
					capture, capErr = frames[frame], nil
					if frame == "err" {
						capErr = errors.New("tmux gone")
					}
					clock.Advance(time.Duration(secs) * time.Second)
					det.Tick(ctx)
				}
				g.logf("  %s unconfirmed=%t leased=%v effects=%v", step, det.Unconfirmed(id), leased, store.lines)
				store.lines, leased = nil, []bool{}
			}
		}
	}

	for _, listErr := range []error{nil, errors.New("list failed")} {
		store := &goldenAckStore{applied: true, listErr: listErr, rows: []*domain.GuidanceRequestData{
			{ID: rowID, AskerSessionUUID: "s1", DeliveryRoute: domain.DeliveryRouteLiveKeystroke, DeliveryState: domain.DeliveryStateSent},
		}}
		det := guidance.NewGuidanceAckDetector(store, func(context.Context, string, bool) (string, error) { return up, nil }, nil, nil)
		det.Restore(ctx)
		det.Receipt(ctx, "s1", goldenAckTool)
		g.logf("restore err=%v effects=%v", listErr, store.lines)
	}
}

func (g *guidanceTranscript) seedRoute() {
	t := g.t
	storage := svctest.CreateTestStorage(t)
	poller := session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	poller.SetInstances([]*instance.Instance{{UUID: goldenAsker, Title: "seed-me", Path: t.TempDir(), Status: domain.Active, Program: "claude"}})
	svc := &SessionService{ReviewQueuePoller: poller}
	mux := http.NewServeMux()
	goldenSeedHandler(svc, storage).RegisterRoutes(mux)
	do := func(label, method, path, body string) {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		g.logf("seed %s %s %s => %d %q %q", label, method, path, w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	const path = "/api/debug/guidance/seed-question"
	one := `{"sessionId":"seed-me","questions":[{"text":"Pick?","header":"H","options":["Red","Blue"]}],"hookCallId":"toolu_s"}`
	do("get", http.MethodGet, path, "")
	do("put", http.MethodPut, path, one)
	do("neighbour", http.MethodPost, path+"/x", one)
	do("malformed", http.MethodPost, path, "{")
	do("unknown session", http.MethodPost, path, `{"sessionId":"nobody","questions":[]}`)
	do("no questions", http.MethodPost, path, `{"sessionId":"seed-me","questions":[]}`)
	do("no mirror", http.MethodPost, path, one)
	pending := questionkit.NewPendingQuestionStore(nil)
	mirror := newRealStoreMirror(storage, pending, &mirrorCounts{})
	svc.guidanceMirror.Store(mirror)
	do("hook", http.MethodPost, path, one)
	do("hook by uuid again", http.MethodPost, path, strings.Replace(one, `"seed-me"`, `"`+goldenAsker+`"`, 1))
	do("agent", http.MethodPost, path, `{"sessionId":"seed-me","source":"agent_mcp","questions":[{"text":"Agent?","header":"A","multiSelect":true,"options":["x"]}]}`)
	do("unknown source", http.MethodPost, path, `{"sessionId":"seed-me","source":"peer","hookCallId":"toolu_p","questions":[{"text":"Peerish?","header":"P","options":["x","y"]}]}`)
	rows, err := storage.ListAllPendingGuidanceRequests(context.Background())
	require.NoError(t, err)
	sort.Slice(rows, func(i, j int) bool { return rows[i].QuestionText < rows[j].QuestionText })
	for _, r := range rows {
		_, registered := pending.Peek(questionkit.QuestionKey{SessionUUID: goldenAsker, QuestionID: r.ID})
		g.logf("seed row %s scope=%s src=%s session=%s asker=%s text=%q type=%s options=%q multi=%t route=%q call=%q questions=%q registered=%t",
			r.ID, r.Scope, r.Source, r.SessionUUID, r.AskerSessionUUID, r.QuestionText, r.QuestionType, r.Options, r.MultiSelect, r.DeliveryRoute, r.HookCallID, r.QuestionsJSON, registered)
	}
}

func TestGuidanceGolden_should_MatchTheRecordedTranscript(t *testing.T) {
	t.Parallel()
	g := &guidanceTranscript{t: t, ids: map[string]string{}}
	for _, s := range []struct {
		name string
		run  func()
	}{
		{"create and read", g.createAndRead}, {"answer and notify", g.answerAndNotify}, {"authorize", g.authorize},
		{"saved and derived", g.savedAndDerived}, {"waiver and peer cancel", g.waiverAndPeerCancel}, {"reaper", g.reaper},
		{"ack detector", g.ackDetector}, {"seed route", g.seedRoute},
	} {
		g.logf("=== %s", s.name)
		s.run()
	}
	got := g.b.String()
	if os.Getenv("GUIDANCE_GOLDEN_UPDATE") != "" {
		require.NoError(t, os.WriteFile(guidanceGoldenFile, []byte(got), 0o600))
	}
	want, err := os.ReadFile(guidanceGoldenFile)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
}

// The races: every transition is one conditional update, so exactly one caller wins.

func TestGuidanceRace_should_ApplyExactlyOneAnswer_When_ManyAnswerAtOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	storage := svctest.CreateTestStorage(t)
	bus := events.NewEventBus(64)
	svc := goldenGuidanceService(storage, bus, nil)
	var mu sync.Mutex
	heard := 0
	svc.SetAnswerListener(func(context.Context, string) { mu.Lock(); heard++; mu.Unlock() })
	ch, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)
	id := goldenAgentRow(t, storage, "who wins?")

	var wg sync.WaitGroup
	winners := map[string]bool{}
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text := fmt.Sprintf("answer %d", i)
			_, applied, err := svc.AnswerAs(ctx, id, text, guidance.HumanAnswerer())
			assert.NoError(t, err)
			if applied {
				mu.Lock()
				winners[text] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	require.Len(t, winners, 1)
	assert.Equal(t, 1, heard)
	assert.Len(t, ch, 1, "one notification")
	row, err := storage.GetGuidanceRequest(ctx, uuid.MustParse(id))
	require.NoError(t, err)
	assert.True(t, winners[row.Answer], "the stored answer is the winner's")
}

func TestGuidanceRace_should_LetOneSideWin_When_AnswerCancelAndReapCollide(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	storage := svctest.CreateTestStorage(t)
	svc := goldenGuidanceService(storage, nil, nil)
	var mu sync.Mutex
	heard := map[string]int{}
	svc.SetAnswerListener(func(_ context.Context, id string) { mu.Lock(); heard[id]++; mu.Unlock() })

	for i := range 12 {
		id := goldenPeerRow(t, storage, fmt.Sprintf("race %d?", i))
		var cleared []string
		reaper := guidance.NewGuidanceSessionReaper(storage, goldenEnds{}, nil, func(r string) { mu.Lock(); cleared = append(cleared, r); mu.Unlock() })
		var wg sync.WaitGroup
		var answered, cancelled bool
		wg.Add(3)
		go func() {
			defer wg.Done()
			_, applied, err := svc.AnswerAs(ctx, id, "a", guidance.SessionAnswerer(goldenPeer))
			assert.NoError(t, err)
			answered = applied
		}()
		go func() {
			defer wg.Done()
			_, applied, err := svc.CancelPeerHelp(ctx, id)
			assert.NoError(t, err)
			cancelled = applied
		}()
		go func() {
			defer wg.Done()
			reaper.HandleEvent(ctx, pkgevents.NewSessionArchivedEvent(goldenAsker))
		}()
		wg.Wait()

		wins := len(cleared)
		for _, won := range []bool{answered, cancelled} {
			if won {
				wins++
			}
		}
		require.Equal(t, 1, wins, "round %d: answered=%t cancelled=%t reaped=%v", i, answered, cancelled, cleared)
		row, err := storage.GetGuidanceRequest(ctx, uuid.MustParse(id))
		require.NoError(t, err)
		assert.Equal(t, answered, row.AnsweredAt != nil)
		mu.Lock()
		assert.Equal(t, len(cleared) == 0, heard[id] == 1, "the listener hears an answer or an operator cancel, not a reap")
		mu.Unlock()
	}
}

func TestGuidanceRace_should_NotFinishTwice_When_AReceiptArrivesDuringASample(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := &goldenAckStore{applied: true}
	clock := newManualClock()
	idle, err := os.ReadFile("../../session/testdata/dialog_presence/idle_composer.txt")
	require.NoError(t, err)
	var det *guidance.GuidanceAckDetector
	captures := 0
	det = guidance.NewGuidanceAckDetector(store, func(context.Context, string, bool) (string, error) {
		// The hook lands during the capture that would complete the clean streak.
		if captures++; captures == 2 {
			det.Receipt(ctx, "s1", "AskUserQuestion")
			assert.Len(t, store.lines, 1, "the receipt records its state at once")
		}
		return string(idle), nil
	}, nil, clock.Now)
	det.Sent(questionkit.SentDelivery{RowID: "7c1e0000-0000-4000-8000-00000000b002", SessionUUID: "s1", Dialog: colorDialog, Route: domain.DeliveryRouteLiveKeystroke})

	for range 2 {
		clock.Advance(3 * time.Second)
		det.Tick(ctx)
	}

	assert.Equal(t, 2, captures)
	assert.Len(t, store.lines, 1, "the stale sample does not finish the row again")
}

// Pinned as found: the 60 s decision does not re-check tracking after its capture, so a
// receipt that lands meanwhile is followed by the decision's own write; the store's
// transition rules have the last word.
func TestGuidanceRace_should_StillWriteTheDecision_When_AReceiptArrivesDuringTheDecidingCapture(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := &goldenAckStore{applied: true}
	clock := newManualClock()
	own := strings.ReplaceAll(replyDialogFixture(t, "question_single_select_3opts.txt"), "2. Green", "2. "+replyColorLabel)
	var det *guidance.GuidanceAckDetector
	det = guidance.NewGuidanceAckDetector(store, func(context.Context, string, bool) (string, error) {
		det.Receipt(ctx, "s1", goldenAckTool)
		return own, nil
	}, nil, clock.Now)
	det.Sent(questionkit.SentDelivery{RowID: "7c1e0000-0000-4000-8000-00000000b003", SessionUUID: "s1", Dialog: colorDialog, Route: domain.DeliveryRouteLiveKeystroke})

	clock.Advance(61 * time.Second)
	det.Tick(ctx)

	require.Len(t, store.lines, 2)
	assert.Contains(t, store.lines[0], string(domain.DeliveryStateAcknowledged))
	assert.Contains(t, store.lines[1], string(domain.DeliveryStateFailed))
}

func TestGuidanceRace_should_BeRaceFree_When_SentReceiptAndTickRunTogether(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := &goldenAckStore{applied: true}
	clock := newManualClock()
	det := guidance.NewGuidanceAckDetector(store, func(context.Context, string, bool) (string, error) { return "junk", nil }, nil, clock.Now)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			det.Sent(questionkit.SentDelivery{RowID: uuid.NewString(), SessionUUID: "s1", Dialog: colorDialog})
		}()
		go func() { defer wg.Done(); det.Receipt(ctx, "s1", goldenAckTool) }()
		go func() { defer wg.Done(); clock.Advance(3 * time.Second); det.Tick(ctx) }()
	}
	wg.Wait()
	det.Receipt(ctx, "s1", goldenAckTool)
	clock.Advance(61 * time.Second)
	det.Tick(ctx)
	// Two receipts may both write one row (the store's transition rules dedupe), so count rows, not writes.
	rows := map[string]bool{}
	for _, line := range store.lines {
		assert.Contains(t, line, string(domain.DeliveryStateAcknowledged))
		rows[strings.Fields(line)[1]] = true
	}
	assert.Len(t, rows, 4, "every sent row is acknowledged")
}
