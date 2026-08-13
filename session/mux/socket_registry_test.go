package mux

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSocketRegistry_SetGet_Roundtrip(t *testing.T) {
	reg := NewSocketRegistry(t.TempDir())

	entry := RegistryEntry{
		SocketPath:  "/tmp/ssq-mux-123.sock",
		SessionName: "my-session",
		LastSeen:    time.Now().UTC().Truncate(time.Second),
	}
	reg.Set("my-session", entry)

	got, ok := reg.Get("my-session")
	require.True(t, ok)
	assert.Equal(t, entry.SocketPath, got.SocketPath)
	assert.Equal(t, entry.SessionName, got.SessionName)
	assert.True(t, entry.LastSeen.Equal(got.LastSeen))
}

func TestSocketRegistry_Get_Missing(t *testing.T) {
	reg := NewSocketRegistry(t.TempDir())
	_, ok := reg.Get("does-not-exist")
	assert.False(t, ok)
}

func TestSocketRegistry_Delete(t *testing.T) {
	reg := NewSocketRegistry(t.TempDir())
	reg.Set("sess", RegistryEntry{SocketPath: "/tmp/x.sock", SessionName: "sess", LastSeen: time.Now()})
	reg.Delete("sess")
	_, ok := reg.Get("sess")
	assert.False(t, ok)
}

func TestSocketRegistry_LoadSave_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	reg1 := NewSocketRegistry(dir)
	reg1.Set("a", RegistryEntry{SocketPath: "/tmp/a.sock", SessionName: "a", LastSeen: time.Now().UTC().Truncate(time.Second)})
	reg1.Set("b", RegistryEntry{SocketPath: "/tmp/b.sock", SessionName: "b", LastSeen: time.Now().UTC().Truncate(time.Second)})

	reg2 := NewSocketRegistry(dir)
	require.NoError(t, reg2.Load())

	entA, ok := reg2.Get("a")
	require.True(t, ok)
	assert.Equal(t, "/tmp/a.sock", entA.SocketPath)

	entB, ok := reg2.Get("b")
	require.True(t, ok)
	assert.Equal(t, "/tmp/b.sock", entB.SocketPath)
}

func TestSocketRegistry_Load_MissingFile_NoError(t *testing.T) {
	reg := NewSocketRegistry(t.TempDir())
	require.NoError(t, reg.Load())
}

func TestSocketRegistry_PruneStale_RemovesOldMissingSocket(t *testing.T) {
	reg := NewSocketRegistry(t.TempDir())
	// Stale entry — socket does not exist on disk.
	reg.Set("stale", RegistryEntry{
		SocketPath:  "/tmp/nonexistent-socket-xyzzy.sock",
		SessionName: "stale",
		LastSeen:    time.Now().Add(-48 * time.Hour),
	})

	reg.PruneStale(24 * time.Hour)

	_, ok := reg.Get("stale")
	assert.False(t, ok, "stale entry with missing socket should be pruned")
}

func TestSocketRegistry_PruneStale_KeepsEntryWithExistingSocket(t *testing.T) {
	dir := t.TempDir()
	// Create an actual socket file so PruneStale won't remove it.
	socketPath := filepath.Join(dir, "live.sock")
	f, err := os.Create(socketPath)
	require.NoError(t, err)
	f.Close()

	reg := NewSocketRegistry(t.TempDir())
	reg.Set("live", RegistryEntry{
		SocketPath:  socketPath,
		SessionName: "live",
		LastSeen:    time.Now().Add(-48 * time.Hour), // old timestamp but socket exists
	})

	reg.PruneStale(24 * time.Hour)

	_, ok := reg.Get("live")
	assert.True(t, ok, "entry with existing socket file should NOT be pruned even if old")
}
