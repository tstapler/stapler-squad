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
//
// Content generation is delegated to engine.SlashCommandSet (Epic 1.5, Story 1.5.2) —
// this function only owns directory creation and the disk-write loop. engine may be
// nil, in which case content generation falls back to buildDefaultSlashCommandSet
// directly, matching CachingPipelineEngine's own default-mode behavior; this keeps
// tests that don't care about PipelineEngine free to pass nil. Both real callers
// (server/services/backlog_service_triage.go's SpawnSessionFromItem and
// backlog_service_sync.go's AttachSessionToItem) must pass the SAME shared engine
// instance (BacklogService.pipelineEngine) — passing two different engines would
// reintroduce the "2 independent callers can drift" regression this seam closes.
func WriteSlashCommands(engine PipelineEngine, item *BacklogItemData, worktreePath string) error {
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

	var files map[string]string
	var err error
	if engine != nil {
		files, err = engine.SlashCommandSet(item)
	} else {
		files, err = buildDefaultSlashCommandSet(item)
	}
	if err != nil {
		return err
	}
	for name, content := range files {
		if err := writeFile(filepath.Join(cmdDir, name), content); err != nil {
			return err
		}
	}

	addWorktreeExcludes(worktreePath)
	return nil
}

// buildDefaultSlashCommandSet returns the filename→rendered-content map for
// the built-in default pipeline's slash commands: status.md, one done-N.md/
// fail-N.md pair per acceptance criterion, review.md, ship.md, and help.md.
//
// This is exactly the content-generation logic WriteSlashCommands used to
// build inline before the PipelineEngine refactor (backlog-configurable-
// pipeline, Epic 1.3) — extracted unchanged so CachingPipelineEngine's
// default-mode short-circuit (session/pipeline_engine.go) and this on-disk
// writer share one source of truth with zero behavior drift.
func buildDefaultSlashCommandSet(item *BacklogItemData) (map[string]string, error) {
	itemID := item.ID
	criteria, err := ParseAcCriteria(item.AcceptanceCriteria)
	if err != nil {
		return nil, fmt.Errorf("buildDefaultSlashCommandSet: failed to parse AC criteria: %w", err)
	}

	files := make(map[string]string, len(criteria)*2+4)

	// status.md
	files["status.md"] = fmt.Sprintf("Call the get_backlog_item MCP tool with item_id=%s.\nFormat the response as a numbered checklist.\n", itemID)

	// Per-criterion done-N.md and fail-N.md
	for _, c := range criteria {
		files[fmt.Sprintf("done-%d.md", c.Index)] = fmt.Sprintf("Call report_progress with item_id=%s, criteria_index=%d, status=pass\n", itemID, c.Index)
		files[fmt.Sprintf("fail-%d.md", c.Index)] = fmt.Sprintf("Call report_progress with item_id=%s, criteria_index=%d, status=fail\n", itemID, c.Index)
	}

	// review.md
	files["review.md"] = fmt.Sprintf("Call request_review with item_id=%s and a 2-3 sentence summary of what was built.\n\n"+
		"Do NOT end your session after this. Wait a bit, then call get_backlog_item (or /backlog/status) again — "+
		"the verdict appears under \"Latest Review Verdict\" once the reviewer submits it. PASS → you're done. "+
		"FAIL/PARTIAL → fix the noted gaps in this same session and run /backlog/review again. Keep looping until PASS.\n", itemID)

	// ship.md
	files["ship.md"] = "You are ready to ship your work as a pull request.\n\n" +
		"Before shipping, confirm all acceptance criteria are marked complete (`/backlog/status`).\n\n" +
		"Steps:\n" +
		"1. Create the pull request:\n" +
		"   Run `/github:pr-ship` — this drives the PR through local CI, code review, remote CI, and\n" +
		"   merge-conflict resolution. It will stop short of actually merging; the final merge is left to\n" +
		"   the human reviewer.\n\n" +
		"2. Once `/github:pr-ship` reports all gates green, request the automated review:\n" +
		"   Run `/backlog/review` with a 2-3 sentence summary of what was built and the PR number.\n\n" +
		"Note: if the repository has no GitHub remote, run `gh pr create` manually — do NOT use `--fill`, which\n" +
		"just concatenates commit messages with no test plan. Write `--title` using Conventional Commits format\n" +
		"and a `--body` structured as `## Summary` (why this change was made, from the backlog item above),\n" +
		"`## What Changed` (a short bullet list), and `## Test plan` (a checklist of concrete verification steps).\n" +
		"Then run `/backlog/review`.\n"

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
	files["help.md"] = helpSb.String()

	return files, nil
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
	sb.WriteString("\n## Before You Start\n")
	sb.WriteString("This worktree's branch may be behind main (this file is rewritten on every spawn and re-attach, but the branch itself is not auto-synced). ")
	sb.WriteString("Run `git merge main` before starting substantive work. If it merges cleanly, continue. ")
	sb.WriteString("If it conflicts, resolve them as part of this task — you have the context to do it correctly; a background process does not.\n")

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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
