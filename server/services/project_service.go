package services

import (
	"context"
	"fmt"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ProjectService handles Project CRUD RPCs.
type ProjectService struct {
	storage *session.Storage
}

// NewProjectService creates a ProjectService backed by Storage.
// Returns nil if storage is nil (test environments).
func NewProjectService(storage *session.Storage) *ProjectService {
	return &ProjectService{storage: storage}
}

func projectDataToProto(p session.ProjectData) *sessionv1.Project {
	return &sessionv1.Project{
		Id:          p.Name,
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   timestamppb.New(p.CreatedAt),
		UpdatedAt:   timestamppb.New(p.UpdatedAt),
	}
}

// CreateProject creates a new project.
func (s *ProjectService) CreateProject(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateProjectRequest],
) (*connect.Response[sessionv1.CreateProjectResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}
	now := time.Now()
	data := session.ProjectData{
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	created, err := s.storage.CreateProject(ctx, data)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create project: %w", err))
	}
	return connect.NewResponse(&sessionv1.CreateProjectResponse{
		Project: projectDataToProto(*created),
	}), nil
}

// ListProjects returns all projects.
func (s *ProjectService) ListProjects(
	ctx context.Context,
	req *connect.Request[sessionv1.ListProjectsRequest],
) (*connect.Response[sessionv1.ListProjectsResponse], error) {
	if s.storage == nil {
		return connect.NewResponse(&sessionv1.ListProjectsResponse{}), nil
	}
	projects, err := s.storage.ListProjects(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list projects: %w", err))
	}
	protos := make([]*sessionv1.Project, len(projects))
	for i, p := range projects {
		protos[i] = projectDataToProto(p)
	}
	return connect.NewResponse(&sessionv1.ListProjectsResponse{
		Projects: protos,
	}), nil
}

// UpdateProject modifies an existing project.
func (s *ProjectService) UpdateProject(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateProjectRequest],
) (*connect.Response[sessionv1.UpdateProjectResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	data := session.ProjectData{
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
		UpdatedAt:   time.Now(),
	}
	updated, err := s.storage.UpdateProject(ctx, data)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update project: %w", err))
	}
	return connect.NewResponse(&sessionv1.UpdateProjectResponse{
		Project: projectDataToProto(*updated),
	}), nil
}

// DeleteProject removes a project; sessions are unassigned.
func (s *ProjectService) DeleteProject(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteProjectRequest],
) (*connect.Response[sessionv1.DeleteProjectResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	if err := s.storage.DeleteProject(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete project: %w", err))
	}
	return connect.NewResponse(&sessionv1.DeleteProjectResponse{}), nil
}

// AssignSessionsToProject links sessions to a project.
func (s *ProjectService) AssignSessionsToProject(
	ctx context.Context,
	req *connect.Request[sessionv1.AssignSessionsToProjectRequest],
) (*connect.Response[sessionv1.AssignSessionsToProjectResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.ProjectId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}
	if err := s.storage.AssignSessionsToProject(ctx, req.Msg.ProjectId, req.Msg.SessionIds); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to assign sessions to project: %w", err))
	}
	return connect.NewResponse(&sessionv1.AssignSessionsToProjectResponse{}), nil
}
