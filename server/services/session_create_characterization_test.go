package services

// Golden characterization of the session-creation RPCs and spawn entry points,
// recorded before their code moved out of server/services. It drives only
// SessionService methods, so it also pins the delegators and the late setters.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/tagpoller"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

var updateSessionCreateGolden = flag.Bool("update-session-create-golden", false, "rewrite testdata/session_create.golden")

var (
	createCharNanos  = regexp.MustCompile(`:[0-9a-f]{12,}\b`)
	createCharBranch = regexp.MustCompile(`branch="(main|master)"`)
)

type createChar struct {
	t       *testing.T
	svc     *SessionService
	storage *session.Storage
	poller  *session.ReviewQueuePoller
	tags    *tagpoller.SessionTagClassificationPoller
	dir     string
	agent   string
	mu      sync.Mutex
	lines   []string
	phases  []string
	events  []*events.Event
}

func newCreateChar(t *testing.T) *createChar {
	t.Helper()
	svctest.WithFakeHome(t)
	svctest.WithIsolatedStateDir(t)
	c := &createChar{t: t, storage: svctest.CreateTestStorage(t), dir: t.TempDir()}
	c.agent = filepath.Join(c.dir, "agent")
	require.NoError(t, os.WriteFile(c.agent, []byte("#!/bin/sh\nexec sleep 300\n"), 0o755)) //nolint:gosec // test script

	bus := events.NewEventBus(256)
	t.Cleanup(bus.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, _ := bus.Subscribe(ctx)
	go func() {
		for e := range ch {
			c.mu.Lock()
			c.events = append(c.events, e)
			c.mu.Unlock()
		}
	}()
	c.svc = NewSessionService(c.storage, bus)
	t.Cleanup(func() { c.svc.Shutdown() })

	// Everything below is installed after construction, as server wiring does.
	c.poller = session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
	c.svc.SetReviewQueuePoller(c.poller)
	c.svc.SetMCPServerURL(func() string { return "http://mcp.test/mcp" })
	c.tags = tagpoller.NewSessionTagClassificationPollerWithConfig(&fakeSessionTagPoolClient{}, classifier.NewTaggingEngine(),
		tagpoller.SessionTagPollerConfig{PollInterval: time.Hour, CallTimeout: time.Second})
	c.svc.SetSessionTagPoller(c.tags)
	c.svc.creationPhaseHook = func(msg string) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.phases = append(c.phases, msg)
	}
	return c
}

func (c *createChar) line(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	s = strings.ReplaceAll(s, c.dir, "<dir>")
	s = adminUUID.ReplaceAllString(s, "<uuid>")
	s = createCharBranch.ReplaceAllString(s, `branch="<default>"`)
	c.lines = append(c.lines, createCharNanos.ReplaceAllString(s, ":<n>"))
}

func (c *createChar) errLine(step string, err error) {
	c.line("%s: %s", step, svctest.CanonicalError(err))
}

func (c *createChar) takePhases() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	got := c.phases
	c.phases = nil
	return got
}

// creationEvents renders the events only the create paths publish for title; the
// asynchronous status listeners publish others with timing-dependent order.
func (c *createChar) creationEvents(title string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, e := range c.events {
		if e.Session == nil || e.Session.Snapshot().Title != title {
			continue
		}
		switch {
		case e.Type == events.EventSessionCreated:
			out = append(out, string(e.Type))
		case slices.Contains(e.UpdatedFields, "creation_progress") || slices.Contains(e.UpdatedFields, "path"):
			out = append(out, string(e.Type)+" "+strings.Join(e.UpdatedFields, ","))
		}
	}
	return out
}

func (c *createChar) waitTerminal(title string) {
	c.t.Helper()
	wait.RequireEventually(c.t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, e := range c.events {
			if e.Session != nil && e.Session.Snapshot().Title == title &&
				slices.Equal(e.UpdatedFields, []string{"status", "creation_progress"}) {
				return true
			}
		}
		return false
	}, pipelineEventuallyTimeout, 10*time.Millisecond, "no terminal event for %s", title)
}

func (c *createChar) waitCreated(title string) {
	c.t.Helper()
	wait.RequireEventually(c.t, func() bool { return slices.Contains(c.creationEvents(title), "session.created") },
		pipelineEventuallyTimeout, 10*time.Millisecond, "no created event for %s", title)
	c.line("%s events=%q", title, c.creationEvents(title))
}

