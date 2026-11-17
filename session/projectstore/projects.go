// Package projectstore persists the projects that group sessions. session.EntRepository
// embeds *Projects so existing callers keep their method set.
package projectstore

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/project"
	"github.com/tstapler/stapler-squad/session/ent/session"
)

// Projects owns the projects table and the session-to-project link.
type Projects struct {
	client *ent.Client
}

// New returns Projects over client, which must already hold the migrated schema.
func New(client *ent.Client) *Projects {
	return &Projects{client: client}
}

func projectToData(p *ent.Project) domain.ProjectData {
	return domain.ProjectData{
		ID:          p.Name, // Name is the string external ID
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// CreateProject inserts a new project.
func (p *Projects) CreateProject(ctx context.Context, data domain.ProjectData) (*domain.ProjectData, error) {
	c := p.client.Project.Create().
		SetName(data.Name).
		SetCreatedAt(data.CreatedAt).
		SetUpdatedAt(data.UpdatedAt)
	if data.Description != "" {
		c.SetDescription(data.Description)
	}
	row, err := c.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}
	result := projectToData(row)
	return &result, nil
}

// ListProjects returns all projects.
func (p *Projects) ListProjects(ctx context.Context) ([]domain.ProjectData, error) {
	//nolint:entfullscan small, bounded-by-nature table (one row per configured repo), intentionally returns all.
	projects, err := p.client.Project.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	result := make([]domain.ProjectData, len(projects))
	for i, row := range projects {
		result[i] = projectToData(row)
	}
	return result, nil
}

// UpdateProject modifies an existing project.
func (p *Projects) UpdateProject(ctx context.Context, data domain.ProjectData) (*domain.ProjectData, error) {
	row, err := p.client.Project.Query().Where(project.Name(data.Name)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find project '%s': %w", data.Name, err)
	}
	u := p.client.Project.UpdateOne(row).SetUpdatedAt(data.UpdatedAt)
	if data.Description != "" {
		u.SetDescription(data.Description)
	}
	updated, err := u.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update project: %w", err)
	}
	result := projectToData(updated)
	return &result, nil
}

// DeleteProject removes a project; sessions are unassigned (FK cleared) atomically.
func (p *Projects) DeleteProject(ctx context.Context, name string) error {
	tx, err := p.client.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	row, err := tx.Project.Query().Where(project.Name(name)).Only(ctx)
	if err != nil {
		return fmt.Errorf("failed to find project '%s': %w", name, err)
	}

	// Clear the project FK on all sessions that belong to this project before deleting.
	if err = tx.Session.Update().
		Where(session.HasProjectWith(project.Name(name))).
		ClearProject().
		Exec(ctx); err != nil {
		return fmt.Errorf("failed to unassign sessions from project '%s': %w", name, err)
	}

	if err = tx.Project.DeleteOne(row).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete project '%s': %w", name, err)
	}

	return tx.Commit()
}

// AssignSessionsToProject links sessions (by title) to a project (by name).
func (p *Projects) AssignSessionsToProject(ctx context.Context, projectName string, sessionTitles []string) error {
	proj, err := p.client.Project.Query().Where(project.Name(projectName)).Only(ctx)
	if err != nil {
		return fmt.Errorf("failed to find project '%s': %w", projectName, err)
	}
	for _, title := range sessionTitles {
		sess, err := p.client.Session.Query().Where(session.Title(title)).Only(ctx)
		if err != nil {
			return fmt.Errorf("failed to find session '%s': %w", title, err)
		}
		if err := p.client.Session.UpdateOne(sess).SetProjectID(proj.ID).Exec(ctx); err != nil {
			return fmt.Errorf("failed to assign session '%s' to project '%s': %w", title, projectName, err)
		}
	}
	return nil
}
