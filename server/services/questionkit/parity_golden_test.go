package questionkit

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/panereply"
)

// The golden is a transcript of every decision the moved code makes over a matrix
// of inputs (parse, claim, flight, delivery, mirror, sampler, canary, tracker).
// testdata/parity.golden.gz was recorded by running this same scenario file against
// the pre-move code in package services (through a name shim); a diff means a
// behaviour change. TestParityGolden_Regenerate rewrites it when QUESTIONKIT_RECORD=1.

const parityGoldenPath = "testdata/parity.golden.gz"

type transcript struct{ b strings.Builder }

func (t *transcript) w(format string, args ...any) { fmt.Fprintf(&t.b, format+"\n", args...) }

func (t *transcript) section(name string) { t.w("\n## %s", name) }

var parityNow = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

type parityClock struct {
	mu sync.Mutex
	t  time.Time
}

func newParityClock() *parityClock { return &parityClock{t: parityNow} }

func (c *parityClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *parityClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func parityText(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(sessionTestdataRoot, "reply_dialogs", name))
	require.NoError(t, err)
	return string(b)
}

func parityQuestions(qs ...domain.Question) domain.QuestionPayload {
	p, err := domain.NewQuestionPayload(qs)
	if err != nil {
		return domain.QuestionPayload{}
	}
	return p
}

func parityPayloadJSON(t *testing.T, qs ...domain.Question) string {
	t.Helper()
	s, err := parityQuestions(qs...).JSON()
	require.NoError(t, err)
	return s
}

func opts(labels ...string) []domain.Option {
	out := make([]domain.Option, len(labels))
	for i, l := range labels {
		out[i] = domain.Option{Label: l, Description: "d-" + l}
	}
	return out
}

var paritySeq int

// detUUID is a deterministic id so the transcript is stable across runs.
func detUUID() uuid.UUID {
	paritySeq++
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", paritySeq))
}

func recordParity(t *testing.T) string {
	t.Helper()
	paritySeq = 0
	tr := &transcript{}
	paritySanitize(tr)
	parityParse(t, tr)
	parityStore(tr)
	parityFlights(tr)
	parityDelivery(t, tr)
	parityRows(t, tr)
	parityJudge(t, tr)
	parityMirror(t, tr)
	paritySampler(t, tr)
	parityCanary(tr)
	parityMisc(t, tr)
	return tr.b.String()
}

func paritySanitize(tr *transcript) {
	tr.section("sanitize")
	for _, s := range []string{"", "plain", "a\nb", "a\r\nb", "tab\there", "\x00\x01nul", "esc\x1b[31mred", "uni code", "del\x7f", "emoji 🎉 ok", "  pad  ", "us\x1fsep", "sp\x20ace", "\x1e\x1f\x20"} {
		tr.w("%q -> %q", s, sanitizeText(s))
	}
}

func parityParse(t *testing.T, tr *transcript) {
	tr.section("parse")
	inputs := map[string]map[string]interface{}{
		"nil":          nil,
		"empty":        {},
		"noquestions":  {"questions": []interface{}{}},
		"badtype":      {"questions": "x"},
		"baditem":      {"questions": []interface{}{"x"}},
		"nomulti":      {"questions": []interface{}{map[string]interface{}{"question": "Q?", "header": "H", "options": []interface{}{map[string]interface{}{"label": "A"}, map[string]interface{}{"label": "B"}}}}},
		"single":       {"questions": []interface{}{map[string]interface{}{"question": "Q?", "header": "H", "multiSelect": false, "options": []interface{}{map[string]interface{}{"label": "A"}, map[string]interface{}{"label": "B"}}}}},
		"noheader":     {"questions": []interface{}{map[string]interface{}{"question": "Q?", "multiSelect": false, "options": []interface{}{map[string]interface{}{"label": "A"}}}}},
		"multiselect":  {"questions": []interface{}{map[string]interface{}{"question": "Q?", "header": "H", "multiSelect": true, "options": []interface{}{map[string]interface{}{"label": "A"}}}}},
		"nl-injection": {"questions": []interface{}{map[string]interface{}{"question": "Q\n?", "header": "H\r", "multiSelect": false, "options": []interface{}{map[string]interface{}{"label": "A\nB", "description": "x\ny"}}}}},
		"dup-labels":   {"questions": []interface{}{map[string]interface{}{"question": "Q?", "header": "H", "multiSelect": false, "options": []interface{}{map[string]interface{}{"label": "A"}, map[string]interface{}{"label": "A"}}}}},
		"nolabels":     {"questions": []interface{}{map[string]interface{}{"question": "Q?", "header": "H", "multiSelect": false, "options": []interface{}{}}}},
		"longlabel":    {"questions": []interface{}{map[string]interface{}{"question": "Q?", "header": "H", "multiSelect": false, "options": []interface{}{map[string]interface{}{"label": strings.Repeat("x", 120)}}}}},
		"eightopts":    {"questions": []interface{}{map[string]interface{}{"question": "Q?", "header": "H", "multiSelect": false, "options": []interface{}{map[string]interface{}{"label": "1"}, map[string]interface{}{"label": "2"}, map[string]interface{}{"label": "3"}, map[string]interface{}{"label": "4"}, map[string]interface{}{"label": "5"}, map[string]interface{}{"label": "6"}, map[string]interface{}{"label": "7"}, map[string]interface{}{"label": "8"}}}}},
		"twoq": {"questions": []interface{}{
			map[string]interface{}{"question": "One?", "header": "One", "multiSelect": false, "options": []interface{}{map[string]interface{}{"label": "A"}, map[string]interface{}{"label": "B"}}},
			map[string]interface{}{"question": "Two?", "header": "Two", "multiSelect": true, "options": []interface{}{map[string]interface{}{"label": "X"}, map[string]interface{}{"label": "Y"}}},
		}},
		"twoq-nomulti": {"questions": []interface{}{
			map[string]interface{}{"question": "One?", "header": "One", "options": []interface{}{map[string]interface{}{"label": "A"}}},
			map[string]interface{}{"question": "Two?", "header": "Two", "options": []interface{}{map[string]interface{}{"label": "X"}}},
		}},
		"threeq": {"questions": []interface{}{
			map[string]interface{}{"question": "1?", "header": "1", "multiSelect": false, "options": []interface{}{map[string]interface{}{"label": "A"}}},
			map[string]interface{}{"question": "2?", "header": "2", "multiSelect": false, "options": []interface{}{map[string]interface{}{"label": "A"}}},
			map[string]interface{}{"question": "3?", "header": "3", "multiSelect": false, "options": []interface{}{map[string]interface{}{"label": "A"}}},
		}},
	}
	names := make([]string, 0, len(inputs))
	for n := range inputs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		a := ParseAskUserQuestion(inputs[n])
		pj, _ := a.Payload.JSON()
		tr.w("%s: shape=%s q=%q h=%q labels=%q replyable=%v tabbed=%v zero=%v payload=%s", n, a.Shape, a.Question, a.Header, a.Labels, a.Replyable, a.Tabbed, a.Payload.IsZero(), pj)
	}
	entries, err := os.ReadDir(filepath.Join(questionTestdata, "askuserquestion"))
	require.NoError(t, err)
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(questionTestdata, "askuserquestion", e.Name()))
		require.NoError(t, err)
		tr.w("fixture %s sha=%x", e.Name(), sha256.Sum256(b))
	}
}