func (c *createChar) recordInstance(step string, inst *instance.Instance) {
	c.t.Helper()
	require.NotNil(c.t, inst)
	data := inst.ToInstanceData()
	env := make([]string, 0, len(inst.EnvVars))
	for k, v := range inst.EnvVars {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	c.line("%s title=%q program=%q type=%s status=%s hidden=%t oneshot=%t autoyes=%t autoapprove=%t category=%q tags=%v",
		step, data.Title, data.Program, data.SessionType, data.Status, data.Hidden, data.OneShot,
		data.AutoYes, data.AutoApprove, data.Category, data.Tags)
	c.line("%s path=%q branch=%q prompt=%q initial=%q env=%v cliflags=%q extra=%v tools=%q permission=%q project=%q workflow=%q mcp=%q isolatedSocket=%t",
		step, data.Path, data.Branch, inst.Prompt, inst.InitialPrompt, env, inst.CLIFlags, inst.ExtraArgs, inst.AllowedTools,
		inst.PermissionMode, inst.ProjectID, inst.WorkflowID, inst.GetMCPServerURL(),
		c.svc.testTmuxServerSocket != "" && inst.TmuxServerSocket == c.svc.testTmuxServerSocket)
}

func (c *createChar) recordRegistration(step, title string) {
	inPoller := c.poller.FindInstance(title) != nil
	inTags := false
	for _, inst := range c.tags.Instances() {
		inTags = inTags || inst.MatchesID(title)
	}
	rows, err := c.storage.ListInstanceData()
	require.NoError(c.t, err)
	stored := ""
	for _, r := range rows {
		if r.Title == title {
			stored = r.Status.String()
		}
	}
	c.line("%s poller=%t tagPoller=%t stored=%s", step, inPoller, inTags, stored)
}

func (c *createChar) destroy(id string) {
	c.t.Cleanup(func() { destroyCreatedSession(c.t, c.svc, id) })
}

func (c *createChar) create(msg *sessionv1.CreateSessionRequest) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	return c.svc.CreateSession(context.Background(), connect.NewRequest(msg))
}

func (c *createChar) rejections() {
	file := filepath.Join(c.dir, "missing")
	for _, tc := range []struct {
		step string
		msg  *sessionv1.CreateSessionRequest
	}{
		{"no-title", &sessionv1.CreateSessionRequest{Path: c.dir}},
		{"no-path", &sessionv1.CreateSessionRequest{Title: "r"}},
		{"duplicate-title", &sessionv1.CreateSessionRequest{Title: "taken", Path: c.dir}},
		{"bad-resume-id", &sessionv1.CreateSessionRequest{Title: "r", Path: c.dir, ResumeId: "nope"}},
		{"missing-fork-source", &sessionv1.CreateSessionRequest{Title: "r", Path: c.dir, ForkSourceId: "00000000-0000-0000-0000-000000000001"}},
		{"unknown-alias", &sessionv1.CreateSessionRequest{Title: "r", AliasName: "nope"}},
		{"unknown-remote", &sessionv1.CreateSessionRequest{Title: "r", Path: "/srv/x", Remote: &sessionv1.RemoteTarget{RemoteName: "nope"}}},
		{"missing-restart-source", &sessionv1.CreateSessionRequest{Title: "r", RestartFromSessionId: "nope"}},
		{"missing-dir", &sessionv1.CreateSessionRequest{Title: "r", Path: file}},
		{"missing-dir-resume", &sessionv1.CreateSessionRequest{Title: "r", Path: file, ResumeId: "00000000-0000-0000-0000-000000000002"}},
		{"auto-approve", &sessionv1.CreateSessionRequest{Title: "r", Path: c.dir, Program: "ssh", AutoApprove: true}},
		{"bad-session-type", &sessionv1.CreateSessionRequest{Title: "r", Path: c.dir, SessionType: sessionv1.SessionType(99)}},
	} {
		_, err := c.create(tc.msg)
		c.errLine("create "+tc.step, err)
	}
}

