package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogstage"
	"github.com/tstapler/stapler-squad/session/ent/stagetransition"
	"github.com/tstapler/stapler-squad/session/gitwiring"
	"github.com/tstapler/stapler-squad/session/tmux"
)

func TestForkPressureHealthStatus(t *testing.T) {
	cases := map[tmux.ForkPressureLevel]string{
		tmux.ForkPressureOK:       "ok",
		tmux.ForkPressureWarning:  "degraded",
		tmux.ForkPressureCritical: "critical",
	}
	for level, want := range cases {
		if got := forkPressureHealthStatus(level); got != want {
			t.Errorf("forkPressureHealthStatus(%v) = %q, want %q", level, got, want)
		}
	}
}

func TestGoroutineHealthStatus(t *testing.T) {
	if got := goroutineHealthStatus(highGoroutineCountThreshold); got != "ok" {
		t.Errorf("at threshold: got %q, want ok", got)
	}
	if got := goroutineHealthStatus(highGoroutineCountThreshold + 1); got != "degraded" {
		t.Errorf("above threshold: got %q, want degraded", got)
	}
}

// TestHandleActuatorHealth_ReturnsOK_InNormalConditions asserts the common
// case (no fork pressure, ordinary goroutine count during a test run):
// 200 OK with an "ok" status and both components reporting "ok".
func TestHandleActuatorHealth_ReturnsOK_InNormalConditions(t *testing.T) {
	rec := httptest.NewRecorder()

	// Inputs are pinned: the live goroutine count and fork pressure are
	// process-global, so other tests' leftovers made this flaky under -count=N.
	writeActuatorHealth(rec, tmux.ForkPressureOK, 10)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Status     string                       `json:"status"`
		Components map[string]actuatorComponent `json:"components"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
	if _, ok := body.Components["fork_pressure"]; !ok {
		t.Error("missing fork_pressure component")
	}
	if _, ok := body.Components["runtime"]; !ok {
		t.Error("missing runtime component")
	}
}

// TestHandleActuatorMetrics_ReturnsExpectedShape asserts the JSON has the
// three top-level sections callers/dashboards rely on, so a refactor can't
// silently drop one without a test noticing.
func TestHandleActuatorMetrics_ReturnsExpectedShape(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/actuator/metrics", nil)
	rec := httptest.NewRecorder()

	handleActuatorMetrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"blob_cache", "fork_pressure", "runtime", "workflow_engine_bypass_total"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing top-level key %q in /actuator/metrics response", key)
		}
	}
}

func TestHandleActuatorMetrics_ShowsEngineBypassCounter(t *testing.T) {
	const caller = session.TransitionCaller("actuator_test_caller")
	session.RecordEngineBypass(caller)

	rec := httptest.NewRecorder()
	handleActuatorMetrics(rec, httptest.NewRequest(http.MethodGet, "/actuator/metrics", nil))

	var body struct {
		Bypass map[string]int64 `json:"workflow_engine_bypass_total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Bypass[string(caller)] < 1 {
		t.Errorf("workflow_engine_bypass_total[%s] = %d, want >= 1", caller, body.Bypass[string(caller)])
	}
}

func TestWriteGitBackendSoak_JSONAndText(t *testing.T) {
	report := gitwiring.SoakReport{Cohorts: map[string]string{"refs": "shadow"}}

	rec := httptest.NewRecorder()
	writeGitBackendSoak(rec, httptest.NewRequest(http.MethodGet, "/actuator/git-backend", nil), report)
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON body: %v", err)
	}
	if rec.Header().Get("X-Git-Soak-Ready") != "false" {
		t.Errorf("a report with no shadowed cohort must not be ready, header = %q", rec.Header().Get("X-Git-Soak-Ready"))
	}
	if rec.Header().Get("Content-Type") != "application/json" || got["cohorts"] == nil {
		t.Errorf("json response = %q %v", rec.Header().Get("Content-Type"), got)
	}

	rec = httptest.NewRecorder()
	writeGitBackendSoak(rec, httptest.NewRequest(http.MethodGet, "/actuator/git-backend?format=text", nil), report)
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || !strings.Contains(rec.Body.String(), "git backend soak") {
		t.Errorf("text response = %q %q", rec.Header().Get("Content-Type"), rec.Body.String())
	}
}

