package services

// Differential golden for the admin RPCs (logs, debug snapshot, window focus,
// projects, slash commands, errors, diagnose, capture tap, TCC, tmux, hooks),
// recorded from origin/main before their code moved into leaves. It drives
// SessionService, so it also pins the delegators.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/diagnosesvc"
	"github.com/tstapler/stapler-squad/server/services/errorregistry"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tcc"
	"github.com/tstapler/stapler-squad/session/tmux"
)

var updateAdminRPCGolden = flag.Bool("update-admin-rpc-golden", false, "rewrite testdata/admin_rpc.golden")

type adminGolden struct {
	t       *testing.T
	b       strings.Builder
	replace []string // old, new pairs for machine-specific paths and ids
}

var (
	adminUUID = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	adminTime = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T[0-9:.]+(Z|[+-]\d{2}:\d{2})`)
)

func (g *adminGolden) line(format string, args ...any) {
	s := strings.NewReplacer(g.replace...).Replace(fmt.Sprintf(format, args...))
	s = adminUUID.ReplaceAllString(s, "<uuid>")
	g.b.WriteString(adminTime.ReplaceAllString(s, "<time>") + "\n")
}

func (g *adminGolden) record(step string, msg proto.Message, err error) {
	if err != nil {
		g.line("%s err code=%v %v", step, connect.CodeOf(err), err)
		return
	}
	raw, merr := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(msg)
	require.NoError(g.t, merr)
	var v any
	require.NoError(g.t, json.Unmarshal(raw, &v))
	norm, merr := json.Marshal(v)
	require.NoError(g.t, merr)
	g.line("%s %s", step, norm)
}

func adminMsg[T any, P interface {
	*T
	proto.Message
}](r *connect.Response[T]) proto.Message {
	if r == nil {
		return nil
	}
	return P(r.Msg)
}

const adminLogFixture = `{"time":"2026-01-02T03:04:05Z","level":"INFO","msg":"started","session":"golden-session","pid":7,"token":"abc"}
{"time":"2026-01-02T03:04:06Z","level":"ERROR","msg":"boom","source":"plain-string","n":1000000}
[golden] WARN:2026/01/02 03:04:07 file.go:12: legacy warning
not a log line
{"time":"2026-01-02T03:04:08Z","msg":"no level"}
`

func goldenLogs(t *testing.T, g *adminGolden, svc *SessionService, logPath string) {
	str := func(s string) *string { return &s }
	num := func(n int32) *int32 { return &n }
	cases := []struct {
		name string
		req  *sessionv1.GetLogsRequest
	}{
		{"all", &sessionv1.GetLogsRequest{}},
		{"search", &sessionv1.GetLogsRequest{SearchQuery: str("BOOM")}},
		{"level", &sessionv1.GetLogsRequest{Levels: []string{"error", "warn"}}},
		{"page", &sessionv1.GetLogsRequest{Limit: num(1), Offset: num(1)}},
		{"session-uuid", &sessionv1.GetLogsRequest{SessionId: str("golden-uuid")}},
		{"past-end", &sessionv1.GetLogsRequest{Offset: num(50)}},
	}
	for _, c := range cases {
		require.NoError(t, os.WriteFile(logPath, []byte(adminLogFixture), 0o600))
		resp, err := svc.GetLogs(context.Background(), connect.NewRequest(c.req))
		require.NoError(t, err)
		g.line("logs/%s total=%d more=%v", c.name, resp.Msg.TotalCount, resp.Msg.HasMore)
		for _, e := range resp.Msg.Entries {
			zone := time.UTC
			if e.GetSource() != "" { // legacy lines carry local wall time
				zone = time.Local
			}
			g.line("  %s %s %q source=%q", e.Timestamp.AsTime().In(zone).Format("15:04:05"), e.Level, e.Message, e.GetSource())
		}
	}
	require.NoError(t, os.Remove(logPath))
	resp, err := svc.GetLogs(context.Background(), connect.NewRequest(&sessionv1.GetLogsRequest{}))
	g.record("logs/no-file", adminMsg(resp), err)
}

func goldenSnapshot(t *testing.T, g *adminGolden, svc *SessionService, logPath string) {
	require.NoError(t, os.WriteFile(logPath, []byte("one\ntwo\nthree\n"), 0o600))
	note, lines := "golden note", int32(2)
	resp, err := svc.CreateDebugSnapshot(context.Background(), connect.NewRequest(&sessionv1.CreateDebugSnapshotRequest{Note: &note, LogLines: &lines}))
	require.NoError(t, err)
	summary, _, _ := strings.Cut(resp.Msg.Summary, " (")
	g.line("snapshot summary=%q dir=%s size>0=%v", summary, filepath.Dir(resp.Msg.FilePath), resp.Msg.FileSizeBytes > 0)
	raw, err := os.ReadFile(resp.Msg.FilePath)
	require.NoError(t, err)
	var snap struct {
		Version int                        `json:"version"`
		Note    string                     `json:"note"`
		Server  map[string]json.RawMessage `json:"server"`
		Logs    json.RawMessage            `json:"recent_logs"`
	}
	require.NoError(t, json.Unmarshal(raw, &snap))
	var uptime int64
	require.NoError(t, json.Unmarshal(snap.Server["uptime_seconds"], &uptime))
	var compact map[string]any
	require.NoError(t, json.Unmarshal(snap.Logs, &compact))
	logs, err := json.Marshal(compact)
	require.NoError(t, err)
	g.line("snapshot file version=%d note=%q serverKeys=%d uptime>=0=%v logs=%s", snap.Version, snap.Note, len(snap.Server), uptime >= 0, logs)
	require.NoError(t, os.Remove(logPath))
}

func goldenFocus(g *adminGolden, svc *SessionService) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name, goos, realIP, forwarded string
		req                           *sessionv1.FocusWindowRequest
	}{
		{"empty", "linux", "", "", &sessionv1.FocusWindowRequest{}},
		{"linux", "linux", "", "", &sessionv1.FocusWindowRequest{AppName: str("Code")}},
		{"blocked-bundle", "darwin", "", "", &sessionv1.FocusWindowRequest{BundleId: str("com.apple.Music")}},
		{"blocked-name", "darwin", "", "", &sessionv1.FocusWindowRequest{AppName: str("Photos")}},
		{"real-ip", "darwin", "203.0.113.9", "", &sessionv1.FocusWindowRequest{AppName: str("Photos")}},
		{"real-ip-local", "linux", "127.0.0.1", "", &sessionv1.FocusWindowRequest{AppName: str("Code")}},
		{"forwarded", "linux", "", "203.0.113.9, 127.0.0.1", &sessionv1.FocusWindowRequest{AppName: str("Code")}},
	}
	for _, c := range cases {
		g.t.Setenv("GOOS", c.goos)
		req := connect.NewRequest(c.req)
		if c.realIP != "" {
			req.Header().Set("X-Real-IP", c.realIP)
		}
		if c.forwarded != "" {
			req.Header().Set("X-Forwarded-For", c.forwarded)
		}
		resp, err := svc.FocusWindow(context.Background(), req)
		g.record("focus/"+c.name, adminMsg(resp), err)
	}
}

func goldenProjects(g *adminGolden, svc *SessionService) {
	ctx := context.Background()
	for _, name := range []string{"", "alpha"} {
		resp, err := svc.CreateProject(ctx, connect.NewRequest(&sessionv1.CreateProjectRequest{Name: name, Description: "d"}))
		g.record("project/create/"+name, adminMsg(resp), err)
	}
	up, err := svc.UpdateProject(ctx, connect.NewRequest(&sessionv1.UpdateProjectRequest{Name: "alpha", Description: "d2"}))
	g.record("project/update", adminMsg(up), err)
	for _, req := range []*sessionv1.AssignSessionsToProjectRequest{{}, {ProjectId: "alpha", SessionIds: []string{"x"}}} {
		resp, aerr := svc.AssignSessionsToProject(ctx, connect.NewRequest(req))
		g.record("project/assign", adminMsg(resp), aerr)
	}
	list, err := svc.ListProjects(ctx, connect.NewRequest(&sessionv1.ListProjectsRequest{}))
	g.record("project/list", adminMsg(list), err)
	for _, id := range []string{"", "alpha", "missing"} {
		resp, derr := svc.DeleteProject(ctx, connect.NewRequest(&sessionv1.DeleteProjectRequest{Id: id}))
		g.record("project/delete/"+id, adminMsg(resp), derr)
	}
}

func goldenSlash(t *testing.T, g *adminGolden, svc *SessionService, home string) {
	write := func(root, rel, body string) {
		p := filepath.Join(root, ".claude", "commands", rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
	project := t.TempDir()
	g.replace = append(g.replace, project, "<project>")
	write(home, "code/fix.md", "---\ntitle: User Fix\ndescription: from home\n---\n")
	write(home, "help.md", "no frontmatter")
	write(home, "README.md", "skipped")
	write(project, "code/fix.md", "---\ntitle: Project Fix\n---\n")
	for _, dir := range []string{"", project, "~/", "relative/dir"} {
		resp, err := svc.ListSlashCommands(context.Background(), connect.NewRequest(&sessionv1.ListSlashCommandsRequest{TargetDirectory: dir}))
		g.record("slash/"+dir, adminMsg(resp), err)
	}
}

func goldenErrors(t *testing.T, g *adminGolden, svc *SessionService) {
	ctx := context.Background()
	list := func(step string, all bool) []*sessionv1.ErrorEventRecord {
		resp, err := svc.ListErrors(ctx, connect.NewRequest(&sessionv1.ListErrorsRequest{IncludeAcknowledged: all}))
		require.NoError(t, err)
		for _, r := range resp.Msg.Errors {
			g.line("%s %s %q %s count=%d ack=%v fp=%s stack=%v", step, r.ErrorType, r.Message, r.RpcProcedure, r.OccurrenceCount, r.Acknowledged, r.Fingerprint, r.StackTrace != "")
		}
		g.line("%s n=%d", step, len(resp.Msg.Errors))
		return resp.Msg.Errors
	}
	list("errors/unwired", true)
	storage := svctest.CreateTestStorage(t)
	registry := errorregistry.NewErrorRegistry(storage.GetEntClient(), true)
	svc.SetErrorRegistry(registry)
	registry.Record(ctx, errors.New("read /var/lib/a/b: code 12"), "/p.A")
	registry.Record(ctx, errors.New("read /var/lib/c/d: code 99"), "/p.A")
	rows := list("errors/recorded", false)
	require.Len(t, rows, 1)
	ack, err := svc.AcknowledgeError(ctx, connect.NewRequest(&sessionv1.AcknowledgeErrorRequest{Fingerprint: rows[0].Fingerprint}))
	g.record("errors/ack", adminMsg(ack), err)
	list("errors/unacked", false)
	list("errors/all", true)
}

type adminSpawner struct{ prompt string }

func (s *adminSpawner) SpawnDiagnosticSession(_ context.Context, _ *backlogdto.BacklogItemData, prompt string) (*instance.Instance, error) {
	s.prompt = prompt
	return &instance.Instance{UUID: "diag-uuid"}, nil
}

func goldenDiagnose(t *testing.T, g *adminGolden) {
	ctx := context.Background()
	storage := svctest.CreateTestStorage(t)
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title: "Golden item", Description: "Why it is stuck.", Status: string(domain.BacklogStatusReview),
	})
	require.NoError(t, err)
	spawner := &adminSpawner{}
	d := diagnosesvc.NewDiagnosticService(storage, spawner)
	for _, id := range []string{"", "00000000-0000-0000-0000-000000000001", item.ID} {
		resp, aerr := d.AssembleDiagnosticBundle(ctx, connect.NewRequest(&sessionv1.AssembleDiagnosticBundleRequest{ItemId: id}))
		g.record("diagnose/assemble", adminMsg(resp), aerr)
	}
	for _, reason := range []sessionv1.StuckReason{sessionv1.StuckReason_STUCK_REASON_UNSPECIFIED, sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX} {
		resp, derr := d.DispatchDiagnose(ctx, connect.NewRequest(&sessionv1.DispatchDiagnoseRequest{ItemId: item.ID, StuckReason: reason, NudgeTargetSessionUuid: "gone"}))
		g.record("diagnose/dispatch", adminMsg(resp), derr)
		g.line("diagnose/prompt %q", spawner.prompt)
	}
	resp, err := diagnosesvc.NewDiagnosticService(storage, nil).DispatchDiagnose(ctx, connect.NewRequest(&sessionv1.DispatchDiagnoseRequest{ItemId: item.ID}))
	g.record("diagnose/no-spawner", adminMsg(resp), err)
}

func goldenCaptureTap(t *testing.T, g *adminGolden) {
	dir := t.TempDir()
	g.replace = append(g.replace, dir, "<tapdir>")
	svc := &SessionService{tapRegistry: streamhub.NewTapRegistry(streamhub.TapRegistryOptions{
		Now:       func() time.Time { return time.Unix(1_700_000_000, 0) },
		DirFn:     func() (string, error) { return dir, nil },
		AfterFunc: func(time.Duration, func()) func() bool { return nil },
	})}
	// A direct call has no peer address, so the loopback gate refuses it.
	set, err := svc.SetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.SetCaptureTapRequest{Enabled: true}))
	g.record("tap/set-no-peer", adminMsg(set), err)
	get, err := svc.GetCaptureTap(context.Background(), connect.NewRequest(&sessionv1.GetCaptureTapRequest{}))
	g.record("tap/get-no-peer", adminMsg(get), err)
}

func goldenTCC(t *testing.T, g *adminGolden) {
	ctx := context.Background()
	get := func(step string, svc *SessionService) {
		resp, err := svc.GetTCCEnrichmentStatus(ctx, connect.NewRequest(&sessionv1.GetTCCEnrichmentStatusRequest{}))
		g.record(step, adminMsg(resp), err)
	}
	cont := func(step string, ctx context.Context, svc *SessionService, id string) {
		resp, err := svc.ContinueTCCEnrichment(ctx, connect.NewRequest(&sessionv1.ContinueTCCEnrichmentRequest{RootId: id}))
		g.record(step, adminMsg(resp), err)
	}
	cfg := VerdictServiceConfig("localhost:8543", nil)
	local := StampedContext(t, &cfg, access.ListenerLocal, false, "localhost:8543", nil)
	unset := &SessionService{}
	get("tcc/unset", unset)
	cont("tcc/unset-continue", local, unset, "x")

	gate := tcc.NewConsentGate(func(string) tcc.Result { return tcc.Result{Protected: true, Root: "/protected"} }, nil, nil)
	enricher := instance.NewWorktreeEnricher(instance.WorktreeEnricherConfig{Ready: gate.Ready, Probe: func(string) (*instance.WorktreeInfo, error) { return &instance.WorktreeInfo{}, nil }})
	t.Cleanup(enricher.Stop)
	svc := &SessionService{}
	svc.SetTCCEnrichment(gate, enricher, true)
	gate.Ready("/protected/repo")
	get("tcc/status", svc)
	cont("tcc/unknown", local, svc, "/arbitrary")
	cont("tcc/not-local", ctx, svc, "/arbitrary")
}

func goldenTmuxAndHooks(t *testing.T, g *adminGolden, svc *SessionService, home string) {
	ctx := context.Background()
	ver, err := svc.GetTmuxVersionStatus(ctx, connect.NewRequest(&sessionv1.GetTmuxVersionStatusRequest{}))
	g.record("tmux/version", adminMsg(ver), err)
	restart, err := svc.RestartTmuxServer(ctx, connect.NewRequest(&sessionv1.RestartTmuxServerRequest{ServerSocket: "ssq-golden-no-server"}))
	g.record("tmux/restart-down", adminMsg(restart), err)

	status := func(step string) {
		resp, serr := svc.GetHookStatus(ctx, connect.NewRequest(&sessionv1.GetHookStatusRequest{}))
		require.NoError(t, serr)
		// Handler availability also looks beside the test binary, so it is left out.
		g.line("%s rules=%v notif=%v rulesAvail=%v agy=%v gemini=%v", step, resp.Msg.RulesInstalled, resp.Msg.NotificationsInstalled,
			resp.Msg.RulesAvailable, resp.Msg.AgyRulesInstalled, resp.Msg.GeminiRulesInstalled)
	}
	install := func(step string) {
		resp, ierr := svc.InstallHooks(ctx, connect.NewRequest(&sessionv1.InstallHooksRequest{InstallRules: true, InstallAgyRules: true, InstallGeminiRules: true}))
		if ierr != nil {
			g.line("%s err code=%v %v", step, connect.CodeOf(ierr), ierr)
			return
		}
		g.line("%s %q", step, resp.Msg.Messages)
	}
	status("hooks/fresh")
	install("hooks/no-binary")
	bin := filepath.Join(home, ".local", "bin", "ssq-hooks")
	require.NoError(t, os.MkdirAll(filepath.Dir(bin), 0o755))
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o700))
	install("hooks/installed")
	status("hooks/after")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\necho nope\nexit 3\n"), 0o700))
	install("hooks/failing")
}

// Not parallel: HOME, PATH, GOOS, the state directory and the tmux socket are process environment.
func TestAdminRPCGolden(t *testing.T) {
	home := svctest.WithFakeHome(t)
	state := svctest.WithIsolatedStateDir(t)
	tmuxBin, err := exec.LookPath(tmux.Binary())
	require.NoError(t, err)
	t.Setenv("TMUX_BIN", tmuxBin) // PATH is emptied below to hide any installed ssq-hooks
	t.Setenv("PATH", filepath.Join(home, "empty-bin"))
	t.Setenv(tmux.EnvSocketVar, fmt.Sprintf("ssq-golden-%d", os.Getpid()))
	g := &adminGolden{t: t, replace: []string{state, "<state>", home, "<home>"}}

	storage := svctest.CreateTestStorage(t)
	svc := newCreateTestService(t, storage)
	svc.ReviewQueuePoller.SetInstances([]*instance.Instance{{UUID: "golden-uuid", Title: "golden-session", Status: domain.Paused, Program: "claude"}})
	logPath := filepath.Join(state, "logs", "staplersquad.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(logPath), 0o700))

	goldenLogs(t, g, svc, logPath)
	svc.ReviewQueuePoller.SetInstances(nil)
	goldenSnapshot(t, g, svc, logPath)
	goldenFocus(g, svc)
	goldenProjects(g, svc)
	goldenSlash(t, g, svc, home)
	goldenErrors(t, g, svc)
	goldenDiagnose(t, g)
	goldenCaptureTap(t, g)
	goldenTCC(t, g)
	goldenTmuxAndHooks(t, g, svc, home)

	path := filepath.Join("testdata", "admin_rpc.golden")
	if *updateAdminRPCGolden {
		require.NoError(t, os.WriteFile(path, []byte(g.b.String()), 0o600))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), g.b.String())
}