func parityStore(tr *transcript) {
	tr.section("store")
	clk := newParityClock()
	s := NewPendingQuestionStore(clk.Now)
	dialog := func(q string, labels ...string) panereply.QuestionDialog {
		return panereply.QuestionDialog{Question: q, Header: "H", Labels: labels}
	}
	reg := func(sess, id, q string) {
		tr.w("register %s/%s -> pending=%d", sess, id, s.Register(PendingQuestion{QuestionID: id, SessionUUID: sess, Dialog: dialog(q, "A", "B"), Token: QuestionToken(q)}))
	}
	reg("s1", "q1", "first?")
	reg("s1", "q1", "first?") // duplicate id, same dialog
	reg("s1", "q2", "second?")
	reg("s2", "q3", "other?")
	tr.w("token %q %q", QuestionToken("  Hello\nWorld  "), QuestionToken(strings.Repeat("é", 60)))
	for _, n := range []int{39, 40, 41} {
		tr.w("token len %d -> %d", n, len(QuestionToken(strings.Repeat("a", n))))
	}
	tr.w("len=%d count s1=%d s2=%d s3=%d", s.Len(), s.PendingCount("s1"), s.PendingCount("s2"), s.PendingCount("s3"))
	for _, k := range []QuestionKey{{"s1", "q1"}, {"s2", "q1"}, {"s1", "nope"}} {
		q, ok := s.Peek(k)
		tr.w("peek %v -> %q %v", k, q.Dialog.Question, ok)
	}
	tr.w("peekq %d", len(s.PeekQuestions([]QuestionKey{{"s1", "q1"}, {"s1", "q2"}, {"s9", "q2"}})))
	c1, r1 := s.Claim(QuestionKey{"s1", "q1"})
	tr.w("claim q1 -> %v valid=%v deleted=%v sess=%s question=%q", r1, c1.Valid(), c1.Deleted(), c1.SessionUUID(), c1.Question().Dialog.Question)
	s.Forget(QuestionKey{"s2", "q3"})
	s.Forget(QuestionKey{"s2", "missing"})
	tr.w("forget -> len=%d s2=%d", s.Len(), s.PendingCount("s2"))
	id1, id2 := NewQuestionID(), NewQuestionID()
	tr.w("question id len=%d distinct=%v", len(id1), id1 != id2)
	_, r1b := s.Claim(QuestionKey{"s1", "q1"})
	tr.w("claim q1 again -> %v", r1b)
	s.Release(c1)
	_, r1c := s.Claim(QuestionKey{"s1", "q1"})
	tr.w("claim after release -> %v", r1c)
	c2, _ := s.Claim(QuestionKey{"s1", "q2"})
	s.SessionDeleted("s1")
	tr.w("claim deleted=%v after SessionDeleted len=%d", c2.Deleted(), s.Len())
	s.Consume(c2)
	tr.w("after consume len=%d", s.Len())
	var zero PendingQuestionClaim
	tr.w("zero claim valid=%v deleted=%v", zero.Valid(), zero.Deleted())
	s.Release(zero)
	s.Consume(zero)
	// per-session cap and parallel window
	for i := 0; i < 10; i++ {
		reg("cap", fmt.Sprintf("c%d", i), fmt.Sprintf("cap %d?", i))
	}
	tr.w("cap count=%d len=%d", s.PendingCount("cap"), s.Len())
	s.NoteUnreplyable("par")
	reg("par", "p1", "par?")
	clk.Advance(30 * time.Second)
	reg("par", "p2", "par2?")
	clk.Advance(time.Second)
	reg("par", "p3", "par3?")
	s.NoteUnreplyable("edge")
	clk.Advance(30*time.Second + time.Nanosecond)
	reg("edge", "e1", "edge?")
	tr.w("edge count=%d", s.PendingCount("edge"))
	reg("tt", "t1", "ttl?")
	clk.Advance(30 * time.Minute)
	_, tr1 := s.Claim(QuestionKey{"tt", "t1"})
	tr.w("claim at exactly the ttl -> %v", tr1)
	reg("tt", "t2", "ttl2?")
	clk.Advance(30*time.Minute + time.Nanosecond)
	_, tr2 := s.Claim(QuestionKey{"tt", "t2"})
	tr.w("claim just past the ttl -> %v", tr2)
	var vz PendingQuestionClaim
	tr.w("valid zero=%v", vz.Valid())
	tr.w("valid token only=%v", claimWithTokenOnly().Valid())
	tr.w("valid store only=%v", claimWithStoreOnly(s).Valid())
	clk.Advance(31 * time.Minute)
	tr.w("after ttl len=%d count cap=%d", s.Len(), s.PendingCount("cap"))
	_, rr := s.Claim(QuestionKey{"cap", "c9"})
	tr.w("claim expired -> %v", rr)
	for i := 0; i < 300; i++ {
		s.Register(PendingQuestion{QuestionID: fmt.Sprintf("g%d", i), SessionUUID: fmt.Sprintf("gs%d", i%40), Dialog: dialog(fmt.Sprintf("g%d?", i), "A"), CreatedAt: clk.Now()})
		clk.Advance(time.Second)
	}
	tr.w("global cap len=%d", s.Len())
}

