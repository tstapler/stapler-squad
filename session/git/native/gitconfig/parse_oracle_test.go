//go:build gitoracle

package gitconfig

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

// TestTokenizerAgreesWithGit runs every tokenizer row through `git config -f f.cfg --list -z`.
// Git's verdict is the oracle: a row where the two disagree is a failing test, not a limit.
func TestTokenizerAgreesWithGit(t *testing.T) {
	gitoracle.RequireGit(t)
	home := t.TempDir()
	for _, c := range tokenizerCases() {
		t.Run(c.name, func(t *testing.T) {
			f := filepath.Join(home, "f.cfg")
			writeTestFile(t, f, c.content)
			out, _, err := gitRun(t, home, cleanEnv(home), "config", "-f", f, "--list", "-z")
			got, perr := Parse([]byte(c.content))
			if err != nil {
				assert.Error(t, perr, "git rejects this file, the tokenizer must too")
				return
			}
			require.NoError(t, perr, "git accepts this file")
			assert.Equal(t, parseZList(out), listing(got))
			assert.Equal(t, c.wantErr, false, "the table claims an error but git accepts the file")
		})
	}
}
