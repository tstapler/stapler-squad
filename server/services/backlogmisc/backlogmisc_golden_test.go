package backlogmisc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/headless"
)

// The golden was recorded from the pre-move code in server/services (origin/main) with
// BACKLOGMISC_GOLDEN_RECORD=1; it names only identifiers present in both packages, so the
// same file replays against either. Every line is something the four moved files send to
// a client (status, headers, bodies, RPC codes and chunks, published bus events) or write
// to the backlog store (item rows after each debug mutation), or hand to the headless
// runner (args and stdin): any diff is a behaviour change.
const bmGoldenPath = "testdata/backlogmisc.golden"

func bmSection(g *strings.Builder, name string) {
	fmt.Fprintf(g, "== %s\n", name)
}

// ---- backlog item event publisher ---------------------------------------------------

func bmEventLine(ev *events.Event) string {
	p := ev.BacklogItemPayload
	if p == nil {
		return fmt.Sprintf("type=%s payload=nil", ev.Type)
	}
	itemID := "nil"
	if p.Item != nil {
		itemID = p.Item.ID + "/" + p.Item.Title
	}
	archived := "nil"
	if p.ArchivedAt != nil {
		archived = p.ArchivedAt.UTC().Format(time.RFC3339Nano)
		if d := time.Since(*p.ArchivedAt); d > -5*time.Minute && d < 5*time.Minute {
			archived = "now" // the store stamps the archive time itself
		}
	}
	verdict, note := "nil", "nil"
	if p.Verdict != nil {
		verdict = fmt.Sprintf("%+v", *p.Verdict)
	}
	if p.ActivityNote != nil {
		note = fmt.Sprintf("%+v", *p.ActivityNote)
	}
	return fmt.Sprintf("type=%s kind=%s item=%s old=%q new=%q fields=%q session=%q archived=%s removed=%q verdict=%s claimant=%q note=%s",
		ev.Type, p.Kind, itemID, p.OldStatus, p.NewStatus, p.UpdatedFields, p.SessionID, archived, p.RemovedReason, verdict, p.ClaimantHostID, note)
}

func bmDrain(ch <-chan *events.Event) []*events.Event {
	var out []*events.Event
	for {
		select {
		case ev := <-ch:
			out = append(out, ev)
		case <-time.After(50 * time.Millisecond):
			return out
		}
	}
}

