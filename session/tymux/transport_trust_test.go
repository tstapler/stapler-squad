package tymux

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/tymux/clients/go/selecting"
)

// Upstream's CheckedDial refuses sockets another user could have planted. These tests pin that the
// socket paths squad and tymuxd actually produce (under the real os.TempDir(), e.g. /var/folders
// behind the root-owned /var symlink on macOS) pass it, and that a loosened directory still fails.

func listenUnix(t *testing.T, path string) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
}

func checkedDial(t *testing.T, path string) error {
	t.Helper()
	conn, err := selecting.CheckedDial(path, nil)(context.Background(), "unix", path, 0)
	if err == nil {
		_ = conn.Close()
	}
	return err
}

func TestCheckedDial_InstanceHashedSocketPathIsTrusted(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_INSTANCE", "trust-shape")
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("TYMUXD_SOCKET_PATH", "")
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	sock := ResolveDaemonConfig().SocketPath
	require.NotEmpty(t, sock)
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(sock)) })
	require.True(t, socketPathUsable(sock), "the resolved path fits sun_path")

	listenUnix(t, sock)

	require.NoError(t, checkedDial(t, sock), "instance-hashed path %s", sock)
}

func TestCheckedDial_TymuxdDefaultPathShapeIsTrusted(t *testing.T) {
	base, err := os.MkdirTemp(os.TempDir(), "ssq-trust")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	dir := filepath.Join(base, "tymuxd-"+strconv.Itoa(os.Getuid()))
	require.NoError(t, os.Mkdir(dir, 0o700))
	sock := filepath.Join(dir, "tymuxd.sock")

	listenUnix(t, sock)

	require.NoError(t, checkedDial(t, sock), "<TMPDIR>/tymuxd-<uid>/tymuxd.sock shape %s", sock)
}

func TestCheckedDial_GroupWritableDirectoryIsRefusedAndProbedAsAccessDenied(t *testing.T) {
	base, err := os.MkdirTemp(os.TempDir(), "ssq-trust")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	dir := filepath.Join(base, "loose")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.Chmod(dir, 0o770))
	sock := filepath.Join(dir, "tymuxd.sock")
	listenUnix(t, sock)

	require.ErrorIs(t, checkedDial(t, sock), selecting.ErrSocketUntrusted)
	require.Equal(t, daemonProbeAccessDenied, probeEndpoint(context.Background(), "unix", sock, nil),
		"an untrusted socket is alive-but-forbidden: never killed, never falls back to TCP")
}

func TestUnixSocketPath_UncleanOverrideIsCleaned(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", "http://127.0.0.1:19997")
	t.Setenv("TYMUXD_SOCKET_PATH", "/tmp//a/../x.sock")
	require.Equal(t, "/tmp/x.sock", unixSocketPath(ResolveDaemonConfig()))
}
