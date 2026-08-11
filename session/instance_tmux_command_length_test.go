package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// TestBuildLaunchCommand_LargePromptSurvivesRealTmuxNewSession is an
// end-to-end regression test for the review-gate spawn bug: reproduces the
// original failure against a real tmux binary and proves the fix avoids it.
//
// Background: BacklogLifecycle.ReconcileStuckReviewGates kept re-spawning the
// same review-gate session every ~8 minutes, and every attempt failed with
// `error starting tmux session: command too long (exit status 1)` because the
// review prompt (a large description plus many verbose acceptance criteria)
// was embedded directly, inline, in the `tmux new-session` command string.
// tmux's client/server protocol caps the entire new-session command at
// somewhere between 16000 and 16500 bytes -- confirmed empirically against
// the tmux binary below -- and that check happens before any shell ever runs,
// so no amount of careful shell-quoting of the inline prompt can dodge it.
//
// This test:
//  1. Reproduces the original bug directly against tmux: an inline command
//     embedding a >16KB prompt is rejected with "command too long".
//  2. Proves the fix: the command instance.buildLaunchCommand actually
//     produces for the same oversized prompt is accepted by tmux, and the
//     spawned process receives the full, untruncated prompt content.
func TestBuildLaunchCommand_LargePromptSurvivesRealTmuxNewSession(t *testing.T) {
	checkTmuxAvailable(t)

	// Two distinct sockets, not one shared between steps 1 and 2: tmux's
	// default exit-empty behavior tears the server down as soon as it has
	// zero sessions and zero attached clients, and step 1 (below)
	// deliberately produces zero sessions (its new-session is expected to be
	// rejected). Reusing that now-dead server for step 2 races tmux's
	// teardown and intermittently fails with "server exited unexpectedly" --
	// a test-harness flake, not a reproduction of the bug being guarded
	// against here.
	killSocket := func(socket string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = safeexec.CommandContext(ctx, "tmux", "-L", socket, "kill-server").Run()
	}
	reproSocket := fmt.Sprintf("test_cmdlen_repro_%d_%d", os.Getpid(), time.Now().UnixNano())
	fixedSocket := fmt.Sprintf("test_cmdlen_fixed_%d_%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() { killSocket(reproSocket) })
	t.Cleanup(func() { killSocket(fixedSocket) })

	// A prompt shaped like the real trigger: well past both maxInlinePromptBytes
	// and the ~16KB tmux command-length limit. Deliberately does not end in a
	// newline: promptArg's temp-file/command-substitution path strips trailing
	// newlines (a documented, accepted POSIX command-substitution quirk -- see
	// promptArg's doc comment), which is unrelated to the bug this test guards
	// against, so the prompt avoids it here to make the byte-for-byte
	// comparison below a clean proof of "no truncation/corruption".
	var sb strings.Builder
	sb.WriteString("--- BACKLOG ITEM DATA ---\nRich File Browser\n")
	for n := 0; n < 40; n++ {
		sb.WriteString(strings.Repeat("x", 500))
		sb.WriteString("\n")
	}
	sb.WriteString("item_id: deadbeef-0000-0000-0000-000000000000")
	prompt := sb.String()
	if len(prompt) < 16*1024 {
		t.Fatalf("test setup bug: prompt (%d bytes) should exceed the ~16KB tmux command-length limit", len(prompt))
	}

	// Step 1: reproduce the original bug directly against tmux -- an inline
	// command embedding the oversized prompt must be rejected.
	inlineCmd := "echo " + shellQuote(prompt)
	out, err := safeexec.CommandContext(context.Background(), "tmux", "-L", reproSocket,
		"new-session", "-d", "-s", "repro-inline", inlineCmd).CombinedOutput()
	if err == nil {
		t.Fatalf("expected tmux to reject an inline command embedding a %d-byte prompt with 'command too long', but it succeeded", len(prompt))
	}
	if !strings.Contains(string(out), "command too long") {
		t.Fatalf("expected tmux to fail with 'command too long', got: %s (%v)", out, err)
	}

	// Step 2: prove the fix. Use a fake "claude" executable (isClaude() only
	// checks basename) that writes its final positional argument -- the
	// prompt -- to OUT_FILE, so we can verify the spawned process actually
	// receives the full, untruncated prompt content.
	tmpDir := t.TempDir()
	fakeClaudePath := filepath.Join(tmpDir, "claude")
	outFile := filepath.Join(tmpDir, "captured-prompt.txt")
	script := "#!/bin/sh\nset -eu\nshift 4\nprintf '%s' \"$1\" > \"$OUT_FILE.tmp\" && mv \"$OUT_FILE.tmp\" \"$OUT_FILE\"\n"
	if err := os.WriteFile(fakeClaudePath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake claude script: %v", err)
	}

	inst := &Instance{Program: fakeClaudePath, OneShot: true, Prompt: prompt}
	fixedCmd := inst.buildLaunchCommand("")
	if strings.Contains(fixedCmd, prompt) {
		t.Fatalf("fixed command still embeds the large prompt inline: %s", fixedCmd)
	}

	newSessionArgs := []string{"-L", fixedSocket, "new-session", "-d", "-s", "repro-fixed", "-e", "OUT_FILE=" + outFile, fixedCmd}
	if out, err := safeexec.CommandContext(context.Background(), "tmux", newSessionArgs...).CombinedOutput(); err != nil {
		t.Fatalf("tmux rejected the fixed command (this is the bug re-appearing): %s (%v)\ncommand was: %s", out, err, fixedCmd)
	}

	// Wait for the fake claude script to run and capture the prompt. A
	// successful read alone isn't proof of a complete write -- os.ReadFile
	// can observe a torn/short read if it races the script's write, so only
	// accept a read once its content exactly matches the expected prompt
	// (matching length alone wouldn't rule out a same-length, wrong-content
	// torn read).
	deadline := time.Now().Add(5 * time.Second)
	for {
		if data, readErr := os.ReadFile(outFile); readErr == nil && string(data) == prompt {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("fake claude script never wrote the full %d-byte prompt to %s within the deadline", len(prompt), outFile)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