// remoteStores records the rejection before SetRemoteDeps and the dial attempt
// after it, so a setter installed late must be read at call time.
func (c *createChar) remoteStores() {
	require.NoError(c.t, config.UpdateConfig(func(cfg *config.Config) error {
		cfg.Remotes = []config.RemoteConfig{{Name: "far", Host: "127.0.0.1:1", User: "u", BasePath: "/srv/base"}}
		return nil
	}))
	remote := &sessionv1.RemoteTarget{RemoteName: "far"}
	for _, tc := range []struct {
		step string
		msg  *sessionv1.CreateSessionRequest
	}{
		{"remote-tilde", &sessionv1.CreateSessionRequest{Title: "r", Path: "~/x", Remote: remote}},
		{"remote-type", &sessionv1.CreateSessionRequest{Title: "r", Path: "/srv/x", Remote: remote, SessionType: sessionv1.SessionType_SESSION_TYPE_ONE_OFF}},
		{"remote-unconfigured", &sessionv1.CreateSessionRequest{Title: "r", Path: "/srv/x", Remote: remote, SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY}},
	} {
		_, err := c.create(tc.msg)
		c.errLine("create "+tc.step, err)
	}
	knownHosts, err := sshremote.NewKnownHostsStore()
	require.NoError(c.t, err)
	c.svc.SetRemoteDeps(sshremote.NewKeyStore(), knownHosts)
	_, err = c.create(&sessionv1.CreateSessionRequest{Title: "r", Path: "/srv/x", Remote: remote, SessionType: sessionv1.SessionType_SESSION_TYPE_DIRECTORY})
	c.line("create remote-configured code=%s dialed=%t", connect.CodeOf(err),
		strings.Contains(fmt.Sprint(err), `failed to connect to remote "far"`))
}

func (c *createChar) otherRejections() {
	ctx := context.Background()
	base := &sessionv1.CreateSessionRequest{Title: "taken", Path: c.dir}
	for _, tc := range []struct {
		step string
		msg  *sessionv1.FanOutSessionRequest
	}{
		{"count-low", &sessionv1.FanOutSessionRequest{Count: 1, Base: base}},
		{"count-high", &sessionv1.FanOutSessionRequest{Count: 6, Base: base}},
		{"no-base", &sessionv1.FanOutSessionRequest{Count: 2}},
		{"too-many-overrides", &sessionv1.FanOutSessionRequest{Count: 2, Base: base, ProgramOverrides: []string{"a", "b", "c"}}},
		{"bad-branch", &sessionv1.FanOutSessionRequest{Count: 2, Base: &sessionv1.CreateSessionRequest{Title: "f", Path: c.dir, Branch: "-x"}}},
	} {
		_, err := c.svc.FanOutSession(ctx, connect.NewRequest(tc.msg))
		c.errLine("fanout "+tc.step, err)
	}
	_, err := c.svc.PickFanoutWinner(ctx, connect.NewRequest(&sessionv1.PickFanoutWinnerRequest{}))
	c.errLine("pick empty", err)
	_, err = c.svc.PickFanoutWinner(ctx, connect.NewRequest(&sessionv1.PickFanoutWinnerRequest{GroupId: "g", WinnerSessionId: "w"}))
	c.errLine("pick unknown-group", err)

	for _, msg := range []*sessionv1.PreviewDestinationPathRequest{
		{Mode: "bogus"},
		{Mode: "github_url", Input: "not a url"},
		{Mode: "new_worktree"},
	} {
		resp, previewErr := c.svc.PreviewDestinationPath(ctx, connect.NewRequest(msg))
		if previewErr != nil {
			c.errLine("preview "+msg.Mode, previewErr)
			continue
		}
		c.line("preview %s: %s", msg.Mode, svctest.CanonicalProto(c.t, resp.Msg))
	}

	for _, id := range []string{"", "nope", "taken"} {
		_, retryErr := c.svc.RetrySessionCreation(ctx, connect.NewRequest(&sessionv1.RetrySessionCreationRequest{Id: id}))
		c.errLine("retry "+id, retryErr)
		_, cancelErr := c.svc.CancelSessionCreation(ctx, connect.NewRequest(&sessionv1.CancelSessionCreationRequest{Id: id}))
		c.errLine("cancel "+id, cancelErr)
	}

	batch, err := c.svc.BatchCreateSessions(ctx, connect.NewRequest(&sessionv1.BatchCreateSessionsRequest{Sessions: []*sessionv1.BatchSessionRequest{
		{Path: c.dir},
		{Title: "nopath"},
		{Title: "taken", Path: c.dir},
		{Title: "dup", Path: filepath.Join(c.dir, "missing")},
		{Title: "dup", Path: c.dir},
	}}))
	require.NoError(c.t, err)
	c.line("batch: %s", svctest.CanonicalProto(c.t, batch.Msg))
}

