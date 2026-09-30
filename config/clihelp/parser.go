package clihelp

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxFlags            = 500
	maxDescriptionChars = 300
	maxLineBytes        = 2048
	maxCandidateIndent  = 8
	tabWidth            = 8
	// rawDescriptionBytes bounds accumulation while joining wrapped lines;
	// the final description is cut to maxDescriptionChars runes.
	rawDescriptionBytes = maxDescriptionChars * 4
)

// Flag is one option parsed from a program's --help text.
type Flag struct {
	// Name is the primary long form, or the short form when no long form exists.
	Name string
	// Short is the single-letter form (e.g. "-a") when Name is a long form.
	Short string
	// Aliases are extra long forms of the same option (e.g. "--allowed-tools").
	Aliases     []string
	TakesValue  bool
	Description string
}

var envTagPattern = regexp.MustCompile(`\s*\[env(?: var)?:[^\]]*\]`)

// ParseHelp extracts flags from --help output. It is precision-first: only
// lines whose first non-blank character is '-' are candidates, and every name
// must look like a flag. It never fails; unparseable input yields an empty,
// non-nil slice. At most maxFlags flags are returned.
func ParseHelp(text string) []Flag {
	b := &flagBuilder{cur: -1}
	text = stripANSI(text)
	for len(text) > 0 && len(b.raw) < maxFlags {
		var line string
		line, text = nextLine(text)
		b.feed(line)
	}
	return b.result()
}

// nextLine splits off the first line; the newline is dropped.
func nextLine(text string) (line, rest string) {
	i := strings.IndexByte(text, '\n')
	if i < 0 {
		return text, ""
	}
	return text[:i], text[i+1:]
}

// flagBuilder accumulates raw flags and attaches wrapped description lines
// to the most recent flag until a blank or shallower line ends the block.
type flagBuilder struct {
	raw       []Flag
	cur       int // index into raw accepting continuation lines, or -1
	curIndent int
}

func (b *flagBuilder) feed(line string) {
	trimmed := strings.TrimLeft(line, " \t")
	indent := indentWidth(line[:len(line)-len(trimmed)])
	trimmed = strings.TrimRight(trimmed, " \t\r")
	if trimmed == "" || len(line) > maxLineBytes {
		b.cur = -1
		return
	}
	if indent <= maxCandidateIndent && trimmed[0] == '-' {
		if f, ok := parseFlagLine(trimmed); ok {
			b.raw = append(b.raw, f)
			b.cur, b.curIndent = len(b.raw)-1, indent
			return
		}
	}
	if b.cur >= 0 && indent > b.curIndent {
		b.appendDescription(trimmed)
		return
	}
	b.cur = -1
}

func (b *flagBuilder) appendDescription(s string) {
	f := &b.raw[b.cur]
	if len(f.Description) >= rawDescriptionBytes {
		return
	}
	if f.Description != "" {
		f.Description += " "
	}
	f.Description += s
}

func indentWidth(ws string) int {
	n := 0
	for i := 0; i < len(ws); i++ {
		if ws[i] == '\t' {
			n += tabWidth
		} else {
			n++
		}
	}
	return n
}

// result finalizes descriptions and merges duplicate names, preferring the
// entry that carries a description.
func (b *flagBuilder) result() []Flag {
	out := make([]Flag, 0, len(b.raw))
	seen := make(map[string]int, len(b.raw))
	for _, f := range b.raw {
		f = finalizeFlag(f)
		i, dup := seen[f.Name]
		if !dup {
			seen[f.Name] = len(out)
			out = append(out, f)
			continue
		}
		if out[i].Description == "" && f.Description != "" {
			f.TakesValue = f.TakesValue || out[i].TakesValue
			out[i] = f
		} else {
			out[i].TakesValue = out[i].TakesValue || f.TakesValue
		}
	}
	return out
}

func finalizeFlag(f Flag) Flag {
	desc := strings.ToValidUTF8(f.Description, "")
	if valueTagged(desc) {
		f.TakesValue = true
	}
	desc = envTagPattern.ReplaceAllString(desc, "")
	desc = strings.Join(strings.Fields(desc), " ")
	f.Description = truncateRunes(desc, maxDescriptionChars)
	return f
}

// valueTagged reports whether yargs/clap metadata tags in the description
// prove the flag takes a value.
func valueTagged(desc string) bool {
	for _, tag := range []string{"[string]", "[array]", "[number]", "[possible values:", "[choices:"} {
		if strings.Contains(desc, tag) {
			return true
		}
	}
	return false
}

func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}

// stripANSI removes CSI/OSC/other escape sequences, carriage returns and
// man-style overstrike (X\b) so coloured or bold help parses like plain text.
func stripANSI(s string) string {
	if !strings.ContainsAny(s, "\x1b\r\b") {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case 0x1b:
			i = skipEscape(s, i)
		case '\r':
		case '\b':
			out = dropLastRune(out)
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}

// skipEscape returns the index of the last byte of the escape sequence that
// starts at s[i] (an ESC byte).
func skipEscape(s string, i int) int {
	if i+1 >= len(s) {
		return i
	}
	switch s[i+1] {
	case '[':
		for j := i + 2; j < len(s); j++ {
			if s[j] >= 0x40 && s[j] <= 0x7e {
				return j
			}
		}
		return len(s) - 1
	case ']':
		for j := i + 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 1
			}
		}
		return len(s) - 1
	default:
		return i + 1
	}
}

func dropLastRune(b []byte) []byte {
	if len(b) == 0 || b[len(b)-1] == '\n' {
		return b
	}
	_, size := utf8.DecodeLastRune(b)
	return b[:len(b)-size]
}
