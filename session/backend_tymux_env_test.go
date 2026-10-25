package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTymuxBackend_AppendLaunchEnv_ForwardsToManager(t *testing.T) {
	tests := []struct {
		name string
		env  []string
	}{
		{"single", []string{"DISPLAY=:99"}},
		{"several", []string{"A=1", "B=2"}},
		{"none", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeTymuxManagerForSetCommand{}
			NewTymuxBackend(fake).AppendLaunchEnv(tc.env...)
			assert.Equal(t, tc.env, fake.gotEnv)
		})
	}
}