func (c *createChar) success() {
	resp, err := c.create(&sessionv1.CreateSessionRequest{
		Title: "full", Path: c.dir, Program: c.agent, Category: "cat", Prompt: "p", InitialPrompt: "hello",
		Tags: []string{"a", "b"}, EnvVars: map[string]string{"K": "v"}, CliFlags: "--x", ExtraArgs: []string{"--y"},
		ProjectId: "proj", WorkflowId: "wf", AllowedTools: "Read", PermissionMode: "plan", AutoYes: true, OneShot: false,
	})
	require.NoError(c.t, err)
	c.destroy(resp.Msg.Session.Id)
	c.line("full response title=%q status=%s program=%q category=%q", resp.Msg.Session.Title, resp.Msg.Session.Status,
		resp.Msg.Session.Program, resp.Msg.Session.Category)
	c.waitTerminal("full")
	c.line("full phases=%q", c.takePhases())
	c.line("full events=%q", c.creationEvents("full"))
	c.recordInstance("full", c.svc.FindLiveInstance("full"))
	c.recordRegistration("full", "full")
	used, err := c.svc.promptStore.List(10)
	require.NoError(c.t, err)
	for _, p := range used {
		c.line("full prompt-history %q", p.Text)
	}
}

func (c *createChar) resolutionFailure() {
	c.svc.githubResolver = func(context.Context, string, []string) (string, *instance.GitHubRef, error) {
		return "", nil, errors.New("clone refused")
	}
	resp, err := c.create(&sessionv1.CreateSessionRequest{Title: "ghfail", Path: "https://github.com/acme/widgets", Program: c.agent})
	require.NoError(c.t, err)
	c.destroy(resp.Msg.Session.Id)
	c.line("ghfail response status=%s", resp.Msg.Session.Status)
	c.waitTerminal("ghfail")
	c.line("ghfail phases=%q", c.takePhases())
	c.line("ghfail events=%q", c.creationEvents("ghfail"))
	c.recordRegistration("ghfail", "ghfail")
	status, reason := c.svc.FindLiveInstance("ghfail").StatusAndFailureReason()
	c.line("ghfail status=%s reason=%q progress=%q", status, reason, c.svc.FindLiveInstance("ghfail").Snapshot().CreationProgress)

	retry, err := c.svc.RetrySessionCreation(context.Background(), connect.NewRequest(&sessionv1.RetrySessionCreationRequest{Id: "ghfail"}))
	require.NoError(c.t, err)
	c.line("ghfail retry: %s", svctest.CanonicalProto(c.t, retry.Msg))
	// A failed retry stays Creating: the epoch TryStartRetry bumps is never
	// persisted, so UpdateInstanceIfEpoch rejects the terminal write.
	var phases []string
	wait.RequireEventually(c.t, func() bool {
		phases = append(phases, c.takePhases()...)
		return slices.ContainsFunc(phases, func(p string) bool { return strings.HasPrefix(p, "Startup failed:") })
	}, pipelineEventuallyTimeout, 10*time.Millisecond)
	c.svc.waitForPendingCleanup()
	status, reason = c.svc.FindLiveInstance("ghfail").StatusAndFailureReason()
	c.line("ghfail after-retry status=%s reason=%q phases=%d first=%q", status, reason, len(phases), phases[0])
}

