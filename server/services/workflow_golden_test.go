package services

// Differential golden for the workflow RPC conversions and validation, recorded
// before the package move; the body is unchanged by it.
// Regenerate with -update-workflow-golden.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/stagecfg"
)

var updateWorkflowGolden = flag.Bool("update-workflow-golden", false, "rewrite testdata/workflow.golden")

func TestWorkflowGolden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }

	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	full := &ent.Workflow{
		ID: uuid.MustParse("11111111-2222-3333-4444-555555555555"), Slug: "s", Name: "n", Description: "d",
		Command: "c", TargetDirectory: "/t", InputTemplate: "{{input}}", SessionType: "directory", Model: "m",
		AgentType: "claude", CronExpression: "* * * * *", CronEnabled: true, Enabled: true, CreatedAt: at,
		UpdatedAt: at.Add(time.Hour), KeepSessions: 3, ArchiveAfterHours: 4, TriggerType: "webhook",
		GithubRepo: "o/r", GithubBranch: "b", WebhookSlug: "w", WebhookSecretEncrypted: "never-copied",
		EventFilter: "e", LabelFilter: "l", PromptTemplate: "p", LastFiredAt: &at,
	}
	add("full %s", svctest.CanonicalProto(t, entWorkflowToProto(full)))
	add("empty %s", svctest.CanonicalProto(t, entWorkflowToProto(&ent.Workflow{CreatedAt: at, UpdatedAt: at})))

	for _, trigger := range []string{"", "manual", "cron", "webhook", "github_push"} {
		for _, cron := range []bool{false, true} {
			add("resolve %q %v -> %q", trigger, cron, resolveTriggerType(trigger, cron))
		}
		for _, slug := range []string{"", "x"} {
			for _, repo := range []string{"", "o/r"} {
				add("consistency %q %q %q -> %v", trigger, slug, repo, validateTriggerTypeFieldConsistency(trigger, slug, repo))
			}
		}
	}

	for _, p := range []*events.WorkflowEventPayload{
		nil,
		{Kind: events.WorkflowChangeCreated, Workflow: full},
		{Kind: events.WorkflowChangeUpdated, Workflow: full},
		{Kind: events.WorkflowChangeDeleted, WorkflowID: "gone"},
		{Kind: events.WorkflowChangeRun, WorkflowID: "wf", SessionID: "sess"},
	} {
		evt := convertEventToWorkflowEvent(&events.Event{Timestamp: at, Seq: 9, WorkflowPayload: p})
		require.NotEmpty(t, evt.DeployId)
		evt.DeployId = "<deploy>"
		add("event %s", svctest.CanonicalProto(t, evt))
		forceWorkflowIsSnapshot(evt)
		add("snapshot %s", svctest.CanonicalProto(t, evt))
	}

	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	attachments := filepath.Join(dir, "att")
	require.NoError(t, os.MkdirAll(filepath.Join(attachments, "sub.png"), 0o750))
	for _, name := range []string{"att/a.PNG", "att/a.txt", "outside.png"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600))
	}
	require.NoError(t, os.Symlink(filepath.Join(dir, "outside.png"), filepath.Join(attachments, "link.png")))
	mask := func(s string) string { return strings.ReplaceAll(s, dir, "<dir>") }
	svc := NewWorkflowService(nil, nil, nil)
	check := func(label string, paths ...string) {
		got, err := svc.validateAttachmentPaths(paths)
		add("attach %s -> %s %s", label, mask(fmt.Sprint(got)), mask(svctest.CanonicalError(err)))
	}
	check("unset", filepath.Join(attachments, "a.PNG"))
	svc.SetAttachmentDir(filepath.Join(dir, "missing"))
	check("missing-dir", filepath.Join(attachments, "a.PNG"))
	svc.SetAttachmentDir(attachments)
	check("none")
	check("ok", filepath.Join(attachments, "a.PNG"))
	check("relative", "a.png")
	check("absent", filepath.Join(attachments, "nope.png"))
	check("outside", filepath.Join(dir, "outside.png"))
	check("symlink-out", filepath.Join(attachments, "link.png"))
	check("not-image", filepath.Join(attachments, "a.txt"))
	check("directory", filepath.Join(attachments, "sub.png"))
	check("too-many", make([]string, maxWorkflowAttachments+1)...)

	repo := stagecfg.NewEntWorkflowRepository(createTestEntClient(t).GetEntClient())
	live := NewWorkflowService(repo, nil, nil)
	base := func() *sessionv1.CreateWorkflowRequest {
		return &sessionv1.CreateWorkflowRequest{Slug: "golden", Name: "n", Command: "c", TargetDirectory: dir}
	}
	for label, mutate := range map[string]func(*sessionv1.CreateWorkflowRequest){
		"slug":   func(r *sessionv1.CreateWorkflowRequest) { r.Slug = "Bad Slug" },
		"name":   func(r *sessionv1.CreateWorkflowRequest) { r.Name = "" },
		"cmd":    func(r *sessionv1.CreateWorkflowRequest) { r.Command = "" },
		"target": func(r *sessionv1.CreateWorkflowRequest) { r.TargetDirectory = "" },
		"cron":   func(r *sessionv1.CreateWorkflowRequest) { r.CronExpression = "nope" },
	} {
		req := base()
		mutate(req)
		_, err := live.CreateWorkflow(ctx, connect.NewRequest(req))
		add("create %s -> %s", label, mask(svctest.CanonicalError(err)))
	}
	_, err = svc.CreateWorkflow(ctx, connect.NewRequest(base()))
	add("create no-repo -> %s", svctest.CanonicalError(err))
	_, err = live.ArchiveWorkflowSessions(ctx, connect.NewRequest(&sessionv1.ArchiveWorkflowSessionsRequest{WorkflowId: "x"}))
	add("archive no-storage -> %s", svctest.CanonicalError(err))
	fired, err := live.ListTriggerFireEvents(ctx, connect.NewRequest(&sessionv1.ListTriggerFireEventsRequest{}))
	require.NoError(t, err)
	add("fire-events no-repo -> %s", svctest.CanonicalProto(t, fired.Msg))

	// Map iteration above is random, so the golden is compared sorted.
	slices.Sort(out)
	svctest.CompareGolden(t, filepath.Join("testdata", "workflow.golden"), out, *updateWorkflowGolden)
}
