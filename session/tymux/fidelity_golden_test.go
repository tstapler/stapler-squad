package tymux

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// Daemon-free fidelity checks. testdata/fidelity/golden/<area>/<case>.json is the grid xterm.js
// builds from the raw corpus file (what the browser shows live). Regenerate with
//
//	UPDATE_FIDELITY_GOLDEN=1 go test ./session/tymux -run FidelityGolden_Update
//
// - RendererRoundTrip: golden grid -> v1 cells -> CellsToSGR -> xterm.js must equal the golden
//   grid. One bun call for the whole corpus, no daemon or pty.
// - RedrawFromDirtyTerminal: the same redraw replayed into a terminal with every resettable mode
//   turned on, so a dropped reset in RenderSnapshotRedraw fails.
// - The emulator side (raw bytes -> daemon cells == golden) lives in the tymux repo: it vendors a
//   subset of this corpus and these goldens (crates/vt100/tests/fixtures/fidelity) and runs the
//   full set when TYMUX_FIDELITY_DIR points at this testdata/fidelity directory.

const (
	goldenRows = 24
	goldenCols = 80
)

// goldenFile stores only non-default cells: [row, col, text, fg, bg, flags, ulStyle, ulColor, link].
type goldenFile struct {
	Rows  int         `json:"rows"`
	Cols  int         `json:"cols"`
	State oracleState `json:"state"`
	Cells [][]any     `json:"cells"`
}

var blankCell = oracleCell{" ", "default", "default", "", "", "default", ""}

func goldenPath(area, name string) string {
	return filepath.Join(fidelityDir, "golden", area, name+".json")
}

// requireOracle returns the oracle directory. When bun or the oracle's node_modules is missing
// it skips (local runs without bun), unless FIDELITY_REQUIRE_ORACLE is set, which CI sets in the
// jobs that install the oracle (.github/actions/setup-fidelity-oracle): there a missing oracle
// must fail, or the whole corpus would silently go untested. It's an opt-in rather than keyed on
// CI because other CI jobs also run this package without bun.
func requireOracle(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(fidelityDir, "oracle")
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
	return dir
}

func TestFidelityGolden_Update(t *testing.T) {
	if os.Getenv("UPDATE_FIDELITY_GOLDEN") == "" {
		t.Skip("set UPDATE_FIDELITY_GOLDEN=1 to regenerate the golden grids")
	}
	dir := requireOracle(t)
	streams := map[string][]byte{}
	for _, area := range corpusCases(t) {
		for _, c := range area.files {
			raw, err := os.ReadFile(c.path)
			require.NoError(t, err)
			streams[area.name+"/"+c.name] = raw
		}
	}
	grids := runOracle(t, dir, goldenRows, goldenCols, streams)
	for key, screen := range grids {
		area, name, _ := strings.Cut(key, "/")
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath(area, name)), 0o755))
		require.NoError(t, os.WriteFile(goldenPath(area, name), encodeGolden(screen), 0o644))
	}
}

func TestFidelityGolden_RendererRoundTrip(t *testing.T) {
	dir := requireOracle(t)
	goldens := loadGoldens(t)
	require.NotEmpty(t, goldens, "no golden grids; run the Update test")

	streams := map[string][]byte{}
	for key, g := range goldens {
		redraw, err := RenderSnapshotRedraw(goldenToSnapshot(g))
		require.NoError(t, err)
		streams[key] = []byte(redraw)
	}
	redrawn := runOracle(t, dir, goldenRows, goldenCols, streams)

	keys := make([]string, 0, len(goldens))
	for k := range goldens {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			diffs := diffScreens(goldens[key], redrawn[key])
			require.Emptyf(t, diffs, "%d cells differ after CellsToSGR:\n%s", len(diffs), strings.Join(firstN(diffs, 25), "\n"))
		})
	}
}