func parityFlights(tr *transcript) {
	tr.section("flights")
	clk := newParityClock()
	tab := newFlightTable(clk.Now)
	h1 := hashPayload("s", "q", '1')
	h2 := hashPayload("s", "q", '2')
	tr.w("hash eq=%v diff=%v", h1 == hashPayload("s", "q", '1'), h1 == h2)
	f1, r := flightBegin(tab, "r1", h1)
	tr.w("begin r1 -> %s", r)
	_, r = flightBegin(tab, "r1", h1)
	tr.w("begin r1 again -> %s", r)
	_, r = flightBegin(tab, "r1", h2)
	tr.w("begin r1 other payload -> %s", r)
	flightFinish(tab, "r1", f1, replyResult(sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT), true)
	res, err := flightWait(f1, context.Background())
	tr.w("wait -> %v %v", res.Outcome, err)
	_, r = flightBegin(tab, "r1", h1)
	tr.w("begin r1 cached -> %s size=%d", r, flightSize(tab))
	clk.Advance(10 * time.Minute)
	_, r = flightBegin(tab, "r1", h1)
	tr.w("begin r1 at exactly the ttl -> %s", r)
	clk.Advance(time.Nanosecond)
	_, r = flightBegin(tab, "r1", h1)
	tr.w("begin r1 just past the ttl -> %s", r)
	clk.Advance(time.Hour)
	_, r = flightBegin(tab, "r1", h1)
	tr.w("begin r1 after ttl -> %s", r)
	f2, _ := flightBegin(tab, "r2", h1)
	flightFinish(tab, "r2", f2, replyResult(sessionv1.ReplyOutcome_REPLY_OUTCOME_NOT_SENT), false)
	_, r = flightBegin(tab, "r2", h1)
	tr.w("begin r2 after uncached finish -> %s size=%d", r, flightSize(tab))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f3, _ := flightBegin(tab, "r3", h1)
	_, err = flightWait(f3, ctx)
	tr.w("wait cancelled -> %v", err)
	for i := 0; i < 1100; i++ {
		id := fmt.Sprintf("e%d", i)
		f, _ := flightBegin(tab, id, h1)
		flightFinish(tab, id, f, replyResult(sessionv1.ReplyOutcome_REPLY_OUTCOME_SENT), true)
	}
	tr.w("after fill size=%d", flightSize(tab))
	_, r = flightBegin(tab, "e0", h1)
	tr.w("oldest evicted -> %s", r)
}

type parityRowsStore struct {
	mu   sync.Mutex
	tr   *transcript
	rows map[uuid.UUID]*domain.GuidanceRequestData
	fail map[string]error
}

func newParityRows(tr *transcript) *parityRowsStore {
	return &parityRowsStore{tr: tr, rows: map[uuid.UUID]*domain.GuidanceRequestData{}, fail: map[string]error{}}
}

