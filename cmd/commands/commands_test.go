package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

type fakeService struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	created   *sessionv1.CreateSessionRequest
	listed    *sessionv1.ListSessionsRequest
	updated   *sessionv1.UpdateSessionRequest
	getStatus []sessionv1.SessionStatus // sequence returned by GetSession
	getCalls  atomic.Int32
	updateErr error
	diff      string
}

func (f *fakeService) CreateSession(_ context.Context, r *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	f.created = r.Msg
	return connect.NewResponse(&sessionv1.CreateSessionResponse{Session: &sessionv1.Session{
		Id: "id-1", Title: r.Msg.Title, ActiveDir: r.Msg.Path, Status: sessionv1.SessionStatus_SESSION_STATUS_CREATING}}), nil
}

func (f *fakeService) GetSession(_ context.Context, r *connect.Request[sessionv1.GetSessionRequest]) (*connect.Response[sessionv1.GetSessionResponse], error) {
	i := int(f.getCalls.Add(1)) - 1
	st := f.getStatus[min(i, len(f.getStatus)-1)]
	return connect.NewResponse(&sessionv1.GetSessionResponse{Session: &sessionv1.Session{Id: r.Msg.Id, Title: "t", Status: st}}), nil
}

func (f *fakeService) ListSessions(_ context.Context, r *connect.Request[sessionv1.ListSessionsRequest]) (*connect.Response[sessionv1.ListSessionsResponse], error) {
	f.listed = r.Msg
	return connect.NewResponse(&sessionv1.ListSessionsResponse{Sessions: []*sessionv1.Session{
		{Id: "a", Title: "one", ActiveDir: "/p", Status: sessionv1.SessionStatus_SESSION_STATUS_ACTIVE}}}), nil
}

func (f *fakeService) UpdateSession(_ context.Context, r *connect.Request[sessionv1.UpdateSessionRequest]) (*connect.Response[sessionv1.UpdateSessionResponse], error) {
	f.updated = r.Msg
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return connect.NewResponse(&sessionv1.UpdateSessionResponse{}), nil
}

func (f *fakeService) GetSessionDiff(context.Context, *connect.Request[sessionv1.GetSessionDiffRequest]) (*connect.Response[sessionv1.GetSessionDiffResponse], error) {
	return connect.NewResponse(&sessionv1.GetSessionDiffResponse{DiffStats: &sessionv1.DiffStats{Added: 1, Content: f.diff}}), nil
}

func newFake(t *testing.T) (*fakeService, string) {
	t.Helper()
	f := &fakeService{}
	mux := http.NewServeMux()
	mux.Handle(sessionv1connect.NewSessionServiceHandler(f))
	srv := httptest.NewServer(http.StripPrefix("/api", mux))
	t.Cleanup(srv.Close)
	return f, srv.URL + "/api"
}

func run(t *testing.T, cmd *cobra.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), errb.String(), err
}

func TestResolveServerURL(t *testing.T) {
	t.Setenv("SSQ_SERVER", "")
	if got := ResolveServerURL(""); got != "http://localhost:8543/api" {
		t.Errorf("default = %q", got)
	}
	t.Setenv("SSQ_SERVER", "http://env/api")
	if got := ResolveServerURL(""); got != "http://env/api" {
		t.Errorf("env = %q", got)
	}
	if got := ResolveServerURL("http://flag/api"); got != "http://flag/api" {
		t.Errorf("flag = %q", got)
	}
}

func TestCreateSession_PrintsIDAndSendsFields(t *testing.T) {
	f, url := newFake(t)
	out, _, err := run(t, NewCreateSessionCmd(), "--title", "x", "--path", "/r", "--branch", "b", "--auto-yes", "--one-shot", "-s", url)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "session-id: id-1\n") {
		t.Errorf("stdout = %q", out)
	}
	if f.created.Branch != "b" || !f.created.AutoYes || !f.created.OneShot {
		t.Errorf("request = %v", f.created)
	}
}

func TestCreateSession_UnreachableServer(t *testing.T) {
	t.Setenv("SSQ_SERVER", "")
	out, errOut, err := run(t, NewCreateSessionCmd(), "--title", "x", "--path", "/r", "--server", "http://127.0.0.1:1/api")
	if err == nil || out != "" || !strings.Contains(errOut, "server unreachable") {
		t.Errorf("err=%v out=%q stderr=%q", err, out, errOut)
	}
}

func TestCreateSession_RequiresTitleAndPath(t *testing.T) {
	if _, _, err := run(t, NewCreateSessionCmd(), "--path", "/r"); err == nil {
		t.Error("expected error without --title")
	}
	if _, _, err := run(t, NewCreateSessionCmd(), "--title", "x"); err == nil {
		t.Error("expected error without --path")
	}
}