// dirtyTerminalPrefix turns ON every piece of state a redraw must reset, the way a client that
// was showing a different app would be: input/mouse modes, a scroll region, IRM/LNM/DECOM/DECSCNM,
// no autowrap, custom tab stops, DEC graphics, a cursor style and a hidden cursor, a title, and
// an alt screen with content. Primary-buffer state (region, tab stops, charsets) is set BEFORE
// ?1049h: xterm.js scopes it to the buffer / restores it on ?1049l, so setting it after would
// be discarded by the redraw's own screen switch and prove nothing.
const dirtyTerminalPrefix = esc + "[?2004h" + esc + "[?1004h" + esc + "[?1h" + esc + "=" + // paste, focus, DECCKM, DECKPAM
	esc + "[?1003h" + esc + "[?1006h" + // any-motion mouse, SGR encoding
	esc + "[5;10r" + // DECSTBM
	esc + "[4h" + esc + "[20h" + esc + "[?6h" + esc + "[?7l" + esc + "[?5h" + // IRM, LNM, DECOM, no autowrap, DECSCNM
	esc + "[3g" + esc + "[1;5H" + esc + "H" + // custom tab stop at column 5
	esc + "(0" + esc + ")0" + "\x0e" + // DEC graphics in G0/G1, shifted out
	esc + "[3 q" + esc + "[?25l" + // blinking underline cursor, hidden
	esc + "]2;stale title" + st +
	esc + "[?1049h" + "ALT SCREEN CONTENT" // alt screen with content

// TestFidelityGolden_RedrawFromDirtyTerminal replays the redraw into a terminal that is in the
// state above and expects exactly the golden state, so dropping any reset in RenderSnapshotRedraw
// fails here (a redraw into a pristine terminal can't tell a missing reset from a no-op).
func TestFidelityGolden_RedrawFromDirtyTerminal(t *testing.T) {
	dir := requireOracle(t)
	goldens := loadGoldens(t)
	require.NotEmpty(t, goldens, "no golden grids; run the Update test")

	streams := map[string][]byte{}
	for key, g := range goldens {
		redraw, err := RenderSnapshotRedraw(goldenToSnapshot(g))
		require.NoError(t, err)
		streams[key] = []byte(dirtyTerminalPrefix + redraw)
	}
	redrawn := runOracle(t, dir, goldenRows, goldenCols, streams)

	keys := make([]string, 0, len(goldens))
	for k := range goldens {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			diffs := diffScreens(goldens[key], redrawn[key])
			require.Emptyf(t, diffs, "%d differences after redrawing into a dirty terminal:\n%s", len(diffs), strings.Join(firstN(diffs, 25), "\n"))
		})
	}
}

func encodeGolden(screen oracleScreen) []byte {
	grid := screen.Grid
	var b strings.Builder
	state, _ := json.Marshal(screen.State)
	b.WriteString("{\"rows\":" + strconv.Itoa(len(grid)) + ",\"cols\":" + strconv.Itoa(len(grid[0])) +
		",\n\"state\":" + string(state) + ",\n\"cells\":[\n")
	first := true
	for y, row := range grid {
		for x, c := range row {
			if c == blankCell {
				continue
			}
			if !first {
				b.WriteString(",\n")
			}
			first = false
			line, _ := json.Marshal([]any{y, x, c[0], c[1], c[2], c[3], c[4], c[5], c[6]})
			b.Write(line)
		}
	}
	b.WriteString("\n]}\n")
	return []byte(b.String())
}

func loadGoldens(t *testing.T) map[string]oracleScreen {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(fidelityDir, "golden", "*", "*.json"))
	require.NoError(t, err)
	out := map[string]oracleScreen{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		require.NoError(t, err)
		var f goldenFile
		require.NoError(t, json.Unmarshal(raw, &f))
		grid := make([][]oracleCell, f.Rows)
		for y := range grid {
			grid[y] = make([]oracleCell, f.Cols)
			for x := range grid[y] {
				grid[y][x] = blankCell
			}
		}
		for _, c := range f.Cells {
			var cell oracleCell
			for i := range cell {
				cell[i], _ = c[2+i].(string)
			}
			grid[int(c[0].(float64))][int(c[1].(float64))] = cell
		}
		key := filepath.Base(filepath.Dir(p)) + "/" + strings.TrimSuffix(filepath.Base(p), ".json")
		out[key] = oracleScreen{Grid: grid, State: f.State}
	}
	return out
}