func (s *parityRowsStore) GetGuidanceRequest(_ context.Context, id uuid.UUID) (*domain.GuidanceRequestData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tr.w("  rows.Get %s", id)
	if err := s.fail["get"]; err != nil {
		return nil, err
	}
	r, ok := s.rows[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *r
	return &cp, nil
}

func (s *parityRowsStore) AnswerGuidanceRequest(_ context.Context, id uuid.UUID, answer string) (bool, error) {
	s.tr.w("  rows.Answer %s %q", id, answer)
	return true, s.fail["answer"]
}

func (s *parityRowsStore) SetGuidanceDelivery(_ context.Context, id uuid.UUID, route domain.DeliveryRoute, state domain.DeliveryState, add int) (bool, error) {
	s.tr.w("  rows.SetDelivery %s route=%s state=%s add=%d", id, route, state, add)
	if err := s.fail["delivery"]; err != nil {
		return false, err
	}
	return s.fail["notapplied"] == nil, nil
}

type paritySink struct{ tr *transcript }

func (s paritySink) Sent(d SentDelivery) {
	s.tr.w("  sink.Sent row=%s sess=%s route=%s indeterminate=%v matches=%v dialog=%q", d.RowID, d.SessionUUID, d.Route, d.Indeterminate, d.Matches != nil, d.Dialog.Question)
}

func parityDelivery(t *testing.T, tr *transcript) {
	tr.section("delivery")
	ctx := context.Background()
	store := newParityRows(tr)
	mk := func(status string, state domain.DeliveryState, route domain.DeliveryRoute) uuid.UUID {
		id := detUUID()
		d := &domain.GuidanceRequestData{ID: id.String(), DeliveryState: state, DeliveryRoute: route}
		now := parityNow
		switch status {
		case "answered":
			d.AnsweredAt = &now
		case "cancelled":
			d.CancelledAt = &now
		}
		store.rows[id] = d
		return id
	}
	// begin: which rows are tracked
	cases := []struct {
		name   string
		status string
		state  domain.DeliveryState
		route  domain.DeliveryRoute
	}{
		{"pending", "pending", "", ""},
		{"answered-failed-live", "answered", domain.DeliveryStateFailed, domain.DeliveryRouteLiveKeystroke},
		{"answered-failed-hook", "answered", domain.DeliveryStateFailed, domain.DeliveryRouteHookReply},
		{"answered-sent", "answered", domain.DeliveryStateSent, domain.DeliveryRouteLiveKeystroke},
		{"cancelled", "cancelled", "", ""},
	}
	ids := map[string]uuid.UUID{}
	for _, c := range cases {
		ids[c.name] = mk(c.status, c.state, c.route)
		tr.w("begin %s", c.name)
		trk := beginTrack(ctx, store, paritySink{tr}, ids[c.name].String())
		trk.Pending(ctx)
	}
	for _, q := range []string{"not-a-uuid", detUUID().String()} {
		tr.w("begin unknown %v", len(q))
		beginTrack(ctx, store, paritySink{tr}, q).Pending(ctx)
	}
	tr.w("begin nil rows")
	beginTrack(ctx, nil, paritySink{tr}, ids["pending"].String()).Pending(ctx)

	outcomes := []sessionv1.ActionOutcome{
		sessionv1.ActionOutcome_ACTION_OUTCOME_UNSPECIFIED, sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED,
		sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, sessionv1.ActionOutcome_ACTION_OUTCOME_STALE,
		sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, sessionv1.ActionOutcome_ACTION_OUTCOME_RATE_LIMITED,
		sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, sessionv1.ActionOutcome_ACTION_OUTCOME_FORBIDDEN,
		sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF, sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE,
	}
	sent := SentDelivery{RowID: ids["pending"].String(), SessionUUID: "sess", Route: domain.DeliveryRouteLiveKeystroke, Dialog: panereply.QuestionDialog{Question: "Q?"}}
	for _, o := range outcomes {
		for _, stale := range []string{"", staleAnswerPartial()} {
			for _, ferr := range []error{nil, errors.New("boom")} {
				for _, acks := range []bool{true, false} {
					tr.w("finish outcome=%s stale=%q err=%v acks=%v", o, stale, ferr, acks)
					var sink deliverySinkLike
					if acks {
						sink = paritySink{tr}
					}
					trk := beginTrack(ctx, store, sink, ids["pending"].String())
					trk.Finish(ctx, ActionResult{Outcome: o, StaleReason: stale}, ferr, sent, "ans")
				}
			}
		}
	}
	for _, o := range outcomes {
		tr.w("finish untracked outcome=%s", o)
		beginTrack(ctx, store, paritySink{tr}, ids["cancelled"].String()).Finish(ctx, ActionResult{Outcome: o}, nil, sent, "x")
		beginTrack(ctx, store, paritySink{tr}, ids["cancelled"].String()).Finish(ctx, ActionResult{Outcome: o}, errors.New("e"), sent, "x")
	}
	store.fail["answer"] = errors.New("answer failed")
	beginTrack(ctx, store, paritySink{tr}, ids["pending"].String()).Finish(ctx, ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED}, nil, sent, "a")
	delete(store.fail, "answer")
	store.fail["notapplied"] = errors.New("x")
	beginTrack(ctx, store, nil, ids["pending"].String()).Pending(ctx)
	delete(store.fail, "notapplied")
	store.fail["delivery"] = errors.New("db down")
	beginTrack(ctx, store, nil, ids["pending"].String()).Pending(ctx)
	delete(store.fail, "delivery")
	store.fail["get"] = errors.New("get failed")
	beginTrack(ctx, store, nil, ids["pending"].String()).Pending(ctx)

	for _, r := range []ActionResult{applied(), outcomeResult(sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, "d"), notWiredResult()} {
		tr.w("action result %v %q %q %d", r.Outcome, r.Detail, r.StaleReason, r.RetryAfter)
	}
}

func parityRowFixtures(t *testing.T) map[string]*domain.GuidanceRequestData {
	t.Helper()
	single := parityPayloadJSON(t, domain.Question{Text: "Which color?", Header: "Color", Options: opts("Red", "Green", "Blue")})
	twoq := parityPayloadJSON(t,
		domain.Question{Text: "Pick a cache", Header: "Cache", Options: opts("Redis", "Memcached", "None")},
		domain.Question{Text: "Pick letters", Header: "Letters", MultiSelect: true, Options: opts("A", "B", "C")})
	multi := parityPayloadJSON(t, domain.Question{Text: "Multi?", Header: "M", MultiSelect: true, Options: opts("A", "B")})
	noheader := parityPayloadJSON(t, domain.Question{Text: "No header?", Options: opts("A", "B")})
	two := parityPayloadJSON(t, domain.Question{Text: "One?", Header: "One", Options: opts("A")}, domain.Question{Text: "Two?", Header: "Two", Options: opts("B")})
	base := func(q string, route domain.DeliveryRoute, src domain.QuestionSource) *domain.GuidanceRequestData {
		return &domain.GuidanceRequestData{ID: detUUID().String(), Source: src, DeliveryRoute: route, QuestionsJSON: q, AskerSessionUUID: "sess", SessionUUID: "sess", CreatedAt: parityNow}
	}
	live, hook := domain.DeliveryRouteLiveKeystroke, domain.QuestionSourceHook
	return map[string]*domain.GuidanceRequestData{
		"single":         base(single, live, hook),
		"single-nolive":  base(single, "", hook),
		"single-peer":    base(single, live, domain.QuestionSourcePeer),
		"twoq-tabbed":    base(twoq, live, hook),
		"twoq-nolive":    base(twoq, "", hook),
		"multi":          base(multi, live, hook),
		"noheader":       base(noheader, live, hook),
		"two-unmeasured": base(two, live, hook),
		"empty-json":     base("", live, hook),
		"bad-json":       base("{not json", live, hook),
	}
}

// parityThreeTabs is the two-question frame with a third tab: its first two headers still match the row.
func parityThreeTabs(t *testing.T) string {
	t.Helper()
	f := strings.Replace(parityText(t, "claude_auq_twoq_step1_single_select.txt"), "☐ Letters  ", "☐ Letters  ☐ Third  ", 1)
	require.Contains(t, f, "☐ Third")
	return f
}