func bmGoldenPublisher(t *testing.T, g *strings.Builder) {
	bmSection(g, "publisher")
	when := time.Date(2026, 11, 25, 1, 2, 3, 4, time.UTC)
	kinds := []backlogdto.BacklogChangeKind{
		backlogdto.ChangeStatusTransition, backlogdto.ChangeVerdictRecorded, backlogdto.ChangeSessionAttached,
		backlogdto.ChangeItemUpdated, backlogdto.ChangeItemArchived, backlogdto.ChangeItemRemoved,
		backlogdto.ChangeTriageProgressUpdated, backlogdto.ChangeActivityNoteAdded,
		"not-a-real-kind", "",
	}
	bus := events.NewEventBus(64)
	defer bus.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)
	pub := &BacklogItemEventPublisher{Bus: bus}

	for _, k := range kinds {
		for _, withItem := range []bool{true, false} {
			var item *backlogdto.BacklogItemData
			if withItem {
				item = &backlogdto.BacklogItemData{ID: "item-1", Title: "t-" + string(k)}
			}
			pub.PublishItemChanged(item, backlogdto.BacklogItemChange{
				Kind: k, OldStatus: "review", NewStatus: "done", UpdatedFields: []string{"title", "description"},
				SessionID: "sess-9", ArchivedAt: &when, RemovedReason: "dup", ClaimantHostID: "host-1",
				Verdict:      &backlogdto.ReviewVerdictData{ItemSessionID: "is-1", OverallOutcome: domain.ReviewOutcome("pass"), Summary: "ok", DiffTokenCount: 7, OverrideAt: &when},
				ActivityNote: &backlogdto.ActivityNoteData{ID: "n-1", Message: "note", AuthorSessionUUID: "u", AuthorSessionTitle: "T", CreatedAt: when},
			})
			evs := bmDrain(ch)
			fmt.Fprintf(g, "kind=%q item=%v delivered=%d\n", k, withItem, len(evs))
			for _, ev := range evs {
				fmt.Fprintf(g, "  %s\n", bmEventLine(ev))
			}
		}
	}

	pub.PublishItemChanged(&backlogdto.BacklogItemData{ID: "z"}, backlogdto.BacklogItemChange{Kind: backlogdto.ChangeItemUpdated})
	for _, ev := range bmDrain(ch) {
		fmt.Fprintf(g, "zero-change %s\n", bmEventLine(ev))
	}

	// nil bus, nil receiver and a closed bus never panic and deliver nothing.
	(&BacklogItemEventPublisher{}).PublishItemChanged(&backlogdto.BacklogItemData{ID: "a"}, backlogdto.BacklogItemChange{Kind: backlogdto.ChangeItemUpdated})
	var nilPub *BacklogItemEventPublisher
	nilPub.PublishItemChanged(nil, backlogdto.BacklogItemChange{Kind: backlogdto.ChangeItemUpdated})
	closed := events.NewEventBus(4)
	closed.Close()
	(&BacklogItemEventPublisher{Bus: closed}).PublishItemChanged(nil, backlogdto.BacklogItemChange{Kind: backlogdto.ChangeItemUpdated})
	fmt.Fprintf(g, "nil-bus, nil-receiver and closed-bus publishes returned without panic; stray=%d\n", len(bmDrain(ch)))

	// Two independent buses never cross-deliver.
	busB := events.NewEventBus(8)
	defer busB.Close()
	chB, _ := busB.Subscribe(ctx)
	pub.PublishItemChanged(&backlogdto.BacklogItemData{ID: "only-a"}, backlogdto.BacklogItemChange{Kind: backlogdto.ChangeItemUpdated})
	fmt.Fprintf(g, "bus A got %d, bus B got %d\n", len(bmDrain(ch)), len(bmDrain(chB)))
}

// ---- backlog debug mutate handler -----------------------------------------------------

type bmDebugEnv struct {
	storage *session.Storage
	mux     *http.ServeMux
	bus     *events.EventBus
	ch      <-chan *events.Event
	ids     map[string]string
}

func (e *bmDebugEnv) norm(s string) string {
	for id, name := range e.ids {
		s = strings.ReplaceAll(s, id, name)
	}
	return s
}

