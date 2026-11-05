package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tymux"
)

// daemonPane is a tymux daemon stand-in whose redraw is the real RenderSnapshotRedraw of a snapshot.
type daemonPane struct {
	paneFake
	snap *v1.PaneSnapshot
}

func (d *daemonPane) CapturePaneRedraw(context.Context) (string, bool, bool, error) {
	d.redrawCalls.Add(1)
	data, err := tymux.RenderSnapshotRedraw(d.snap)
	return data, d.snap.GetAltScreen(), d.snap.GetModes() != nil, err
}

func (d *daemonPane) SetWindowSize(int, int) error { return nil }
func (d *daemonPane) SubscribeToControlModeUpdates() (string, chan []byte) {
	ch := make(chan []byte)
	close(ch) // quiescence returns at once
	return "e2e", ch
}
func (d *daemonPane) UnsubscribeFromControlModeUpdates(string) {}

type e2eOracleScreen struct {
	Grid  [][][]string `json:"grid"`
	State struct {
		Title        string `json:"title"`
		AltScreen    bool   `json:"altScreen"`
		ScrollTop    uint32 `json:"scrollTop"`
		ScrollBottom uint32 `json:"scrollBottom"`
		CursorHidden bool   `json:"cursorHidden"`
		Modes        struct {
			AppCursorKeys  bool `json:"appCursorKeys"`
			BracketedPaste bool `json:"bracketedPaste"`
		} `json:"modes"`
		Cursor struct{ Row, Col uint32 } `json:"cursor"`
	} `json:"state"`
}

// replayInXterm feeds stream to the xterm.js oracle (session/tymux/testdata/fidelity/oracle) and
// returns the resulting screen. Skips when bun or the oracle's node_modules is missing, unless
// FIDELITY_REQUIRE_ORACLE is set (CI), mirroring session/tymux's requireOracle.
func replayInXterm(t *testing.T, rows, cols int, stream []byte) e2eOracleScreen {
	t.Helper()
	dir := filepath.Join("..", "..", "session", "tymux", "testdata", "fidelity", "oracle")
	missing := ""
	if _, err := exec.LookPath("bun"); err != nil {
		missing = "bun is not installed"
	} else if _, err := os.Stat(filepath.Join(dir, "node_modules")); err != nil {
		missing = "run `bun install --frozen-lockfile` in " + dir
	}
	if missing != "" {
		if os.Getenv("FIDELITY_REQUIRE_ORACLE") != "" {
			t.Fatalf("FIDELITY_REQUIRE_ORACLE is set but the xterm.js oracle is unavailable: %s", missing)
		}
		t.Skip(missing)
	}
	in, err := json.Marshal(map[string]any{"rows": rows, "cols": cols,
		"streams": map[string]string{"s": base64.StdEncoding.EncodeToString(stream)}})
	require.NoError(t, err)
	cmd := safeexec.CommandContext(context.Background(), "bun", "oracle.mjs")
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(in)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoErrorf(t, err, "oracle failed: %s", stderr.String())
	var screens map[string]e2eOracleScreen
	require.NoError(t, json.Unmarshal(out, &screens))
	return screens["s"]
}

func e2eRow(text string) *v1.Row {
	row := &v1.Row{}
	for _, r := range text {
		row.Cells = append(row.Cells, &v1.Cell{Text: string(r)})
	}
	return row
}

func rowText(row [][]string) string {
	var b strings.Builder
	for _, c := range row {
		b.WriteString(c[0])
	}
	return strings.TrimRight(b.String(), " ")
}

// A pane in a TUI's state (alt screen, app cursor keys, bracketed paste, scroll region, title,
// cursor mid-screen) flows through the real Instance, the real hub FirstPaint and the real
// WebSocket framing; replayed into xterm.js it reproduces that state, then a resize broadcast
// keeps it.
func TestFirstPaint_should_RestoreTerminalStateInXterm_EndToEnd(t *testing.T) {
	const rows, cols = 5, 20
	pane := &daemonPane{snap: &v1.PaneSnapshot{
		Grid:      []*v1.Row{e2eRow("hello"), e2eRow("world"), {}, {}, {}},
		Modes:     &v1.TerminalModes{AppCursorKeys: true, BracketedPaste: true},
		AltScreen: true, Title: "vim", ScrollTop: 1, ScrollBottom: 3, CursorRow: 2, CursorCol: 4,
	}}
	inst := session.NewStartedInstanceForTest(t, "e2e", session.NewTymuxBackend(pane))
	hub := streamhub.NewStreamHub("e2e", inst, streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(5*time.Millisecond), streamhub.WithQuiescenceQuietPeriod(time.Millisecond))
	stream, conn, cleanup := createTestWebSocketPair(t)
	defer cleanup()
	id := attachFirstPaintSubscriber(t, stream, hub)

	require.NoError(t, newFirstPaintHandler().sendHubInitialSnapshot(stream, hub, id, inst, "e2e"))
	first := readOutput(t, conn)
	hub.RequestResize(context.Background(), id, mustHubSize(t, cols, rows))
	broadcast := readOutput(t, conn)

	assert.True(t, first.GetAltScreenActive())
	for name, out := range map[string]*[]byte{"first paint": &first.Data, "resize broadcast": &broadcast.Data} {
		screen := replayInXterm(t, rows, cols, *out)
		assert.Equal(t, "hello", rowText(screen.Grid[0]), name)
		assert.Equal(t, "world", rowText(screen.Grid[1]), name)
		assert.Equal(t, "vim", screen.State.Title, name)
		assert.True(t, screen.State.AltScreen, name)
		assert.True(t, screen.State.Modes.AppCursorKeys, name)
		assert.True(t, screen.State.Modes.BracketedPaste, name)
		assert.EqualValues(t, 1, screen.State.ScrollTop, name)
		assert.EqualValues(t, 3, screen.State.ScrollBottom, name)
		assert.EqualValues(t, 2, screen.State.Cursor.Row, name)
		assert.EqualValues(t, 4, screen.State.Cursor.Col, name)
	}
	assert.EqualValues(t, 2, pane.redrawCalls.Load(), "one capture for the first paint, one for the resize broadcast, none at attach")
	assert.EqualValues(t, 0, pane.textCaptures.Load())
}

func mustHubSize(t *testing.T, cols, rows int) streamhub.TerminalSize {
	t.Helper()
	size, err := streamhub.NewTerminalSize(cols, rows)
	require.NoError(t, err)
	return size
}