// --- /actuator/workflow-engine ---------------------------------------------

var soakTestNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func soakReadyLedger() session.ShadowLedger {
	cl := &session.ClassLedger{
		Calls: map[string]int64{"CanTransition|transition": 300}, Divergence: map[string]int64{},
		NotCompared: map[string]int64{}, ConfigSeen: map[string]int64{session.ConfigSeeded: 300},
		Edges: map[string]session.EdgeStats{}, FirstSampleAt: soakTestNow.Add(-8 * 24 * time.Hour),
	}
	for _, edge := range session.ExpectedEdgesForClass(session.ClassOperator) {
		cl.Edges[session.EdgeKey(edge.From, edge.To)] = session.EdgeStats{TransitionSamples: 25}
	}
	return session.ShadowLedger{
		Version: 1, BinaryStarts: []time.Time{soakTestNow.Add(-8 * 24 * time.Hour)},
		Classes: map[string]*session.ClassLedger{string(session.ClassOperator): cl},
	}
}

func soakTestSource(l session.ShadowLedger, err error) *WorkflowEngineSoakSource {
	return &WorkflowEngineSoakSource{
		Ledger: func() (session.ShadowLedger, error) { return l, err },
		Now:    func() time.Time { return soakTestNow }, ReproPath: "/cfg/ledger.json",
	}
}

func getSoak(t *testing.T, src *WorkflowEngineSoakSource, query string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	writeWorkflowEngineSoak(rec, httptest.NewRequest(http.MethodGet, "/actuator/workflow-engine?"+query, nil), src)
	return rec
}

func soakExit(rec *httptest.ResponseRecorder) string {
	return rec.Header().Get("X-Workflow-Engine-Soak-Exit")
}

func TestSoakStatus_ExitsOne_WhenZeroShadowCalls_NoEvidence(t *testing.T) {
	t.Parallel()
	for name, l := range map[string]session.ShadowLedger{
		"fresh ledger":   {Version: 1, Classes: map[string]*session.ClassLedger{}},
		"shadow was off": {Version: 1, BinaryStarts: []time.Time{soakTestNow.Add(-30 * 24 * time.Hour)}, Classes: map[string]*session.ClassLedger{}},
	} {
		rec := getSoak(t, soakTestSource(l, nil), "format=text&class=operator")
		if soakExit(rec) != "1" || !strings.Contains(rec.Body.String(), "no evidence") {
			t.Errorf("%s: exit=%q body=%q, want exit 1 with 'no evidence'", name, soakExit(rec), rec.Body.String())
		}
	}
}

