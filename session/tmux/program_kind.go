package tmux

import (
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

// ProgramKindOf classifies a launch command. It matches the historical
// string checks exactly: Claude only when the whole command renders as the
// bare word `claude`; Aider/Gemini when the rendered command starts with
// the program name (so a quoted Literal or a leading env assignment is KindOther).
func ProgramKindOf(cmd launchcmd.LaunchCommand) ProgramKind {
	switch c := cmd.(type) {
	case launchcmd.ExecCommand:
		if len(c.Env) > 0 || len(c.Argv) == 0 {
			return KindOther
		}
		head := launchcmd.ExecCommand{Argv: c.Argv[:1]}.ShellString()
		if head == ProgramClaude && len(c.Argv) == 1 && !c.HasRedirects() {
			return KindClaude
		}
		return kindByPrefix(head)
	case launchcmd.ShellScript:
		text := c.ShellString()
		if text == ProgramClaude {
			return KindClaude
		}
		return kindByPrefix(text)
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
