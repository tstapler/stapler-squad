// Package gitconfig resolves the configuration the git CLI would see for a repository: git's
// own file grammar, scope order, include/includeIf expansion and GIT_CONFIG_* variables. It is
// a pure, standard-library-only parser used for routing decisions (ADR-006). go-git's config
// decoder is deliberately not used: it merges repeated sections (losing an include's
// position), reads a bare key as the empty string and follows no include at all.
//
// Every rule here is checked against `git config --list --show-origin -z` by the gitoracle
// tests; where this package and git disagree the test fails, it is not a documented limit.
package gitconfig

import (
	"fmt"
	"strings"
)

// Entry is one `key[ = value]` line, positioned in stream order.
type Entry struct {
	// Section and Key are lowercased (git compares them case-insensitively). Subsection is
	// verbatim for the `[a "Sub"]` form and lowercased for the deprecated `[a.Sub]` form.
	// Section is empty for a key that precedes any section header; such an entry is kept (git
	// lists it) but is never matched by Get.
	Section, Subsection, Key string
	HasSubsection            bool
	// Value is the decoded value. HasValue is false for a bare `key` line, which git reads as
	// boolean true, while `key =` and `key = ""` have HasValue true and an empty Value.
	Value    string
	HasValue bool
	Line     int
}

// Name is the dotted key as `git config --list` prints it.
func (e Entry) Name() string {
	switch {
	case e.Section == "":
		return e.Key
	case e.HasSubsection:
		return e.Section + "." + e.Subsection + "." + e.Key
	}
	return e.Section + "." + e.Key
}

// Sub is the closed set of reasons resolution fails closed. Each maps onto a metric label, so
// they are never free text.
type Sub string

const (
	SubParse                Sub = "parse_error"
	SubUnreadable           Sub = "unreadable_file"
	SubTooLarge             Sub = "file_too_large"
	SubIncludeDepth         Sub = "include_depth"
	SubUnknownCondition     Sub = "unknown_condition"
	SubUnsupportedGlob      Sub = "unsupported_glob"
	SubRemoteURLInInclude   Sub = "remote_url_in_include"
	SubBareRemoteURL        Sub = "bare_remote_url"
	SubGitdirSymlink        Sub = "gitdir_symlink_ambiguous"
	SubUnsupportedInclude   Sub = "unsupported_include_path"
	SubBadInclude           Sub = "bad_include"
	SubEnvOverride          Sub = "env_override"
	SubHomeUnknown          Sub = "home_unknown"
	SubCommonDir            Sub = "commondir"
	SubCanceled             Sub = "canceled"
	SubIO                   Sub = "io_error"
	SubConfigParametersUsed Sub = "config_parameters"
)

// AllSubs lists every Sub, for tests that keep label enums in sync.
func AllSubs() []Sub {
	return []Sub{
		SubParse, SubUnreadable, SubTooLarge, SubIncludeDepth, SubUnknownCondition, SubUnsupportedGlob,
		SubRemoteURLInInclude, SubBareRemoteURL, SubGitdirSymlink, SubUnsupportedInclude, SubBadInclude,
		SubEnvOverride, SubHomeUnknown, SubCommonDir, SubCanceled, SubIO, SubConfigParametersUsed,
	}
}

// Error is a resolution failure that git itself would die on or that this package refuses to
// guess about. The caller routes the operation to the CLI.
type Error struct {
	Sub  Sub
	Path string // the file involved, when there is one
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("gitconfig: ")
	b.WriteString(string(e.Sub))
	if e.Path != "" {
		b.WriteString(" (" + e.Path + ")")
	}
	if e.Msg != "" {
		b.WriteString(": " + e.Msg)
	}
	if e.Err != nil {
		b.WriteString(": " + e.Err.Error())
	}
	return b.String()
}

func (e *Error) Unwrap() error { return e.Err }

func parseErr(line int, format string, args ...any) *Error {
	return &Error{Sub: SubParse, Msg: fmt.Sprintf("line %d: %s", line, fmt.Sprintf(format, args...))}
}

// Parse tokenizes one config file's content the way git's config.c does (git_parse_source,
// get_base_var, get_extended_base_var, get_value, parse_value). A syntax error is an *Error
// with SubParse: git dies with "bad config line" there, so the caller must not guess.
func Parse(data []byte) ([]Entry, error) {
	p := &parser{data: data}
	return p.run()
}

type parser struct {
	data []byte
	pos  int
	line int
	eof  bool
}

// next mirrors get_next_char: CRLF reads as LF, and the end of input reads once as a final
// newline so a last line without one is accepted.
func (p *parser) next() int {
	if p.pos >= len(p.data) {
		p.eof = true
		p.line++
		return '\n'
	}
	c := p.data[p.pos]
	p.pos++
	if c == '\r' && p.pos < len(p.data) && p.data[p.pos] == '\n' {
		p.pos++
		c = '\n'
	}
	if c == '\n' {
		p.line++
	}
	return int(c)
}

// isSpace is git's isspace (sane-ctype): space, tab, LF and CR only.
func isSpace(c int) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func isAlpha(c int) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isKeyChar(c int) bool { return isAlpha(c) || (c >= '0' && c <= '9') || c == '-' }

func lower(c int) byte {
	if c >= 'A' && c <= 'Z' {
		return byte(c + 'a' - 'A')
	}
	return byte(c)
}

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

