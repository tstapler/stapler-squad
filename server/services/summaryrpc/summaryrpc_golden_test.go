package summaryrpc

// Golden differential: the transcript below is recorded from the pre-move
// handlers (package services, origin/main) by running this same file with only
// the package clause changed, with UPDATE_SUMMARYRPC_GOLDEN=1. The moved leaf
// must reproduce it byte for byte.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/handoffsummary"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/summaries"
)

const goldenPath = "testdata/summaryrpc.golden"

// goldenSentinel is a secret-shaped token placed in session content so the
// transcript proves what reaches the AI client.
const goldenSentinel = "sk-ant-api03-GOLDENSENTINEL0123456789abcdefABCDEF"

type goldenLog struct{ b strings.Builder }

func (g *goldenLog) section(name string) { fmt.Fprintf(&g.b, "== %s\n", name) }
func (g *goldenLog) line(format string, a ...any) {
	fmt.Fprintf(&g.b, format, a...)
	g.b.WriteString("\n")
}

// msg renders a proto message as key-sorted compact JSON; maskTimes replaces
// every *At timestamp that depends on the wall clock.
func (g *goldenLog) msg(label string, m proto.Message, maskTimes bool) {
	if m == nil || !m.ProtoReflect().IsValid() {
		g.line("%s: <nil>", label)
		return
	}
	raw, err := protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}.Marshal(m)
	if err != nil {
		g.line("%s: marshal error %v", label, err)
		return
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		g.line("%s: unmarshal error %v", label, err)
		return
	}
	if maskTimes {
		goldenMask(v)
	}
	out, _ := json.Marshal(v)
	g.line("%s: %s", label, out)
}

func goldenMask(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if strings.HasSuffix(k, "_at") && val != nil {
				x[k] = "<time>"
				continue
			}
			goldenMask(val)
		}
	case []any:
		for _, e := range x {
			goldenMask(e)
		}
	}
}

func (g *goldenLog) err(label string, err error) {
	if err == nil {
		g.line("%s: err=<nil>", label)
		return
	}
	g.line("%s: code=%s msg=%q", label, connect.CodeOf(err), err.Error())
}

var goldenT0 = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func goldenSessionStatuses() []string {
	return []string{"pending", "generating", "ready", "error", "", "bogus", "READY"}
}

