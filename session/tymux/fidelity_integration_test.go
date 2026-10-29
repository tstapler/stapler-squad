//go:build integration

package tymux

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/session/launchcmd"
)

// Round-trip fidelity: replay each testdata/fidelity/corpus/*.ansi file into a live tymuxd,
// render its snapshot with CellsToSGR (the web terminal's first paint / resync path), and diff
// the cell grid xterm.js builds from that against the grid it builds from the original bytes
// (what the live Attach stream shows). Any difference is a cell the redraw gets wrong.
// Needs bun plus `bun install` in testdata/fidelity/oracle; skips otherwise.

func TestLiveTymuxd_SnapshotRedraw_RoundTripsTerminalFidelity(t *testing.T) {
	bin := findTymuxdBinary(t)
	oracleDir := filepath.Join(fidelityDir, "oracle")
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not installed; the xterm.js oracle needs it")
	}
	if _, err := os.Stat(filepath.Join(oracleDir, "node_modules")); err != nil {
		t.Skipf("run `bun install` in %s first", oracleDir)
	}
	cases := corpusCases(t)
	require.NotEmpty(t, cases)

	daemon := startLiveTymuxd(t, bin, t.TempDir(), reserveLoopbackAddr(t))
	// Cleanup, not defer: parallel subtests only run after this function returns.
	t.Cleanup(func() { daemon.stop(t) })

	for _, area := range cases {
		t.Run(area.name, func(t *testing.T) {
			for _, c := range area.files {
				t.Run(c.name, func(t *testing.T) {
					t.Parallel()
					runFidelityCase(t, daemon.addr, oracleDir, c, area.name == knownGapArea)
				})
			}
		})
	}
}

func runFidelityCase(t *testing.T, addr, oracleDir string, c corpusCase, knownGap bool) {
	raw, err := os.ReadFile(c.path)
	require.NoError(t, err)
	abs, err := filepath.Abs(c.path)
	require.NoError(t, err)

	sess := NewTymuxGRPCSession(NewRealTransport("http://" + addr))
	// cat, not the shell: no prompt or rc files, and the pane stays alive after the dump.
	// -opost so the pty doesn't rewrite LF to CRLF: the oracle replays the file's exact bytes.
	sess.SetCommand(launchcmd.ExecCommand{Argv: []launchcmd.Arg{
		launchcmd.Word("sh"), launchcmd.Literal("-c"),
		launchcmd.Literal("stty -opost; cat " + launchcmd.Quote(abs) + "; sleep 60"),
	}})
	require.NoError(t, sess.Start(t.TempDir()))
	// Close blocks up to 5s waiting on the attach reader; the case is already decided, so don't wait.
	defer func() { go sess.Close() }()

	snap, rows, cols := settledRender(t, sess.(*tymuxGRPCSession))
	redraw, err := RenderSnapshotRedraw(snap)
	require.NoError(t, err)

	grids := runOracle(t, oracleDir, rows, cols, map[string][]byte{
		"live":    raw,
		"redrawn": []byte(redraw),
	})
	if !knownGap {
		require.GreaterOrEqual(t, nonBlankCells(grids["live"].Grid), 10, "corpus file leaves a (nearly) blank screen, so it can't prove anything")
	}
	diffs := diffScreens(grids["live"], grids["redrawn"])
	writeDiffReport(t, c, diffs)

	if knownGap {
		t.Logf("known gap: %d differing cells\n%s", len(diffs), strings.Join(firstN(diffs, 12), "\n"))
		return
	}
	require.Emptyf(t, diffs, "%d cells differ between the live stream and the snapshot redraw:\n%s",
		len(diffs), strings.Join(firstN(diffs, 25), "\n"))
}

// writeDiffReport dumps every differing cell to $FIDELITY_REPORT_DIR/<area>/<case>.diff (removing
// a stale file when the case now passes) so a whole-suite run can be triaged without -v log digging.
func writeDiffReport(t *testing.T, c corpusCase, diffs []string) {
	dir := os.Getenv("FIDELITY_REPORT_DIR")
	if dir == "" {
		return
	}
	out := filepath.Join(dir, filepath.Base(filepath.Dir(c.path)), c.name+".diff")
	require.NoError(t, os.MkdirAll(filepath.Dir(out), 0o755))
	if len(diffs) == 0 {
		_ = os.Remove(out)
		return
	}
	require.NoError(t, os.WriteFile(out, []byte(strings.Join(diffs, "\n")+"\n"), 0o644))
}

// settledRender polls until the pane's rendered text stops changing, then returns it with the grid size.
func settledRender(t *testing.T, s *tymuxGRPCSession) (snap *v1.PaneSnapshot, rows, cols int) {
	t.Helper()
	var last *v1.PaneSnapshot
	stable := 0
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		cur, err := s.capturePane(context.Background(), 0)
		require.NoError(t, err)
		text, err := CellsToSGR(cur.GetGrid(), cur.GetHyperlinks())
		require.NoError(t, err)
		if strings.TrimSpace(strings.ReplaceAll(text, "\n", "")) != "" && last != nil && proto.Equal(cur, last) {
			if stable++; stable >= 3 {
				return cur, len(cur.GetGrid()), len(cur.GetGrid()[0].GetCells())
			}
		} else {
			stable = 0
		}
		last = cur
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatalf("pane never settled; last snapshot %v", last)
	return
}
