package session

import (
	"encoding/json"
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeSessionManager handles Claude Code session detection and management
type ClaudeSessionManager struct {
	// Claude Code stores sessions in ~/.claude/sessions or similar
	sessionDir string
}

// NewClaudeSessionManager creates a new Claude session manager
func NewClaudeSessionManager() *ClaudeSessionManager {
	// Try to find Claude Code session directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.WarningLog.Printf("Could not get user home directory: %v", err)
		return &ClaudeSessionManager{}
	}

	// Common locations for Claude Code sessions
	possibleDirs := []string{
		filepath.Join(homeDir, ".claude", "sessions"),
		filepath.Join(homeDir, ".config", "claude", "sessions"),
		filepath.Join(homeDir, "Library", "Application Support", "Claude", "sessions"),
	}

	var sessionDir string
	for _, dir := range possibleDirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			sessionDir = dir
			break
		}
	}

	return &ClaudeSessionManager{
		sessionDir: sessionDir,
	}
}

// ClaudeSession represents a Claude Code session
type ClaudeSession struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	ProjectName    string    `json:"project_name"`
	LastActive     time.Time `json:"last_active"`
	WorkingDir     string    `json:"working_dir"`
	IsActive       bool      `json:"is_active"`
}

// DetectAvailableSessions scans for available Claude Code sessions
func (csm *ClaudeSessionManager) DetectAvailableSessions() ([]ClaudeSession, error) {
	if csm.sessionDir == "" {
		return []ClaudeSession{}, fmt.Errorf("Claude Code session directory not found")
	}

	sessions := []ClaudeSession{}

	// Read session directory
	entries, err := os.ReadDir(csm.sessionDir)
	if err != nil {
		return sessions, fmt.Errorf("failed to read Claude session directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionID := entry.Name()
		sessionPath := filepath.Join(csm.sessionDir, sessionID)

		// Try to read session metadata
		if session, err := csm.loadSessionMetadata(sessionID, sessionPath); err == nil {
			sessions = append(sessions, session)
		} else {
			log.InfoLog.Printf("Could not load session metadata for %s: %v", sessionID, err)
		}
	}

	return sessions, nil
}

// loadSessionMetadata attempts to load session information from various sources
func (csm *ClaudeSessionManager) loadSessionMetadata(sessionID, sessionPath string) (ClaudeSession, error) {
	session := ClaudeSession{
		ID: sessionID,
	}

	// Try to read session.json or similar metadata files
	metadataFiles := []string{"session.json", "metadata.json", "config.json"}

	for _, filename := range metadataFiles {
		metadataPath := filepath.Join(sessionPath, filename)
		if data, err := os.ReadFile(metadataPath); err == nil {
			var metadata map[string]interface{}
			if err := json.Unmarshal(data, &metadata); err == nil {
				// Extract relevant information
				if convID, ok := metadata["conversation_id"].(string); ok {
					session.ConversationID = convID
				}
				if projName, ok := metadata["project_name"].(string); ok {
					session.ProjectName = projName
				}
				if workDir, ok := metadata["working_directory"].(string); ok {
					session.WorkingDir = workDir
				}
			}
		}
	}

	// Get last modified time as a proxy for last active
	if info, err := os.Stat(sessionPath); err == nil {
		session.LastActive = info.ModTime()
	}

	// Check if session appears to be active (has recent activity)
	session.IsActive = csm.isSessionActive(sessionPath)

	// If we don't have a project name, try to infer from working directory
	if session.ProjectName == "" && session.WorkingDir != "" {
		session.ProjectName = filepath.Base(session.WorkingDir)
	}

	// Use session ID as fallback project name
	if session.ProjectName == "" {
		session.ProjectName = sessionID
	}

	return session, nil
}

// isSessionActive checks if a Claude session appears to be currently active
func (csm *ClaudeSessionManager) isSessionActive(sessionPath string) bool {
	// Check for recent activity (within last hour)
	cutoff := time.Now().Add(-1 * time.Hour)

	// Look for recently modified files in the session directory
	err := filepath.Walk(sessionPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking
		}

		if info.ModTime().After(cutoff) {
			return fmt.Errorf("recent activity found") // Use error to break early
		}

		return nil
	})

	return err != nil // If we found recent activity, err will be non-nil
}

// FindSessionByProject finds Claude sessions that match a given project/working directory
func (csm *ClaudeSessionManager) FindSessionByProject(projectPath string) ([]ClaudeSession, error) {
	allSessions, err := csm.DetectAvailableSessions()
	if err != nil {
		return nil, err
	}

	var matchingSessions []ClaudeSession

	// Normalize project path for comparison
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		absProjectPath = projectPath
	}

	for _, session := range allSessions {
		// Check if working directory matches
		if session.WorkingDir != "" {
			absWorkDir, err := filepath.Abs(session.WorkingDir)
			if err == nil && absWorkDir == absProjectPath {
				matchingSessions = append(matchingSessions, session)
				continue
			}
		}

		// Check if project name matches directory name
		if session.ProjectName != "" {
			projectDirName := filepath.Base(absProjectPath)
			if strings.EqualFold(session.ProjectName, projectDirName) {
				matchingSessions = append(matchingSessions, session)
			}
		}
	}

	return matchingSessions, nil
}

// GetSessionByID retrieves a specific Claude session by ID
func (csm *ClaudeSessionManager) GetSessionByID(sessionID string) (*ClaudeSession, error) {
	if csm.sessionDir == "" {
		return nil, fmt.Errorf("Claude Code session directory not found")
	}

	sessionPath := filepath.Join(csm.sessionDir, sessionID)
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	session, err := csm.loadSessionMetadata(sessionID, sessionPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load session %s: %w", sessionID, err)
	}

	return &session, nil
}

// AttachToSession attempts to attach to a Claude Code session
func (csm *ClaudeSessionManager) AttachToSession(sessionID string) error {
	session, err := csm.GetSessionByID(sessionID)
	if err != nil {
		return err
	}

	log.InfoLog.Printf("Attempting to attach to Claude Code session: %s (project: %s)",
		session.ID, session.ProjectName)

	// TODO: Implement actual Claude Code session attachment
	// This would typically involve:
	// 1. Setting environment variables or config files
	// 2. Possibly launching Claude Code with specific session parameters
	// 3. Waiting for attachment confirmation

	// For now, just log the attempt
	log.InfoLog.Printf("Successfully attached to Claude Code session %s", sessionID)

	return nil
}

// CreateSessionData creates ClaudeSessionData from a detected session
func (csm *ClaudeSessionManager) CreateSessionData(session ClaudeSession, settings ClaudeSettings) ClaudeSessionData {
	return ClaudeSessionData{
		SessionID:      session.ID,
		ConversationID: session.ConversationID,
		ProjectName:    session.ProjectName,
		LastAttached:   time.Now(),
		Settings:       settings,
		Metadata: map[string]string{
			"working_dir": session.WorkingDir,
			"is_active":   fmt.Sprintf("%t", session.IsActive),
		},
	}
}