func parityRows(t *testing.T, tr *transcript) {
	tr.section("rows")
	rows := parityRowFixtures(t)
	names := make([]string, 0, len(rows))
	for n := range rows {
		names = append(names, n)
	}
	sort.Strings(names)
	frames := map[string]string{
		"single3": parityText(t, "question_single_select_3opts.txt"), "step1": parityText(t, "claude_auq_twoq_step1_single_select.txt"),
		"step2": parityText(t, "claude_auq_twoq_step2_multiselect_initial.txt"), "bash": parityText(t, "permission_bash.txt"),
		"review": parityText(t, "claude_auq_twoq_review_submit.txt"), "threetabs": parityThreeTabs(t),
	}
	for _, n := range names {
		r := rows[n]
		d, ok := rowDialog(r)
		tr.w("%s: dialog=%q/%q/%q ok=%v", n, d.Question, d.Header, d.Labels, ok)
		qs, tabbed := rowTabbed(r)
		tr.w("%s: tabbed=%v qs=%+v", n, tabbed, qs)
		if tabbed {
			for _, fn := range sortedKeys(frames) {
				tr.w("%s: headersMatch(%s)=%v", n, fn, tabbedHeaders(qs)(frames[fn]))
			}
		}
		pb := &sessionv1.GuidanceRequest{}
		setMirrorFields(pb, r)
		tr.w("%s: proto=%s", n, protoHex(pb))
	}
	exp := parityNow.Add(time.Hour)
	full := rows["single"]
	cp := *full
	cp.ExpiresAt = &exp
	cp.AnswererKind, cp.DeliveryState, cp.DeliveryAttempts, cp.HookCallID, cp.MultiSelect = domain.AnswererKindSession, domain.DeliveryStateSent, 3, "toolu_1", true
	pb := &sessionv1.GuidanceRequest{}
	setMirrorFields(pb, &cp)
	tr.w("full: %s", protoHex(pb))
}

// protoHex is the deterministic wire form; fmt.Sprint of a message varies its spacing per binary.
func protoHex(m proto.Message) string {
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
	if err != nil {
		return "marshal error: " + err.Error()
	}
	return fmt.Sprintf("%x", b)
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func parityJudge(t *testing.T, tr *transcript) {
	tr.section("judge")
	rows := parityRowFixtures(t)
	frames := map[string]string{
		"single3": parityText(t, "question_single_select_3opts.txt"), "wrapped": parityText(t, "question_wrapped_2opts.txt"),
		"step1": parityText(t, "claude_auq_twoq_step1_single_select.txt"), "step2": parityText(t, "claude_auq_twoq_step2_multiselect_initial.txt"),
		"review": parityText(t, "claude_auq_twoq_review_submit.txt"), "bash": parityText(t, "permission_bash.txt"),
		"threetabs": parityThreeTabs(t), "empty": "", "text": "just some output\nmore", "composer": "some output\n\n> \n", "composer2": "done.\n\n❯ \n",
	}
	names := make([]string, 0, len(rows))
	for n := range rows {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		for _, fn := range sortedKeys(frames) {
			v := judge(rows[n], frames[fn], nil)
			reason, terminal := verdictReason(v)
			tr.w("%s x %s -> %d %q %v", n, fn, v, reason, terminal)
		}
		v := judge(rows[n], "", errors.New("capture failed"))
		tr.w("%s x err -> %d", n, v)
	}
}

type parityMirrorStore struct {
	tr      *transcript
	next    int
	rows    map[string]*domain.GuidanceRequestData
	create  func(n int) error
	listErr error
	purge   int
	// quiet keeps a late write off the transcript: the timeout case returns after Mirror gave up.
	quiet bool
}

func (s *parityMirrorStore) CreateMirroredGuidanceRequest(_ context.Context, req domain.GuidanceRequest, capLimit int) (*domain.GuidanceRequestData, error) {
	s.next++
	if s.create != nil {
		if err := s.create(s.next); err != nil {
			if !s.quiet {
				s.tr.w("  store.Create #%d -> %v", s.next, err)
			}
			return nil, err
		}
	}
	p := req.Params()
	pj, _ := p.Payload.JSON()
	row := &domain.GuidanceRequestData{
		ID: detUUID().String(), Source: p.Source, AskerSessionUUID: p.AskerSessionUUID, SessionUUID: p.SessionUUID,
		QuestionsJSON: pj, DeliveryRoute: p.DeliveryRoute, HookCallID: p.HookCallID, CreatedAt: parityNow, QuestionText: p.QuestionText,
	}
	s.rows[row.ID] = row
	s.tr.w("  store.Create #%d scope=%s source=%s asker=%s hook=%s route=%s cap=%d text=%q payload=%s", s.next, p.Scope, p.Source, p.AskerSessionUUID, p.HookCallID, p.DeliveryRoute, capLimit, p.QuestionText, pj)
	return row, nil
}

func (s *parityMirrorStore) CancelGuidanceRequest(_ context.Context, id uuid.UUID, reason string) (bool, error) {
	r, ok := s.rows[id.String()]
	s.tr.w("  store.Cancel reason=%s known=%v", reason, ok)
	if !ok {
		return false, nil
	}
	if r.CancelledAt != nil {
		return false, nil
	}
	now := parityNow
	r.CancelledAt = &now
	return true, nil
}

func (s *parityMirrorStore) GetGuidanceRequest(_ context.Context, id uuid.UUID) (*domain.GuidanceRequestData, error) {
	r, ok := s.rows[id.String()]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *r
	return &cp, nil
}

func (s *parityMirrorStore) sortedRows(open bool, asker string) []*domain.GuidanceRequestData {
	var out []*domain.GuidanceRequestData
	for _, r := range s.rows {
		if open && (r.CancelledAt != nil || r.AnsweredAt != nil) {
			continue
		}
		if asker != "" && r.AskerSessionUUID != asker {
			continue
		}
		cp := *r
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HookCallID < out[j].HookCallID })
	return out
}

func (s *parityMirrorStore) ListAllPendingGuidanceRequests(context.Context) ([]*domain.GuidanceRequestData, error) {
	return s.sortedRows(true, ""), s.listErr
}

