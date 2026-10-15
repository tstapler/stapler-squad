package tmux

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
)

// End to end over a real SSHRunner and an in-process sshd that runs commands through sh: the
// remote login has a German locale and a git that only speaks English under LC_ALL=C. The cli
// backend must still classify, because it carries RequiredGitEnv on the command line (an SSH
// session cannot carry env). Not parallel: it sets PATH and the locale for the sshd's children.
func TestCLIBackendOverSSHClassifiesUnderGermanRemoteLocale(t *testing.T) {
	bin := t.TempDir()
	stub := `#!/bin/sh
if [ "$LC_ALL" = "C" ]; then
  echo "fatal: not a git repository (or any of the parent directories): .git" >&2
else
  echo "fatal: kein Git-Repository (oder irgendein Elternverzeichnis): .git" >&2
fi
exit 128
`
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(stub), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LANG", "de_DE.UTF-8")
	t.Setenv("LC_ALL", "de_DE.UTF-8")

	srv := startRealExecTestSSHServer(t)
	runner := newTestSSHRunner(t, "git-env-remote", srv.Addr, newTestClientConfig(t, srv.HostKey))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := cli.New(nil).RepoRoot(ctx, backend.Remote{Host: "h", Path: backend.RemotePath(t.TempDir()), Runner: runner})

	assert.ErrorIs(t, err, backend.ErrNotARepo)
}
