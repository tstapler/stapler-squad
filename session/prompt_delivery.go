package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/launchcmd"
)

// promptChannel is how an agent kind accepts a prompt at process start.
type promptChannel int

const (
	// promptChannelNone: the builder never passes Instance.Prompt at launch.
	promptChannelNone promptChannel = iota
	// promptChannelArgv: the prompt is the trailing positional argument.
	promptChannelArgv
)

// promptMode is whether the agent runs an interactive TUI or a one-shot `-p` run.
type promptMode int

const (
	promptInteractive promptMode = iota
	promptNonInteractive
)

// PromptDelivery is how Instance.Prompt reaches the agent at launch. See ADR-031.
type PromptDelivery int

const (
	// PromptNone: nothing is passed.
	PromptNone PromptDelivery = iota
	// PromptInline: the prompt text is the argv entry.
	PromptInline
	// PromptFileBacked: the argv entry is the contents of a temp file (shell "$(cat f)").
	PromptFileBacked
	// PromptReferenceFile: the argv entry is a short instruction naming a file the agent reads.
	PromptReferenceFile
	// PromptStdin: the prompt file is the process's stdin (one-shot only).
	PromptStdin
)

// choosePromptDelivery is the only place prompt size is compared against a
// limit. size is len(Instance.Prompt) in bytes.
func choosePromptDelivery(ch promptChannel, mode promptMode, size int) PromptDelivery {
	switch {
	case size == 0 || ch == promptChannelNone:
		return PromptNone
	case size < maxInlinePromptBytes:
		return PromptInline
	case size < launchcmd.MaxArgBytes:
		return PromptFileBacked
	case mode == promptNonInteractive:
		return PromptStdin
	default:
		return PromptReferenceFile
	}
}

var (
	flagAddDir       = launchcmd.MustFlag("--add-dir")
	flagEndOfOptions = launchcmd.MustFlag("--")
)

// largePromptFile names one kind of large-prompt file on disk.
type largePromptFile struct{ kind, ext string }

var (
	referencePromptFile = largePromptFile{kind: "ref", ext: ".md"}
	stdinPromptFile     = largePromptFile{kind: "stdin", ext: ".txt"}
)

// appendLaunchPrompt adds i.Prompt to cmd by the delivery choosePromptDelivery
// picks. The "--" separator keeps a prompt starting with "--" from being parsed
// as flags; --add-dir must precede it.
func (i *Instance) appendLaunchPrompt(cmd launchcmd.ExecCommand, ch promptChannel, mode promptMode) launchcmd.ExecCommand {
	delivery := choosePromptDelivery(ch, mode, len(i.Prompt))
	switch delivery {
	case PromptNone:
		return cmd
	case PromptInline, PromptFileBacked:
		return cmd.With(flagEndOfOptions, i.promptArg())
	case PromptReferenceFile:
		path, err := i.writeLargePromptFile(referencePromptFile)
		if err != nil {
			return i.loudInlineFallback(cmd, err)
		}
		return cmd.With(flagAddDir, launchcmd.Literal(filepath.Dir(path)), flagEndOfOptions, launchcmd.Literal(referenceInstruction(path)))
	case PromptStdin:
		path, err := i.writeLargePromptFile(stdinPromptFile)
		if err != nil {
			return i.loudInlineFallback(cmd, err)
		}
		cmd.StdinFile = path
		return cmd
	default:
		panic(fmt.Sprintf("session: unhandled PromptDelivery %d", delivery))
	}
}

// loudInlineFallback keeps the full prompt inline when its file cannot be
// written. At this size the backend rejects the launch, which is the intended
// outcome: a visible start error, never a truncated or dropped prompt.
func (i *Instance) loudInlineFallback(cmd launchcmd.ExecCommand, err error) launchcmd.ExecCommand {
	log.Error("large prompt file could not be written; launch will be rejected rather than truncate the prompt", "session", i.Title, "promptBytes", len(i.Prompt), "err", err)
	return cmd.With(flagEndOfOptions, launchcmd.Literal(i.Prompt))
}

func referenceInstruction(path string) string {
	return "Your complete task instructions are in the file " + path +
		". Read the entire file (in several reads if it is long) and follow it exactly."
}

// writeLargePromptFile writes i.Prompt to a file named from the session id and
// the prompt, so repeated launchCommand calls (restore, relaunch) reuse it. It
// lives until cleanupPromptFile, not on a timer, because a relaunch needs it.
func (i *Instance) writeLargePromptFile(f largePromptFile) (string, error) {
	sum := sha256.Sum256([]byte(i.GetStableID() + "\x00" + i.Prompt))
	dir := promptFileDir()
	if dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, "prompt-"+f.kind+"-"+hex.EncodeToString(sum[:8])+f.ext)
	if st, err := os.Stat(path); err == nil && st.Size() == int64(len(i.Prompt)) {
		i.promptFilePath.Store(&path)
		return path, nil
	}
	if err := os.WriteFile(path, []byte(i.Prompt), 0o600); err != nil {
		return "", fmt.Errorf("write prompt file: %w", err)
	}
	i.promptFilePath.Store(&path)
	return path, nil
}