func (p *parser) run() ([]Entry, error) {
	p.line = 1
	var (
		entries []Entry
		cur     struct {
			section, sub string
			hasSub       bool
		}
		comment bool
		bom     = utf8BOM
	)
	for {
		c := p.next()
		if len(bom) > 0 {
			if c == int(bom[0]) && !p.eof {
				bom = bom[1:]
				continue
			}
			if len(bom) != len(utf8BOM) {
				return nil, parseErr(p.line, "partial byte order mark")
			}
			bom = nil
		}
		if c == '\n' {
			if p.eof {
				return entries, nil
			}
			comment = false
			continue
		}
		if comment || isSpace(c) {
			continue
		}
		if c == '#' || c == ';' {
			comment = true
			continue
		}
		if c == '[' {
			section, sub, hasSub, err := p.header()
			if err != nil {
				return nil, err
			}
			cur.section, cur.sub, cur.hasSub = section, sub, hasSub
			continue
		}
		if !isAlpha(c) {
			return nil, parseErr(p.line, "unexpected character %q", rune(c))
		}
		line := p.line
		e, err := p.entry(c)
		if err != nil {
			return nil, err
		}
		e.Section, e.Subsection, e.HasSubsection, e.Line = cur.section, cur.sub, cur.hasSub, line
		entries = append(entries, e)
	}
}

// header reads after "[" (get_base_var / get_extended_base_var). A name is letters, digits,
// '-' and '.'; whitespace after it opens the `"subsection"` form.
func (p *parser) header() (section, sub string, hasSub bool, err error) {
	var name []byte
	for {
		c := p.next()
		if p.eof {
			return "", "", false, parseErr(p.line, "unterminated section header")
		}
		switch {
		case c == ']':
			return splitDeprecated(name, p.line)
		case isSpace(c):
			return p.extendedHeader(name, c)
		case isKeyChar(c) || c == '.':
			name = append(name, lower(c))
		default:
			return "", "", false, parseErr(p.line, "bad section header")
		}
	}
}

// splitDeprecated splits the `[section.sub]` form: the first dot separates, the subsection is
// lowercased (git lowercases the whole header in this form).
func splitDeprecated(name []byte, line int) (string, string, bool, error) {
	if len(name) < 1 {
		return "", "", false, parseErr(line, "empty section name")
	}
	s := string(name)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return s[:i], s[i+1:], true, nil
	}
	return s, "", false, nil
}

func (p *parser) extendedHeader(name []byte, c int) (string, string, bool, error) {
	for {
		if c == '\n' {
			return "", "", false, parseErr(p.line, "incomplete section header")
		}
		c = p.next()
		if !isSpace(c) {
			break
		}
	}
	if c != '"' || len(name) == 0 {
		return "", "", false, parseErr(p.line, "bad section header")
	}
	var sub []byte
	for {
		c := p.next()
		if c == '\n' {
			return "", "", false, parseErr(p.line, "incomplete section header")
		}
		if c == '"' {
			break
		}
		if c == '\\' {
			c = p.next()
			if c == '\n' {
				return "", "", false, parseErr(p.line, "incomplete section header")
			}
		}
		sub = append(sub, byte(c))
	}
	if p.next() != ']' {
		return "", "", false, parseErr(p.line, "bad section header")
	}
	return strings.ToLower(string(name)), string(sub), true, nil
}

// entry reads a key (first character already consumed) and its optional `= value` (get_value).
func (p *parser) entry(first int) (Entry, error) {
	e := Entry{Key: string(lower(first))}
	var c int
	for {
		c = p.next()
		if p.eof || !isKeyChar(c) {
			break
		}
		e.Key += string(lower(c))
	}
	for c == ' ' || c == '\t' {
		c = p.next()
	}
	if c != '\n' {
		if c != '=' {
			return e, parseErr(p.line, "bad config line")
		}
		v, err := p.value()
		if err != nil {
			return e, err
		}
		e.Value, e.HasValue = v, true
	}
	return e, nil
}

// value mirrors parse_value: surrounding whitespace is dropped, interior whitespace outside
// quotes is kept verbatim, quotes toggle, ; and # start a comment outside quotes,
// and a trailing backslash joins the next line.
func (p *parser) value() (string, error) {
	var (
		b       []byte
		quote   bool
		comment bool
		pending []byte // whitespace seen since the last value byte; kept only if more value follows
	)
	for {
		c := p.next()
		if c == '\n' {
			if quote {
				return "", parseErr(p.line, "unterminated quote")
			}
			return string(b), nil
		}
		if comment {
			continue
		}
		if isSpace(c) && !quote {
			if len(b) > 0 {
				pending = append(pending, byte(c))
			}
			continue
		}
		if !quote && (c == ';' || c == '#') {
			comment = true
			continue
		}
		b, pending = append(b, pending...), pending[:0]
		if c == '\\' {
			c = p.next()
			switch c {
			case '\n':
				continue
			case 't':
				c = '\t'
			case 'b':
				c = '\b'
			case 'n':
				c = '\n'
			case '\\', '"':
			default:
				return "", parseErr(p.line, "bad escape sequence")
			}
			b = append(b, byte(c))
			continue
		}
		if c == '"' {
			quote = !quote
			continue
		}
		b = append(b, byte(c))
	}
}
