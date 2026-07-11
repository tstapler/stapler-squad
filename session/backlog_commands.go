package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// backlogCommandsDir is the relative path from worktree root for slash command files.
const backlogCommandsDir = ".claude/commands/backlog"

// WriteSlashCommands creates the .claude/commands/backlog/ directory and writes
// per-item slash command markdown files. Retries directory creation up to 3 times.
func WriteSlashCommands(item *BacklogItemData, worktreePath string) error {
	cmdDir := filepath.Join(worktreePath, backlogCommandsDir)

	var mkErr error
	for attempt := 0; attempt < 3; attempt++ {
		mkErr = os.MkdirAll(cmdDir, 0o755)
		if mkErr == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if mkErr != nil {
		return fmt.Errorf("WriteSlashCommands: failed to create commands dir %s: %w", cmdDir, mkErr)
	}

	itemID := item.ID

	// status.md
	if err := writeFile(filepath.Join(cmdDir, "status.md"),
		fmt.Sprintf("Call the get_backlog_item MCP tool with item_id=%s.\nFormat the response as a numbered checklist.\n", itemID),
	); err != nil {
		return err
	}

	// Per-criterion done-N.md and fail-N.md
	criteria, err := ParseAcCriteria(item.AcceptanceCriteria)
	if err != nil {
		return fmt.Errorf("WriteSlashCommands: failed to parse AC criteria: %w", err)
	}
	for _, c := range criteria {
		doneContent := fmt.Sprintf("Call report_progress with item_id=%s, criteria_index=%d, status=pass\n", itemID, c.Index)
		if err := writeFile(filepath.Join(cmdDir, fmt.Sprintf("done-%d.md", c.Index)), doneContent); err != nil {
			return err
		}
		failContent := fmt.Sprintf("Call report_progress with item_id=%s, criteria_index=%d, status=fail\n", itemID, c.Index)
		if err := writeFile(filepath.Join(cmdDir, fmt.Sprintf("fail-%d.md", c.Index)), failContent); err != nil {
			return err
		}
	}

	// review.md
	if err := writeFile(filepath.Join(cmdDir, "review.md"),
		fmt.Sprintf("Call request_review with item_id=%s and a 2-3 sentence summary of what was built.\n", itemID),
	); err != nil {
		return err
	}

	// ship.md
	if err := writeFile(filepath.Join(cmdDir, "ship.md"),
		"You are ready to ship your work as a pull request.\n\n"+
			"Before shipping, confirm all acceptance criteria are marked complete (`/backlog/status`).\n\n"+
			"Steps:\n"+
			"1. Create the pull request:\n"+
			"   Run `/github:pr-ship` — this drives the PR through local CI, code review, remote CI, and\n"+
			"   merge-conflict resolution. It will stop short of actually merging; the final merge is left to\n"+
			"   the human reviewer.\n\n"+
			"2. Once `/github:pr-ship` reports all gates green, request the automated review:\n"+
			"   Run `/backlog/review` with a 2-3 sentence summary of what was built and the PR number.\n\n"+
			"Note: if the repository has no GitHub remote, use `gh pr create --fill` to create the PR manually,\n"+
			"then run `/backlog/review`.\n",
	); err != nil {
		return err
	}

	// help.md — list all available commands
	var helpSb strings.Builder
	helpSb.WriteString("# Available Backlog Commands\n\n")
	helpSb.WriteString("- `/backlog/status` — Show current item status and checklist\n")
	for _, c := range criteria {
		fmt.Fprintf(&helpSb, "- `/backlog/done-%d` — Mark criterion %d as complete\n", c.Index, c.Index)
		fmt.Fprintf(&helpSb, "- `/backlog/fail-%d` — Mark criterion %d as failed\n", c.Index, c.Index)
	}
	helpSb.WriteString("- `/backlog/review` — Submit for review with a summary\n")
	helpSb.WriteString("- `/backlog/ship` — Create a PR with /github:pr-ship and submit for review\n")
	if err := writeFile(filepath.Join(cmdDir, "help.md"), helpSb.String()); err != nil {
		return err
	}

	addWorktreeExcludes(worktreePath)
	return nil
}

// CleanupSlashCommands removes the backlog slash command directory.
// Logs but does not return an error if the directory is absent.
func CleanupSlashCommands(worktreePath string) error {
	cmdDir := filepath.Join(worktreePath, backlogCommandsDir)
	if err := os.RemoveAll(cmdDir); err != nil {
		if !os.IsNotExist(err) {
			log.WarningLog.Printf("CleanupSlashCommands: failed to remove %s: %v", cmdDir, err)
		}
	}
	return nil
}

// WriteBacklogContextFile builds the full context prompt and writes it atomically
// to .backlog-context.md in the worktree root. Appends a fallback instructions block.
// priorSessions must match what was passed to the live CLI prompt (BuildTokenBudgetedPrompt)
// so the on-disk fallback the agent re-reads after context compaction doesn't lose history.
func WriteBacklogContextFile(item *BacklogItemData, priorSessions []ItemSessionSummary, worktreePath string) error {
	prompt := BuildSessionInitialPrompt(item, priorSessions)

	var sb strings.Builder
	sb.WriteString(prompt)
	sb.WriteString("\n## Fallback Instructions\n")
	sb.WriteString("If MCP tools are unavailable, continue using the acceptance criteria above.\n")
	sb.WriteString("Record completed criteria in commit messages. Run git commit after each criterion is done.\n")

	content := sb.String()

	destPath := filepath.Join(worktreePath, ".backlog-context.md")
	tmpPath := destPath + ".tmp"

	if err := os.WriteFile(tmpPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("WriteBacklogContextFile: failed to write tmp file: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("WriteBacklogContextFile: failed to rename tmp to dest: %w", err)
	}
	addWorktreeExcludes(worktreePath)
	return nil
}

// CleanupBacklogContextFile removes .backlog-context.md from the worktree root.
// Logs but does not fail if the file is absent.
func CleanupBacklogContextFile(worktreePath string) error {
	path := filepath.Join(worktreePath, ".backlog-context.md")
	if err := os.Remove(path); err != nil {
		if !os.IsNotExist(err) {
			log.WarningLog.Printf("CleanupBacklogContextFile: failed to remove %s: %v", path, err)
		}
	}
	return nil
}

// writeFile is a helper that writes content to a file, creating it if needed.
func writeFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writeFile: failed to write %s: %w", path, err)
	}
	return nil
}

