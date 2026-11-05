package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
)

// Every (backend, managed, STAPLER_SQUAD_USE_CONTROL_MODE) combination: only tymux overrides
// the env opt-out, because the capture-pane fallback addresses a tmux session by name.
func TestControlModeStreamingFor_Matrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     string
		managed bool
		backend session.ProcessManagerBackend
		want    bool
	}{
		{"tmux default", "", true, session.BackendTmux, true},
		{"tmux explicit true", "true", true, session.BackendTmux, true},
		{"tmux opted out", "false", true, session.BackendTmux, false},
		{"unpinned opted out", "false", true, "", false},
		{"tymux default", "", true, session.BackendTymux, true},
		{"tymux opted out still streams", "false", true, session.BackendTymux, true},
		{"unmanaged tmux", "", false, session.BackendTmux, false},
		{"unmanaged tymux", "", false, session.BackendTymux, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, controlModeStreamingFor(tc.env, tc.managed, tc.backend))
		})
	}
}
