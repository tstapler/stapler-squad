// Package shellstore persists custom per-session shells. session.EntRepository embeds
// *Shells so existing callers keep their method set.
package shellstore

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/session"
	entshell "github.com/tstapler/stapler-squad/session/ent/shell"
)

// Shells owns the shells table.
type Shells struct {
	client *ent.Client
}

// New returns Shells over client, which must already hold the migrated schema.
func New(client *ent.Client) *Shells {
	return &Shells{client: client}
}

// nilIfEmpty returns nil for an empty string so Ent's SetNillable* builders leave the column NULL.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// CreateShell persists a new Shell entity for the given session title.
func (s *Shells) CreateShell(ctx context.Context, sessionTitle string, data domain.ShellData) error {
	sess, err := s.client.Session.Query().Where(session.Title(sessionTitle)).Only(ctx)
	if err != nil {
		return fmt.Errorf("CreateShell: session '%s' not found: %w", sessionTitle, err)
	}
	_, err = s.client.Shell.Create().
		SetID(data.ID).
		SetName(data.Name).
		SetCommand(data.Command).
		SetNillableWorkingDir(nilIfEmpty(data.WorkingDir)).
		SetTmuxSessionName(data.TmuxSessionName).
		SetStatus(domain.ShellStatusRunning).
		SetOrderIndex(data.OrderIndex).
		SetSessionID(sess.ID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("CreateShell: %w", err)
	}
	return nil
}

// ListShells returns all shells for the given session title, ordered by order_index.
func (s *Shells) ListShells(ctx context.Context, sessionTitle string) ([]domain.ShellData, error) {
	sess, err := s.client.Session.Query().Where(session.Title(sessionTitle)).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListShells: session '%s' not found: %w", sessionTitle, err)
	}
	shells, err := s.client.Shell.Query().
		Where(entshell.HasSessionWith(session.ID(sess.ID))).
		Order(ent.Asc(entshell.FieldOrderIndex)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListShells: %w", err)
	}
	out := make([]domain.ShellData, len(shells))
	for idx, sh := range shells {
		out[idx] = domain.ShellData{
			ID:              sh.ID,
			Name:            sh.Name,
			Command:         sh.Command,
			WorkingDir:      sh.WorkingDir,
			TmuxSessionName: sh.TmuxSessionName,
			OrderIndex:      sh.OrderIndex,
			Status:          sh.Status,
			ExitCode:        sh.ExitCode,
			StartedAt:       sh.StartedAt,
		}
	}
	return out, nil
}

// UpdateShellStatus updates the status (and optionally exit code + stopped_at) for a shell.
func (s *Shells) UpdateShellStatus(ctx context.Context, shellID, status string, exitCode *int) error {
	upd := s.client.Shell.UpdateOneID(shellID).SetStatus(status)
	if exitCode != nil {
		upd = upd.SetExitCode(*exitCode)
	}
	if status != domain.ShellStatusRunning {
		upd = upd.SetStoppedAt(time.Now())
	}
	if err := upd.Exec(ctx); err != nil {
		return fmt.Errorf("UpdateShellStatus: %w", err)
	}
	return nil
}

// DeleteShell removes a Shell entity by ID.
func (s *Shells) DeleteShell(ctx context.Context, shellID string) error {
	if err := s.client.Shell.DeleteOneID(shellID).Exec(ctx); err != nil {
		return fmt.Errorf("DeleteShell: %w", err)
	}
	return nil
}