// backlogExcludePatterns are the git exclude patterns for all files stapler-squad
// writes into worktrees. These must never be committed to the target repo.
var backlogExcludePatterns = []string{
	".backlog-context.md",
	".claude/commands/backlog/",
	"web-app/.next/",
}

// addWorktreeExcludes writes backlog-generated file patterns to
// $GIT_DIR/info/exclude so they are invisible to git without touching
// .gitignore (which would pollute the target repo).
func addWorktreeExcludes(worktreePath string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		log.WarningLog.Printf("[addWorktreeExcludes] git rev-parse --git-dir in %s: %v", worktreePath, err)
		return
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(worktreePath, gitDir)
	}

	excludeFile := filepath.Join(gitDir, "info", "exclude")
	if mkErr := os.MkdirAll(filepath.Dir(excludeFile), 0o755); mkErr != nil {
		log.WarningLog.Printf("[addWorktreeExcludes] mkdir %s: %v", filepath.Dir(excludeFile), mkErr)
		return
	}

	existingBytes, _ := os.ReadFile(excludeFile)
	existing := string(existingBytes)

	f, openErr := os.OpenFile(excludeFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if openErr != nil {
		log.WarningLog.Printf("[addWorktreeExcludes] open %s: %v", excludeFile, openErr)
		return
	}
	defer f.Close()

	for _, p := range backlogExcludePatterns {
		if !strings.Contains(existing, p) {
			fmt.Fprintln(f, p)
		}
	}
}
