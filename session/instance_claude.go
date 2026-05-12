package session

// instance_claude.go contains Claude session management methods for Instance:
// history file detection, UUID extraction, and conversation reattachment.

import (
	"bytes"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// staleResumePattern is the prefix Claude CLI emits when --resume is used with a
// conversation ID that no longer exists in Claude's backend.
const staleResumePattern = "No conversation found with session ID"

// isStaleResumeExit returns true when the PTY exit tail contains the Claude CLI error
// that indicates a stale or expired --resume argument.  ANSI escape sequences are
// stripped before the check so colour output does not prevent matching.
func isStaleResumeExit(exitContent []byte) bool {
	if len(exitContent) == 0 {
		return false
	}
	return bytes.Contains(stripANSISimple(exitContent), []byte(staleResumePattern))
}

// stripANSISimple removes ANSI CSI/OSC/single-char escape sequences so that
// pattern matching works regardless of terminal colour output.  It is intentionally
// minimal — only the forms emitted by common terminal programs are handled.
func stripANSISimple(b []byte) []byte {
	out := make([]byte, 0, len(b))
	i := 0
	for i < len(b) {
		if b[i] != 0x1b {
			out = append(out, b[i])
			i++
			continue
		}
		if i+1 >= len(b) {
			break
		}
		switch b[i+1] {
		case '[': // CSI: ESC [ <params> <final>
			i += 2
			for i < len(b) && b[i] >= 0x20 && b[i] <= 0x3f {
				i++
			}
			if i < len(b) && b[i] >= 0x40 && b[i] <= 0x7e {
				i++
			}
		case ']': // OSC: ESC ] ... ST or BEL
			i += 2
			for i < len(b) {
				if b[i] == 0x07 {
					i++
					break
				}
				if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		default: // ESC + single char
			i += 2
		}
	}
	return out
}

// recoverFromStaleResume clears the stale conversation UUID and restarts the session
// fresh (without --resume) so it does not loop forever on the same bad UUID.
// Safe to call from a goroutine; uses startMu to serialise concurrent calls.
func (i *Instance) recoverFromStaleResume() {
	log.Info("stale --resume uuid detected, clearing and restarting fresh", "session", i.Title)
	log.ForSession(i.Title).Info("stale --resume uuid detected, clearing conversation state and restarting fresh")

	// Remove the UUID so the next Start does not inject --resume.
	i.ClearConversationState()

	// Reset state machine so Start(false) can proceed from Stopped.
	i.RecoverFromStopped()

	if err := i.Start(false); err != nil {
		log.Error("stale-resume auto-recovery failed", "session", i.Title, "err", err)
		log.ForSession(i.Title).Error("stale-resume auto-recovery failed", "err", err)
		return
	}

	log.Info("auto-recovered from stale --resume uuid", "session", i.Title)
	log.ForSession(i.Title).Info("auto-recovered from stale --resume uuid, session restarted fresh")
}

// handleClaudeSessionReattachment attempts to re-attach to stored Claude Code session.
func (i *Instance) handleClaudeSessionReattachment() error {
	if i.claudeSession == nil {
		log.Info("no claude code session data stored", "session", i.Title)
		return nil
	}

	// Check if auto-reattachment is enabled
	if !i.claudeSession.Settings.AutoReattach {
		log.Info("auto-reattachment disabled", "session", i.Title)
		return nil
	}

	// Check if session is too old (based on timeout settings)
	timeoutMinutes := i.claudeSession.Settings.SessionTimeoutMinutes
	if timeoutMinutes > 0 {
		timeout := time.Duration(timeoutMinutes) * time.Minute
		if time.Since(i.claudeSession.LastAttached) > timeout {
			log.Info("claude code session has timed out, skipping re-attachment", "session", i.Title, "elapsed", time.Since(i.claudeSession.LastAttached))
			return nil
		}
	}

	// Initialize Claude session manager
	sessionManager := NewClaudeSessionManager()

	// Try to find and attach to the stored session
	if i.claudeSession.ConversationUUID != "" {
		log.Info("attempting to re-attach to claude code session", "session", i.Title, "uuid", i.claudeSession.ConversationUUID)

		// Verify the session still exists
		session, err := sessionManager.GetSessionByID(i.claudeSession.ConversationUUID)
		if err != nil {
			if i.claudeSession.Settings.CreateNewOnMissing {
				log.Info("stored claude session not found, will create new session", "session", i.Title)
				return i.createNewClaudeSession()
			}
			return fmt.Errorf("stored Claude session '%s' not found: %w", i.claudeSession.ConversationUUID, err)
		}

		// Attempt to attach to the existing session
		if err := sessionManager.AttachToSession(session.ID); err != nil {
			return fmt.Errorf("failed to attach to Claude session '%s': %w", session.ID, err)
		}

		// Update last attached timestamp
		i.claudeSession.LastAttached = time.Now()
		log.Info("successfully re-attached to claude code session", "session_id", session.ID)
	} else {
		// No specific session ID stored, try to find matching sessions by project
		if i.gitManager.HasWorktree() {
			return i.findAndAttachToProjectSession(sessionManager)
		}
	}

	return nil
}

// createNewClaudeSession creates a new Claude Code session for this instance.
func (i *Instance) createNewClaudeSession() error {
	log.Info("creating new claude code session", "session", i.Title)

	// TODO: Implement actual Claude Code session creation
	// This would typically involve:
	// 1. Launching Claude Code with the project directory
	// 2. Waiting for session initialization
	// 3. Capturing the new session ID

	// For now, create placeholder session data
	// sessionManager := NewClaudeSessionManager() // TODO: Use this when implementing actual Claude session creation

	// Generate a placeholder session ID (in practice, this would come from Claude Code)
	newSessionID := fmt.Sprintf("session_%s_%d", i.Title, time.Now().Unix())

	newSession := ClaudeSession{
		ID:             newSessionID,
		ConversationID: "",
		ProjectName:    i.Title,
		LastActive:     time.Now(),
		WorkingDir:     i.GetWorkingDirectory(),
		IsActive:       true,
	}

	// Update the instance's Claude session data
	i.claudeSession = &ClaudeSessionData{
		ConversationUUID: newSession.ID,
		SquadSessionID:   newSession.ConversationID,
		ProjectName:      newSession.ProjectName,
		LastAttached:     time.Now(),
		Settings:         i.claudeSession.Settings, // Preserve existing settings
		Metadata: map[string]string{
			"working_dir": newSession.WorkingDir,
			"created_at":  time.Now().Format(time.RFC3339),
		},
	}

	log.Info("created new claude code session", "session_id", newSessionID, "session", i.Title)

	return nil
}

// findAndAttachToProjectSession finds Claude sessions matching this instance's project.
func (i *Instance) findAndAttachToProjectSession(sessionManager *ClaudeSessionManager) error {
	projectPath := i.GetWorkingDirectory()
	if projectPath == "" {
		return fmt.Errorf("no working directory available for project matching")
	}

	// Find sessions that match this project
	matchingSessions, err := sessionManager.FindSessionByProject(projectPath)
	if err != nil {
		return fmt.Errorf("failed to find matching Claude sessions: %w", err)
	}

	if len(matchingSessions) == 0 {
		if i.claudeSession.Settings.CreateNewOnMissing {
			log.Info("no matching claude sessions found for project, creating new session", "path", projectPath)
			return i.createNewClaudeSession()
		}
		return fmt.Errorf("no matching Claude sessions found for project '%s'", projectPath)
	}

	// Use the most recently active session
	var selectedSession ClaudeSession
	for _, session := range matchingSessions {
		if selectedSession.ID == "" || session.LastActive.After(selectedSession.LastActive) {
			selectedSession = session
		}
	}

	// Attach to the selected session
	if err := sessionManager.AttachToSession(selectedSession.ID); err != nil {
		return fmt.Errorf("failed to attach to Claude session '%s': %w", selectedSession.ID, err)
	}

	// Update the instance's Claude session data
	if i.claudeSession == nil {
		i.claudeSession = &ClaudeSessionData{}
	}
	i.claudeSession.ConversationUUID = selectedSession.ID
	i.claudeSession.SquadSessionID = selectedSession.ConversationID
	i.claudeSession.ProjectName = selectedSession.ProjectName
	i.claudeSession.LastAttached = time.Now()
	if i.claudeSession.Metadata == nil {
		i.claudeSession.Metadata = make(map[string]string)
	}
	i.claudeSession.Metadata["working_dir"] = selectedSession.WorkingDir

	log.Info("successfully attached to claude code session for project", "session_id", selectedSession.ID, "path", projectPath)

	return nil
}

// GetClaudeSession returns the Claude session data for this instance.
func (i *Instance) GetClaudeSession() *ClaudeSessionData {
	return i.claudeSession
}

// SetClaudeSession sets the Claude session data for this instance.
func (i *Instance) SetClaudeSession(sessionData *ClaudeSessionData) {
	i.claudeSession = sessionData
}

// HasClaudeSession returns true if this instance has Claude session data.
func (i *Instance) HasClaudeSession() bool {
	return i.claudeSession != nil && i.claudeSession.ConversationUUID != ""
}

// ClearConversationState removes the stored Claude conversation UUID and history
// file path so that the next Resume starts a fresh conversation rather than
// attempting --resume with a potentially stale or path-mismatched UUID.
func (i *Instance) ClearConversationState() {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	if i.claudeSession != nil {
		i.claudeSession.ConversationUUID = ""
	}
	i.HistoryFilePath = ""
}

// tryExtractConversationUUID attempts to detect the Claude conversation UUID
// by inspecting the open files of the tmux pane process. This uses the
// HistoryFileDetector to find JSONL files in ~/.claude/projects/.
//
// IMPORTANT: This method assumes stateMutex is already held by the caller.
// It must NOT be called without the lock (e.g., from SwitchWorkspace which
// holds stateMutex). It sets claudeSession fields directly.
//
// The tmux session must be alive for this to work, because it inspects
// the foreground process's open file descriptors via proc_pidinfo.
func (i *Instance) tryExtractConversationUUID() {
	// Skip if we already have a conversation UUID.
	if i.claudeSession != nil && i.claudeSession.ConversationUUID != "" {
		return
	}

	detector := i.historyDetector
	if detector == nil {
		detector = NewHistoryFileDetectorWithRealInspector()
	}
	var info *HistoryFileInfo

	// Fast path: inspect open files of the live tmux pane process.
	if i.tmuxManager.DoesSessionExist() {
		pid, err := i.tmuxManager.GetPanePID()
		if err != nil {
			log.Debug("tryextractconversationuuid: could not get pane pid", "session", i.Title, "err", err)
		} else {
			info, err = detector.Detect(pid)
			if err != nil {
				log.Warn("tryextractconversationuuid: detect error", "session", i.Title, "pid", pid, "err", err)
			}
		}
	}

	// Fallback: scan the project directory by path (works after reboot / tmux kill).
	// Use the effective root dir (worktree path for worktree sessions) so we look in
	// the right ~/.claude/projects/ subdirectory, not the base repository path.
	if info == nil {
		effectivePath := i.GetEffectiveRootDir()
		if effectivePath == "" {
			return
		}
		var err error
		info, err = detector.DetectByPath(effectivePath)
		if err != nil {
			log.Warn("tryextractconversationuuid: path-based detect error", "session", i.Title, "err", err)
		}
		if info != nil {
			log.Info("tryextractconversationuuid: found conversation via path fallback", "session", i.Title)
		}
	}

	if info == nil {
		log.Debug("tryextractconversationuuid: no jsonl file found", "session", i.Title)
		return
	}

	// Set the fields directly (caller holds stateMutex).
	if i.claudeSession == nil {
		i.claudeSession = &ClaudeSessionData{}
	}
	i.claudeSession.ConversationUUID = info.ConversationUUID
	i.HistoryFilePath = info.HistoryFilePath
	log.ForSession(i.Title).Info("uuid assigned via tryextractconversationuuid", "uuid", info.ConversationUUID, "path", info.HistoryFilePath)
}

// GetConversationUUID returns the Claude conversation UUID, or "" if not linked.
func (i *Instance) GetConversationUUID() string {
	if i.claudeSession == nil {
		return ""
	}
	return i.claudeSession.ConversationUUID
}

// SetHistoryInfo updates the conversation UUID and history file path.
// Thread-safe: acquires stateMutex write lock.
// No-op if the UUID is already set to the same value.
func (i *Instance) SetHistoryInfo(conversationUUID, historyFilePath string) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	currentUUID := ""
	if i.claudeSession != nil {
		currentUUID = i.claudeSession.ConversationUUID
	}
	if currentUUID == conversationUUID && i.HistoryFilePath == historyFilePath {
		return
	}

	if i.claudeSession == nil {
		i.claudeSession = &ClaudeSessionData{}
	}
	i.claudeSession.ConversationUUID = conversationUUID
	i.HistoryFilePath = historyFilePath
	log.ForSession(i.Title).Info("conversation uuid set", "uuid", conversationUUID, "history", historyFilePath)
}