func TestSoakStatus_ExitCodes_ReadyNotReadyUnreachable(t *testing.T) {
	t.Parallel()
	ready := soakReadyLedger()
	if rec := getSoak(t, soakTestSource(ready, nil), "format=text&class=operator"); soakExit(rec) != "0" {
		t.Fatalf("a fully evidenced seeded ledger must be READY, got exit=%q body=%s", soakExit(rec), rec.Body.String())
	}

	readDiv := soakReadyLedger()
	readDiv.Classes["operator"].Divergence["AllowedTransitions|graph|seeded|read"] = 1
	rec := getSoak(t, soakTestSource(readDiv, nil), "format=text&class=operator")
	if soakExit(rec) != "1" || !strings.Contains(rec.Body.String(), "/cfg/ledger.json") {
		t.Errorf("read-path divergence under seeded must alarm (exit 1) and print the repro path, got exit=%q body=%s", soakExit(rec), rec.Body.String())
	}

	edited := soakReadyLedger()
	edited.Classes["operator"].ConfigSeen[session.ConfigEdited] = 3
	var got workflowEngineReport
	rec = getSoak(t, soakTestSource(edited, nil), "class=operator")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Verdicts[0].ConfigWindow != session.ConfigEdited {
		t.Errorf("a window spanning an edit must be labeled edited, got %q", got.Verdicts[0].ConfigWindow)
	}

	stateful := soakReadyLedger()
	stateful.Classes["operator"].Edges["review->done"] = session.EdgeStats{TransitionSamples: 50, StatefulSkips: 50}
	rec = getSoak(t, soakTestSource(stateful, nil), "format=text&class=operator")
	if soakExit(rec) != "1" || !strings.Contains(rec.Body.String(), "review->done UNVERIFIED in shadow") {
		t.Errorf("stateful edges must be listed UNVERIFIED and block READY, got exit=%q body=%s", soakExit(rec), rec.Body.String())
	}

	if rec := getSoak(t, nil, "class=operator"); rec.Code != http.StatusServiceUnavailable || soakExit(rec) != "2" {
		t.Errorf("an unwired actuator must read as unreachable (503, exit 2), got %d exit=%q", rec.Code, soakExit(rec))
	}
	if rec := getSoak(t, soakTestSource(session.ShadowLedger{}, errors.New("ledger read failed")), "class=operator"); soakExit(rec) != "2" {
		t.Errorf("a ledger error is UNAVAILABLE (exit 2), never READY, got %q", soakExit(rec))
	}
}

func TestSoakStatus_ExitsOne_WhenFloorSpanOrExpectedEdgeUnmet(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*session.ClassLedger, *session.ShadowLedger)
		failure string
	}{
		{"below floor", func(c *session.ClassLedger, _ *session.ShadowLedger) { c.Calls["CanTransition|transition"] = 99 }, "floor is 100"},
		{"span shorter than W", func(c *session.ClassLedger, l *session.ShadowLedger) {
			c.FirstSampleAt = soakTestNow.Add(-24 * time.Hour)
			l.BinaryStarts = []time.Time{soakTestNow.Add(-24 * time.Hour)}
		}, "window W is 168h"},
		{"expected edge below N", func(c *session.ClassLedger, _ *session.ShadowLedger) {
			c.Edges["ready->queued"] = session.EdgeStats{TransitionSamples: 19}
		}, "expected edge ready->queued has 19 of 20"},
	}
	for _, tc := range cases {
		l := soakReadyLedger()
		tc.mutate(l.Classes["operator"], &l)
		rec := getSoak(t, soakTestSource(l, nil), "format=text&class=operator")
		if soakExit(rec) != "1" || !strings.Contains(rec.Body.String(), tc.failure) {
			t.Errorf("%s: exit=%q, want 1 naming %q; body=%s", tc.name, soakExit(rec), tc.failure, rec.Body.String())
		}
	}

	l := soakReadyLedger()
	l.Classes["operator"].Edges["ready->queued"] = session.EdgeStats{TransitionSamples: 19}
	src := soakTestSource(l, nil)
	src.Acks = func() session.SoakAcks { return session.SoakAcks{Edges: []string{"ready->queued"}} }
	if rec := getSoak(t, src, "format=text&class=operator"); soakExit(rec) != "0" {
		t.Errorf("the same edge, acknowledged by the owner, must pass; exit=%q body=%s", soakExit(rec), rec.Body.String())
	}
}

