package instancestore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
)

var seedTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func seedBare(t *testing.T, c *ent.Client, title string) {
	t.Helper()
	_, err := c.Session.Create().SetTitle(title).SetUUID("u-" + title).SetPath("/p").SetUpdatedAt(seedTime).SetStatus(1).SetProgram("claude").Save(context.Background())
	require.NoError(t, err)
}

func row(t *testing.T, c *ent.Client, title string) *ent.Session {
	t.Helper()
	s, err := c.Session.Query().Where(session.Title(title)).Only(context.Background())
	require.NoError(t, err)
	return s
}

func TestInstances_should_WriteOnlyItsColumn_When_SingleColumnUpdaterCalled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name  string
		call  func(in *Instances) error
		check func(t *testing.T, s *ent.Session)
	}{
		{"github_pr_number", func(in *Instances) error { return in.UpdateGitHubPRNumber(ctx, "a", 42) },
			func(t *testing.T, s *ent.Session) { assert.Equal(t, 42, s.GithubPrNumber) }},
		{"github_pr_status_terminal", func(in *Instances) error { return in.UpdateGitHubPRStatusTerminal(ctx, "a", true) },
			func(t *testing.T, s *ent.Session) { assert.True(t, s.GithubPrStatusTerminal) }},
		{"session_artifacts", func(in *Instances) error { return in.UpdateSessionArtifacts(ctx, "a", `{"x":1}`) },
			func(t *testing.T, s *ent.Session) { assert.JSONEq(t, `{"x":1}`, s.SessionArtifacts) }},
		{"last_user_response", func(in *Instances) error { return in.UpdateLastUserResponse(ctx, "a", ts) },
			func(t *testing.T, s *ent.Session) {
				require.NotNil(t, s.LastUserResponse)
				assert.True(t, ts.Equal(*s.LastUserResponse))
			}},
		{"last_added_to_queue", func(in *Instances) error { return in.UpdateLastAddedToQueue(ctx, "a", ts) },
			func(t *testing.T, s *ent.Session) {
				require.NotNil(t, s.LastAddedToQueue)
				assert.True(t, ts.Equal(*s.LastAddedToQueue))
			}},
		{"initial_prompt_sent_at", func(in *Instances) error { return in.UpdateInitialPromptSentAt(ctx, "a", ts) },
			func(t *testing.T, s *ent.Session) {
				require.NotNil(t, s.InitialPromptSentAt)
				assert.True(t, ts.Equal(*s.InitialPromptSentAt))
			}},
		{"last_acknowledged", func(in *Instances) error { return in.UpdateLastAcknowledged(ctx, "a", ts) },
			func(t *testing.T, s *ent.Session) {
				require.NotNil(t, s.LastAcknowledged)
				assert.True(t, ts.Equal(*s.LastAcknowledged))
			}},
		{"last_viewed", func(in *Instances) error { return in.UpdateLastViewed(ctx, "a", ts) },
			func(t *testing.T, s *ent.Session) {
				require.NotNil(t, s.LastViewed)
				assert.True(t, ts.Equal(*s.LastViewed))
			}},
		{"timestamps", func(in *Instances) error { return in.UpdateTimestamps(ctx, "a", ts, ts.Add(time.Hour), "sig") },
			func(t *testing.T, s *ent.Session) {
				require.NotNil(t, s.LastTerminalUpdate)
				require.NotNil(t, s.LastMeaningfulOutput)
				assert.True(t, ts.Equal(*s.LastTerminalUpdate))
				assert.True(t, ts.Add(time.Hour).Equal(*s.LastMeaningfulOutput))
				assert.Equal(t, "sig", s.LastOutputSignature)
			}},
		{"review_queue_state", func(in *Instances) error {
			return in.UpdateReviewQueueState(ctx, "a", ts, ts.Add(time.Minute), ts.Add(time.Second), "psig")
		}, func(t *testing.T, s *ent.Session) {
			require.NotNil(t, s.LastUserResponse)
			require.NotNil(t, s.ProcessingGraceUntil)
			require.NotNil(t, s.LastPromptDetected)
			assert.True(t, ts.Equal(*s.LastUserResponse))
			assert.True(t, ts.Add(time.Minute).Equal(*s.ProcessingGraceUntil))
			assert.True(t, ts.Add(time.Second).Equal(*s.LastPromptDetected))
			assert.Equal(t, "psig", s.LastPromptSignature)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := entdbtest.NewClient(t)
			seedBare(t, c, "a")
			seedBare(t, c, "other")
			require.NoError(t, tt.call(New(c)))
			tt.check(t, row(t, c, "a"))
			assert.True(t, row(t, c, "a").UpdatedAt.After(seedTime.Add(time.Hour)), "updated_at must advance")
			// A WHERE title=? regression would leak the write onto the neighbour row.
			other := row(t, c, "other")
			assert.Zero(t, other.GithubPrNumber)
			assert.False(t, other.GithubPrStatusTerminal)
			assert.Empty(t, other.SessionArtifacts)
			assert.Nil(t, other.LastViewed)
			assert.Nil(t, other.LastUserResponse)
			assert.Nil(t, other.LastTerminalUpdate)
			assert.Nil(t, other.LastAddedToQueue)
			assert.Nil(t, other.InitialPromptSentAt)
			assert.Nil(t, other.LastAcknowledged)
			assert.True(t, other.UpdatedAt.Equal(seedTime), "neighbour updated_at must not move")
		})
	}
}

