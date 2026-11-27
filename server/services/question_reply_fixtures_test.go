package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/auditkit"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/notifyactions"
	"github.com/tstapler/stapler-squad/server/services/qareply"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/panereply"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// Reply fixtures duplicated from server/services/qareply's tests, for the tests
// here that drive the Reply path through SessionService.

const (
	replyLoopbackPeer = "127.0.0.1:50000"
	replyColorLabel   = "Green-label-sentinel"
)

var colorDialog = panereply.QuestionDialog{
	Question: "Which color should the spike use?",
	Header:   "Color",
	Labels:   []string{"Red", replyColorLabel, "Blue"},
}

// replyDialogFixture loads a verbatim capture of a real Claude Code dialog.
func replyDialogFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "session", "testdata", "reply_dialogs", name))
	require.NoError(t, err)
	return string(b)
}

// replyPM is a recording pane: it shows dialog until a key was written, then after.
type replyPM struct {
	instance.ProcessManager
	mu     sync.Mutex
	sent   []string
	dialog string
	after  string
}

func (r *replyPM) GetSessionIdentifier() string     { return "" }
func (r *replyPM) GetPanePID() (int32, error)       { return 0, errors.New("no pane") }
func (r *replyPM) HasSession() bool                 { return true }
func (r *replyPM) IsAlive() bool                    { return true }
func (r *replyPM) HasMeaningfulContent(string) bool { return false }

func (r *replyPM) SendKeys(keys string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, keys)
	return len(keys), nil
}

func (r *replyPM) CapturePaneContent() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.sent) > 0 {
		return r.after, nil
	}
	return r.dialog, nil
}

func (r *replyPM) writes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sent...)
}

// replyEnv is a steer env whose Reply path is built with test seams, plus a
// registered question on a hidden session.
type replyEnv struct {
	*steerEnv
	clock *gateTestClock
	pm    *replyPM
	inst  *instance.Instance
	qid   string
}

// seamReplyService installs the production ports plus deterministic seams. It
// must run before anything else builds the service's Reply path.
func seamReplyService(t *testing.T, svc *SessionService, clock *gateTestClock) {
	t.Helper()
	p := svc.replyPorts()
	p.Now = clock.Now
	p.PeerAddr = func(connect.AnyRequest) string { return replyLoopbackPeer }
	p.ControllerActive = func(*instance.Instance) bool { return true }
	// Poll timers fire at once; the 5s send timeout never does (nothing sleeps).
	p.After = func(d time.Duration) <-chan time.Time {
		if d >= time.Second {
			return nil
		}
		ch := make(chan time.Time, 1)
		ch <- time.Time{}
		return ch
	}
	built := false
	svc.replyOnce.Do(func() { svc.reply, built = qareply.New(p), true })
	require.True(t, built, "the Reply path was built before the seams were installed")
}

func newReplyEnv(t *testing.T) *replyEnv {
	t.Helper()
	e := newSteerEnv(t)
	clock := newGateTestClock()
	seamReplyService(t, e.fix.svc, clock)
	e.fix.svc.wireReplyFlag() // no persisted value: on

	dialog := strings.ReplaceAll(replyDialogFixture(t, "question_single_select_3opts.txt"), "2. Green", "2. "+replyColorLabel)
	pm := &replyPM{dialog: dialog, after: "● User answered Claude's questions\n❯ \n"}
	re := &replyEnv{steerEnv: e, clock: clock, pm: pm}
	re.inst = re.addPane("reply-hidden", pm, func(i *instance.Instance) { i.Hidden = true })
	re.qid = re.register(re.inst, colorDialog)
	return re
}

// addPane registers a started instance titled title on pm.
func (e *replyEnv) addPane(title string, pm instance.ProcessManager, tweak ...func(*instance.Instance)) *instance.Instance {
	e.t.Helper()
	inst := instance.NewStartedInstanceForTest(e.t, title, pm)
	inst.UUID = title + "-uuid"
	inst.Path = e.t.TempDir()
	inst.Status = domain.Active
	inst.Program = "claude"
	inst.CreatedAt, inst.UpdatedAt = time.Now(), time.Now()
	for _, f := range tweak {
		f(inst)
	}
	svctest.AddInstanceToPoller(e.fix.poller, inst)
	return inst
}

func (e *replyEnv) register(inst *instance.Instance, d panereply.QuestionDialog) string {
	id := questionkit.NewQuestionID()
	e.fix.svc.PendingQuestions().Register(questionkit.PendingQuestion{
		QuestionID: id, SessionUUID: inst.UUID, Dialog: d, Token: questionkit.QuestionToken(d.Question),
	})
	return id
}

func (e *replyEnv) replyOK(text, replyID string) *sessionv1.ReplyToPendingQuestionResponse {
	e.t.Helper()
	resp, err := e.fix.svc.ReplyToPendingQuestion(e.localCtx(), connect.NewRequest(&sessionv1.ReplyToPendingQuestionRequest{
		SessionId: e.inst.UUID, QuestionId: e.qid, ReplyText: text, ReplyId: replyID,
	}))
	require.NoError(e.t, err)
	return resp.Msg
}

func (e *replyEnv) counter(label string) uint64 {
	return e.gate.Metrics().Value(deliverygate.CounterReply, label)
}

func (e *replyEnv) replyLines() []auditkit.AuditLine {
	return steerLines(e.auditLines(), auditkit.AuditKindReply)
}

// openFlags resolves every inline action and answer route on.
type openFlags struct{}

func (openFlags) Inline(string) bool              { return true }
func (openFlags) Answer(flagsvc.AnswerRoute) bool { return true }

// routerEnv is a reply env plus the answer router over real storage.
type routerEnv struct {
	*replyEnv
	router *qareply.AnswerRouter
}

