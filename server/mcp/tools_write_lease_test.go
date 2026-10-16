package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

func submitHarnessInstance(th *terminalHandlers) *session.Instance {
	return th.store.(*stubStore).instances[0]
}

// T-WL-04 (MCP half): a held lease is a retryable WRITE_IN_PROGRESS with 0 writes,
// and the write goes through once the lease frees.
func TestMCPWriteTools_ShouldReturnRetryableWriteInProgressWithZeroWrites_WhenTheLeaseIsHeld(t *testing.T) {
	t.Parallel()
	th, pm := newSubmitHarness(t)
	inst := submitHarnessInstance(th)
	held, ok := inst.TryTerminalWriteLease(session.LeaseWriterDriver)
	require.True(t, ok)

	calls := map[string]func() (map[string]interface{}, error){
		"write_to_session": func() (map[string]interface{}, error) {
			res, err := th.writeToSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": "submit-s1", "input": "x"}))
			return parseResult(t, res), err
		},
		"run_command": func() (map[string]interface{}, error) {
			res, err := th.runCommand(context.Background(), makeToolReq(map[string]interface{}{"session_id": "submit-s1", "command": "ls", "timeout_seconds": float64(1)}))
			return parseResult(t, res), err
		},
		"steer_session": func() (map[string]interface{}, error) {
			res, err := th.steerSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": "submit-s1", "message": "m"}))
			return parseResult(t, res), err
		},
		"send_control": func() (map[string]interface{}, error) {
			res, err := th.sendControl(context.Background(), makeToolReq(map[string]interface{}{"session_id": "submit-s1", "key": "C"}))
			return parseResult(t, res), err
		},
	}
	for name, call := range calls {
		m, err := call()
		require.NoError(t, err, name)
		assert.Equal(t, false, m["success"], name)
		assert.Equal(t, errWriteInProgress, m["error"].(map[string]interface{})["code"], name)
	}
	assert.Empty(t, pm.writes(), "a held lease must perform 0 writes")

	held.Release()
	m, err := calls["write_to_session"]()
	require.NoError(t, err)
	assert.Equal(t, true, m["success"])
	session.AssertLeaseFree(t, inst)
}

// T-WL-04 (nudge half): a busy lease never consumes the dispatch's one nudge attempt.
func TestWriteNudge_ShouldNotConsumeTheNudgeAttempt_WhenTheLeaseIsBusy(t *testing.T) {
	storage := newTestBacklogStorage(t)
	_, sessUUID := setupDiagnoseSession(t, storage)
	dh := &diagnoseHandlers{storage: storage}
	pm := &recordingPM{}
	target := session.NewStartedInstanceForTest(t, "nudge-target", pm)
	ctx := context.Background()

	held, ok := target.TryTerminalWriteLease(session.LeaseWriterDriver)
	require.True(t, ok)
	res := dh.writeNudge(ctx, sessUUID, target, "please continue")
	require.NotNil(t, res)
	assert.Equal(t, errWriteInProgress, parseResult(t, res)["error"].(map[string]interface{})["code"])
	assert.Empty(t, pm.writes())

	held.Release()
	assert.Nil(t, dh.writeNudge(ctx, sessUUID, target, "please continue"),
		"the attempt was not claimed while busy, so the retry is allowed and writes")
	assert.Equal(t, []string{"please continue", session.EnterKeySequence}, pm.writes())
	session.AssertLeaseFree(t, target)
}
