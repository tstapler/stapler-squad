package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/tmux"
)

func TestTmuxSessionKillAllowed_OwnershipPolicy(t *testing.T) {
	orig := readSessionOwnerUUID
	t.Cleanup(func() { readSessionOwnerUUID = orig })

	stub := func(marker string, err error) {
		readSessionOwnerUUID = func(context.Context, tmux.Socket, string) (string, error) { return marker, err }
	}

	tests := []struct {
		name    string
		marker  string
		err     error
		allowed []string
		want    bool
	}{
		{"owner matches", "uuid-a", nil, []string{"uuid-a"}, true},
		{"owner is a different instance", "uuid-other", nil, []string{"uuid-a"}, false},
		{"empty allowed list refuses", "uuid-a", nil, nil, false},
		{"marker never set (pre-fix pane) refuses", "", errors.New("unknown variable: STAPLER_SESSION_UUID"), []string{"uuid-a"}, false},
		{"session already gone is a no-op kill", "", errors.New("can't find session: x"), []string{"uuid-a"}, true},
		{"no tmux server running is a no-op kill", "", errors.New("no server running on /tmp/x"), nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub(tt.marker, tt.err)
			require.Equal(t, tt.want, tmuxSessionKillAllowed(t.Context(), "staplersquad_x", tt.allowed))
		})
	}
}
