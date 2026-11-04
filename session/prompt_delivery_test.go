package session

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/launchcmd"
)

func TestChoosePromptDelivery(t *testing.T) {
	const (
		kib = 1024
		mib = 1024 * kib
	)
	sizes := []int{0, 127 * kib, launchcmd.MaxArgBytes - 1, launchcmd.MaxArgBytes, 2 * mib}
	modes := []promptMode{promptInteractive, promptNonInteractive}

	// Every launch builder is an agent kind; an unrecognised program has no builder
	// and so is covered by promptChannelNone below.
	channels := map[string]promptChannel{"unrecognised": promptChannelNone}
	for _, b := range launchBuilders {
		channels[fmt.Sprintf("%T", b)] = b.PromptChannel()
	}
	require.Equal(t, promptChannelArgv, (&claudeLaunchBuilder{}).PromptChannel())

	for name, ch := range channels {
		for _, mode := range modes {
			for _, size := range sizes {
				t.Run(fmt.Sprintf("%s/mode=%d/%dB", name, mode, size), func(t *testing.T) {
					assert.Equal(t, wantPromptDelivery(ch, mode, size), choosePromptDelivery(ch, mode, size))
				})
			}
		}
	}
}

// wantPromptDelivery spells the ADR-031 table out literally, with the byte
// boundaries written as numbers, so the test does not restate the chooser.
func wantPromptDelivery(ch promptChannel, mode promptMode, size int) PromptDelivery {
	if ch == promptChannelNone || size == 0 {
		return PromptNone
	}
	switch {
	case size < 4096:
		return PromptInline
	case size < 131072:
		return PromptFileBacked
	case mode == promptNonInteractive:
		return PromptStdin
	default:
		return PromptReferenceFile
	}
}

func TestPromptLimitsMatchLinux(t *testing.T) {
	assert.Equal(t, 4096, maxInlinePromptBytes)
	assert.Equal(t, 131072, launchcmd.MaxArgBytes)
}

// fakeAgent is a `claude` stand-in that records every argv entry and stdin to files.
type fakeAgent struct {
	path string // the executable, named claude so isClaude matches
	out  string // output prefix: <out>.argN and <out>.stdin
}

func newFakeAgent(t *testing.T) fakeAgent {
	t.Helper()
	dir := t.TempDir()
	a := fakeAgent{path: filepath.Join(dir, "claude"), out: filepath.Join(dir, "rec")}
	script := fmt.Sprintf(`#!/bin/sh
n=0
for a in "$@"; do n=$((n+1)); printf '%%s' "$a" > %q.arg$n; done
printf '%%s' "$n" > %q.argc
cat > %q.stdin
`, a.out, a.out, a.out)
	require.NoError(t, os.WriteFile(a.path, []byte(script), 0o755))
	return a
}

func (a fakeAgent) arg(t *testing.T, n int) string {
	t.Helper()
	b, err := os.ReadFile(fmt.Sprintf("%s.arg%d", a.out, n))
	require.NoError(t, err)
	return string(b)
}

func (a fakeAgent) stdin(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(a.out + ".stdin")
	require.NoError(t, err)
	return string(b)
}

func (a fakeAgent) argc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(a.out + ".argc")
	require.NoError(t, err)
	return string(b)
}

// awkwardPrompt is size bytes of newline-, quote-, $- and UTF-8-bearing text
// with no trailing newline (the $(cat) route strips those by design).
func awkwardPrompt(size int) string {
	unit := "line with 'quotes' \"dq\" $HOME `tick` \\ back é世界\n"
	var b strings.Builder
	for b.Len() < size {
		b.WriteString(unit)
	}
	return strings.TrimRight(b.String()[:size], "\n") + "x"
}

// runViaShell is what the tmux backend does: run the shell string.
func runViaShell(t *testing.T, cmd launchcmd.ExecCommand) {
	t.Helper()
	out, err := safeexec.CommandContext(context.Background(), "/bin/sh", "-c", cmd.ShellString()).CombinedOutput() //nolint:gosec // test-built command
	require.NoError(t, err, string(out))
}

