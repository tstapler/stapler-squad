package instancestore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent/session"
)

// GetSessionArtifacts loads the raw JSON artifact blob for a session.
// Returns ("", nil) if the session exists but has no artifacts stored yet.
func (i *Instances) GetSessionArtifacts(ctx context.Context, title string) (string, error) {
	sess, err := i.client.Session.Query().
		Where(session.Title(title)).
		Select(session.FieldSessionArtifacts).
		Only(ctx)
	if err != nil {
		return "", fmt.Errorf("GetSessionArtifacts: %w", err)
	}
	return sess.SessionArtifacts, nil
}

// GetAllSessionArtifacts returns a map of title → raw artifacts JSON for all sessions
// that have a non-empty session_artifacts column. Single query replaces N per-session
// queries in LoadInstances (M-4 fix).
func (i *Instances) GetAllSessionArtifacts(ctx context.Context) (map[string]string, error) {
	rows, err := i.client.Session.Query().
		Where(session.SessionArtifactsNEQ("")).
		Select(session.FieldTitle, session.FieldSessionArtifacts).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetAllSessionArtifacts: %w", err)
	}
	result := make(map[string]string, len(rows))
	for _, row := range rows {
		result[row.Title] = row.SessionArtifacts
	}
	return result, nil
}

// GetSessionArtifactsByUUID loads and parses the artifact blob for the session
// identified by its stable UUID (not its title). Returns (nil, nil) when the
// session exists but has no stored artifacts. Returns ErrNotFound when no
// session with that UUID exists.
func (i *Instances) GetSessionArtifactsByUUID(ctx context.Context, sessionUUID string) (*domain.SessionArtifactsBlob, error) {
	sess, err := i.client.Session.Query().
		Where(session.UUIDEQ(sessionUUID)).
		Select(session.FieldSessionArtifacts).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetSessionArtifactsByUUID: %w", err)
	}
	if sess.SessionArtifacts == "" {
		return nil, nil //nolint:nilnil // documented contract: no stored artifacts is not an error
	}
	var blob domain.SessionArtifactsBlob
	if err := json.Unmarshal([]byte(sess.SessionArtifacts), &blob); err != nil {
		return nil, fmt.Errorf("GetSessionArtifactsByUUID: unmarshal: %w", err)
	}
	return &blob, nil
}
