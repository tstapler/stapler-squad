package github

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPRKey_should_NormalizeHostAndRejectInvalid_When_Constructed(t *testing.T) {
	k, err := NewPRKey("", "Acme", "api", 3)
	require.NoError(t, err)
	require.Equal(t, "github.com", k.Host())
	require.Equal(t, "github.com/Acme/api#3", k.String())
	require.Equal(t, mustPRKey(t, "GitHub.com", "acme", "API", 3).Key(), k.Key())

	for _, bad := range []struct {
		owner, repo string
		n           int
	}{{"", "r", 1}, {"o", "", 1}, {"o", "r", 0}, {"o", "r", -2}} {
		_, err := NewPRKey("", bad.owner, bad.repo, bad.n)
		require.Error(t, err)
	}
	require.False(t, PRKey{}.IsValid())
}