func (e *bmDebugEnv) do(g *strings.Builder, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == "" && method != http.MethodPost {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	e.mux.ServeHTTP(rec, req)
	respBody := rec.Body.String()
	if path == "/api/debug/backlog/mutate-create" && rec.Code == http.StatusOK {
		// Create responses carry random ids; record the shape and learn the id for later steps.
		var itemID, publicID string
		if i := strings.Index(respBody, `"itemId":"`); i >= 0 {
			itemID = respBody[i+len(`"itemId":"`):]
			itemID = itemID[:strings.Index(itemID, `"`)]
		}
		if i := strings.Index(respBody, `"publicId":"`); i >= 0 {
			publicID = respBody[i+len(`"publicId":"`):]
			publicID = publicID[:strings.Index(publicID, `"`)]
		}
		name := fmt.Sprintf("<item%d>", len(e.ids)+1)
		e.ids[itemID] = name
		respBody = fmt.Sprintf("{itemId:%s publicId-present:%v raw-has-publicId-key:%v}", name, publicID != "", strings.Contains(rec.Body.String(), "publicId"))
	}
	fmt.Fprintf(g, "%s %s %q -> %d ct=%q allow=%q body=%q\n", method, path, e.norm(body), rec.Code,
		rec.Header().Get("Content-Type"), rec.Header().Get("Allow"), e.norm(respBody))
	return rec
}

func (e *bmDebugEnv) itemID(name string) string {
	for id, n := range e.ids {
		if n == name {
			return id
		}
	}
	return "missing-" + name
}

func (e *bmDebugEnv) dumpItems(g *strings.Builder) {
	items, err := e.storage.ListBacklogItems(context.Background(), backlogdto.BacklogItemFilter{})
	if err != nil {
		fmt.Fprintf(g, "  list error: %v\n", err)
		return
	}
	lines := make([]string, 0, len(items))
	for _, it := range items {
		lines = append(lines, fmt.Sprintf("  row %s status=%q title=%q desc=%q prio=%d repo=%q ac=%s archivedAtSet=%v",
			e.ids[it.ID], it.Status, it.Title, it.Description, it.Priority, it.RepoPath, string(it.AcceptanceCriteria), it.ArchivedAt != nil))
	}
	sort.Strings(lines)
	fmt.Fprintf(g, "  rows=%d\n", len(items))
	for _, l := range lines {
		fmt.Fprintln(g, l)
	}
	for _, ev := range bmDrain(e.ch) {
		line := bmEventLine(ev)
		line = e.norm(line)
		fmt.Fprintf(g, "  event %s\n", line)
	}
}

func bmGoldenDebugMutate(t *testing.T, g *strings.Builder) {
	bmSection(g, "debug-mutate")
	storage := svctest.CreateTestStorage(t)
	bus := events.NewEventBus(64)
	t.Cleanup(bus.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, _ := bus.Subscribe(ctx)
	storage.SetItemChangePublisher(&BacklogItemEventPublisher{Bus: bus})

	h := NewBacklogDebugMutateHandler(storage)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	e := &bmDebugEnv{storage: storage, mux: mux, bus: bus, ch: ch, ids: map[string]string{}}
	routes := []string{"create", "transition", "update", "archive", "delete"}

	// Method gating on every route, before any body is read.
	for _, r := range routes {
		for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead, http.MethodOptions} {
			e.do(g, m, "/api/debug/backlog/mutate-"+r, "")
		}
	}
	e.do(g, http.MethodPost, "/api/debug/backlog/mutate-unknown", "{}")

	// Body validation on every route.
	for _, r := range routes {
		for _, body := range []string{"", "not json", "[]", "{}", `{"itemId":""}`, `{"title":""}`, `{"unknownField":1}`, `{"itemId":5}`} {
			e.do(g, http.MethodPost, "/api/debug/backlog/mutate-"+r, body)
		}
	}
	e.dumpItems(g)

	// Create: defaults, every field, a status the lifecycle never allows, ac criteria.
	e.do(g, http.MethodPost, "/api/debug/backlog/mutate-create", `{"title":"plain"}`)
	e.dumpItems(g)
	e.do(g, http.MethodPost, "/api/debug/backlog/mutate-create", `{"title":"full","description":"d","status":"review","priority":1,"repoPath":"/r","acCriteria":["one","two"]}`)
	e.dumpItems(g)
	e.do(g, http.MethodPost, "/api/debug/backlog/mutate-create", `{"title":"weird status","status":"nonsense","priority":-2}`)
	e.dumpItems(g)
	e.do(g, http.MethodPost, "/api/debug/backlog/mutate-create", `{"title":"done directly","status":"done","acCriteria":[]}`)
	e.do(g, http.MethodPost, "/api/debug/backlog/mutate-create", `{"title":"unicode é世","description":"line1\nline2","priority":9999}`)
	e.dumpItems(g)

	plain, full, weird := e.itemID("<item1>"), e.itemID("<item2>"), e.itemID("<item3>")

	// Transition: forced edges, same status, archived, unknown status, unknown item.
	for _, target := range []string{"in_progress", "done", "idea", "archived", "review", "bogus", "pr_pending"} {
		e.do(g, http.MethodPost, "/api/debug/backlog/mutate-transition", fmt.Sprintf(`{"itemId":%q,"targetStatus":%q}`, plain, target))
		e.dumpItems(g)
	}
	e.do(g, http.MethodPost, "/api/debug/backlog/mutate-transition", `{"itemId":"no-such-item","targetStatus":"done"}`)
	e.do(g, http.MethodPost, "/api/debug/backlog/mutate-transition", fmt.Sprintf(`{"itemId":%q}`, plain))
	e.do(g, http.MethodPost, "/api/debug/backlog/mutate-transition", `{"targetStatus":"done"}`)

	// Update: title only, description only, both, neither, null, empty string.
	for _, body := range []string{
		fmt.Sprintf(`{"itemId":%q,"title":"renamed"}`, full),
		fmt.Sprintf(`{"itemId":%q,"description":"new desc"}`, full),
		fmt.Sprintf(`{"itemId":%q,"title":"both","description":"both d"}`, full),
		fmt.Sprintf(`{"itemId":%q}`, full),
		fmt.Sprintf(`{"itemId":%q,"title":null,"description":null}`, full),
		fmt.Sprintf(`{"itemId":%q,"description":""}`, full),
		`{"itemId":"no-such-item","title":"x"}`,
	} {
		e.do(g, http.MethodPost, "/api/debug/backlog/mutate-update", body)
		e.dumpItems(g)
	}

	// Archive and delete, including repeats and unknown ids.
	for _, id := range []string{full, full, "no-such-item"} {
		e.do(g, http.MethodPost, "/api/debug/backlog/mutate-archive", fmt.Sprintf(`{"itemId":%q}`, id))
		e.dumpItems(g)
	}
	for _, id := range []string{weird, weird, "no-such-item"} {
		e.do(g, http.MethodPost, "/api/debug/backlog/mutate-delete", fmt.Sprintf(`{"itemId":%q}`, id))
		e.dumpItems(g)
	}

	// The two exported handlers called directly (server/server.go registers through RegisterRoutes).
	rec := httptest.NewRecorder()
	h.HandleTransition(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	fmt.Fprintf(g, "direct HandleTransition GET -> %d\n", rec.Code)
	rec = httptest.NewRecorder()
	h.HandleArchive(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	fmt.Fprintf(g, "direct HandleArchive GET -> %d\n", rec.Code)

	// nil storage: every route 503s after the method check, before the body is read.
	nilMux := http.NewServeMux()
	NewBacklogDebugMutateHandler(nil).RegisterRoutes(nilMux)
	ne := &bmDebugEnv{mux: nilMux, ids: map[string]string{}}
	for _, r := range routes {
		ne.do(g, http.MethodPost, "/api/debug/backlog/mutate-"+r, `{"title":"x","itemId":"y","targetStatus":"done"}`)
		ne.do(g, http.MethodPost, "/api/debug/backlog/mutate-"+r, "not json")
		ne.do(g, http.MethodGet, "/api/debug/backlog/mutate-"+r, "")
	}
}

// ---- headless service -------------------------------------------------------------------

func bmChunks(stream *connect.ServerStreamForClient[sessionv1.RunHeadlessCallResponse]) string {
	var parts []string
	for stream.Receive() {
		m := stream.Msg()
		parts = append(parts, fmt.Sprintf("{text=%q done=%v err=%v msg=%q cost=%v}", m.Text, m.Done, m.IsError, m.ErrorMessage, m.CostUsd))
	}
	end := "ok"
	if err := stream.Err(); err != nil {
		end = fmt.Sprintf("code=%s msg=%q", connect.CodeOf(err), err.Error())
	}
	return strings.Join(parts, " ") + " end:" + end
}

func bmGoldenHeadless(t *testing.T, g *strings.Builder) {
	bmSection(g, "headless")
	newClient := func(pool *headless.Pool) sessionv1connect.HeadlessServiceClient {
		mux := http.NewServeMux()
		path, handler := sessionv1connect.NewHeadlessServiceHandler(NewHeadlessService(pool))
		mux.Handle(path, handler)
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		return sessionv1connect.NewHeadlessServiceClient(srv.Client(), srv.URL)
	}
	call := func(client sessionv1connect.HeadlessServiceClient, req *sessionv1.RunHeadlessCallRequest) string {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stream, err := client.RunHeadlessCall(ctx, connect.NewRequest(req))
		if err != nil {
			return fmt.Sprintf("call error code=%s msg=%q", connect.CodeOf(err), err.Error())
		}
		defer stream.Close()
		return bmChunks(stream)
	}
	result := func(id, text string) string {
		return `{"type":"result","session_id":"` + id + `","result":"` + text + `","total_cost_usd":0.25}`
	}

	fmt.Fprintf(g, "nil pool: %s\n", call(newClient(nil), &sessionv1.RunHeadlessCallRequest{FeatureKey: "custom", UserPrompt: "u"}))
	fmt.Fprintf(g, "allowed keys: %q\n", headless.AllowedFeatureKeyList())

	scripted := make([]string, 40)
	for i := range scripted {
		scripted[i] = result("s1", fmt.Sprintf("reply-%d", i))
	}
	runner := headless.NewFakeRunner(scripted...)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{}, runner)
	client := newClient(pool)

	keys := []string{"", "custom", "triage", "completion-review", "CUSTOM", " custom", "nope", "review", "ac", "summarize"}
	for _, k := range keys {
		fmt.Fprintf(g, "key %q: %s\n", k, call(client, &sessionv1.RunHeadlessCallRequest{FeatureKey: k, UserPrompt: "hello", SystemPrompt: "sys"}))
	}

	big := func(n int) string { return strings.Repeat("a", n) }
	for _, n := range []int{99_999, 100_000, 100_001} {
		fmt.Fprintf(g, "system_prompt %d: %s\n", n, call(client, &sessionv1.RunHeadlessCallRequest{FeatureKey: "custom", SystemPrompt: big(n), UserPrompt: "u"}))
		fmt.Fprintf(g, "user_prompt %d: %s\n", n, call(client, &sessionv1.RunHeadlessCallRequest{FeatureKey: "custom", UserPrompt: big(n)}))
	}
	for _, ts := range []int32{0, -5, 1, 30, 100000} {
		fmt.Fprintf(g, "timeout %d: %s\n", ts, call(client, &sessionv1.RunHeadlessCallRequest{FeatureKey: "custom", UserPrompt: "t", TimeoutSeconds: ts}))
	}
	for _, raw := range []string{"not json at all", `{"type":"result","is_error":true,"result":"boom"}`, "", `{"type":"result","result":"","total_cost_usd":1.5}`} {
		one := newClient(headless.NewPoolWithRunner(headless.PoolConfig{}, headless.NewFakeRunner(raw)))
		fmt.Fprintf(g, "raw output %q: %s\n", raw, call(one, &sessionv1.RunHeadlessCallRequest{FeatureKey: "custom", UserPrompt: "u"}))
	}
	modelRunner := headless.NewFakeRunner(result("m1", "model reply"))
	modelClient := newClient(headless.NewPoolWithRunner(headless.PoolConfig{}, modelRunner))
	fmt.Fprintf(g, "model override: %s\n", call(modelClient, &sessionv1.RunHeadlessCallRequest{FeatureKey: "custom", UserPrompt: "m", Model: "opus", SystemPrompt: "s"}))
	fmt.Fprintf(g, "model override args: %q\n", modelRunner.ArgsForCall(0))

	// A runner that fails to start surfaces as CodeInternal.
	failing := headless.NewFakeRunner()
	failing.SetErrors(fmt.Errorf("spawn failed"))
	fmt.Fprintf(g, "start error: %s\n", call(newClient(headless.NewPoolWithRunner(headless.PoolConfig{}, failing)), &sessionv1.RunHeadlessCallRequest{FeatureKey: "custom", UserPrompt: "u"}))

	fmt.Fprintf(g, "runner calls=%d\n", runner.CallCount())
	for i := 0; i < runner.CallCount(); i++ {
		args := append([]string(nil), runner.ArgsForCall(i)...)
		for j := range args {
			if j > 0 && (args[j-1] == "--session-id" || args[j-1] == "--resume") {
				args[j] = "<session-id>"
			}
		}
		fmt.Fprintf(g, "  call %d args=%q stdin-len=%d\n", i, args, len(runner.Stdins[i]))
		if len(runner.Stdins[i]) < 200 {
			fmt.Fprintf(g, "    stdin=%q\n", runner.Stdins[i])
		}
	}
}

// ---- LLM backend service -------------------------------------------------------------------

type bmBackend struct {
	name string
	caps headless.Caps
	up   bool
}

func (b *bmBackend) CallBlocking(context.Context, headless.FeatureKey, string, string, headless.CallOptions, headless.CostSink) (string, error) {
	return "", nil
}
func (b *bmBackend) Name() string                { return b.name }
func (b *bmBackend) Capabilities() headless.Caps { return b.caps }
func (b *bmBackend) Available() bool             { return b.up }

func bmSettings(s *sessionv1.LLMBackendSettings) string {
	if s == nil {
		return "nil"
	}
	var maps []string
	for _, m := range s.ModelMaps {
		keys := make([]string, 0, len(m.Aliases))
		for k := range m.Aliases {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var kv []string
		for _, k := range keys {
			kv = append(kv, k+"="+m.Aliases[k])
		}
		maps = append(maps, m.Backend+"{"+strings.Join(kv, ",")+"}")
	}
	pf := make([]string, 0, len(s.PerFeature))
	for k, v := range s.PerFeature {
		pf = append(pf, k+"="+v)
	}
	sort.Strings(pf)
	return fmt.Sprintf("default=%q perFeature=%q consolette=%q anthropic=%q modelMaps=%q", s.DefaultBackend, pf, s.ConsoletteBaseUrl, s.AnthropicBaseUrl, maps)
}

func bmGoldenLLMBackend(t *testing.T, g *strings.Builder) {
	bmSection(g, "llm-backend")
	svctest.WithIsolatedStateDir(t)
	config.ResetLiveLLMBackendsForTest()
	t.Cleanup(config.ResetLiveLLMBackendsForTest)
	ctx := context.Background()

	get := func(svc *LLMBackendService) {
		resp, err := svc.GetLLMBackendSettings(ctx, connect.NewRequest(&sessionv1.GetLLMBackendSettingsRequest{}))
		if err != nil {
			fmt.Fprintf(g, "  get error: %v\n", err)
			return
		}
		fmt.Fprintf(g, "  get settings: %s\n", bmSettings(resp.Msg.Settings))
		fmt.Fprintf(g, "  get featureKeys: %q\n", resp.Msg.FeatureKeys)
		for _, b := range resp.Msg.Backends {
			fmt.Fprintf(g, "  get backend: %+v\n", [6]any{b.Name, b.Available, b.SupportsResume, b.SupportsSystemPrompt, b.SupportsToolRestriction, b.SupportsWorkdir})
		}
		fmt.Fprintf(g, "  get backends=%d\n", len(resp.Msg.Backends))
	}
	update := func(svc *LLMBackendService, label string, s *sessionv1.LLMBackendSettings) {
		resp, err := svc.UpdateLLMBackendSettings(ctx, connect.NewRequest(&sessionv1.UpdateLLMBackendSettingsRequest{Settings: s}))
		if err != nil {
			fmt.Fprintf(g, "update %s: code=%s msg=%q\n", label, connect.CodeOf(err), err.Error())
			return
		}
		fmt.Fprintf(g, "update %s: ok %s\n", label, bmSettings(resp.Msg.Settings))
	}

	fmt.Fprintln(g, "nil selector")
	get(NewLLMBackendService(nil))

	sel := headless.NewSelector(func() headless.BackendSettings { return BackendSettingsFromConfig(config.LiveLLMBackends()) })
	sel.Register(&bmBackend{name: "claude", caps: headless.Caps{Resume: true, SystemPrompt: true, ToolRestriction: true, WorkDir: true}, up: true})
	sel.Register(&bmBackend{name: "agy", caps: headless.Caps{SystemPrompt: true}, up: false})
	sel.Register(&bmBackend{name: "consolette"})
	svc := NewLLMBackendService(sel)
	fmt.Fprintln(g, "selector with three backends")
	get(svc)

	features := headless.OverridableFeatureKeys()
	feature := "rules-generation"
	if len(features) > 0 {
		feature = features[0]
	}
	update(svc, "agy default", &sessionv1.LLMBackendSettings{DefaultBackend: "agy"})
	get(svc)
	update(svc, "full", &sessionv1.LLMBackendSettings{
		DefaultBackend: "claude", PerFeature: map[string]string{feature: "agy"},
		ConsoletteBaseUrl: "http://127.0.0.1:1234", AnthropicBaseUrl: "https://example.com/v1",
		ModelMaps: []*sessionv1.LLMBackendModelMap{
			{Backend: "gemini", Aliases: map[string]string{"opus": "g-pro", "haiku": "g-flash"}},
			{Backend: "agy", Aliases: map[string]string{"sonnet": "a"}},
			{Backend: "agy", Aliases: map[string]string{"sonnet": "b"}},
		},
	})
	get(svc)
	update(svc, "unknown default", &sessionv1.LLMBackendSettings{DefaultBackend: "nope"})
	update(svc, "unknown feature", &sessionv1.LLMBackendSettings{PerFeature: map[string]string{"nope-feature": "claude"}})
	update(svc, "unknown backend for feature", &sessionv1.LLMBackendSettings{PerFeature: map[string]string{feature: "nope"}})
	update(svc, "bad consolette url", &sessionv1.LLMBackendSettings{ConsoletteBaseUrl: "ftp://x"})
	update(svc, "bad anthropic url", &sessionv1.LLMBackendSettings{AnthropicBaseUrl: "not a url"})
	update(svc, "unknown model map backend", &sessionv1.LLMBackendSettings{ModelMaps: []*sessionv1.LLMBackendModelMap{{Backend: "nope", Aliases: map[string]string{"a": "b"}}}})
	update(svc, "nil settings", nil)
	get(svc)
	update(svc, "reset", &sessionv1.LLMBackendSettings{})
	get(svc)

	// Settings conversion, both directions, through the exported helper.
	for _, c := range []config.LLMBackendsConfig{
		{},
		{Default: "claude", PerFeature: map[string]string{"a": "b"}, ConsoletteBaseURL: "c", AnthropicBaseURL: "d", ModelMaps: map[string]map[string]string{"x": {"y": "z"}}},
	} {
		bs := BackendSettingsFromConfig(c)
		fmt.Fprintf(g, "BackendSettingsFromConfig %+v -> %+v\n", c, bs)
	}
}

func TestBacklogMiscGolden(t *testing.T) {
	var g strings.Builder
	bmGoldenPublisher(t, &g)
	bmGoldenDebugMutate(t, &g)
	bmGoldenHeadless(t, &g)
	bmGoldenLLMBackend(t, &g)
	// encoding/json names the target type's package in its parse-error text, so the
	// pre-move recording says services.X where the leaf says backlogmisc.X.
	got := strings.ReplaceAll(g.String(), "Go value of type backlogmisc.", "Go value of type services.")

	if os.Getenv("BACKLOGMISC_GOLDEN_RECORD") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(bmGoldenPath), 0o755))
		require.NoError(t, os.WriteFile(bmGoldenPath, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(bmGoldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
