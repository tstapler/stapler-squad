package services

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// builtinClaudeCommands lists hardcoded Claude Code CLI built-ins that are always shown.
var builtinClaudeCommands = []*sessionv1.SlashCommandInfo{
	{Name: "help", Title: "Help", Description: "Show help and available commands", Source: "builtin"},
	{Name: "clear", Title: "Clear", Description: "Clear the conversation history", Source: "builtin"},
	{Name: "compact", Title: "Compact", Description: "Compact conversation to reduce context", Source: "builtin"},
	{Name: "history", Title: "History", Description: "Show conversation history", Source: "builtin"},
	{Name: "review", Title: "Review", Description: "Review current diff", Source: "builtin"},
	{Name: "memory", Title: "Memory", Description: "Show memory file contents", Source: "builtin"},
	{Name: "config", Title: "Config", Description: "Open configuration settings", Source: "builtin"},
	{Name: "model", Title: "Model", Description: "Select the active model", Source: "builtin"},
}

// SlashCommandService resolves slash commands from disk and built-ins.
type SlashCommandService struct{}

// NewSlashCommandService returns an initialized SlashCommandService.
func NewSlashCommandService() *SlashCommandService {
	return &SlashCommandService{}
}

// ListSlashCommands walks ~/.claude/commands/ and target_directory/.claude/commands/,
// merging results with built-in Claude Code commands. Project commands take precedence
// over user commands; built-ins fill any remaining gaps.
func (s *SlashCommandService) ListSlashCommands(
	ctx context.Context,
	req *connect.Request[sessionv1.ListSlashCommandsRequest],
) (*connect.Response[sessionv1.ListSlashCommandsResponse], error) {
	targetDir := req.Msg.GetTargetDirectory()

	seen := map[string]bool{}
	var commands []*sessionv1.SlashCommandInfo

	// Built-ins have lowest precedence — add them last so project/user can override.
	defer func() {
		for _, c := range builtinClaudeCommands {
			if !seen[c.Name] {
				commands = append(commands, c)
				seen[c.Name] = true
			}
		}
	}()

	// Project commands take highest precedence.
	if targetDir != "" {
		expanded, err := expandTilde(targetDir)
		if err == nil {
			projectDir := filepath.Join(expanded, ".claude", "commands")
			if real, err := filepath.EvalSymlinks(projectDir); err == nil {
				projectDir = real
			}
			for _, c := range walkCommandDir(projectDir, "project") {
				if !seen[c.Name] {
					commands = append(commands, c)
					seen[c.Name] = true
				}
			}
		}
	}

	// User (~/.claude/commands/) next — resolve symlinks so WalkDir traverses the real path.
	if homeDir, err := os.UserHomeDir(); err == nil {
		userDir := filepath.Join(homeDir, ".claude", "commands")
		if real, err := filepath.EvalSymlinks(userDir); err == nil {
			userDir = real
		}
		for _, c := range walkCommandDir(userDir, "user") {
			if !seen[c.Name] {
				commands = append(commands, c)
				seen[c.Name] = true
			}
		}
	}

	return connect.NewResponse(&sessionv1.ListSlashCommandsResponse{
		Commands: commands,
	}), nil
}

// walkCommandDir walks dir recursively, returning one SlashCommandInfo per .md file.
// Skips CLAUDE.md and README.md (meta files, not commands).
func walkCommandDir(dir, source string) []*sessionv1.SlashCommandInfo {
	var commands []*sessionv1.SlashCommandInfo
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".md") {
			return nil
		}
		if name == "CLAUDE.md" || name == "README.md" {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		cmdName := relPathToCommandName(rel)
		title, desc := parseCommandFrontmatter(path)
		if title == "" {
			title = cmdName
		}
		commands = append(commands, &sessionv1.SlashCommandInfo{
			Name:        cmdName,
			Title:       title,
			Description: desc,
			Source:      source,
		})
		return nil
	})
	return commands
}

// relPathToCommandName converts "code/fix-loop.md" → "code:fix-loop".
func relPathToCommandName(rel string) string {
	rel = strings.TrimSuffix(rel, ".md")
	return strings.ReplaceAll(rel, string(filepath.Separator), ":")
}

// parseCommandFrontmatter extracts title and description from YAML frontmatter.
// Frontmatter is delimited by "---" lines at the top of the file.
func parseCommandFrontmatter(path string) (title, description string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break
		}
		if !inFrontmatter {
			break
		}
		if after, ok := strings.CutPrefix(line, "title:"); ok {
			title = strings.TrimSpace(after)
		} else if after, ok := strings.CutPrefix(line, "description:"); ok {
			description = strings.TrimSpace(after)
		}
	}
	return title, description
}
