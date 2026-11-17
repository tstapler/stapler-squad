package projectstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent/project"
	"github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
)

var t0 = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func newProjects(t testing.TB, sessionTitles ...string) *Projects {
	t.Helper()
	client := entdbtest.NewClient(t)
	for _, title := range sessionTitles {
		_, err := client.Session.Create().SetTitle(title).SetPath("/tmp/x").SetStatus(0).SetProgram("claude").Save(context.Background())
		require.NoError(t, err)
	}
	return New(client)
}

func (p *Projects) sessionProject(t testing.TB, title string) string {
	t.Helper()
	proj, err := p.client.Project.Query().Where(project.HasSessionsWith(session.Title(title))).Only(context.Background())
	if err != nil {
		return ""
	}
	return proj.Name
}

func TestProjects_should_RoundTripFieldsAndUseNameAsID_When_Created(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := newProjects(t)
	got, err := p.CreateProject(ctx, domain.ProjectData{Name: "alpha", Description: "d", CreatedAt: t0, UpdatedAt: t0.Add(time.Hour)})
	require.NoError(t, err)
	assert.Equal(t, "alpha", got.ID)
	assert.Equal(t, "d", got.Description)
	assert.True(t, got.CreatedAt.Equal(t0))
	assert.True(t, got.UpdatedAt.Equal(t0.Add(time.Hour)))

	list, err := p.ListProjects(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, *got, list[0])

	_, err = p.CreateProject(ctx, domain.ProjectData{Name: "alpha", CreatedAt: t0, UpdatedAt: t0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create project")
}

func TestProjects_should_KeepDescriptionWhenUpdateOmitsIt_When_Updated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := newProjects(t)
	_, err := p.CreateProject(ctx, domain.ProjectData{Name: "alpha", Description: "keep", CreatedAt: t0, UpdatedAt: t0})
	require.NoError(t, err)

	got, err := p.UpdateProject(ctx, domain.ProjectData{Name: "alpha", UpdatedAt: t0.Add(time.Minute)})
	require.NoError(t, err)
	assert.Equal(t, "keep", got.Description)
	assert.True(t, got.UpdatedAt.Equal(t0.Add(time.Minute)))

	got, err = p.UpdateProject(ctx, domain.ProjectData{Name: "alpha", Description: "new", UpdatedAt: t0.Add(2 * time.Minute)})
	require.NoError(t, err)
	assert.Equal(t, "new", got.Description)

	_, err = p.UpdateProject(ctx, domain.ProjectData{Name: "ghost"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find project 'ghost'")
}

func TestProjects_should_LinkOnlyNamedSessions_When_Assigned(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := newProjects(t, "s1", "s2", "s3")
	_, err := p.CreateProject(ctx, domain.ProjectData{Name: "alpha", CreatedAt: t0, UpdatedAt: t0})
	require.NoError(t, err)

	require.NoError(t, p.AssignSessionsToProject(ctx, "alpha", []string{"s1", "s2"}))
	assert.Equal(t, "alpha", p.sessionProject(t, "s1"))
	assert.Equal(t, "alpha", p.sessionProject(t, "s2"))
	assert.Empty(t, p.sessionProject(t, "s3"))

	err = p.AssignSessionsToProject(ctx, "ghost", []string{"s3"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find project 'ghost'")
	err = p.AssignSessionsToProject(ctx, "alpha", []string{"nope"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find session 'nope'")
}

func TestProjects_should_UnassignSessionsAndRemoveRow_When_Deleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := newProjects(t, "s1", "s2")
	for _, n := range []string{"alpha", "beta"} {
		_, err := p.CreateProject(ctx, domain.ProjectData{Name: n, CreatedAt: t0, UpdatedAt: t0})
		require.NoError(t, err)
	}
	require.NoError(t, p.AssignSessionsToProject(ctx, "alpha", []string{"s1"}))
	require.NoError(t, p.AssignSessionsToProject(ctx, "beta", []string{"s2"}))

	require.NoError(t, p.DeleteProject(ctx, "alpha"))

	list, err := p.ListProjects(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "beta", list[0].Name)
	assert.Empty(t, p.sessionProject(t, "s1"))
	assert.Equal(t, "beta", p.sessionProject(t, "s2"), "other projects keep their sessions")
	n, err := p.client.Session.Query().Where(session.Title("s1")).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the session itself survives")

	err = p.DeleteProject(ctx, "alpha")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find project 'alpha'")
}