func TestSummaryRPC_Golden(t *testing.T) {
	svctest.WithIsolatedStateDirPath(t, t.TempDir())
	svctest.WithFakeHomePath(t, t.TempDir())

	g := &goldenLog{}
	ctx := context.Background()

	// ---- SessionSummaryService.GetSessionSummary ----
	g.section("session get")
	{
		entClient := newTestSessionSummaryEntClient(t)
		pool := &fakePoolClient{response: "narr"}
		gen := summaries.NewSessionSummaryGenerator(entClient, pool, nil, nil, nil)
		svc := NewSessionSummaryService(gen, nil)

		resp, err := svc.GetSessionSummary(ctx, connect.NewRequest(&sessionv1.GetSessionSummaryRequest{SessionId: "missing"}))
		g.err("missing", err)
		g.msg("missing", resp.Msg, false)

		for i, st := range goldenSessionStatuses() {
			id := fmt.Sprintf("sess-status-%d", i)
			_, cerr := entClient.SessionSummary.Create().SetID("row-" + id).SetSessionID(id).SetSessionTitle("t-" + st).SetStatus(st).Save(ctx)
			require.NoError(t, cerr)
			resp, err = svc.GetSessionSummary(ctx, connect.NewRequest(&sessionv1.GetSessionSummaryRequest{SessionId: id}))
			g.err("status="+st, err)
			g.msg("status="+st, resp.Msg, false)
		}

		start, stop := goldenT0, goldenT0.Add(90*time.Second)
		_, cerr := entClient.SessionSummary.Create().SetID("row-full").SetSessionID("sess-full").SetSessionTitle("full").
			SetStatus("ready").SetNarrative("narrative text").SetNarrativeFallbackUsed(true).
			SetDiffFilesChanged(3).SetDiffAdded(40).SetDiffRemoved(5).
			SetDecisionsAutoApproved(1).SetDecisionsManuallyApproved(2).SetDecisionsDenied(3).
			SetDecisionsReviewQueueResolved(4).SetDecisionsStillOpen(5).
			SetSessionStartedAt(start).SetSessionStoppedAt(stop).SetDurationMs(90000).
			SetTotalTokens(12345).SetEstimatedCostUsd(1.25).SetCostDataUnavailable(true).
			SetMarkdown("# md").SetErrorMessage("em").SetErrorStage("es").
			SetGenerationStartedAt(start).SetGeneratedAt(stop).Save(ctx)
		require.NoError(t, cerr)
		resp, err = svc.GetSessionSummary(ctx, connect.NewRequest(&sessionv1.GetSessionSummaryRequest{SessionId: "sess-full"}))
		g.err("full", err)
		g.msg("full", resp.Msg, false)

		_, cerr = entClient.SessionSummary.Create().SetID("row-stale").SetSessionID("sess-stale").SetSessionTitle("stale").
			SetStatus("generating").SetGenerationStartedAt(time.Now().Add(-10 * time.Minute)).Save(ctx)
		require.NoError(t, cerr)
		resp, err = svc.GetSessionSummary(ctx, connect.NewRequest(&sessionv1.GetSessionSummaryRequest{SessionId: "sess-stale"}))
		g.err("stale", err)
		g.msg("stale", resp.Msg, true)

		_, cerr = entClient.SessionSummary.Create().SetID("row-fresh").SetSessionID("sess-fresh").SetSessionTitle("fresh").
			SetStatus("generating").SetGenerationStartedAt(time.Now()).Save(ctx)
		require.NoError(t, cerr)
		resp, err = svc.GetSessionSummary(ctx, connect.NewRequest(&sessionv1.GetSessionSummaryRequest{SessionId: "sess-fresh"}))
		g.err("fresh", err)
		g.msg("fresh", resp.Msg, true)

		require.NoError(t, entClient.Close())
		resp, err = svc.GetSessionSummary(ctx, connect.NewRequest(&sessionv1.GetSessionSummaryRequest{SessionId: "sess-full"}))
		g.err("closed-db", err)
		g.line("closed-db resp nil: %v", resp == nil)
	}

	// ---- SessionSummaryService.RegenerateSessionSummary ----
	g.section("session regenerate")
	{
		entClient := newTestSessionSummaryEntClient(t)
		pool := &fakePoolClient{response: "regen narrative"}
		gen := summaries.NewSessionSummaryGenerator(entClient, pool, nil, nil, nil)

		svc := NewSessionSummaryService(gen, nil)
		resp, err := svc.RegenerateSessionSummary(ctx, connect.NewRequest(&sessionv1.RegenerateSessionSummaryRequest{SessionId: "nothing"}))
		g.err("nil finder, no row", err)
		g.line("resp nil: %v", resp == nil)

		svc = NewSessionSummaryService(gen, &fakeLiveInstanceFinder{})
		_, err = svc.RegenerateSessionSummary(ctx, connect.NewRequest(&sessionv1.RegenerateSessionSummaryRequest{SessionId: "nothing"}))
		g.err("empty finder, no row", err)

		// Live instance, no row: PENDING synthesized response, prompt carries
		// the live diff content and goal.
		live := &instance.Instance{Title: "live title", UUID: "sess-live", CreatedAt: time.Now().Add(-time.Hour)}
		live.SetSessionGoalCached(&instance.SessionGoalData{Goal: "ship it " + goldenSentinel})
		svc = NewSessionSummaryService(gen, &fakeLiveInstanceFinder{instances: map[string]*instance.Instance{"sess-live": live}})
		resp, err = svc.RegenerateSessionSummary(ctx, connect.NewRequest(&sessionv1.RegenerateSessionSummaryRequest{SessionId: "sess-live"}))
		g.err("live no row", err)
		g.msg("live no row", resp.Msg, false)
		row := waitForStatus(t, entClient, "sess-live", 5*time.Second, summaries.SessionSummaryStatusReady, summaries.SessionSummaryStatusError)
		g.line("live final: status=%s title=%q added=%d removed=%d files=%d fallback=%v narrative=%q err=%q/%q",
			row.Status, row.SessionTitle, row.DiffAdded, row.DiffRemoved, row.DiffFilesChanged, row.NarrativeFallbackUsed, row.Narrative, row.ErrorStage, row.ErrorMessage)
		g.line("live prompt:\n%s", pool.lastUserPrompt())
		g.line("live pool calls: %d", pool.callCount())

		// Row, no live instance: persisted-field fallback.
		start := goldenT0
		_, cerr := entClient.SessionSummary.Create().SetID("row-persisted").SetSessionID("sess-persisted").SetSessionTitle("persisted title").
			SetStatus("ready").SetNarrative("old narrative").SetDiffFilesChanged(2).SetDiffAdded(10).SetDiffRemoved(4).
			SetSessionStartedAt(start).SetGeneratedAt(time.Now().Add(-time.Hour)).Save(ctx)
		require.NoError(t, cerr)
		before := time.Now()
		resp, err = svc.RegenerateSessionSummary(ctx, connect.NewRequest(&sessionv1.RegenerateSessionSummaryRequest{SessionId: "sess-persisted"}))
		g.err("persisted row", err)
		g.msg("persisted row", resp.Msg, true)
		row = waitForNewGeneratedRow(t, entClient, "sess-persisted", before, 5*time.Second)
		g.line("persisted final: status=%s title=%q added=%d removed=%d files=%d started=%v narrative=%q",
			row.Status, row.SessionTitle, row.DiffAdded, row.DiffRemoved, row.DiffFilesChanged, row.SessionStartedAt != nil && row.SessionStartedAt.Equal(start), row.Narrative)
		g.line("persisted prompt:\n%s", pool.lastUserPrompt())

		require.NoError(t, entClient.Close())
		_, err = svc.RegenerateSessionSummary(ctx, connect.NewRequest(&sessionv1.RegenerateSessionSummaryRequest{SessionId: "sess-persisted"}))
		g.err("closed-db", err)
	}

	// ---- HandoffSummaryService.GetHandoffSummary ----
	g.section("handoff get")
	{
		entClient := newTestHandoffSummaryEntClient(t)
		gen := summaries.NewHandoffSummaryGenerator(entClient, &fakePoolClient{response: "handoff text"})
		svc := NewHandoffSummaryService(gen)

		resp, err := svc.GetHandoffSummary(ctx, connect.NewRequest(&sessionv1.GetHandoffSummaryRequest{SessionId: "missing"}))
		g.err("missing", err)
		g.msg("missing", resp.Msg, false)

		for i, st := range goldenSessionStatuses() {
			id := fmt.Sprintf("hs-status-%d", i)
			_, cerr := entClient.HandoffSummary.Create().SetID("row-" + id).SetSessionID(id).SetSessionTitle("t-" + st).SetStatus(st).Save(ctx)
			require.NoError(t, cerr)
			resp, err = svc.GetHandoffSummary(ctx, connect.NewRequest(&sessionv1.GetHandoffSummaryRequest{SessionId: id}))
			g.err("status="+st, err)
			g.msg("status="+st, resp.Msg, false)
		}

		_, cerr := entClient.HandoffSummary.Create().SetID("row-full").SetSessionID("hs-full").SetSessionTitle("full").
			SetStatus("ready").SetActiveTask("task").SetSummaryText("text").SetMiddleMessagesSummarized(17).
			SetErrorStage("es").SetErrorMessage("em").SetGenerationStartedAt(goldenT0).SetGeneratedAt(goldenT0.Add(time.Minute)).Save(ctx)
		require.NoError(t, cerr)
		resp, err = svc.GetHandoffSummary(ctx, connect.NewRequest(&sessionv1.GetHandoffSummaryRequest{SessionId: "hs-full"}))
		g.err("full", err)
		g.msg("full", resp.Msg, false)

		_, cerr = entClient.HandoffSummary.Create().SetID("row-stale").SetSessionID("hs-stale").SetSessionTitle("stale").
			SetStatus("generating").SetGenerationStartedAt(time.Now().Add(-10 * time.Minute)).Save(ctx)
		require.NoError(t, cerr)
		resp, err = svc.GetHandoffSummary(ctx, connect.NewRequest(&sessionv1.GetHandoffSummaryRequest{SessionId: "hs-stale"}))
		g.err("stale", err)
		g.msg("stale", resp.Msg, true)

		require.NoError(t, entClient.Close())
		_, err = svc.GetHandoffSummary(ctx, connect.NewRequest(&sessionv1.GetHandoffSummaryRequest{SessionId: "hs-full"}))
		g.err("closed-db", err)
	}

	// ---- HandoffSummaryService.TriggerHandoffSummary ----
	g.section("handoff trigger")
	{
		stateDir := t.TempDir()
		svctest.WithIsolatedStateDirPath(t, stateDir)
		cfg := filepath.Join(stateDir, "config.json")
		require.NoError(t, os.WriteFile(cfg, []byte(`{"handoff_summary": {"enabled": false}}`), 0o600))

		entClient := newTestHandoffSummaryEntClient(t)
		pool := &fakePoolClient{response: "handoff generated"}
		gen := summaries.NewHandoffSummaryGenerator(entClient, pool)
		svc := NewHandoffSummaryService(gen)

		resp, err := svc.TriggerHandoffSummary(ctx, connect.NewRequest(&sessionv1.TriggerHandoffSummaryRequest{SessionId: "hs-trig"}))
		g.err("disabled", err)
		g.line("disabled resp nil: %v", resp == nil)
		_, findErr := gen.FindRowBySessionID(ctx, "hs-trig")
		g.line("disabled row persisted: %v", findErr == nil)

		require.NoError(t, os.WriteFile(cfg, []byte(`{"handoff_summary": {"enabled": true}}`), 0o600))
		writeHandoffConversationFixture(t, "hs-trig")

		resp, err = svc.TriggerHandoffSummary(ctx, connect.NewRequest(&sessionv1.TriggerHandoffSummaryRequest{SessionId: "hs-trig"}))
		g.err("enabled first", err)
		g.msg("enabled first", resp.Msg, true)
		g.line("enabled first has generation_started_at: %v", resp.Msg.GetSummary().GetGenerationStartedAt() != nil)
		goldenAwaitHandoff(t, entClient, "hs-trig")
		row, _ := gen.FindRowBySessionID(ctx, "hs-trig")
		g.line("first final: status=%s title=%q task=%q text=%q stage=%q msg=%q", row.Status, row.SessionTitle, row.ActiveTask, row.SummaryText, row.ErrorStage, row.ErrorMessage)
		g.line("first prompt:\n%s", goldenNormalizeHandoffPrompt(pool.lastUserPrompt()))

		goldenAwaitReleased(t, gen, "hs-trig")
		// Existing row title is reused on the second trigger.
		_, uerr := entClient.HandoffSummary.Update().Where(handoffsummary.SessionID("hs-trig")).SetSessionTitle("persisted handoff title").Save(ctx)
		require.NoError(t, uerr)
		resp, err = svc.TriggerHandoffSummary(ctx, connect.NewRequest(&sessionv1.TriggerHandoffSummaryRequest{SessionId: "hs-trig"}))
		g.err("enabled second", err)
		g.msg("enabled second", resp.Msg, true)
		goldenAwaitHandoff(t, entClient, "hs-trig")

		// Missing transcript: still GENERATING in the response, ERROR later.
		resp, err = svc.TriggerHandoffSummary(ctx, connect.NewRequest(&sessionv1.TriggerHandoffSummaryRequest{SessionId: "hs-no-transcript"}))
		g.err("no transcript", err)
		g.msg("no transcript", resp.Msg, true)
		goldenAwaitHandoff(t, entClient, "hs-no-transcript")
		row, _ = gen.FindRowBySessionID(ctx, "hs-no-transcript")
		g.line("no transcript final: status=%s stage=%q msg=%q", row.Status, row.ErrorStage, row.ErrorMessage)

		// In-flight dedup: the second call reads the persisted row.
		blocking := &fakePoolClient{block: true, started: make(chan struct{}, 1)}
		gen2 := summaries.NewHandoffSummaryGenerator(entClient, blocking)
		svc2 := NewHandoffSummaryService(gen2)
		writeHandoffConversationFixture(t, "hs-inflight")
		bctx, cancel := context.WithCancel(ctx)
		release, startedAt, started, berr := gen2.BeginGeneration(bctx, "hs-inflight", "inflight title")
		require.NoError(t, berr)
		require.True(t, started)
		done := make(chan struct{})
		go func() {
			defer close(done)
			gen2.GenerateAndPersist(bctx, "hs-inflight", "inflight title", release, startedAt)
		}()
		<-blocking.started
		resp, err = svc2.TriggerHandoffSummary(ctx, connect.NewRequest(&sessionv1.TriggerHandoffSummaryRequest{SessionId: "hs-inflight"}))
		g.err("in-flight", err)
		g.msg("in-flight", resp.Msg, true)
		cancel()
		<-done

		require.NoError(t, entClient.Close())
		_, err = svc2.TriggerHandoffSummary(ctx, connect.NewRequest(&sessionv1.TriggerHandoffSummaryRequest{SessionId: "hs-closed"}))
		g.err("closed-db", err)
	}

	got := g.b.String()
	if os.Getenv("UPDATE_SUMMARYRPC_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}

// goldenAwaitReleased waits for the generator's dedup guard: the row turns ready before the
// deferred release runs, and a trigger in that gap is deduplicated and re-reads the old row.
func goldenAwaitReleased(t *testing.T, gen *summaries.HandoffSummaryGenerator, sessionID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		release, _, started, err := gen.BeginGeneration(context.Background(), sessionID, sessionID)
		if started {
			release()
		}
		return err == nil && started
	}, 5*time.Second, time.Millisecond)
}

func goldenAwaitHandoff(t *testing.T, client *ent.Client, sessionID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		row, err := client.HandoffSummary.Query().Where(handoffsummary.SessionID(sessionID)).Only(context.Background())
		return err == nil && (row.Status == "ready" || row.Status == "error")
	}, 5*time.Second, 10*time.Millisecond)
}

// goldenNormalizeHandoffPrompt blanks the temp-dir path the fixture embeds.
func goldenNormalizeHandoffPrompt(p string) string {
	lines := strings.Split(p, "\n")
	for i, l := range lines {
		if strings.Contains(l, "/tmp") || strings.Contains(l, "TestSummaryRPC") {
			lines[i] = "<path line>"
		}
	}
	return strings.Join(lines, "\n")
}
