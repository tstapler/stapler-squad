package github

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// CommitStatusState is the state of a commit status posted via the GitHub Statuses API.
type CommitStatusState string

const (
	CommitStatusStatePending CommitStatusState = "pending"
	CommitStatusStateSuccess CommitStatusState = "success"
	CommitStatusStateError   CommitStatusState = "error"
	CommitStatusStateFailure CommitStatusState = "failure"
)

func (s CommitStatusState) isValid() bool {
	switch s {
	case CommitStatusStatePending, CommitStatusStateSuccess, CommitStatusStateError, CommitStatusStateFailure:
		return true
	default:
		return false
	}
}

// CommitStatusRequest bundles the fields needed to post a commit status, avoiding a
// same-typed-string parameter pile on SetCommitStatus (see primitive-obsession-checklist.md).
type CommitStatusRequest struct {
	SHA           string
	State         CommitStatusState
	StatusContext string
	Description   string
	TargetURL     string
}

// NewCommitStatusRequest validates and constructs a CommitStatusRequest.
// TargetURL is optional and may be left empty.
func NewCommitStatusRequest(sha string, state CommitStatusState, statusContext, description string) (CommitStatusRequest, error) {
	if sha == "" {
		return CommitStatusRequest{}, fmt.Errorf("sha is required")
	}
	if statusContext == "" {
		return CommitStatusRequest{}, fmt.Errorf("statusContext is required")
	}
	if description == "" {
		return CommitStatusRequest{}, fmt.Errorf("description is required")
	}
	if !state.isValid() {
		return CommitStatusRequest{}, fmt.Errorf("invalid commit status state: %q", state)
	}

	return CommitStatusRequest{
		SHA:           sha,
		State:         state,
		StatusContext: statusContext,
		Description:   description,
	}, nil
}

// SetCommitStatus posts a commit status to the GitHub Statuses API
// (POST /repos/{owner}/{repo}/statuses/{sha}) via the gh CLI.
//
// This targets the Statuses API rather than the Checks API because Checks API writes
// are restricted to GitHub Apps, and this repo's entire GitHub write path is
// PAT/OAuth-token-based via the gh CLI (see ADR-001 in project_plans/pr-comment-check-runs).
func SetCommitStatus(repo RepoRef, req CommitStatusRequest) error {
	if err := CheckGHAuth(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := []string{
		"api",
		fmt.Sprintf("repos/%s/%s/statuses/%s", repo.Owner(), repo.Repo(), req.SHA),
		"-f", fmt.Sprintf("state=%s", req.State),
		"-f", fmt.Sprintf("context=%s", req.StatusContext),
		"-f", fmt.Sprintf("description=%s", req.Description),
	}
	if req.TargetURL != "" {
		args = append(args, "-f", fmt.Sprintf("target_url=%s", req.TargetURL))
	}

	cmd := safeexec.CommandContext(ctx, "gh", args...)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to set commit status on %s: %s", req.SHA, string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to set commit status on %s: %w", req.SHA, err)
	}

	return nil
}
