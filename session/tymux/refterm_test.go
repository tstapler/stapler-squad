package tymux

import (
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"
)

// refTerm is a deliberately small vt model, the reference both sides of the consistency harness
// share: the authoritative pane (what the daemon's emulator holds) and the client (what a
// browser's emulator holds after parsing the bytes the fanout delivered). It understands the
// subset Claude Code style redraws and RenderSnapshotRedraw use (printing, CR/LF/BS, CUU/CUD/
// CUF/CUB, CUP, ED, EL) and ignores every other escape sequence. It is not a vt100; it only has
// to be deterministic, so any divergence between the two instances is lost, duplicated or
// reordered bytes, never emulator disagreement.
type refTerm struct {
	mu         sync.Mutex
	rows, cols int
	grid       [][]rune
	cr, cc     int
	state      int
	params     []byte
	inter      []byte
}

const (
	rtGround = iota
	rtEsc
	rtCSI
	rtOSC
	rtOSCEsc
	rtEscInter
)

func newRefTerm(rows, cols int) *refTerm {
	t := &refTerm{rows: rows, cols: cols}
	t.grid = make([][]rune, rows)
	for i := range t.grid {
		t.grid[i] = t.blankRow()
	}
	return t
}

func (t *refTerm) blankRow() []rune {
	r := make([]rune, t.cols)
	for i := range r {
		r[i] = ' '
	}
	return r
}

// Write feeds bytes through the parser.
func (t *refTerm) Write(b []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for len(b) > 0 {
		c := b[0]
		if c >= 0x80 && t.state == rtGround {
			r, n := utf8.DecodeRune(b)
			t.print(r)
			b = b[n:]
			continue
		}
		b = b[1:]
		t.feed(c)
	}
}

func (t *refTerm) feed(c byte) {
	switch t.state {
	case rtGround:
		switch {
		case c == 0x1b:
			t.state = rtEsc
		case c == '\r':
			t.cc = 0
		case c == '\n':
			t.lineFeed()
		case c == '\b':
			if t.cc > 0 {
				t.cc--
			}
		case c >= 0x20 && c < 0x7f:
			t.print(rune(c))
		}
	case rtEsc:
		switch {
		case c == '[':
			t.state, t.params, t.inter = rtCSI, nil, nil
		case c == ']':
			t.state = rtOSC
		case c >= 0x20 && c <= 0x2f: // charset designation etc.: one more byte follows
			t.state = rtEscInter
		default:
			t.state = rtGround
		}
	case rtEscInter:
		t.state = rtGround
	case rtCSI:
		switch {
		case c >= 0x30 && c <= 0x3f:
			t.params = append(t.params, c)
		case c >= 0x20 && c <= 0x2f:
			t.inter = append(t.inter, c)
		default:
			t.csi(c)
			t.state = rtGround
		}
	case rtOSC:
		switch c {
		case 0x07:
			t.state = rtGround
		case 0x1b:
			t.state = rtOSCEsc
		}
	case rtOSCEsc:
		t.state = rtGround // ESC \ terminator (or an abort)
	}
}

func (t *refTerm) print(r rune) {
	if t.cc >= t.cols { // deferred wrap
		t.cc = 0
		t.lineFeed()
	}
	t.grid[t.cr][t.cc] = r
	t.cc++
}

func (t *refTerm) lineFeed() {
	if t.cr < t.rows-1 {
		t.cr++
		return
	}
	copy(t.grid, t.grid[1:])
	t.grid[t.rows-1] = t.blankRow()
}

func (t *refTerm) num(i, def int) int {
	var parts []string
	if p := strings.TrimLeft(string(t.params), "?>="); p != "" {
		parts = strings.Split(p, ";")
	}
	if i >= len(parts) || parts[i] == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(parts[i], "%d", &n); err != nil {
		return def
	}
	return n
}

func (t *refTerm) csi(final byte) {
	if len(t.inter) > 0 || strings.ContainsAny(string(t.params), "?>=") {
		return // DECSCUSR, DEC private modes: not modeled
	}
	switch final {
	case 'A':
		t.cr = max(0, t.cr-t.num(0, 1))
	case 'B':
		t.cr = min(t.rows-1, t.cr+t.num(0, 1))
	case 'C':
		t.cc = min(t.cols-1, t.cc+t.num(0, 1))
	case 'D':
		t.cc = max(0, t.cc-t.num(0, 1))
	case 'H', 'f':
		t.cr = min(t.rows-1, max(0, t.num(0, 1)-1))
		t.cc = min(t.cols-1, max(0, t.num(1, 1)-1))
	case 'J':
		t.eraseDisplay(t.num(0, 0))
	case 'K':
		t.eraseLine(t.num(0, 0))
	}
}

func (t *refTerm) eraseDisplay(mode int) {
	switch mode {
	case 0:
		t.eraseLine(0)
		for r := t.cr + 1; r < t.rows; r++ {
			t.grid[r] = t.blankRow()
		}
	case 1:
		t.eraseLine(1)
		for r := 0; r < t.cr; r++ {
			t.grid[r] = t.blankRow()
		}
	default:
		for r := range t.grid {
			t.grid[r] = t.blankRow()
		}
	}
}

func (t *refTerm) eraseLine(mode int) {
	row := t.grid[t.cr]
	from, to := t.cc, t.cols
	switch mode {
	case 1:
		from, to = 0, min(t.cc+1, t.cols)
	case 2:
		from, to = 0, t.cols
	}
	for c := from; c < to && c < t.cols; c++ {
		row[c] = ' '
	}
}

// Screen is the grid as text plus the cursor, the value two terminals must agree on.
func (t *refTerm) Screen() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var b strings.Builder
	for _, row := range t.grid {
		b.WriteString(strings.TrimRight(string(row), " "))
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "cursor=%d,%d", t.cr, min(t.cc, t.cols-1))
	return b.String()
}