// goldenToSnapshot inverts the oracle's cell encoding into the proto the daemon would send.
func goldenToSnapshot(screen oracleScreen) *v1.PaneSnapshot {
	snap := snapshotStateFromOracle(screen.State)
	linkIdx := map[string]uint32{}
	for _, row := range screen.Grid {
		r := &v1.Row{}
		for _, c := range row {
			cell := &v1.Cell{Text: c[0], Fg: packOracleColor(c[1]), Bg: packOracleColor(c[2]), UnderlineColor: packOracleColor(c[5])}
			for _, f := range strings.Split(c[3], ",") {
				cell.Attrs |= oracleFlagBits[f]
			}
			cell.UnderlineStyle = oracleUnderlineStyles[c[4]]
			if c[6] != "" {
				idx, ok := linkIdx[c[6]]
				if !ok {
					uri, id, _ := strings.Cut(c[6], "\x00")
					snap.Hyperlinks = append(snap.Hyperlinks, &v1.Hyperlink{Uri: uri, Id: id})
					idx = uint32(len(snap.Hyperlinks))
					linkIdx[c[6]] = idx
				}
				cell.Hyperlink = idx
			}
			r.Cells = append(r.Cells, cell)
		}
		snap.Grid = append(snap.Grid, r)
	}
	return snap
}

var oracleFlagBits = map[string]uint32{
	"bold": attrBold, "faint": attrFaint, "italic": attrItalic, "underline": attrUnderline,
	"reverse": attrReverse, "blink": attrBlink, "conceal": attrConceal, "strike": attrStrike, "overline": attrOverline,
}

var oracleUnderlineStyles = map[string]uint32{"": 0, "single": 0, "double": 1, "curly": 2, "dotted": 3, "dashed": 4}

func packOracleColor(s string) uint32 {
	switch {
	case strings.HasPrefix(s, "p:"):
		n, _ := strconv.Atoi(s[2:])
		return colorTagIndexed | uint32(n)
	case strings.HasPrefix(s, "rgb:"):
		n, _ := strconv.ParseUint(s[4:], 16, 32)
		return colorTagRGB | uint32(n)
	}
	return 0
}

// snapshotStateFromOracle fills every non-grid PaneSnapshot field from the oracle's state.
func snapshotStateFromOracle(st oracleState) *v1.PaneSnapshot {
	m := st.Modes
	return &v1.PaneSnapshot{
		Title: st.Title,
		Modes: &v1.TerminalModes{
			AppCursorKeys: m.AppCursorKeys, AppKeypad: m.AppKeypad, BracketedPaste: m.BracketedPaste,
			FocusEvents: m.FocusEvents, MouseMode: m.MouseMode, MouseEncoding: m.MouseEncoding,
			ReverseVideo: m.ReverseVideo, Origin: m.Origin, NoAutowrap: m.NoAutowrap, Insert: m.Insert, Newline: m.Newline,
		},
		CursorHidden: st.CursorHidden, CursorStyle: st.CursorStyle, AltScreen: st.AltScreen,
		ScrollTop: st.ScrollTop, ScrollBottom: st.ScrollBottom, TabStops: st.TabStops,
		Charsets:  &v1.Charsets{G0: st.Charsets.G0, G1: st.Charsets.G1, G2: st.Charsets.G2, G3: st.Charsets.G3, Gl: st.Charsets.Gl},
		CursorRow: st.Cursor.Row, CursorCol: st.Cursor.Col, CursorPendingWrap: st.Cursor.PendingWrap,
	}
}