func TestCreateSession_WaitOutcomes(t *testing.T) {
	const (
		creating = sessionv1.SessionStatus_SESSION_STATUS_CREATING
		active   = sessionv1.SessionStatus_SESSION_STATUS_ACTIVE
	)
	tests := []struct {
		name    string
		seq     []sessionv1.SessionStatus
		timeout string
		wantErr string
	}{
		{"active", []sessionv1.SessionStatus{creating, active}, "5s", ""},
		{"failed", []sessionv1.SessionStatus{sessionv1.SessionStatus_SESSION_STATUS_FAILED}, "5s", "terminal state SESSION_STATUS_FAILED"},
		{"crashed", []sessionv1.SessionStatus{sessionv1.SessionStatus_SESSION_STATUS_CRASHED}, "5s", "terminal state SESSION_STATUS_CRASHED"},
		{"permanently failed", []sessionv1.SessionStatus{sessionv1.SessionStatus_SESSION_STATUS_PERMANENTLY_FAILED}, "5s", "terminal state"},
		{"timeout", []sessionv1.SessionStatus{creating}, "50ms", "timed out waiting for session to become active"},
	}
	old := pollInterval
	pollInterval = 5 * time.Millisecond
	t.Cleanup(func() { pollInterval = old })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, url := newFake(t)
			f.getStatus = tt.seq
			out, errOut, err := run(t, NewCreateSessionCmd(), "--title", "x", "--path", "/r", "--wait", "--wait-timeout", tt.timeout, "-s", url)
			if tt.wantErr == "" {
				if err != nil || !strings.Contains(out, "session-id: id-1") || !strings.Contains(out, "SESSION_STATUS_ACTIVE") {
					t.Errorf("err=%v out=%q", err, out)
				}
				return
			}
			if err == nil || out != "" || !strings.Contains(errOut, tt.wantErr) {
				t.Errorf("err=%v out=%q stderr=%q", err, out, errOut)
			}
		})
	}
}

func TestListSessions(t *testing.T) {
	f, url := newFake(t)
	out, _, err := run(t, NewListSessionsCmd(), "-s", url)
	if err != nil || f.listed.Status != nil {
		t.Fatalf("err=%v status=%v (want nil for all)", err, f.listed.Status)
	}
	if !strings.Contains(out, "ID\tTITLE\tSTATUS\tPATH\na\tone\tSESSION_STATUS_ACTIVE\t/p\n") {
		t.Errorf("stdout = %q", out)
	}

	out, _, err = run(t, NewListSessionsCmd(), "-s", url, "--json", "--status", "paused", "--search", "au")
	if err != nil || f.listed.GetStatus() != sessionv1.SessionStatus_SESSION_STATUS_PAUSED || f.listed.GetSearchQuery() != "au" {
		t.Fatalf("err=%v req=%v", err, f.listed)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil || arr[0]["status"] != "SESSION_STATUS_ACTIVE" {
		t.Errorf("json = %q err=%v", out, err)
	}

	if _, _, err := run(t, NewListSessionsCmd(), "-s", url, "--status", "bogus"); err == nil {
		t.Error("expected invalid --status error")
	}
}

func TestListSessions_EmptyJSONIsArray(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := printSessionsJSON(cmd, nil); err != nil || strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("out=%q err=%v", out.String(), err)
	}
}

func TestSteer(t *testing.T) {
	f, url := newFake(t)
	out, _, err := run(t, NewSteerCmd(), "id-1", "-m", "hi", "-s", url)
	if err != nil || out != "ok\n" || f.updated.GetSteerMessage() != "hi" || f.updated.Id != "id-1" {
		t.Errorf("err=%v out=%q req=%v", err, out, f.updated)
	}

	f.updateErr = connect.NewError(connect.CodeFailedPrecondition, errString("session is hidden"))
	out, errOut, err := run(t, NewSteerCmd(), "id-1", "-m", "hi", "-s", url)
	if err == nil || out != "" || !strings.Contains(errOut, "error: session is hidden") {
		t.Errorf("err=%v out=%q stderr=%q", err, out, errOut)
	}

	if _, _, err := run(t, NewSteerCmd(), "id-1", "-s", url); err == nil {
		t.Error("expected error without --message")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestGetDiff(t *testing.T) {
	f, url := newFake(t)
	f.diff = "diff --git a/x b/x\n"
	out, _, err := run(t, NewGetDiffCmd(), "id-1", "-s", url)
	if err != nil || out != f.diff {
		t.Errorf("err=%v out=%q", err, out)
	}
	out, _, _ = run(t, NewGetDiffCmd(), "id-1", "-s", url, "--json")
	var m map[string]map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil || m["diffStats"]["content"] != f.diff {
		t.Errorf("json=%q err=%v", out, err)
	}
	f.diff = ""
	if out, _, _ = run(t, NewGetDiffCmd(), "id-1", "-s", url); out != "" {
		t.Errorf("empty diff stdout = %q", out)
	}
}

func TestGetSession_UsesEnvServer(t *testing.T) {
	f, url := newFake(t)
	f.getStatus = []sessionv1.SessionStatus{sessionv1.SessionStatus_SESSION_STATUS_ACTIVE}
	t.Setenv("SSQ_SERVER", url)
	out, _, err := run(t, NewGetSessionCmd(), "id-1")
	if err != nil || !strings.Contains(out, `"id": "id-1"`) {
		t.Errorf("err=%v out=%q", err, out)
	}
}
