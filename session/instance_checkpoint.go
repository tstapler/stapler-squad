package session

// instance_checkpoint.go contains checkpoint creation, forking, and retrieval methods.
// The Checkpoint, CheckpointList types and newCheckpointID are defined in checkpoint.go.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// CreateCheckpoint captures a named state bookmark for this session.
// scrollbackSeq should be the current scrollback high-water mark (from ScrollbackManager);
// pass 0 if the caller does not have access to scrollback state.
// Thread-safe: acquires stateMutex write lock.
// Returns an error if the instance is not started.
func (i *Instance) CreateCheckpoint(label string, scrollbackSeq uint64) (*Checkpoint, error) {
	if !i.started {
		return nil, fmt.Errorf("cannot create checkpoint on unstarted instance '%s'", i.Title)
	}

	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	// Collect git SHA — gracefully empty if no worktree.
	gitSHA, _ := i.gitManager.GetCurrentCommitSHA()

	// Conversation UUID — empty if not yet linked.
	convUUID := ""
	if i.claudeSession != nil {
		convUUID = i.claudeSession.ConversationUUID
	}

	// Count non-empty lines in history file for accurate fork truncation later.
	// Uses bufio.NewReader instead of bufio.Scanner to avoid the 64 KB
	// MaxScanTokenSize limit — Claude tool results and image blocks can exceed it.
	var convLineCount uint64
	if i.HistoryFilePath != "" {
		if f, err := os.Open(i.HistoryFilePath); err == nil {
			defer f.Close()
			reader := bufio.NewReader(f)
			for {
				line, readErr := reader.ReadBytes('\n')
				if len(bytes.TrimSpace(line)) > 0 {
					convLineCount++
				}
				if readErr != nil {
					if readErr != io.EOF {
						log.Warn("createcheckpoint: error reading history file", "err", readErr)
					}
					break
				}
			}
		}
	}
	cpID := newCheckpointID()
	var canonicalTurnIndex int
	var canonicalPath string

	var adapter HistoryAdapter
	claude := NewClaudeAdapter()
	agy := NewAgyAdapter()
	if claude.CanHandle(i.Program) {
		adapter = claude
	} else if agy.CanHandle(i.Program) {
		adapter = agy
	}

	if adapter != nil {
		if turns, err := adapter.Import(context.Background(), i); err == nil {
			canonicalTurnIndex = len(turns)
			if configDir, err := config.GetConfigDir(); err == nil {
				cpDir := filepath.Join(configDir, i.Title, "checkpoints")
				if err := os.MkdirAll(cpDir, 0700); err == nil {
					cpPath := filepath.Join(cpDir, cpID+".jsonl")
					if f, err := os.Create(cpPath); err == nil {
						for _, turn := range turns {
							if bytes, err := json.Marshal(turn); err == nil {
								f.Write(bytes)
								f.Write([]byte("\n"))
							}
						}
						f.Close()
						canonicalPath = cpPath
					}
				}
			}
		}
	}

	cp := Checkpoint{
		ID:                 cpID,
		SessionID:          i.Title,
		Label:              label,
		ScrollbackSeq:      scrollbackSeq,
		ClaudeConvUUID:     convUUID,
		ConvLineCount:      convLineCount,
		GitCommitSHA:       gitSHA,
		Timestamp:          time.Now().UTC(),
		CanonicalTurnIndex: canonicalTurnIndex,
		CanonicalPath:      canonicalPath,
	}

	i.Checkpoints = append(i.Checkpoints, cp)
	i.ActiveCheckpoint = cp.ID

	return &cp, nil
}

