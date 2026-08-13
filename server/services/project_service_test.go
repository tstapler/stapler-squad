package services

import (
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func newProjectService(t *testing.T) *ProjectService {
	t.Helper()
	return NewProjectService(createTestStorage(t))
}

func newProjectServiceNilStorage() *ProjectService {
	return NewProjectService(nil)
}

// ─── CreateProject ───────────────────────────────────────────────────────────

func TestCreateProject_Success(t *testing.T) {
	svc := newProjectService(t)
	resp, err := svc.CreateProject(t.Context(), connect.NewRequest(&sessionv1.CreateProjectRequest{
		Name:        "my-project",
		Description: "a test project",
	}))
	require.NoError(t, err)
	assert.Equal(t, "my-project", resp.Msg.Project.Name)
	assert.Equal(t, "a test project", resp.Msg.Project.Description)
}

func TestCreateProject_EmptyName(t *testing.T) {
	svc := newProjectService(t)
	_, err := svc.CreateProject(t.Context(), connect.NewRequest(&sessionv1.CreateProjectRequest{}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestCreateProject_NilStorage(t *testing.T) {
	svc := newProjectServiceNilStorage()
	_, err := svc.CreateProject(t.Context(), connect.NewRequest(&sessionv1.CreateProjectRequest{Name: "x"}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeUnavailable, connErr.Code())
}

// ─── ListProjects ─────────────────────────────────────────────────────────────

func TestListProjects_EmptyInitially(t *testing.T) {
	svc := newProjectService(t)
	resp, err := svc.ListProjects(t.Context(), connect.NewRequest(&sessionv1.ListProjectsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Projects)
}

func TestListProjects_AfterCreate(t *testing.T) {
	svc := newProjectService(t)
	_, err := svc.CreateProject(t.Context(), connect.NewRequest(&sessionv1.CreateProjectRequest{Name: "alpha"}))
	require.NoError(t, err)
	_, err = svc.CreateProject(t.Context(), connect.NewRequest(&sessionv1.CreateProjectRequest{Name: "beta"}))
	require.NoError(t, err)

	resp, err := svc.ListProjects(t.Context(), connect.NewRequest(&sessionv1.ListProjectsRequest{}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Projects, 2)
}

func TestListProjects_NilStorageReturnsEmpty(t *testing.T) {
	svc := newProjectServiceNilStorage()
	resp, err := svc.ListProjects(t.Context(), connect.NewRequest(&sessionv1.ListProjectsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.Projects)
}

// ─── UpdateProject ────────────────────────────────────────────────────────────

func TestUpdateProject_Success(t *testing.T) {
	svc := newProjectService(t)
	_, err := svc.CreateProject(t.Context(), connect.NewRequest(&sessionv1.CreateProjectRequest{
		Name:        "proj",
		Description: "original",
	}))
	require.NoError(t, err)

	resp, err := svc.UpdateProject(t.Context(), connect.NewRequest(&sessionv1.UpdateProjectRequest{
		Name:        "proj",
		Description: "updated",
	}))
	require.NoError(t, err)
	assert.Equal(t, "updated", resp.Msg.Project.Description)
}

func TestUpdateProject_NilStorage(t *testing.T) {
	svc := newProjectServiceNilStorage()
	_, err := svc.UpdateProject(t.Context(), connect.NewRequest(&sessionv1.UpdateProjectRequest{Name: "proj"}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeUnavailable, connErr.Code())
}

// ─── DeleteProject ────────────────────────────────────────────────────────────

func TestDeleteProject_Success(t *testing.T) {
	svc := newProjectService(t)
	_, err := svc.CreateProject(t.Context(), connect.NewRequest(&sessionv1.CreateProjectRequest{Name: "to-delete"}))
	require.NoError(t, err)

	_, err = svc.DeleteProject(t.Context(), connect.NewRequest(&sessionv1.DeleteProjectRequest{Id: "to-delete"}))
	require.NoError(t, err)

	list, err := svc.ListProjects(t.Context(), connect.NewRequest(&sessionv1.ListProjectsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, list.Msg.Projects)
}

func TestDeleteProject_EmptyID(t *testing.T) {
	svc := newProjectService(t)
	_, err := svc.DeleteProject(t.Context(), connect.NewRequest(&sessionv1.DeleteProjectRequest{}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestDeleteProject_NilStorage(t *testing.T) {
	svc := newProjectServiceNilStorage()
	_, err := svc.DeleteProject(t.Context(), connect.NewRequest(&sessionv1.DeleteProjectRequest{Id: "x"}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeUnavailable, connErr.Code())
}

// ─── AssignSessionsToProject ──────────────────────────────────────────────────

func TestAssignSessionsToProject_EmptyProjectID(t *testing.T) {
	svc := newProjectService(t)
	_, err := svc.AssignSessionsToProject(t.Context(), connect.NewRequest(&sessionv1.AssignSessionsToProjectRequest{
		ProjectId:  "",
		SessionIds: []string{"s1"},
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestAssignSessionsToProject_NilStorage(t *testing.T) {
	svc := newProjectServiceNilStorage()
	_, err := svc.AssignSessionsToProject(t.Context(), connect.NewRequest(&sessionv1.AssignSessionsToProjectRequest{
		ProjectId:  "proj",
		SessionIds: []string{"s1"},
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeUnavailable, connErr.Code())
}

func TestAssignSessionsToProject_EmptySessions(t *testing.T) {
	svc := newProjectService(t)
	_, err := svc.CreateProject(t.Context(), connect.NewRequest(&sessionv1.CreateProjectRequest{Name: "proj"}))
	require.NoError(t, err)

	// Assigning empty session list to existing project should succeed (no-op).
	_, err = svc.AssignSessionsToProject(t.Context(), connect.NewRequest(&sessionv1.AssignSessionsToProjectRequest{
		ProjectId:  "proj",
		SessionIds: []string{},
	}))
	require.NoError(t, err)
}