func (c *createChar) cancel() {
	started, returned, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	// Registered after Shutdown's cleanup, so it opens the gate first if the cancel never arrives.
	c.t.Cleanup(func() { close(release) })
	c.svc.githubResolver = func(ctx context.Context, _ string, _ []string) (string, *instance.GitHubRef, error) {
		close(started)
		defer close(returned)
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-release:
			return "", nil, errors.New("released by test cleanup")
		}
	}
	_, err := c.create(&sessionv1.CreateSessionRequest{Title: "cancelme", Path: "https://github.com/acme/widgets", Program: c.agent})
	require.NoError(c.t, err)
	select {
	case <-started:
	case <-time.After(pipelineEventuallyTimeout):
		c.t.Fatal("resolver not called")
	}
	got, err := c.svc.CancelSessionCreation(context.Background(), connect.NewRequest(&sessionv1.CancelSessionCreationRequest{Id: "cancelme"}))
	require.NoError(c.t, err)
	c.line("cancelme: %s", svctest.CanonicalProto(c.t, got.Msg))
	c.recordRegistration("cancelme", "cancelme")
	select {
	case <-returned:
	case <-time.After(pipelineEventuallyTimeout):
		c.t.Fatal("cancel did not stop the resolver")
	}
	c.svc.waitForPendingCleanup()
	c.takePhases()
}

func (c *createChar) spawners() {
	ctx := context.Background()
	dirInst, err := c.svc.CreateDirectorySession(ctx, c.dir, domain.SessionSpawnOptions{
		Title: "spawn-dir", Prompt: "dp", Tags: []string{"t"}, OneShot: true, Hidden: true, ProgramOverride: c.agent, AllowedTools: "Grep",
	})
	require.NoError(c.t, err)
	c.destroy(dirInst.GetStableID())
	c.recordInstance("spawn-dir", dirInst)
	c.recordRegistration("spawn-dir", "spawn-dir")
	c.waitCreated("spawn-dir")

	wt := filepath.Join(c.dir, "repo")
	require.NoError(c.t, os.MkdirAll(wt, 0o750))
	svctest.InitGitRepoWithCommit(c.t, wt)
	wtInst, err := c.svc.CreateWorktreeSession(ctx, wt, wt, domain.SessionSpawnOptions{Title: "spawn-wt", Prompt: "wp", ProgramOverride: c.agent})
	require.NoError(c.t, err)
	c.destroy(wtInst.GetStableID())
	c.recordInstance("spawn-wt", wtInst)
	c.recordRegistration("spawn-wt", "spawn-wt")
	c.waitCreated("spawn-wt")

	_, err = c.svc.CreateDirectorySession(ctx, c.dir, domain.SessionSpawnOptions{Title: "spawn-dir", ProgramOverride: c.agent})
	c.line("spawn duplicate rejected=%t", err != nil)

	// Review and diagnose sessions take their program from config defaults.
	cfgDir := filepath.Join(c.dir, "cfgrepo")
	require.NoError(c.t, os.MkdirAll(cfgDir, 0o750))
	c.useAgentAsDefaultProgram()
	item := &backlogdto.BacklogItemData{ID: "0123456789abcdef", RepoPath: cfgDir}
	review, err := c.svc.SpawnReviewSession(ctx, item, "", "review it")
	require.NoError(c.t, err)
	c.destroy(review.GetStableID())
	c.recordInstance("review", review)
	c.recordRegistration("review", review.Title)
	diag, err := c.svc.SpawnDiagnosticSession(ctx, item, "diagnose it")
	require.NoError(c.t, err)
	c.destroy(diag.GetStableID())
	c.recordInstance("diagnose", diag)
	c.line("diagnose category=%q", diag.Category)
}

func (c *createChar) useAgentAsDefaultProgram() {
	require.NoError(c.t, config.UpdateConfig(func(cfg *config.Config) error {
		cfg.DefaultProgram = c.agent
		return nil
	}))
}

func TestSessionCreateRPCs_ShouldMatchTheRecordedBehaviour_When_DrivenThroughSessionService(t *testing.T) {
	// Not t.Parallel(): isolated HOME, config dir and real tmux sessions.
	c := newCreateChar(t)
	require.NoError(t, c.storage.AddInstance(instance.NewPausedTestInstance("taken")))
	c.poller.AddInstance(instance.NewPausedTestInstance("taken"))

	c.rejections()
	c.remoteStores()
	c.otherRejections()
	c.success()
	c.resolutionFailure()
	c.cancel()
	c.spawners()

	svctest.CompareGolden(t, filepath.Join("testdata", "session_create.golden"), c.lines, *updateSessionCreateGolden)
}