func (s *parityMirrorStore) ListOpenGuidanceRequestsByAsker(_ context.Context, asker string, _ ...domain.QuestionSource) ([]*domain.GuidanceRequestData, error) {
	return s.sortedRows(true, asker), s.listErr
}

func (s *parityMirrorStore) PurgeTerminalGuidanceQuestions(_ context.Context, cutoff time.Time) (int, error) {
	s.tr.w("  store.Purge cutoff=%s", cutoff.Format(time.RFC3339))
	return s.purge, nil
}

func parityMirror(t *testing.T, tr *transcript) {
	tr.section("mirror")
	ctx := context.Background()
	counts := map[string]int{}
	var mu sync.Mutex
	count := func(prefix string) func(string) {
		return func(reason string) {
			mu.Lock()
			counts[prefix+":"+reason]++
			mu.Unlock()
			tr.w("  %s(%s)", prefix, reason)
		}
	}
	newStore := func() *parityMirrorStore {
		return &parityMirrorStore{tr: tr, rows: map[string]*domain.GuidanceRequestData{}}
	}
	asked := func(q, h string, replyable bool, labels ...string) HookMirrorRequest {
		a := AskedQuestion{Question: q, Header: h, Labels: labels, Replyable: replyable, Shape: QuestionShapeSingle,
			Payload: parityQuestions(domain.Question{Text: q, Header: h, Options: opts(labels...)})}
		return HookMirrorRequest{SessionUUID: "sess", HookCallID: "toolu_" + q, Asked: a, Replyable: replyable}
	}
	on := func() bool { return true }
	off := func() bool { return false }

	store := newStore()
	pending := NewPendingQuestionStore(nil)
	m := newMirror(store, pending, count("failed"), count("cleared"), on)
	tr.w("enabled on=%v off=%v nil=%v", m.Enabled(), newMirror(store, pending, nil, nil, off).Enabled(), newMirror(store, pending, nil, nil, nil).Enabled())
	for _, req := range []HookMirrorRequest{
		asked("Which color?", "Color", true, "Red", "Green"),
		asked("View only?", "VO", false, "A", "B"),
		asked("Secret?", "Tok", true, "AKIAIOSFODNN7EXAMPLE", "ghp_abcdefghijklmnopqrstuvwxyz0123456789"),
		{SessionUUID: "sess", HookCallID: "toolu_nopayload", Asked: AskedQuestion{Shape: QuestionShapeUnknown}},
	} {
		id, ok := m.Mirror(req)
		tr.w("mirror %q -> ok=%v idset=%v", req.HookCallID, ok, id != "")
	}
	idOff, okOff := newMirror(store, pending, count("failed"), nil, off).Mirror(asked("Off?", "O", true, "A"))
	tr.w("mirror flag off -> %q %v", idOff, okOff)

	// store errors and cap
	errStore := newStore()
	errStore.create = func(n int) error { return errors.New("db down") }
	_, ok := newMirror(errStore, pending, count("failed"), nil, on).Mirror(asked("E?", "E", true, "A"))
	tr.w("mirror store error -> %v", ok)
	capStore := newStore()
	capStore.create = func(n int) error { return domain.ErrPendingCapExceeded }
	_, ok = newMirror(capStore, pending, count("failed"), nil, on).Mirror(asked("C?", "C", true, "A"))
	tr.w("mirror cap, no capture -> %v", ok)
	capM := newMirror(capStore, pending, count("failed"), count("cleared"), on)
	capM.SetPaneCapture(func(context.Context, string) (string, error) { return "idle\n\n> \n", nil })
	capStore.rows["x"] = &domain.GuidanceRequestData{ID: detUUID().String(), Source: domain.QuestionSourceHook, AskerSessionUUID: "sess", DeliveryRoute: domain.DeliveryRouteLiveKeystroke, QuestionsJSON: parityPayloadJSON(t, domain.Question{Text: "Old?", Header: "Old", Options: opts("A", "B")}), HookCallID: "old"}
	capStore.next = 0
	capStore.create = func(n int) error {
		if n <= 1 {
			return domain.ErrPendingCapExceeded
		}
		return nil
	}
	_, ok = capM.Mirror(asked("C2?", "C2", true, "A"))
	tr.w("mirror cap, reverify clears then retries -> %v", ok)
	slow := newStore()
	slowM := newMirror(slow, pending, count("failed"), nil, on)
	slowM.SetBound(time.Nanosecond)
	slow.quiet = true
	release := make(chan struct{})
	slow.create = func(int) error { <-release; return errors.New("late") }
	_, ok = slowM.Mirror(asked("S?", "S", true, "A"))
	close(release)
	tr.w("mirror timeout -> %v", ok)

	// rehydrate, ensure registered, clear
	rows := store.sortedRows(true, "sess")
	tr.w("open rows=%d", len(rows))
	restarted := NewPendingQuestionStore(nil)
	m2 := newMirror(store, restarted, nil, nil, on)
	n, err := m2.Rehydrate(ctx)
	tr.w("rehydrate -> %d %v len=%d", n, err, restarted.Len())
	store.listErr = errors.New("list failed")
	n, err = m2.Rehydrate(ctx)
	tr.w("rehydrate list error -> %d %v", n, err)
	store.listErr = nil
	var firstID string
	for _, r := range rows {
		if _, ok := rowDialog(r); ok {
			firstID = r.ID
		}
	}
	m3 := newMirror(store, NewPendingQuestionStore(nil), nil, nil, on)
	tr.w("ensure registered ok=%v", m3.EnsureRegistered(ctx, QuestionKey{SessionUUID: "sess", QuestionID: firstID}))
	tr.w("ensure registered again=%v", m3.EnsureRegistered(ctx, QuestionKey{SessionUUID: "sess", QuestionID: firstID}))
	tr.w("ensure bad uuid=%v", m3.EnsureRegistered(ctx, QuestionKey{SessionUUID: "sess", QuestionID: "zz"}))
	tr.w("ensure unknown=%v", m3.EnsureRegistered(ctx, QuestionKey{SessionUUID: "sess", QuestionID: detUUID().String()}))
	tr.w("ensure other session=%v", newMirror(store, NewPendingQuestionStore(nil), nil, nil, on).EnsureRegistered(ctx, QuestionKey{SessionUUID: "other", QuestionID: firstID}))

	var retired []string
	m.SetCardRetirer(func(id string) { retired = append(retired, id) })
	tr.w("clear bad uuid -> %v", m.Clear(ctx, "zz", domain.CancelReasonDialogGone) != nil)
	tr.w("clear unknown -> %v", m.Clear(ctx, detUUID().String(), domain.CancelReasonDialogGone) != nil)
	tr.w("clear first -> %v", m.Clear(ctx, firstID, domain.CancelReasonAnsweredInTerminal))
	tr.w("clear first again -> %v", m.Clear(ctx, firstID, domain.CancelReasonAnsweredInTerminal))
	tr.w("retired after an answered-in-terminal clear=%d", len(retired))
	tr.w("byhook empty -> %v", m.ClearByHookCall(ctx, "sess", "", domain.CancelReasonDialogGone))
	tr.w("byhook match -> %v", m.ClearByHookCall(ctx, "sess", "toolu_View only?", domain.CancelReasonSessionEnded))
	tr.w("retired after a session-ended clear=%d", len(retired))
	store.listErr = errors.New("list failed")
	tr.w("byhook list error -> %v", m.ClearByHookCall(ctx, "sess", "x", domain.CancelReasonDialogGone))
	tr.w("clear open list error -> %v", m.ClearOpenForSession(ctx, "sess", domain.CancelReasonDialogGone))
	store.listErr = nil
	tr.w("clear open -> %v open-after=%d", m.ClearOpenForSession(ctx, "sess", domain.CancelReasonAnsweredInTerminal), len(store.sortedRows(true, "sess")))
	store.purge = 4
	pn, perr := m.PurgeTerminalQuestions(ctx, parityNow)
	tr.w("purge -> %d %v", pn, perr)
	tr.w("counts=%v", sortedCounts(counts))
}

