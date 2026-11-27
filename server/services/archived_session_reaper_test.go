package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/tmux"
)

type rowsStore struct {
	session.InstanceStore
	rows []domain.InstanceData
}

func (s rowsStore) ListInstanceData() ([]domain.InstanceData, error) { return s.rows, nil }

// Not parallel: swaps the package's tmux seams.
func TestReapArchivedSessions_should_KillOnlyArchivedOwnedSessions(t *testing.T) {
	now := time.Now()
	archived := func(title, uuid string) domain.InstanceData {
		return domain.InstanceData{Title: title, UUID: uuid, ArchivedAt: &now}
	}
	name := stapleSquadTmuxName
	rows := []domain.InstanceData{
		archived("gone-work", "u1"),
		{Title: "renamed", TmuxTitle: "named-after", UUID: "u2", ArchivedAt: &now},
		archived("reused", "u3"),
		{Title: "reused-now", TmuxTitle: "reused", UUID: "u4"},
		archived("someone-else", "u5"),
		{Title: "live", UUID: "u6"},
	}
	owners := map[string]string{name("gone-work"): "u1", name("named-after"): "u2", name("someone-else"): "other", name("live"): "u6"}
	running := []string{name("gone-work"), name("gone-work") + "_shell_9", name("named-after"), name("reused"), name("someone-else"), name("live")}

	origList, origKill, origOwner := listTmuxSessionNames, runTmuxKillSession, readSessionOwnerUUID
	t.Cleanup(func() { listTmuxSessionNames, runTmuxKillSession, readSessionOwnerUUID = origList, origKill, origOwner })
	listTmuxSessionNames = func(context.Context) ([]string, error) { return running, nil }
	readSessionOwnerUUID = func(_ context.Context, _ tmux.Socket, n string) (string, error) { return owners[n], nil }
	var killed []string
	runTmuxKillSession = func(_ context.Context, n string) ([]byte, error) { killed = append(killed, n); return nil, nil }

	(&SessionService{storage: rowsStore{rows: rows}}).ReapArchivedSessions(context.Background())

	assert.ElementsMatch(t, []string{name("gone-work"), name("gone-work") + "_shell_9", name("named-after")}, killed)
}
