package tokens

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
)

// skillsPathPattern matches ~/.claude/skills/<name>.md anywhere in text.
var skillsPathPattern = regexp.MustCompile(`/\.claude/skills/([^/\s]+?)(?:\.md)?(?:[^a-zA-Z0-9_-]|$)`)

// commandPattern matches /command or /command:subcommand at the start or after whitespace.
var commandPattern = regexp.MustCompile(`(?:^|\s)/([a-zA-Z][a-zA-Z0-9:_-]*)`)

// detectSkillActivations scans user message content blocks for:
//  1. Text blocks starting with a /command → IsCommand=true
//  2. Tool result blocks whose tool input path contains /.claude/skills/ → IsCommand=false
func detectSkillActivations(contents []jsonlContent, turnIndex int) []SkillActivation {
	var activations []SkillActivation

	for _, c := range contents {
		switch c.Type {
		case "text":
			activations = append(activations, detectCommandsInText(c.Text, turnIndex)...)

		case "tool_result":
			// Check the nested content array for text blocks with skill paths.
			activations = append(activations, detectSkillFromToolResult(c, turnIndex)...)
		}
	}

	return activations
}

// detectCommandsInText finds /command patterns in text content.
func detectCommandsInText(text string, turnIndex int) []SkillActivation {
	var activations []SkillActivation

	matches := commandPattern.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		name := m[1]
		activations = append(activations, SkillActivation{
			Name:      name,
			TurnIndex: turnIndex,
			IsCommand: true,
		})
	}

	return activations
}

// detectSkillFromToolResult checks if a tool_result content block came from a
// Read tool on a skills file.
func detectSkillFromToolResult(c jsonlContent, turnIndex int) []SkillActivation {
	// The tool_result content field can be a string or array.
	// We need to find skill paths in nested content.
	if len(c.Content) == 0 {
		return nil
	}

	// Try to detect skill paths in nested string content.
	var nestedContents []jsonlContent
	if err := json.Unmarshal(c.Content, &nestedContents); err == nil {
		for _, nc := range nestedContents {
			if nc.Type == "text" && isSkillPath(nc.Text) {
				if name := extractSkillName(nc.Text); name != "" {
					return []SkillActivation{{
						Name:      name,
						TurnIndex: turnIndex,
						IsCommand: false,
					}}
				}
			}
		}
	}

	// Also check if the raw content is a string containing a skill path.
	var strContent string
	if err := json.Unmarshal(c.Content, &strContent); err == nil {
		if isSkillPath(strContent) {
			if name := extractSkillName(strContent); name != "" {
				return []SkillActivation{{
					Name:      name,
					TurnIndex: turnIndex,
					IsCommand: false,
				}}
			}
		}
	}

	return nil
}

// isSkillPath returns true if the text contains a ~/.claude/skills/ path.
func isSkillPath(text string) bool {
	return strings.Contains(text, "/.claude/skills/") ||
		strings.Contains(text, ".claude/skills/")
}

// extractSkillName extracts the skill name from a path like ~/.claude/skills/code-review.md.
func extractSkillName(text string) string {
	m := skillsPathPattern.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	name := m[1]
	// Strip .md extension if present.
	name = strings.TrimSuffix(name, ".md")
	// Return just the base name without any directory prefix.
	return filepath.Base(name)
}
