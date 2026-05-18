package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
)

// backlogCommandsDir is the relative path from worktree root for slash command files.
const backlogCommandsDir = ".claude/commands/backlog"

// WriteSlashCommands creates the .claude/commands/backlog/ directory and writes
// per-item slash command markdown files. Retries directory creation up to 3 times.
func WriteSlashCommands(item *ent.BacklogItem, worktreePath string) error {
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

	itemID := item.ID.String()

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

	// help.md — list all available commands
	var helpSb strings.Builder
	helpSb.WriteString("# Available Backlog Commands\n\n")
	helpSb.WriteString("- `/backlog/status` — Show current item status and checklist\n")
	for _, c := range criteria {
		fmt.Fprintf(&helpSb, "- `/backlog/done-%d` — Mark criterion %d as complete\n", c.Index, c.Index)
		fmt.Fprintf(&helpSb, "- `/backlog/fail-%d` — Mark criterion %d as failed\n", c.Index, c.Index)
	}
	helpSb.WriteString("- `/backlog/review` — Submit for review with a summary\n")
	if err := writeFile(filepath.Join(cmdDir, "help.md"), helpSb.String()); err != nil {
		return err
	}

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
func WriteBacklogContextFile(item *ent.BacklogItem, worktreePath string) error {
	prompt := BuildSessionInitialPrompt(item, nil)

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
