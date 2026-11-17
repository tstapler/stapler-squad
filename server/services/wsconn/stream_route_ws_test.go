package wsconn

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Every (backend, managed, STAPLER_SQUAD_USE_CONTROL_MODE) combination: only tymux overrides
// the env opt-out, because the capture-pane fallback addresses a tmux session by name.
func TestControlModeStreamingFor_Matrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     string
		managed bool
		backend domain.ProcessManagerBackend
		want    bool
	}{
		{"tmux default", "", true, domain.BackendTmux, true},
		{"tmux explicit true", "true", true, domain.BackendTmux, true},
		{"tmux opted out", "false", true, domain.BackendTmux, false},
		{"unpinned opted out", "false", true, "", false},
		{"tymux default", "", true, domain.BackendTymux, true},
		{"tymux opted out still streams", "false", true, domain.BackendTymux, true},
		{"unmanaged tmux", "", false, domain.BackendTmux, false},
		{"unmanaged tymux", "", false, domain.BackendTymux, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, controlModeStreamingFor(tc.env, tc.managed, tc.backend))
		})
	}
}