func newRouterEnv(t *testing.T) *routerEnv {
	t.Helper()
	e := &routerEnv{replyEnv: newReplyEnv(t)}
	// A fake guard: the default one asks tmux for client activity.
	guard := access.NewHumanActiveGuard(access.NewHumanActivity(e.clock.Now), func(context.Context, string) ([]tmux.ClientActivity, error) { return nil, nil })
	e.router = e.fix.svc.replyService().NewAnswerRouter(qareply.AnswerRouterDeps{Rows: e.fix.storage, Flags: openFlags{}, Guard: guard})
	return e
}

// addVisible registers a visible session whose pane shows the colour dialog.
func (e *routerEnv) addVisible(title string) (*instance.Instance, *replyPM) {
	e.t.Helper()
	dialog := strings.ReplaceAll(replyDialogFixture(e.t, "question_single_select_3opts.txt"), "2. Green", "2. "+replyColorLabel)
	pm := &replyPM{dialog: dialog, after: "● User answered Claude's questions\n❯ \n"}
	return e.addPane(title, namedPM{replyPM: pm, name: "ssq_" + title}), pm
}

// mirroredRow stores a hook-mirrored row for questions asked by sessionUUID.
func (e *routerEnv) mirroredRow(sessionUUID string, qs ...domain.Question) string {
	e.t.Helper()
	payload, err := domain.NewQuestionPayload(qs)
	require.NoError(e.t, err)
	gr, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: sessionUUID, Source: domain.QuestionSourceHook,
		AskerSessionUUID: sessionUUID, Payload: payload, HookCallID: domain.HookCallID("toolu_" + uuid.NewString()),
		DeliveryRoute: domain.DeliveryRouteLiveKeystroke,
	})
	require.NoError(e.t, err)
	row, err := e.fix.storage.CreateMirroredGuidanceRequest(context.Background(), gr, 0)
	require.NoError(e.t, err)
	return row.ID
}

func options(labels ...string) []domain.Option {
	o := make([]domain.Option, len(labels))
	for i, l := range labels {
		o[i] = domain.Option{Label: l}
	}
	return o
}

// hookRow stores a hook-mirrored row for dialog d and registers it as pending.
func (e *routerEnv) hookRow(inst *instance.Instance, d panereply.QuestionDialog) string {
	e.t.Helper()
	id := e.mirroredRow(inst.UUID, domain.Question{Text: d.Question, Header: d.Header, Options: options(d.Labels...)})
	e.fix.svc.PendingQuestions().Register(questionkit.PendingQuestion{
		QuestionID: id, SessionUUID: inst.UUID, Dialog: d, Token: questionkit.QuestionToken(d.Question),
	})
	return id
}

func (e *routerEnv) row(id string) *domain.GuidanceRequestData {
	e.t.Helper()
	row, err := e.fix.storage.GetGuidanceRequest(context.Background(), uuid.MustParse(id))
	require.NoError(e.t, err)
	return row
}

func (e *routerEnv) answer(inst *instance.Instance, qid, label string) questionkit.ActionResult {
	e.t.Helper()
	res, err := e.router.ReplyToQuestion(e.localCtx(), notifyactions.QuestionAnswer{SessionID: inst.UUID, QuestionID: qid, OptionLabel: label, ReplyID: "r-" + uuid.NewString()})
	require.NoError(e.t, err)
	return res
}

// twoQuestionRow stores the hook-mirrored row of the two-question dialog.
func (e *routerEnv) twoQuestionRow(inst *instance.Instance) string {
	e.t.Helper()
	return e.mirroredRow(inst.UUID,
		domain.Question{Text: "Which cache?", Header: "Cache", Options: options("Redis", "Memcached", "None")},
		domain.Question{Text: "Which letters?", Header: "Letters", Options: options("A", "B", "C"), MultiSelect: true})
}

const closedComposer = "\n────────────────────────────────────────\n❯ \n────────────────────────────────────────\n  ? for shortcuts\n"

// tabbedFrames returns the pane after 0..9 keys of the recorded sequence. The
// intermediate frames are the real AB_toggled capture with only its focus and
// checkbox marks moved, since the fixtures hold five of the ten frames.
func tabbedFrames(t *testing.T) []string {
	t.Helper()
	ab := replyDialogFixture(t, "claude_auq_twoq_step2_multiselect_AB_toggled.txt")
	edit := func(pairs ...string) string {
		out := ab
		for i := 0; i < len(pairs); i += 2 {
			require.Contains(t, out, pairs[i])
			out = strings.Replace(out, pairs[i], pairs[i+1], 1)
		}
		return out
	}
	return []string{
		replyDialogFixture(t, "claude_auq_twoq_step1_single_select.txt"),
		replyDialogFixture(t, "claude_auq_twoq_step2_multiselect_initial.txt"),
		edit("❯ 2. [✔] B", "  2. [ ] B", "  1. [✔] A", "❯ 1. [✔] A"),
		edit("❯ 2. [✔] B", "❯ 2. [ ] B"),
		ab,
		edit("❯ 2. [✔] B", "  2. [✔] B", "  3. [ ] C", "❯ 3. [ ] C"),
		edit("❯ 2. [✔] B", "  2. [✔] B", "  4. [ ] Type something", "❯ 4. [ ] Type something"),
		replyDialogFixture(t, "claude_auq_twoq_step2_multiselect_submit_focused.txt"),
		replyDialogFixture(t, "claude_auq_twoq_review_submit.txt"),
		closedComposer,
	}
}

func archiveNow(inst *instance.Instance) {
	now := time.Now()
	inst.SetArchivedAt(&now)
}
