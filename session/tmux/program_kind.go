package tmux

import (
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/session/launchcmd"
)

// ProgramKind identifies which agent CLI a session runs, so behavior that
// depends on it (prompt detection) never re-reads the quoted launch string.
type ProgramKind int

const (
	KindOther ProgramKind = iota
	KindClaude
	KindAider
	KindGemini
)

// ProgramKindOf classifies a launch command by the basename of its program
// (Argv[0]), so args, env prefixes, redirects, quoting and absolute paths do
// not matter -- the same basename rule as session.isClaude. A ShellScript is
// opaque and keeps the rendered-text check.
func ProgramKindOf(cmd launchcmd.LaunchCommand) ProgramKind {
	switch c := cmd.(type) {
	case launchcmd.ExecCommand:
		prog, ok := c.Program()
		if !ok {
			return KindOther
		}
		return kindByBasename(filepath.Base(prog))
	case launchcmd.ShellScript:
		text := c.ShellString()
		if text == ProgramClaude {
			return KindClaude
		}
		return kindByPrefix(text)
	}
	return KindOther
}

func kindByBasename(base string) ProgramKind {
	switch base {
	case ProgramClaude:
		return KindClaude
	case ProgramAider:
		return KindAider
	case ProgramGemini:
		return KindGemini
	}
	return KindOther
}

func kindByPrefix(rendered string) ProgramKind {
	switch {
	case strings.HasPrefix(rendered, ProgramAider):
		return KindAider
	case strings.HasPrefix(rendered, ProgramGemini):
		return KindGemini
	}
	return KindOther
}

// kindOfShellText classifies a bare program string from the string-taking
// constructors, which have no typed command to inspect.
func kindOfShellText(program string) ProgramKind {
	if program == ProgramClaude {
		return KindClaude
	}
	return kindByPrefix(program)
}

// WithLaunchCommand sets the session's program kind from its typed launch
// command, overriding the string-derived default.
func WithLaunchCommand(cmd launchcmd.LaunchCommand) TmuxSessionOption {
	return func(t *TmuxSession) {
		t.programKind = ProgramKindOf(cmd)
	}
}
