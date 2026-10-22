// Package launchcmd models the command a session backend launches as typed
// data instead of a pre-quoted shell string.
//
// A LaunchCommand is either an ExecCommand (a program plus argv, runnable with
// no shell) or a ShellScript (text that genuinely needs a shell: redirects,
// `&&` chains). Backends that exec directly use ExecCommand.Argv; tmux, which
// always runs a shell string, uses ShellString. Quoting lives only here.
package launchcmd

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// MaxArgBytes is Linux's per-argument limit (MAX_ARG_STRLEN). A resolved
// argument at or above it cannot be exec'd directly.
const MaxArgBytes = 128 * 1024

// ErrArgTooLarge reports an argument that cannot be passed as a single argv entry.
var ErrArgTooLarge = errors.New("launchcmd: argument exceeds MAX_ARG_STRLEN")

// LaunchCommand is the sealed set of launch forms; switch on the concrete type.
type LaunchCommand interface {
	// ShellString renders the command for a backend that runs it through a shell.
	ShellString() string
	isLaunchCommand()
}

// Arg is one argv entry: Literal, Flag, or FileContents.
type Arg interface {
	shellWord() string
	isArg()
}

// Literal is an argv entry taken verbatim, however odd its bytes.
type Literal string

// FileContents is an argv entry whose value is the contents of the file at
// that path with trailing newlines removed, matching shell "$(cat path)" so
// every renderer agrees. It exists because a prompt can be larger than a
// command line comfortably carries.
type FileContents string

// Flag is an option name such as "--resume". Constructed only by NewFlag so a
// Flag never needs quoting.
type Flag struct{ name string }

var flagPattern = regexp.MustCompile(`^--?[A-Za-z0-9][A-Za-z0-9_-]*$`)

// NewFlag validates name as a bare option ("-p", "--resume").
func NewFlag(name string) (Flag, error) {
	if !flagPattern.MatchString(name) {
		return Flag{}, fmt.Errorf("launchcmd: %q is not a valid flag name", name)
	}
	return Flag{name: name}, nil
}

// MustFlag is NewFlag for compile-time constants; it panics on a bad name.
func MustFlag(name string) Flag {
	f, err := NewFlag(name)
	if err != nil {
		panic(err)
	}
	return f
}

// Name returns the flag text.
func (f Flag) Name() string { return f.name }

func (Literal) isArg()      {}
func (FileContents) isArg() {}
func (Flag) isArg()         {}

func (l Literal) shellWord() string      { return Quote(string(l)) }
func (f FileContents) shellWord() string { return `"$(cat ` + Quote(string(f)) + `)"` }
func (f Flag) shellWord() string         { return f.name }

var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// EnvVar is one KEY=value assignment for the launched process.
type EnvVar struct {
	key   string
	Value string
}

// NewEnvVar validates key as a shell-safe identifier.
func NewEnvVar(key, value string) (EnvVar, error) {
	if !envKeyPattern.MatchString(key) {
		return EnvVar{}, fmt.Errorf("launchcmd: %q is not a valid environment variable name", key)
	}
	return EnvVar{key: key, Value: value}, nil
}

// Key returns the variable name.
func (e EnvVar) Key() string { return e.key }

// ExecCommand is a program plus argv that can be exec'd without a shell.
// Argv[0] is the program.
type ExecCommand struct {
	Env  []EnvVar
	Argv []Arg
}

func (ExecCommand) isLaunchCommand() {}

// ShellString renders `KEY='v' prog arg ...`. Flags are emitted bare, every
// other word single-quoted.
func (c ExecCommand) ShellString() string {
	words := make([]string, 0, len(c.Env)+len(c.Argv))
	for _, e := range c.Env {
		words = append(words, e.key+"="+Quote(e.Value))
	}
	for _, a := range c.Argv {
		words = append(words, a.shellWord())
	}
	return strings.Join(words, " ")
}

// Program returns Argv[0]'s text, and false if Argv is empty or its first
// entry is not a Literal.
func (c ExecCommand) Program() (string, bool) {
	if len(c.Argv) == 0 {
		return "", false
	}
	lit, ok := c.Argv[0].(Literal)
	return string(lit), ok
}

// ReadFile loads a FileContents argument.
type ReadFile func(path string) ([]byte, error)

// Args resolves Argv[1:] to plain strings for a backend that execs directly.
// A FileContents argument is read via read and must stay under MaxArgBytes;
// otherwise ErrArgTooLarge is returned and the caller should fall back to a
// ShellScript. The Env is not applied here: pass it to the backend separately.
func (c ExecCommand) Args(read ReadFile) ([]string, error) {
	if len(c.Argv) == 0 {
		return nil, errors.New("launchcmd: empty argv")
	}
	out := make([]string, 0, len(c.Argv)-1)
	for _, a := range c.Argv[1:] {
		s, err := resolve(a, read)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func resolve(a Arg, read ReadFile) (string, error) {
	var s string
	switch v := a.(type) {
	case Literal:
		s = string(v)
	case Flag:
		s = v.name
	case FileContents:
		b, err := read(string(v))
		if err != nil {
			return "", fmt.Errorf("launchcmd: read argument file %q: %w", string(v), err)
		}
		s = strings.TrimRight(string(b), "\n")
	default:
		return "", fmt.Errorf("launchcmd: unknown argument type %T", a)
	}
	if len(s) >= MaxArgBytes {
		return "", ErrArgTooLarge
	}
	return s, nil
}

// ShellText is text that needs a shell. Constructed only by NewShellText.
type ShellText struct{ text string }

// NewShellText rejects empty text and NUL bytes, which no shell accepts.
func NewShellText(text string) (ShellText, error) {
	if strings.TrimSpace(text) == "" {
		return ShellText{}, errors.New("launchcmd: empty shell text")
	}
	if strings.ContainsRune(text, 0) {
		return ShellText{}, errors.New("launchcmd: shell text contains NUL")
	}
	return ShellText{text: text}, nil
}

// String returns the script text.
func (t ShellText) String() string { return t.text }

// ShellScript is a command that cannot be expressed as argv.
type ShellScript struct{ Text ShellText }

func (ShellScript) isLaunchCommand() {}

// ShellString returns the script text unchanged.
func (s ShellScript) ShellString() string { return s.Text.text }

// Quote single-quotes s for a POSIX shell. Single quotes suppress every
// expansion; a literal single quote is written as '\”.
func Quote(s string) string {
	return singleQuote + strings.ReplaceAll(s, singleQuote, `'\''`) + singleQuote
}

const singleQuote = "'"

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ConversationID is a validated Claude conversation UUID.
type ConversationID struct{ id string }

// ParseConversationID accepts a UUID in either case and normalises it to lower case.
func ParseConversationID(s string) (ConversationID, error) {
	lower := strings.ToLower(s)
	if !uuidPattern.MatchString(lower) {
		return ConversationID{}, fmt.Errorf("launchcmd: %q is not a conversation UUID", s)
	}
	return ConversationID{id: lower}, nil
}

// String returns the lower-case UUID.
func (c ConversationID) String() string { return c.id }
