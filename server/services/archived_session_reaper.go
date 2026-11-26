package services

import (
	"context"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tmux"
)

const archivedReapInterval = 5 * time.Minute

// listTmuxSessionNames is a seam so tests don't need a live tmux server.
var listTmuxSessionNames = func(ctx context.Context) ([]string, error) {
	args := tmux.ResolveSocket("").Args("list-sessions", "-F", "#{session_name}")
	out, err := safeexec.CommandContext(ctx, tmux.Binary(), args...).Output()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// releaseArchivedProcesses ends the terminal processes of a just-archived session.
// Archiving only marks the record, so without this the agent keeps running.
func (s *SessionService) releaseArchivedProcesses(ctx context.Context, inst *instance.Instance) {
	for _, shell := range inst.ListShellsInMemory() {
		if shell.Status != instance.ShellStatusRunning {
			continue
		}
		if err := inst.StopShell(ctx, shell.ID); err != nil {
			log.Warn("archive: failed to stop shell", "session", inst.Snapshot().Title, "shell", shell.ID, "err", err)
		}
	}
	if err := inst.KillSession(); err != nil {
		log.Warn("archive: failed to kill terminal session", "session", inst.Snapshot().Title, "err", err)
	}
}

// StartArchivedSessionReaper reaps now and then periodically until ctx ends. It
// covers the archive paths that only write the database (bulk and retention).
func (s *SessionService) StartArchivedSessionReaper(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(archivedReapInterval)
		defer ticker.Stop()
		for {
			s.ReapArchivedSessions(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// ReapArchivedSessions kills tmux sessions that outlived their archived record.
// A pane is killed only if its owner marker matches that record.
func (s *SessionService) ReapArchivedSessions(ctx context.Context) {
	rows, err := s.storage.ListInstanceData()
	if err != nil {
		log.Warn("archived reaper: list sessions", "err", err)
		return
	}
	names, err := listTmuxSessionNames(ctx)
	if err != nil {
		return // no tmux server: nothing to reap
	}
	running := make(map[string]bool, len(names))
	for _, name := range names {
		running[name] = true
	}
	inUse := map[string]bool{}
	for _, d := range rows {
		if d.ArchivedAt == nil {
			inUse[stapleSquadTmuxName(d.TmuxNameTitle())] = true
		}
	}

	for _, d := range rows {
		name := stapleSquadTmuxName(d.TmuxNameTitle())
		if d.ArchivedAt == nil || d.UUID == "" || !running[name] || inUse[name] {
			continue
		}
		if err := killTmuxSessionByTitle(ctx, d.TmuxNameTitle(), false, []string{d.UUID}); err != nil {
			log.Warn("archived reaper: kill failed", "session", d.Title, "err", err)
			continue
		}
		running[name] = false
		log.Info("archived reaper: killed terminal session of archived record", "session", d.Title, "tmux", name)
		// Shell side sessions carry no owner marker; their name embeds the parent's.
		for _, other := range names {
			if strings.HasPrefix(other, name+"_shell_") {
				if out, err := runTmuxKillSession(ctx, other); err != nil && !isTmuxSessionAbsentText(string(out)) {
					log.Warn("archived reaper: kill shell failed", "tmux", other, "err", err)
				}
			}
		}
	}
}