func TestSoakStatus_ReportsPerClassVerdicts_ForOperatorAgentSystem(t *testing.T) {
	t.Parallel()
	var got workflowEngineReport
	rec := getSoak(t, soakTestSource(soakReadyLedger(), nil), "")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	classes := make([]string, 0, len(got.Verdicts))
	for _, v := range got.Verdicts {
		classes = append(classes, string(v.Class)+"="+string(v.Status))
	}
	want := "operator=READY,agent=NOT_READY,system=NOT_READY"
	if strings.Join(classes, ",") != want {
		t.Errorf("per-class verdicts = %v, want %s", classes, want)
	}
	if soakExit(rec) != "1" {
		t.Errorf("overall exit with an unevidenced class must be 1, got %q", soakExit(rec))
	}
	text := getSoak(t, soakTestSource(soakReadyLedger(), nil), "format=text").Body.String()
	if strings.Count(text, "workflow engine soak: class=") != 3 {
		t.Errorf("text must carry one verdict line per class:\n%s", text)
	}
}

func TestSoakStatusTarget_ReportsReviewLine_FromRealLedgerThroughActuator(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rec := session.NewShadowLedgerRecorder(dir, session.WithShadowClock(fixedClock{soakTestNow.Add(-20 * 24 * time.Hour)}))
	rec.RecordShadowCall(session.MethodCanTransition, session.PathTransition)
	if err := rec.Flush(); err != nil {
		t.Fatal(err)
	}
	src := &WorkflowEngineSoakSource{
		Ledger: func() (session.ShadowLedger, error) {
			return session.ReadShadowLedgerFile(session.ShadowLedgerPath(dir))
		},
		Now:       func() time.Time { return soakTestNow },
		ReproPath: session.ShadowLedgerPath(dir),
	}

	body := getSoak(t, src, "format=text&class=operator").Body.String()

	for _, want := range []string{"REVIEW DUE 2026-10-26", "REVIEW OVERDUE: no class is enforced; run the Outcome review"} {
		if !strings.Contains(body, want) {
			t.Errorf("text output missing %q:\n%s", want, body)
		}
	}
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newConfigDiffFixture(t *testing.T) (*ent.Client, *session.ConfiguredWorkflowEngine) {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	client := repo.GetEntClient()
	ctx := context.Background()
	if err := session.EnsureBuiltInWorkflowStages(ctx, client); err != nil {
		t.Fatal(err)
	}
	cfg, err := session.NewConfiguredWorkflowEngine(session.NewEntStageConfigRepository(client), session.NewEntGateSatisfactionRepository(client), nil)
	if err != nil {
		t.Fatal(err)
	}
	return client, cfg
}

func setEdge(t *testing.T, client *ent.Client, cfg *session.ConfiguredWorkflowEngine, from, to string, enabled bool) {
	t.Helper()
	n, err := client.StageTransition.Update().Where(
		stagetransition.HasFromStageWith(backlogstage.Slug(from)), stagetransition.HasToStageWith(backlogstage.Slug(to)),
	).SetEnabled(enabled).Save(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("edge %s->%s: n=%d err=%v", from, to, n, err)
	}
	if err := cfg.InvalidateCache(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConfigDiff_ReportsEditedVsSeeded_WhenLiveConfigDiffersFromSeed(t *testing.T) {
	t.Parallel()
	client, cfg := newConfigDiffFixture(t)
	src := soakTestSource(soakReadyLedger(), nil)
	src.ConfigDiff = NewWorkflowEngineConfigDiff(client, cfg)

	var seeded workflowEngineReport
	if err := json.Unmarshal(getSoak(t, src, "class=operator&config_diff=1").Body.Bytes(), &seeded); err != nil {
		t.Fatal(err)
	}
	if seeded.ConfigDiff == nil || seeded.ConfigDiff.Label != "seeded" || len(seeded.StrandedItems) != 0 {
		t.Fatalf("a fresh seed must read seeded with no stranded items, got %+v", seeded.ConfigDiff)
	}

	setEdge(t, client, cfg, "done", "refining", false)
	tr, err := client.StageTransition.Query().Where(
		stagetransition.HasFromStageWith(backlogstage.Slug("ready")), stagetransition.HasToStageWith(backlogstage.Slug("in_progress")),
	).Only(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	const secret = "run-this-secret-command"
	if _, err := client.TransitionGate.Create().SetTransitionID(tr.ID).SetKind("custom").SetStateful(true).SetEnabled(true).
		SetConfig(map[string]interface{}{"cmd": secret}).Save(context.Background()); err != nil {
		t.Fatal(err)
	}

	rec := getSoak(t, src, "format=text&class=operator&config_diff=1")
	body := rec.Body.String()
	for _, want := range []string{"live config: edited", "- edges: done->refining|enabled=true", "+ edges: done->refining|enabled=false", "+ gates: ready->in_progress|kind=custom"} {
		if !strings.Contains(body, want) {
			t.Errorf("config diff missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, secret) || strings.Contains(getSoak(t, src, "class=operator&config_diff=1").Body.String(), secret) {
		t.Error("gate config content must be redacted from the diff")
	}
}

func TestConfigDiff_ListsStrandedItems_WhenOutgoingEdgesDisabled(t *testing.T) {
	t.Parallel()
	client, cfg := newConfigDiffFixture(t)
	ctx := context.Background()
	item, err := client.BacklogItem.Create().SetTitle("secret title").SetStatus("refining").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	intact, err := client.BacklogItem.Create().SetTitle("untouched").SetStatus("idea").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	setEdge(t, client, cfg, "refining", "ready", false)
	src := soakTestSource(soakReadyLedger(), nil)
	src.ConfigDiff = NewWorkflowEngineConfigDiff(client, cfg)

	rec := getSoak(t, src, "class=operator&config_diff=1")

	var got workflowEngineReport
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.StrandedItems) != 1 {
		t.Fatalf("stranded items = %+v, want exactly the refining item", got.StrandedItems)
	}
	s := got.StrandedItems[0]
	if s.ItemHash != session.HashItemID(item.ID.String()) || s.Status != "refining" || len(s.LostEdges) != 1 || s.LostEdges[0] != "refining->ready" {
		t.Errorf("stranded item = %+v", s)
	}
	if strings.Contains(rec.Body.String(), "secret title") || strings.Contains(rec.Body.String(), item.ID.String()) || strings.Contains(rec.Body.String(), intact.ID.String()) {
		t.Error("the preview must carry id hashes only, never titles or ids")
	}
	if soakExit(rec) != "1" {
		t.Errorf("a stranded preview blocks the flip: exit=%q", soakExit(rec))
	}
	if n := client.BacklogItem.Query().CountX(ctx); n != 2 {
		t.Errorf("the preview is read-only, item count = %d", n)
	}
}

func TestActuatorWorkflowEngineAndNudgeEndpoints_Return401OnRemoteChain_AndHostGuardRejectsForeignHostOnLocal(t *testing.T) {
	t.Parallel()
	// Only /actuator/workflow-engine exists in this story; the nudge-outcomes
	// endpoint named in the test title arrives with its own story and is added
	// to the paths slice there.
	paths := []string{"/actuator/workflow-engine"}
	srv, _ := newChainTestServer(t, "localhost:8543")
	srv.registerActuatorRoutes()
	srv.SetWorkflowEngineSoakSource(soakTestSource(soakReadyLedger(), nil))
	get := func(h http.Handler, host, path string) int {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	remote := srv.remoteChain(middleware.Auth(&typedNilValidator{}), true) // validator rejects every token
	for _, p := range paths {
		if got := get(remote, "onyx.lan:8444", p); got != http.StatusUnauthorized {
			t.Errorf("remote %s without credentials = %d, want 401 (handler must not be reached)", p, got)
		}
		if got := get(srv.localChain(), "evil.example.com", p); got != http.StatusForbidden {
			t.Errorf("local %s with a foreign Host = %d, want 403 from HostGuard", p, got)
		}
		if got := get(srv.localChain(), "localhost:8543", p); got != http.StatusOK {
			t.Errorf("local %s with the real Host = %d, want 200", p, got)
		}
	}
}

func TestSoakStatus_EvaluatesEachClassFromItsOwnLedgerSection_AndExposesGuardNewPerClass(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rec := session.NewShadowLedgerRecorder(dir, session.WithShadowClock(fixedClock{soakTestNow.Add(-1 * time.Hour)}))
	rec.RecordShadowCallFor(session.ClassAgent, session.MethodValidateGates, session.PathTransition)
	rec.RecordDivergenceFor(session.ClassAgent, session.MethodValidateGates, session.ClassGuardNew, session.ConfigSeeded, session.PathTransition)
	rec.RecordObservation(session.ShadowObservation{
		Method: session.MethodValidateGates, Path: session.PathTransition, From: session.BacklogStatusReady, To: session.BacklogStatusInProgress,
		Class: session.ClassGuardNew, Config: session.ConfigSeeded, CallerClass: session.ClassAgent, Caller: session.CallerMCPResumeWork,
	})
	if err := rec.Flush(); err != nil {
		t.Fatal(err)
	}
	src := &WorkflowEngineSoakSource{
		Ledger: func() (session.ShadowLedger, error) {
			return session.ReadShadowLedgerFile(session.ShadowLedgerPath(dir))
		},
		Now: func() time.Time { return soakTestNow },
	}

	agent := getSoak(t, src, "format=text&class=agent").Body.String()
	if strings.Contains(agent, "no evidence") || !strings.Contains(agent, "class=agent") {
		t.Errorf("the agent class must be scored from its own section:\n%s", agent)
	}
	operator := getSoak(t, src, "format=text&class=operator").Body.String()
	if !strings.Contains(operator, "no evidence") {
		t.Errorf("the operator class has no samples and must say so:\n%s", operator)
	}

	var report workflowEngineReport
	if err := json.Unmarshal(getSoak(t, src, "").Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range report.DivergenceTotal {
		found = found || (s.Class == "agent" && s.Divergence == "guard_new" && s.Value == 1)
	}
	if !found {
		t.Errorf("divergence_total must carry agent guard_new=1, got %+v", report.DivergenceTotal)
	}
}

func TestSoakStatus_AckFilePostedWithTheRequest_AcknowledgesPerClass_AndBadFileIs400(t *testing.T) {
	t.Parallel()
	l := soakReadyLedger()
	l.Classes["operator"].Divergence["ValidateGates|guard_new|seeded|transition"] = 2
	l.Classes["operator"].CallerEdgeDivergence = map[string]int64{"operator_submit_manual_review|review->done|guard_new": 2}
	src := soakTestSource(l, nil)

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/actuator/workflow-engine?format=text&class=operator", strings.NewReader(body))
		writeWorkflowEngineSoak(rec, req, src)
		return rec
	}
	if rec := getSoak(t, src, "format=text&class=operator"); soakExit(rec) != "1" || !strings.Contains(rec.Body.String(), "guard_new on operator_submit_manual_review|review->done") {
		t.Fatalf("unacknowledged guard_new must be NOT READY, got exit=%q body=%s", soakExit(rec), rec.Body.String())
	}
	if rec := post(`{"classes":{"operator":{"guard_new":["operator_submit_manual_review|review->done"]}}}`); soakExit(rec) != "0" {
		t.Errorf("the acknowledged triple must pass, got exit=%q body=%s", soakExit(rec), rec.Body.String())
	}
	if rec := post(`{"classes":{"agent":{"guard_new":["operator_submit_manual_review|review->done"]}}}`); soakExit(rec) != "1" {
		t.Errorf("an ack recorded under another class must not apply, got exit=%q", soakExit(rec))
	}
	if rec := post(`{"classes":{"operator":{"guard_nwe":[]}}}`); rec.Code != http.StatusBadRequest || soakExit(rec) != "2" {
		t.Errorf("a malformed ack file must be 400 with exit 2, got code=%d exit=%q", rec.Code, soakExit(rec))
	}
}
