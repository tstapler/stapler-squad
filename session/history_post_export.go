package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// PostExporter is an optional HistoryAdapter hook run after a successful Export so the
// target CLI's own post-switch bookkeeping lives with its adapter, not in PortSessionHistory.
type PostExporter interface {
	AfterExport(ctx context.Context, inst *Instance) error
}

// appendHistoryJSONL appends one JSON line to a CLI's history.jsonl under the user's home.
// Directory/open failures are skipped silently; marshal and write failures are returned.
func appendHistoryJSONL(path, cli string, entry map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil
	}
	data, err := json.Marshal(entry)
	if err != nil {
		log.Warn("PortSessionHistory: failed to marshal "+cli+" history entry", "error", err)
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // #nosec G304 -- path is home dir + hardcoded constant components only
	if err != nil {
		return nil
	}
	if _, werr := f.Write(data); werr != nil {
		_ = f.Close()
		log.Warn("PortSessionHistory: failed to write "+cli+" history data", "error", werr)
		return werr
	}
	if _, werr := f.Write([]byte("\n")); werr != nil {
		_ = f.Close()
		log.Warn("PortSessionHistory: failed to write "+cli+" history newline", "error", werr)
		return werr
	}
	if err := f.Close(); err != nil {
		log.Warn("PortSessionHistory: failed to close "+cli+" history file", "error", err)
	}
	return nil
}

// recordPortedClaudeSession stamps the instance's Claude session data after a port and fires
// the saved-callback outside the lock.
func recordPortedClaudeSession(i *Instance, uuidStr, workspace string) {
	i.claudeSessionMu.Lock()
	if i.claudeSession == nil {
		i.claudeSession = &ClaudeSessionData{}
	}
	i.claudeSession.ConversationUUID = uuidStr
	i.claudeSession.ProjectName = i.Title
	i.claudeSession.LastAttached = time.Now()
	if i.claudeSession.Metadata == nil {
		i.claudeSession.Metadata = make(map[string]string)
	}
	i.claudeSession.Metadata["working_dir"] = workspace
	cb := i.claudeSessionIDSavedCallback
	i.claudeSessionMu.Unlock()

	if cb != nil {
		cb()
	}
}

// AfterExport appends the Antigravity history.jsonl entry.
func (a *AgyAdapter) AfterExport(_ context.Context, inst *Instance) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return appendHistoryJSONL(filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl"), "agy", map[string]interface{}{
		"display":        fmt.Sprintf("Ported from Claude: %s", inst.Title),
		"timestamp":      time.Now().UnixMilli(),
		"workspace":      inst.GetWorkingDirectory(),
		"conversationId": inst.GetClaudeConversationUUID(),
	})
}

// AfterExport appends the Claude history.jsonl entry and records the ported session data.
func (a *ClaudeAdapter) AfterExport(_ context.Context, inst *Instance) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	uuidStr := inst.GetClaudeConversationUUID()
	workspace := inst.GetWorkingDirectory()
	if err := appendHistoryJSONL(filepath.Join(home, ".claude", "history.jsonl"), "claude", map[string]interface{}{
		"display":        fmt.Sprintf("Ported from Antigravity: %s", inst.Title),
		"pastedContents": map[string]interface{}{},
		"timestamp":      time.Now().UnixMilli(),
		"project":        workspace,
		"sessionId":      uuidStr,
	}); err != nil {
		return err
	}
	recordPortedClaudeSession(inst, uuidStr, workspace)
	return nil
}

// AfterExport records the ported session data (opencode has no history.jsonl).
func (a *OpencodeAdapter) AfterExport(_ context.Context, inst *Instance) error {
	recordPortedClaudeSession(inst, inst.GetClaudeConversationUUID(), inst.GetWorkingDirectory())
	return nil
}
