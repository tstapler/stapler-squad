package services

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess proves the
// Claude `--settings '{"env":{...}}'` override (claudeSettingsEnvOverrideArgs) survives
// the real CreateSession -> shell -> tmux path: a fake `claude` executable records its
// argv, and the recorded JSON must equal the registered env with a hostile value intact
// and no command substitution executed.
//
// This is AC4a (flag delivery). Whether Claude itself ranks --settings above a global
// settings.json env block (AC4b) is upstream behaviour this test cannot observe.
//
// Socket rule (tmuxsocketscope): every tmux call here builds its argv with
// tmux.ResolveSocket(...).Args, never a hand-written "-L" literal.
func TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess(t *testing.T) {
	// Not t.Parallel(): real tmux on this service's isolated socket.
	tmuxBin := tmux.Binary()
	if _, err := exec.LookPath(tmuxBin); err != nil {
		t.Skip("tmux not available")
	}
	envtest.NewIsolatedStateDir(t)
	t.Setenv("HOME", t.TempDir())

	tmp := t.TempDir()
	argvOut := filepath.Join(tmp, "argv.txt")
	pwned := filepath.Join(tmp, "pwned")
	// Must be named "claude": the --settings override is only appended when the
	// program is recognised as Claude (session/instance_tmux.go buildClaudeCommand).
	fakeClaude := filepath.Join(tmp, "claude")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > '" + argvOut + ".tmp'\n" +
		"mv '" + argvOut + ".tmp' '" + argvOut + "'\n" +
		"exec sleep 300\n"
	require.NoError(t, os.WriteFile(fakeClaude, []byte(script), 0o755))

	const (
		baseURLKey = "ANTHROPIC_BASE_URL"
		baseURLVal = "http://127.0.0.1:47000"
		hostileKey = "SSQ_HOSTILE"
	)
	hostileVal := "it's $(touch " + pwned + ") `touch " + pwned + "` \"q\" a=b"
	wantEnv := map[string]string{baseURLKey: baseURLVal, hostileKey: hostileVal}

	ctx := context.Background()
	_, err := NewDefaultsService().UpsertProgramConfig(ctx, connect.NewRequest(&sessionv1.UpsertProgramConfigRequest{
		Program: &sessionv1.ProgramConfigProto{
			Id: "netflix-model-gateway", Label: "Netflix Model Gateway",
			Command: fakeClaude,
			Env:     wantEnv,
		},
	}))
	require.NoError(t, err)

	repoDir := t.TempDir()
	initGitRepoWithCommit(t, repoDir)

	svc := newCreateTestService(t, createTestStorage(t))
	resp, err := svc.CreateSession(ctx, connect.NewRequest(&sessionv1.CreateSessionRequest{
		Title:       "settings-env-repro",
		Path:        repoDir,
		Branch:      "settings-env-repro",
		SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
		Program:     "netflix-model-gateway",
	}))
	require.NoError(t, err)
	t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })

	inst := svc.FindLiveInstance(resp.Msg.Session.Id)
	require.NotNil(t, inst)

	wait.RequireEventually(t, func() bool {
		if session.Status(inst.GetStatus()) == session.Stopped {
			t.Fatalf("session reached Stopped before the fake claude recorded its argv: spawn failed")
		}
		_, statErr := os.Stat(argvOut)
		return statErr == nil
	}, 30*time.Second, 100*time.Millisecond, "fake claude must record its argv")

	raw, err := os.ReadFile(argvOut)
	require.NoError(t, err)
	t.Logf("recorded argv (one element per line):\n%s", raw)

	argv := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	idx := -1
	for i, a := range argv {
		if a == "--settings" {
			idx = i
			break
		}
	}
	require.GreaterOrEqual(t, idx, 0, "argv must carry the --settings override")
	require.Less(t, idx+1, len(argv), "--settings must be followed by its JSON value")

	var settings map[string]map[string]string
	require.NoError(t, json.Unmarshal([]byte(argv[idx+1]), &settings), "--settings value must be valid JSON")
	assert.Equal(t, map[string]map[string]string{"env": wantEnv}, settings, "--settings JSON must carry the registered env intact")

	assert.NoFileExists(t, pwned, "hostile value must not be executed by a shell")

	tmuxName := inst.GetTmuxSessionName()
	require.NotEmpty(t, tmuxName)
	args := tmux.ResolveSocket(svc.testTmuxServerSocket).Args("show-environment", "-t", tmuxName)
	out, err := safeexec.CommandContext(ctx, tmuxBin, args...).CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), baseURLKey+"="+baseURLVal)
	assert.Contains(t, string(out), hostileKey+"="+hostileVal, "tmux -e must carry the hostile value byte-for-byte")
}