func sortedCounts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%d ", k, m[k])
	}
	return b.String()
}

type paritySamplerStore struct {
	rows []*domain.GuidanceRequestData
	err  error
}

func (s *paritySamplerStore) ListAllPendingGuidanceRequests(context.Context) ([]*domain.GuidanceRequestData, error) {
	return s.rows, s.err
}

type parityClearer struct {
	tr  *transcript
	err error
}

func (c *parityClearer) Clear(_ context.Context, id string, reason domain.CancelReason) error {
	c.tr.w("  clear %s %s", id[:4], reason)
	return c.err
}

func paritySampler(t *testing.T, tr *transcript) {
	tr.section("sampler")
	ctx := context.Background()
	clk := newParityClock()
	rows := parityRowFixtures(t)
	store := &paritySamplerStore{}
	for i, n := range []string{"single", "twoq-tabbed", "multi", "noheader", "single-peer"} {
		r := *rows[n]
		r.ID = fmt.Sprintf("%04d-%s", i, n)
		r.CreatedAt = clk.Now()
		store.rows = append(store.rows, &r)
	}
	gone := "idle\n\n> \n"
	showing := parityText(t, "question_single_select_3opts.txt")
	capture := gone
	var captured []string
	clearer := &parityClearer{tr: tr}
	s := NewDialogGoneSampler(store, func(_ context.Context, sess string) (string, error) {
		captured = append(captured, sess)
		if capture == "ERR" {
			return "", errors.New("capture failed")
		}
		return capture, nil
	}, clearer, clk.Now)
	for i := 0; i < 6; i++ {
		tr.w("tick %d capture=%q", i, strings.SplitN(capture, "\n", 2)[0])
		s.Tick(ctx)
		tr.w("  captured=%d states=%d", len(captured), len(s.states))
		clk.Advance(20 * time.Second)
		if i == 2 {
			capture = showing
		}
		if i == 3 {
			capture = "ERR"
		}
		if i == 4 {
			capture = gone
			clearer.err = errors.New("clear failed")
		}
	}
	clearer.err = nil
	clk.Advance(10 * time.Minute)
	s.Tick(ctx)
	tr.w("slow tick captured=%d", len(captured))
	clk.Advance(3 * time.Hour)
	s.Tick(ctx)
	tr.w("past stop captured=%d", len(captured))
	store.err = errors.New("list failed")
	s.Tick(ctx)
	store.err = nil
	store.rows = nil
	s.Tick(ctx)
	tr.w("states after drain=%d", len(s.states))
	big := &paritySamplerStore{}
	for i := 0; i < 30; i++ {
		r := *rows["single"]
		r.ID = fmt.Sprintf("%04d-big", i)
		r.CreatedAt = clk.Now()
		big.rows = append(big.rows, &r)
	}
	captured = nil
	bs := NewDialogGoneSampler(big, func(_ context.Context, sess string) (string, error) {
		captured = append(captured, sess)
		return showing, nil
	}, clearer, clk.Now)
	bs.Tick(ctx)
	tr.w("per-pass cap captured=%d", len(captured))
	bs.Tick(ctx)
	tr.w("second pass captured=%d", len(captured))
	exact := &paritySamplerStore{}
	for i := 0; i < SamplerMaxPerPass+1; i++ {
		r := *rows["single"]
		r.ID = fmt.Sprintf("%04d-exact", i)
		r.CreatedAt = clk.Now()
		exact.rows = append(exact.rows, &r)
	}
	captured = nil
	es := NewDialogGoneSampler(exact, func(_ context.Context, sess string) (string, error) {
		captured = append(captured, sess)
		return showing, nil
	}, clearer, clk.Now)
	es.Tick(ctx)
	tr.w("one over the cap captured=%d", len(captured))
	ctxc, cancel := context.WithCancel(ctx)
	cancel()
	bs.Run(ctxc)
	tr.w("run returns on cancelled ctx")
}

