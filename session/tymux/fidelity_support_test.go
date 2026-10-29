package tymux

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Shared by the daemon-backed integration test and the daemon-free golden tests.

const fidelityDir = "testdata/fidelity"

// corpus/known_gap/ holds behavior that is out of scope for a headless cell snapshot (images,
// bell, terminal replies...). Those files are replayed to prove the daemon swallows them
// cleanly, and their diffs are logged, not failed.
const knownGapArea = "known_gap"

// text, fg, bg, flags, underline style, underline color, link (uri NUL id)
type oracleCell [7]string

// oracleScreen is what the oracle reports for one replayed stream: the cell grid plus the terminal
// state a redraw must restore. Field names and numeric conventions mirror tymux's PaneSnapshot proto.
type oracleScreen struct {
	Grid  [][]oracleCell `json:"grid"`
	State oracleState    `json:"state"`
}

type oracleState struct {
	Title        string        `json:"title"`
	Modes        oracleModes   `json:"modes"`
	CursorHidden bool          `json:"cursorHidden"`
	CursorStyle  uint32        `json:"cursorStyle"`
	AltScreen    bool          `json:"altScreen"`
	ScrollTop    uint32        `json:"scrollTop"`
	ScrollBottom uint32        `json:"scrollBottom"`
	TabStops     []uint32      `json:"tabStops"`
	Charsets     oracleCharset `json:"charsets"`
	Cursor       struct {
		Row uint32 `json:"row"`
		Col uint32 `json:"col"`
	} `json:"cursor"`
}

type oracleModes struct {
	AppCursorKeys  bool   `json:"appCursorKeys"`
	AppKeypad      bool   `json:"appKeypad"`
	BracketedPaste bool   `json:"bracketedPaste"`
	FocusEvents    bool   `json:"focusEvents"`
	MouseMode      uint32 `json:"mouseMode"`
	MouseEncoding  uint32 `json:"mouseEncoding"`
	ReverseVideo   bool   `json:"reverseVideo"`
	Origin         bool   `json:"origin"`
	NoAutowrap     bool   `json:"noAutowrap"`
	Insert         bool   `json:"insert"`
	Newline        bool   `json:"newline"`
}

type oracleCharset struct {
	G0 uint32 `json:"g0"`
	G1 uint32 `json:"g1"`
	G2 uint32 `json:"g2"`
	G3 uint32 `json:"g3"`
	Gl uint32 `json:"gl"`
}

type corpusCase struct{ name, path string }

type corpusArea struct {
	name  string
	files []corpusCase
}

// corpusCases groups testdata/fidelity/corpus/<area>/<case>.ansi by area, sorted.
func corpusCases(t *testing.T) []corpusArea {
	t.Helper()
	root := filepath.Join(fidelityDir, "corpus")
	areas, err := os.ReadDir(root)
	require.NoError(t, err)
	var out []corpusArea
	for _, a := range areas {
		if !a.IsDir() {
			continue
		}
		files, err := filepath.Glob(filepath.Join(root, a.Name(), "*.ansi"))
		require.NoError(t, err)
		area := corpusArea{name: a.Name()}
		for _, f := range files {
			area.files = append(area.files, corpusCase{name: strings.TrimSuffix(filepath.Base(f), ".ansi"), path: f})
		}
		if len(area.files) > 0 {
			out = append(out, area)
		}
	}
	return out
}

func runOracle(t *testing.T, dir string, rows, cols int, streams map[string][]byte) map[string]oracleScreen {
	t.Helper()
	enc := map[string]string{}
	for k, v := range streams {
		enc[k] = base64.StdEncoding.EncodeToString(v)
	}
	in, err := json.Marshal(map[string]any{"rows": rows, "cols": cols, "streams": enc})
	require.NoError(t, err)
	cmd := exec.Command("bun", "oracle.mjs")
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(in)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoErrorf(t, err, "oracle failed: %s", stderr.String())
	var grids map[string]oracleScreen
	require.NoError(t, json.Unmarshal(out, &grids))
	return grids
}

func diffGrids(want, got [][]oracleCell) []string {
	var diffs []string
	for y := range want {
		for x := range want[y] {
			if blankInsensitive(want[y][x]) != blankInsensitive(got[y][x]) {
				diffs = append(diffs, fmt.Sprintf("r%d c%d: live=%q got=%q", y, x, want[y][x], got[y][x]))
			}
		}
	}
	return diffs
}

// diffScreens lists every grid cell and state field that differs between want and got.
func diffScreens(want, got oracleScreen) []string {
	diffs := diffGrids(want.Grid, got.Grid)
	w, g := reflect.ValueOf(want.State), reflect.ValueOf(got.State)
	diffs = append(diffs, diffStruct("state", w, g)...)
	return diffs
}

func diffStruct(path string, want, got reflect.Value) []string {
	if want.Kind() != reflect.Struct {
		if want.Kind() == reflect.Slice && want.Len() == 0 && got.Len() == 0 {
			return nil
		}
		if !reflect.DeepEqual(want.Interface(), got.Interface()) {
			return []string{fmt.Sprintf("%s: want %v got %v", path, want.Interface(), got.Interface())}
		}
		return nil
	}
	var diffs []string
	for i := 0; i < want.NumField(); i++ {
		diffs = append(diffs, diffStruct(path+"."+want.Type().Field(i).Name, want.Field(i), got.Field(i))...)
	}
	return diffs
}

func nonBlankCells(g [][]oracleCell) (n int) {
	for _, row := range g {
		for _, c := range row {
			if c[0] != " " && c[0] != "" {
				n++
			}
		}
	}
	return n
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// blankInsensitive drops flags that draw nothing on a space (the daemon keeps them on erased
// cells, xterm.js clears them), so only visible differences count.
func blankInsensitive(c oracleCell) oracleCell {
	if c[0] != " " {
		return c
	}
	var keep []string
	for _, f := range strings.Split(c[3], ",") {
		if f == "underline" || f == "reverse" || f == "strike" || f == "overline" {
			keep = append(keep, f)
		}
	}
	c[3] = strings.Join(keep, ",")
	if !strings.Contains(c[3], "underline") {
		c[4], c[5] = "", "default"
	}
	return c
}