// ForkFromCheckpoint creates a new, unstarted Instance that is an independent branch of i,
// seeded from the state captured at the checkpoint identified by checkpointID.
func (i *Instance) ForkFromCheckpoint(checkpointID, newTitle string, configDir string) (*Instance, error) {
	cp := i.Checkpoints.FindByID(checkpointID)
	if cp == nil {
		return nil, fmt.Errorf("checkpoint %q not found on session %q", checkpointID, i.Title)
	}
	if newTitle == "" {
		return nil, fmt.Errorf("newTitle must not be empty")
	}

	newConvUUID := ""
	var turns []CanonicalTurn
	hasCanonicalForked := false

	if cp.CanonicalPath != "" && cp.CanonicalTurnIndex > 0 {
		if f, err := os.Open(cp.CanonicalPath); err == nil {
			defer f.Close()
			dec := json.NewDecoder(f)
			for {
				var turn CanonicalTurn
				if err := dec.Decode(&turn); err == io.EOF {
					break
				} else if err != nil {
					log.Warn("forkfromcheckpoint: failed to decode canonical turn", "err", err)
					break
				}
				turns = append(turns, turn)
			}
			if len(turns) >= cp.CanonicalTurnIndex {
				hasCanonicalForked = true
				newConvUUID = newCheckpointID()
			}
		}
	}

	// If we couldn't load canonical turns, fall back to legacy ForkClaudeConversation if applicable
	if !hasCanonicalForked && cp.ConvLineCount > 0 && cp.ClaudeConvUUID != "" && i.HistoryFilePath != "" {
		historyDir := filepath.Dir(i.HistoryFilePath)
		uuidStr, err := ForkClaudeConversation(i.HistoryFilePath, cp.ConvLineCount, historyDir)
		if err != nil {
			log.Warn("forkfromcheckpoint: skipping conversation fork", "err", err)
		} else {
			newConvUUID = uuidStr
		}
	}

	// Fork scrollback.
	srcScrollback := filepath.Join(configDir, i.Title, "scrollback.jsonl")
	dstScrollback := filepath.Join(configDir, newTitle, "scrollback.jsonl")
	if err := scrollback.ForkScrollback(srcScrollback, cp.ScrollbackSeq, dstScrollback); err != nil {
		log.Warn("forkfromcheckpoint: skipping scrollback fork", "err", err)
	}

	// Build the new instance.
	opts := InstanceOptions{
		Title:      newTitle,
		Path:       i.Path,
		WorkingDir: i.WorkingDir,
		Program:    i.Program,
		AutoYes:    i.AutoYes,
		Category:   i.Category,
		Tags:       append([]string(nil), i.Tags...),
		ResumeId:   newConvUUID,
	}

	newInst, err := NewInstance(opts)
	if err != nil {
		return nil, fmt.Errorf("fork from checkpoint: create instance: %w", err)
	}

	// Export canonical turns if successfully parsed
	if hasCanonicalForked {
		newInst.stateMutex.Lock()
		if newInst.claudeSession == nil {
			newInst.claudeSession = &ClaudeSessionData{}
		}
		newInst.claudeSession.ConversationUUID = newConvUUID
		newInst.claudeSession.ProjectName = newInst.Title
		newInst.stateMutex.Unlock()

		var adapter HistoryAdapter
		claude := NewClaudeAdapter()
		agy := NewAgyAdapter()
		if claude.CanHandle(newInst.Program) {
			adapter = claude
		} else if agy.CanHandle(newInst.Program) {
			adapter = agy
		}

		if adapter != nil {
			turnsToExport := turns[:cp.CanonicalTurnIndex]
			if err := adapter.Export(context.Background(), turnsToExport, newInst); err != nil {
				log.Warn("forkfromcheckpoint: failed to export canonical turns", "err", err)
			}
		}
	}

	// Attach a git worktree branched from the checkpoint SHA.
	if i.gitManager.HasWorktree() && cp.GitCommitSHA != "" {
		branchName := "fork/" + newTitle
		wt, _, err := git.NewGitWorktreeFromCommitSHA(i.Path, newTitle, branchName, cp.GitCommitSHA)
		if err != nil {
			log.Warn("forkfromcheckpoint: skipping git worktree", "err", err)
		} else {
			newInst.gitManager.SetWorktree(wt)
		}
	}

	newInst.ForkedFromID = i.Title

	return newInst, nil
}

// GetCheckpoints returns a snapshot copy of the checkpoint list, safe for
// concurrent reads from outside the instance's lock domain.
func (i *Instance) GetCheckpoints() CheckpointList {
	i.stateMutex.RLock()
	defer i.stateMutex.RUnlock()
	cp := make(CheckpointList, len(i.Checkpoints))
	copy(cp, i.Checkpoints)
	return cp
}