// runDirect is what tymux and native-style backends do: resolve argv and exec
// with no shell, wiring StdinFile to the process.
func runDirect(t *testing.T, cmd launchcmd.ExecCommand) {
	t.Helper()
	program, ok := cmd.Program()
	require.True(t, ok)
	args, err := cmd.Args(os.ReadFile)
	require.NoError(t, err)
	c := safeexec.CommandContext(context.Background(), program, args...) //nolint:gosec // test-built command
	if cmd.StdinFile != "" {
		f, err := os.Open(cmd.StdinFile)
		require.NoError(t, err)
		defer f.Close()
		c.Stdin = f
	}
	out, err := c.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestLargePromptReachesAgentByteIdentical(t *testing.T) {
	const twoMiB = 2 * 1024 * 1024
	runners := map[string]func(*testing.T, launchcmd.ExecCommand){"tmux-shell": runViaShell, "direct-exec": runDirect}

	cases := []struct {
		name     string
		size     int
		oneShot  bool
		delivery PromptDelivery
	}{
		{"inline", 100, false, PromptInline},
		{"file-backed", 100 * 1024, false, PromptFileBacked},
		{"file-backed one-shot", 100 * 1024, true, PromptFileBacked},
		{"reference file", twoMiB, false, PromptReferenceFile},
		{"stdin one-shot", twoMiB, true, PromptStdin},
	}
	for rname, run := range runners {
		for _, tc := range cases {
			t.Run(rname+"/"+tc.name, func(t *testing.T) {
				agent := newFakeAgent(t)
				prompt := awkwardPrompt(tc.size)
				inst := &Instance{Program: agent.path, OneShot: tc.oneShot, Prompt: prompt, UUID: "e2e-" + rname + "-" + tc.name}
				t.Cleanup(inst.cleanupPromptFile)
				mode := promptInteractive
				if tc.oneShot {
					mode = promptNonInteractive
				}
				require.Equal(t, tc.delivery, choosePromptDelivery(promptChannelArgv, mode, len(prompt)))

				cmd := inst.launchCommand("")
				run(t, cmd)

				switch tc.delivery {
				case PromptInline, PromptFileBacked:
					assert.Equal(t, prompt, lastArg(t, agent))
				case PromptReferenceFile:
					instruction := lastArg(t, agent)
					path := strings.TrimSuffix(strings.TrimPrefix(instruction, "Your complete task instructions are in the file "), ". Read the entire file (in several reads if it is long) and follow it exactly.")
					got, err := os.ReadFile(path)
					require.NoError(t, err)
					assert.True(t, bytes.Equal([]byte(prompt), got), "file the agent is told to read differs from the prompt")
					assert.Less(t, len(instruction), 1024, "argv must stay small")
					assert.Contains(t, argsOf(t, agent), "--add-dir")
					assert.Contains(t, argsOf(t, agent), filepath.Dir(path))
				case PromptStdin:
					assert.True(t, agent.stdin(t) == prompt, "stdin differs from the prompt")
					assert.NotContains(t, argsOf(t, agent), prompt[:200], "prompt must not be in argv")
				default:
					t.Fatalf("unexpected delivery %d", tc.delivery)
				}
			})
		}
	}
}

func argsOf(t *testing.T, a fakeAgent) []string {
	t.Helper()
	var n int
	_, err := fmt.Sscanf(a.argc(t), "%d", &n)
	require.NoError(t, err)
	args := make([]string, n)
	for k := 1; k <= n; k++ {
		args[k-1] = a.arg(t, k)
	}
	return args
}

func lastArg(t *testing.T, a fakeAgent) string {
	t.Helper()
	args := argsOf(t, a)
	require.NotEmpty(t, args)
	return args[len(args)-1]
}

func TestReferenceFilePromptIsStableAcrossRelaunches(t *testing.T) {
	inst := &Instance{Program: "claude", Prompt: awkwardPrompt(launchcmd.MaxArgBytes + 10), UUID: "relaunch-stable"}
	t.Cleanup(inst.cleanupPromptFile)

	first := inst.launchCommand("").ShellString()
	second := inst.launchCommand("").ShellString()

	assert.Equal(t, first, second, "a relaunch must reuse the same prompt file")
	assert.Equal(t, 1, strings.Count(first, "--add-dir"))
}

func TestLargePromptFileRemovedOnCleanup(t *testing.T) {
	inst := &Instance{Program: "claude", OneShot: true, Prompt: awkwardPrompt(launchcmd.MaxArgBytes), UUID: "cleanup-" + time.Now().Format("150405.000")}
	cmd := inst.launchCommand("")
	require.NotEmpty(t, cmd.StdinFile)
	_, err := os.Stat(cmd.StdinFile)
	require.NoError(t, err)

	inst.cleanupPromptFile()

	_, err = os.Stat(cmd.StdinFile)
	assert.True(t, os.IsNotExist(err))
}

func TestNonClaudeKindsNeverGetALaunchPrompt(t *testing.T) {
	prompt := awkwardPrompt(2 * 1024 * 1024)
	for _, program := range []string{"pi", "opencode", "agy", "gemini", "aider"} {
		inst := &Instance{Program: program, Prompt: prompt}
		got := inst.launchCommand("")
		assert.Empty(t, got.StdinFile, program)
		assert.Less(t, len(got.ShellString()), 1024, program)
	}
}
