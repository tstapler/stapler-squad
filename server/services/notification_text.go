package services

import (
	"fmt"

	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// maxNotificationMessageLen is the maximum number of runes to include in a
// notification toast message before truncating with "...".
const maxNotificationMessageLen = 120

// truncateString returns s truncated to at most maxRunes Unicode code points,
// appending "..." when truncation occurs. Safe for any UTF-8 content.
func truncateString(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}

// approvalDetail extracts the human-meaningful part of a tool call — the full command
// (including every sub-command of a compound &&/;/pipe chain, since this is the raw
// string before classifier.ExtractAllCommands splits it) for Bash/PowerShell, or the
// target path for file-targeting tools. Returns "" if toolInput has neither, letting the
// caller supply its own fallback.
func approvalDetail(toolInput map[string]interface{}) string {
	if cmd, ok := toolInput["command"].(string); ok && cmd != "" {
		return truncateString(cmd, maxNotificationMessageLen)
	}
	if filePath, ok := toolInput["file_path"].(string); ok && filePath != "" {
		return filePath
	}
	return ""
}

// buildApprovalMessage builds the human-readable message for an approval notification.
func buildApprovalMessage(approval *approvalkit.PendingApproval) string {
	if detail := approvalDetail(approval.ToolInput); detail != "" {
		return detail
	}
	return fmt.Sprintf("Claude needs permission to use %s", approval.ToolName)
}

// stableIDForData mirrors Instance.GetStableID for raw InstanceData.
func stableIDForData(d domain.InstanceData) string {
	if d.UUID != "" {
		return d.UUID
	}
	return d.Title
}

// matchesIDData mirrors Instance.MatchesID for raw InstanceData without constructing
// an Instance or spawning any subprocesses. Matches by UUID, title, or computed
// tmux session name (prefix + sanitized title). The tmux-name branch requires a
// non-empty TmuxPrefix to avoid a title-only match that would bypass UUID lookup.
//
// TODO: move stableIDForData and matchesIDData to session.InstanceData methods so
// the matching semantics are colocated with the type they operate on.
func matchesIDData(d domain.InstanceData, id string) bool {
	if d.UUID != "" && d.UUID == id {
		return true
	}
	if d.Title == id {
		return true
	}
	// Tmux-name match requires a prefix; without it d.TmuxPrefix+title == title
	// which duplicates the title check above and allows UUID-less resolution.
	if d.TmuxPrefix == "" {
		return false
	}
	// Derive via the canonical sanitizer rather than re-implementing it here —
	// a hand-rolled copy silently drifts from tmux.NewSessionName's rules (#162).
	return tmux.NewSessionName(d.TmuxNameTitle(), d.TmuxPrefix).String() == id
}
