package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/testutil/gitfake"
)

// These tests swap the process-wide git backend (gitprovider), so none of them is parallel.

const refsSvcSHA = "0123456789abcdef0123456789abcdef01234567"

func TestGitLogSummary_RoutesLogThroughBackendAndFormatsOneline(t *testing.T) {
	rig := gitfake.NewRig(t)
	rig.Install(t)
	rig.GoGit.OnLog = func(_ backend.RepoLocation, req backend.LogRequest) ([]backend.LogEntry, error) {
		assert.Equal(t, gitLogOneLineCommits, req.Limit)
		assert.Equal(t, backend.RangeSpec{Exclude: "base123", Include: "feature"}, req.Range)
		return []backend.LogEntry{{SHA: refsSvcSHA, Subject: "second"}, {SHA: "fedcba9876543210fedcba9876543210fedcba98", Subject: "first"}}, nil
	}

	got := gitLogSummary(context.Background(), "/wt", "base123", "feature")

	assert.Equal(t, "0123456 second\nfedcba9 first", got)
	rig.RequireOnlyGoGit(t, backend.OpLog)
}

func TestGitLogSummary_EmptyOnErrorOrMissingInput(t *testing.T) {
	fake := gitfake.New("backend")
	gitfake.Use(t, fake)
	fake.OnLog = func(backend.RepoLocation, backend.LogRequest) ([]backend.LogEntry, error) {
		return nil, errors.New("bad range")
	}

	assert.Empty(t, gitLogSummary(context.Background(), "/wt", "base", "head"))
	assert.Empty(t, gitLogSummary(context.Background(), "", "base", "head"))
	assert.Empty(t, fake.Calls()[1:], "missing input must not reach git")
}