func TestInstances_should_ReportNotFound_When_UpdatingMissingSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	in := New(entdbtest.NewClient(t))
	ts := time.Now()
	errs := map[string]error{
		"pr_number":  in.UpdateGitHubPRNumber(ctx, "x", 1),
		"pr_term":    in.UpdateGitHubPRStatusTerminal(ctx, "x", true),
		"artifacts":  in.UpdateSessionArtifacts(ctx, "x", "{}"),
		"queue":      in.UpdateReviewQueueState(ctx, "x", ts, ts, ts, "s"),
		"user_resp":  in.UpdateLastUserResponse(ctx, "x", ts),
		"timestamps": in.UpdateTimestamps(ctx, "x", ts, ts, "s"),
		"added":      in.UpdateLastAddedToQueue(ctx, "x", ts),
		"prompt":     in.UpdateInitialPromptSentAt(ctx, "x", ts),
		"ack":        in.UpdateLastAcknowledged(ctx, "x", ts),
		"viewed":     in.UpdateLastViewed(ctx, "x", ts),
		"metadata":   in.UpdateSessionMetadata(ctx, "x", nil, nil, nil, nil),
	}
	for name, err := range errs {
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "session not found: x", name)
	}
}

func TestInstances_should_ClearGraceUntil_When_ReviewQueueStateHasZeroGrace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	seedBare(t, c, "a")
	in := New(c)
	ts := time.Now()
	require.NoError(t, in.UpdateReviewQueueState(ctx, "a", ts, ts, time.Time{}, ""))
	require.NotNil(t, row(t, c, "a").ProcessingGraceUntil)
	require.NoError(t, in.UpdateReviewQueueState(ctx, "a", time.Time{}, time.Time{}, time.Time{}, ""))
	s := row(t, c, "a")
	assert.Nil(t, s.ProcessingGraceUntil)
	assert.NotNil(t, s.LastUserResponse, "zero lastUserResponse must not clear the stored value")
}

func TestInstances_should_SkipZeroTimestamps_When_UpdateTimestampsCalled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	seedBare(t, c, "a")
	in := New(c)
	ts := time.Now()
	require.NoError(t, in.UpdateTimestamps(ctx, "a", ts, ts, "sig"))
	require.NoError(t, in.UpdateTimestamps(ctx, "a", time.Time{}, time.Time{}, ""))
	s := row(t, c, "a")
	assert.NotNil(t, s.LastTerminalUpdate)
	assert.NotNil(t, s.LastMeaningfulOutput)
	assert.Equal(t, "sig", s.LastOutputSignature)
}

func TestInstances_should_ApplyGuardedFields_When_UpdateSessionMetadataCalled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	str := func(s string) *string { return &s }

	t.Run("sets all provided fields and renames", func(t *testing.T) {
		t.Parallel()
		c := entdbtest.NewClient(t)
		seedBare(t, c, "old")
		require.NoError(t, New(c).UpdateSessionMetadata(ctx, "old", str("new"), str("cat"), str("note"), str("wd")))
		s := row(t, c, "new")
		assert.Equal(t, "cat", s.Category)
		assert.Equal(t, "note", s.Note)
		assert.Equal(t, "wd", s.WorkingDir)
		n, err := c.Session.Query().Where(session.Title("old")).Count(ctx)
		require.NoError(t, err)
		assert.Zero(t, n)
	})
	t.Run("only the targeted row changes", func(t *testing.T) {
		t.Parallel()
		c := entdbtest.NewClient(t)
		seedBare(t, c, "a")
		seedBare(t, c, "other")
		require.NoError(t, New(c).UpdateSessionMetadata(ctx, "a", str("renamed"), str("cat"), str("note"), str("wd")))
		o := row(t, c, "other")
		assert.Empty(t, o.Category)
		assert.Empty(t, o.Note)
		assert.Empty(t, o.WorkingDir)
		assert.True(t, o.UpdatedAt.Equal(seedTime))
		assert.True(t, row(t, c, "renamed").UpdatedAt.After(seedTime.Add(time.Hour)))
	})
	t.Run("empty strings keep title/category/workingdir but clear note", func(t *testing.T) {
		t.Parallel()
		c := entdbtest.NewClient(t)
		seedBare(t, c, "t")
		in := New(c)
		require.NoError(t, in.UpdateSessionMetadata(ctx, "t", nil, str("cat"), str("note"), str("wd")))
		require.NoError(t, in.UpdateSessionMetadata(ctx, "t", str(""), str(""), str(""), str("")))
		s := row(t, c, "t")
		assert.Equal(t, "cat", s.Category)
		assert.Equal(t, "wd", s.WorkingDir)
		assert.Empty(t, s.Note)
	})
	t.Run("nil pointers leave every column untouched", func(t *testing.T) {
		t.Parallel()
		c := entdbtest.NewClient(t)
		seedBare(t, c, "t")
		in := New(c)
		require.NoError(t, in.UpdateSessionMetadata(ctx, "t", nil, str("cat"), str("note"), str("wd")))
		require.NoError(t, in.UpdateSessionMetadata(ctx, "t", nil, nil, nil, nil))
		s := row(t, c, "t")
		assert.Equal(t, "cat", s.Category)
		assert.Equal(t, "note", s.Note)
		assert.Equal(t, "wd", s.WorkingDir)
	})
}
