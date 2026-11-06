package backend

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type locateFake struct{ remote bool }

func (locateFake) Run(context.Context, string, string, ...string) ([]byte, error) { return nil, nil }
func (f locateFake) IsRemote() bool                                               { return f.remote }

func TestLocate(t *testing.T) {
	remote := &locateFake{remote: true}
	tests := []struct {
		name string
		r    Locatable
		want RepoLocation
	}{
		{"local runner", locateFake{}, Local{Root: "/repo"}},
		{"nil runner is local", nil, Local{Root: "/repo"}},
		{"remote runner", remote, Remote{Host: "box", Path: "/repo", Runner: remote}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Locate(tt.r, "box", "/repo")
			assert.Equal(t, tt.want, got)
			if _, isRemote := tt.want.(Remote); isRemote {
				assert.Same(t, remote, got.(Remote).Runner, "the runner value must travel unchanged")
			}
		})
	}
}

func TestLocateRemoteNeverLocal(t *testing.T) {
	_, ok := Locate(&locateFake{remote: true}, "", "/p").(Local)
	assert.False(t, ok)
}
