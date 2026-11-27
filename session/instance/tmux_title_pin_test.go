package instance

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// A session reloaded after a rename must address the tmux session it already has.
func TestWireTmuxSession_should_UsePinnedTitle_When_SessionWasRenamed(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "after-rename", TmuxTitle: "before-rename"}
	pm := &recordingLaunchPM{}
	inst.processManager = pm

	inst.wireTmuxSession(ShellLaunch("true"))

	assert.Equal(t, tmux.NewSessionName("before-rename", tmux.TmuxPrefix).String(), pm.wired.GetSanitizedName())
}

// recordingLaunchPM captures the tmux session wireTmuxSession builds, without running tmux.
type recordingLaunchPM struct {
	noopProcessManager
	wired *tmux.TmuxSession
}

func (r *recordingLaunchPM) WireLaunch(spec LaunchSpec) { r.wired = spec.NewTmuxSession() }

func TestRename_should_PinOnlyTheFirstTitle_When_SessionHasStarted(t *testing.T) {
	t.Parallel()
	started := &Instance{Title: "first"}
	started.started.Store(true)
	assert.NoError(t, started.Rename("second"))
	assert.NoError(t, started.Rename("third"))
	assert.Equal(t, "first", started.Snapshot().TmuxTitle)

	// No tmux session exists yet, so its name can still follow the title.
	pending := &Instance{Title: "first"}
	assert.NoError(t, pending.Rename("second"))
	assert.Empty(t, pending.Snapshot().TmuxTitle)
}
