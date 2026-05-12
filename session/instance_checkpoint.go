package session

// instance_checkpoint.go contains checkpoint creation, forking, and retrieval methods.
// The Checkpoint, CheckpointList types and newCheckpointID are defined in checkpoint.go.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"time"

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

	// Count lines in history file for accurate fork truncation later.
	var convLineCount uint64
	if i.HistoryFilePath != "" {
		if f, err := os.Open(i.HistoryFilePath); err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				if len(sc.Bytes()) > 0 {
					convLineCount++
				}
			}
			if scanErr := sc.Err(); scanErr != nil {
				log.Warn("createcheckpoint: error scanning history file", "err", scanErr)
			}
		}
	}

	cp := Checkpoint{
		ID:             newCheckpointID(),
		SessionID:      i.Title,
		Label:          label,
		ScrollbackSeq:  scrollbackSeq,
		ClaudeConvUUID: convUUID,
		ConvLineCount:  convLineCount,
		GitCommitSHA:   gitSHA,
		Timestamp:      time.Now().UTC(),
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

	// Fork Claude conversation if we have the data.
	newConvUUID := ""
	if cp.ConvLineCount > 0 && cp.ClaudeConvUUID != "" && i.HistoryFilePath != "" {
		historyDir := filepath.Dir(i.HistoryFilePath)
		uuid, err := ForkClaudeConversation(i.HistoryFilePath, cp.ConvLineCount, historyDir)
		if err != nil {
			log.Warn("forkfromcheckpoint: skipping conversation fork", "err", err)
		} else {
			newConvUUID = uuid
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
