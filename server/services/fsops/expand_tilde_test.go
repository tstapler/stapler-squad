package fsops

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandTilde(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	cases := []struct {
		input    string
		expected string
	}{
		{"~", home + "/"},
		{"~/", home + "/"},
		{"~/projects", home + "/projects"},
		{"~/a/b/c/", home + "/a/b/c/"},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
	}

	for _, tc := range cases {
		got, err := ExpandTilde(tc.input)
		require.NoError(t, err, "input: %q", tc.input)
		assert.Equal(t, tc.expected, got, "input: %q", tc.input)
	}
}