func parityCanary(tr *transcript) {
	tr.section("canary")
	clk := newParityClock()
	var counted, notified int
	c := NewDialogMatchCanary(clk.Now, func(r DialogMatchResult, v string) { counted++; tr.w("  count %s %s", r, v) }, func() { notified++; tr.w("  notify") })
	tr.w("tripped=%v", c.Tripped())
	var nilc *DialogMatchCanary
	nilc.Record(canaryMismatch())
	tr.w("nil tripped=%v", nilc.Tripped())
	for i := 0; i < 4; i++ {
		c.Record(canaryMismatch())
	}
	tr.w("after 4 mismatches tripped=%v", c.Tripped())
	c.Record(canaryMismatch())
	tr.w("after 5 tripped=%v notified=%d", c.Tripped(), notified)
	c.Record(canaryMismatch())
	tr.w("after 6 notified=%d", notified)
	for i := 0; i < 8; i++ {
		c.Record(canaryMatch())
	}
	tr.w("after 8 matches tripped=%v", c.Tripped())
	c.SetVersionSource(func() string { return "2.1.303" })
	c.Record(canaryMatch())
	c.SetVersionSource(func() string { return "" })
	c.Record(canaryMatch())
	clk.Advance(2 * time.Hour)
	tr.w("after window tripped=%v", c.Tripped())
	tr.w("result of %v %v", canaryOf(true), canaryOf(false))
	tr.w("counted=%d notified=%d", counted, notified)
	h := NewDialogMatchCanary(clk.Now, nil, nil)
	for i := 0; i < 3; i++ {
		h.Record(canaryMismatch())
		h.Record(canaryMatch())
	}
	tr.w("exactly half tripped=%v", h.Tripped())
	e := NewDialogMatchCanary(clk.Now, nil, nil)
	for i := 0; i < 5; i++ {
		e.Record(canaryMismatch())
	}
	tr.w("tripped=%v", e.Tripped())
	clk.Advance(61 * time.Minute)
	e.Record(canaryMismatch())
	e.Record(canaryMismatch())
	e.Record(canaryMatch())
	tr.w("few attempts, majority mismatched, still tripped=%v", e.Tripped())
	e.Record(canaryMatch())
	e.Record(canaryMatch())
	e.Record(canaryMatch())
	tr.w("recovered tripped=%v", e.Tripped())
	d := NewDialogMatchCanary(nil, nil, nil)
	for i := 0; i < 6; i++ {
		d.Record(canaryMismatch())
	}
	tr.w("nil hooks tripped=%v", d.Tripped())
}

type parityRole struct {
	row backlogdto.ItemSessionSummary
	err error
}

func (r parityRole) GetItemSessionBySessionUUID(context.Context, string) (backlogdto.ItemSessionSummary, error) {
	return r.row, r.err
}

func parityMisc(t *testing.T, tr *transcript) {
	tr.section("misc")
	ctx := context.Background()
	tr.w("role nil=%s", roleOf(nil, ctx, "s"))
	tr.w("role work=%s", roleOf(parityRole{row: backlogdto.ItemSessionSummary{Role: "work"}}, ctx, "s"))
	tr.w("role empty=%s", roleOf(parityRole{}, ctx, "s"))
	tr.w("role notfound=%s", roleOf(parityRole{err: domain.ErrNotFound}, ctx, "s"))
	tr.w("role err=%s", roleOf(parityRole{err: errors.New("x")}, ctx, "s"))

	clk := newParityClock()
	tk := newTracker(clk.Now)
	req := func(kind sessionv1.ActionKind, sess string) *sessionv1.InvokeNotificationActionRequest {
		return &sessionv1.InvokeNotificationActionRequest{ActionKind: kind, SessionId: sess}
	}
	ans := sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION
	tk.NoteApplied(req(ans, "a"), applied())
	tk.NoteApplied(req(ans, "b"), outcomeResult(sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, ""))
	tk.NoteApplied(req(sessionv1.ActionKind_ACTION_KIND_APPROVE, "c"), applied())
	tk.NoteApplied(req(ans, ""), applied())
	tk.NoteApplied(req(ans, "d"), applied())
	tr.w("tracker pending=%d", trackerPending(tk))
	clk.Advance(61 * time.Second)
	tk.NoteApplied(req(ans, "e"), applied())
	tr.w("tracker pending after window=%d", trackerPending(tk))
	tk.Attached("e")
	tk.Attached("e")
	tk.Attached("zzz")
	tr.w("tracker pending after attach=%d", trackerPending(tk))
	var nilT *ReopenTracker
	nilT.NoteApplied(req(ans, "f"), applied())
	nilT.Attached("f")
	var buf bytes.Buffer
	_ = buf
}

func recordedParity(t *testing.T) string {
	t.Helper()
	f, err := os.Open(parityGoldenPath)
	require.NoError(t, err)
	defer f.Close()
	zr, err := gzip.NewReader(f)
	require.NoError(t, err)
	b, err := io.ReadAll(zr)
	require.NoError(t, err)
	return string(b)
}

func TestParityGolden_should_MatchTheTranscriptRecordedFromTheBasePackage(t *testing.T) {
	got := recordParity(t)
	require.Equal(t, got, recordParity(t), "the transcript must be deterministic")
	if os.Getenv("QUESTIONKIT_RECORD") == "1" {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, err := zw.Write([]byte(got))
		require.NoError(t, err)
		require.NoError(t, zw.Close())
		require.NoError(t, os.WriteFile(parityGoldenPath, buf.Bytes(), 0o644))
		t.Logf("recorded %d bytes sha256=%x", len(got), sha256.Sum256([]byte(got)))
		return
	}
	want := recordedParity(t)
	if dump := os.Getenv("QUESTIONKIT_DUMP"); dump != "" {
		require.NoError(t, os.WriteFile(dump, []byte(got), 0o644))
	}
	require.Equal(t, sha256.Sum256([]byte(want)), sha256.Sum256([]byte(got)), "transcript sha256 differs from the recorded base")
	require.Equal(t, want, got)
}
