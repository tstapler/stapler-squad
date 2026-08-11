package session

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent/sessiongoal"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// goalStaleThreshold is how long a goal can go without an update before a live peer is
// considered "stuck" rather than actively working. Goals are set at task start and only
// touched occasionally, so this is deliberately much longer than the terminal-output idle
// timeout used elsewhere (review_queue_determiner.go's basicIdleThreshold).
// ponytail: fixed threshold, make configurable if this needs to vary per workflow.
const goalStaleThreshold = 30 * time.Minute

// WorkspacePeer is another session sharing the caller's workspace (repo), regardless of
// which worktree/branch it's on.
type WorkspacePeer struct {
	SessionUUID string
	Title       string
	Branch      string
	Path        string
	Status      Status
	Goal        *SessionGoalData // nil when the peer has never set a goal

	// InstanceLive is a best-effort liveness signal: false when the session's Status is
	// Stopped. Callers wanting an authoritative "process confirmed dead" signal should
	// cross-reference LiveTmuxSessionUUIDs and override this field.
	InstanceLive bool
	// StaleGoal is true when the peer is live but its goal hasn't been updated within
	// goalStaleThreshold — independent of InstanceLive, per the two-signal requirement.
	StaleGoal bool
}

// Lifecycle returns a coarse peer status label derived from the two independent signals:
// "gone" (process confirmed dead), "stuck" (alive but goal stale), or "active".
func (p WorkspacePeer) Lifecycle() string {
	if !p.InstanceLive {
		return "gone"
	}
	if p.StaleGoal {
		return "stuck"
	}
	return "active"
}

// ListWorkspacePeers returns other sessions sharing workspaceKey, excluding
// excludeSessionUUID. Returns an empty slice (not an error) when workspaceKey is empty or
// no peers exist. Goal-less peers are still returned (Goal is nil) since AC0 only requires
// "other active sessions", not "sessions with a goal set".
func (s *Storage) ListWorkspacePeers(ctx context.Context, workspaceKey string, excludeSessionUUID string) ([]WorkspacePeer, error) {
	if workspaceKey == "" {
		return nil, nil
	}
	data, err := s.ListInstanceData()
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}

	peers := make([]WorkspacePeer, 0, len(data))
	peerUUIDs := make([]string, 0, len(data))
	for _, d := range data {
		if d.UUID == "" || d.UUID == excludeSessionUUID {
			continue
		}
		if d.WorkspaceKey() != workspaceKey {
			continue
		}
		peers = append(peers, WorkspacePeer{
			SessionUUID:  d.UUID,
			Title:        d.Title,
			Branch:       d.Branch,
			Path:         d.Path,
			Status:       d.Status,
			InstanceLive: d.Status != Stopped,
		})
		peerUUIDs = append(peerUUIDs, d.UUID)
	}
	if len(peers) == 0 {
		return peers, nil
	}

	goals, err := s.sessionGoalsByUUIDs(ctx, peerUUIDs)
	if err != nil {
		return nil, err
	}
	for i := range peers {
		g, ok := goals[peers[i].SessionUUID]
		if !ok {
			continue
		}
		peers[i].Goal = g
		elapsed := time.Since(g.UpdatedAt)
		if elapsed < 0 {
			elapsed = 0 // defensive: clock skew shouldn't read as "very fresh"
		}
		peers[i].StaleGoal = peers[i].InstanceLive && elapsed > goalStaleThreshold
	}
	return peers, nil
}

// sessionGoalsByUUIDs bulk-loads SessionGoal rows for the given session UUIDs, keyed by
// session UUID. Missing rows (no goal set) are simply absent from the map.
func (s *Storage) sessionGoalsByUUIDs(ctx context.Context, uuids []string) (map[string]*SessionGoalData, error) {
	client := s.GetEntClient()
	if client == nil {
		return map[string]*SessionGoalData{}, nil
	}
	rows, err := client.SessionGoal.Query().Where(sessiongoal.SessionUUIDIn(uuids...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load session goals: %w", err)
	}
	out := make(map[string]*SessionGoalData, len(rows))
	for _, g := range rows {
		tasks, decodeErr := DecodeTasks(g.Tasks)
		if decodeErr != nil {
			tasks = []TaskNode{}
		}
		out[g.SessionUUID] = &SessionGoalData{
			UUID:        g.ID.String(),
			SessionUUID: g.SessionUUID,
			Goal:        g.Goal,
			Status:      g.Status,
			Tasks:       tasks,
			SetBy:       g.SetBy,
			UpdatedAt:   g.UpdatedAt,
		}
	}
	return out, nil
}

// SetSessionGoalWorkspaceKey stamps the workspace_key column on an existing goal row.
// Best-effort/no-op if no goal row exists yet for sessionUUID. Kept separate from
// SetSessionGoal so that method's signature (and existing tests/call sites) stay unchanged.
func (s *Storage) SetSessionGoalWorkspaceKey(ctx context.Context, sessionUUID, workspaceKey string) error {
	client := s.GetEntClient()
	if client == nil {
		return fmt.Errorf("goal storage not supported by this backend")
	}
	_, err := client.SessionGoal.Update().
		Where(sessiongoal.SessionUUID(sessionUUID)).
		SetWorkspaceKey(workspaceKey).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to stamp workspace_key: %w", err)
	}
	return nil
}

// LiveTmuxSessionUUIDs returns the STAPLER_SESSION_UUID of every currently-running
// staplersquad_ tmux session, by asking tmux directly (not the DB). Used to give
// ListWorkspacePeers results an authoritative "process confirmed dead" signal instead of
// trusting a possibly-stale Status field, mirroring the identification technique in
// ReconcileOrphanedTmuxSessions. Returns an empty (non-nil) set if tmux isn't running.
func LiveTmuxSessionUUIDs(ctx context.Context) map[string]struct{} {
	socketArgs := tmux.ResolveSocket("").Args
	live := make(map[string]struct{})

	listCtx, listCancel := context.WithTimeout(ctx, 5*time.Second)
	defer listCancel()
	out, err := safeexec.CommandContext(listCtx, tmux.Binary(), socketArgs("list-sessions", "-F", "#{session_name}")...).Output()
	if err != nil {
		return live // tmux not running or no sessions
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || !strings.HasPrefix(name, tmux.TmuxPrefix) {
			continue
		}
		envCtx, envCancel := context.WithTimeout(ctx, 5*time.Second)
		envOut, envErr := safeexec.CommandContext(envCtx, tmux.Binary(), socketArgs("show-environment", "-t", name, "STAPLER_SESSION_UUID")...).Output()
		envCancel()
		if envErr != nil {
			continue
		}
		uuid := strings.TrimPrefix(strings.TrimSpace(string(envOut)), "STAPLER_SESSION_UUID=")
		if uuid != "" {
			live[uuid] = struct{}{}
		}
	}
	return live
}

// maxPeersInInitialPrompt caps how many peers are listed in the session-start nudge, so a
// busy workspace (10+ sessions) doesn't blow out the prompt token budget.
const maxPeersInInitialPrompt = 5

// BuildWorkspacePeersBlock renders a one-time "other active sessions in this workspace"
// nudge for a new session's initial prompt. Returns "" when there are no peers, so callers
// can unconditionally append the result without an empty-state noise check (AC5).
func BuildWorkspacePeersBlock(peers []WorkspacePeer) string {
	if len(peers) == 0 {
		return ""
	}

	// Sort live/stuck peers before "gone" ones (stable, so within each group the original
	// DB order is preserved) so the cap below can't hide a live peer behind stale entries.
	sorted := make([]WorkspacePeer, len(peers))
	copy(sorted, peers)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Lifecycle() != "gone" && sorted[j].Lifecycle() == "gone"
	})

	shown := sorted
	if len(shown) > maxPeersInInitialPrompt {
		shown = shown[:maxPeersInInitialPrompt]
	}

	var sb strings.Builder
	sb.WriteString("## Other Active Sessions In This Workspace\n")
	sb.WriteString("These sessions share your repo (possibly a different branch/worktree). ")
	sb.WriteString("Call the `list_workspace_peers` MCP tool before touching shared files (migrations, generated code, config) to avoid conflicting with them.\n")
	for _, p := range shown {
		if p.Branch != "" {
			fmt.Fprintf(&sb, "- %s (%s, %s)", p.Title, p.Branch, p.Lifecycle())
		} else {
			fmt.Fprintf(&sb, "- %s (%s)", p.Title, p.Lifecycle())
		}
		if p.Goal != nil && p.Goal.Goal != "" {
			fmt.Fprintf(&sb, ": %s", p.Goal.Goal)
		}
		sb.WriteString("\n")
	}
	if len(sorted) > len(shown) {
		fmt.Fprintf(&sb, "- ...and %d more\n", len(sorted)-len(shown))
	}
	sb.WriteString("\n")
	return sb.String()
}

// WorkspacePeersBlockForPath resolves repoPath's workspace identity, looks up its peers
// with authoritative tmux liveness applied, and renders the one-time initial-prompt nudge
// (AC5/AC6). Shared by both SessionService.CreateSession and BacklogService's
// initialPromptFor so the two callers can't drift on how the nudge is built. Returns "" on
// any detection/lookup failure, when storage is nil, or when repoPath is empty — this is a
// best-effort convenience nudge, not required session context, so failures are logged and
// swallowed rather than blocking session creation.
func WorkspacePeersBlockForPath(ctx context.Context, storage *Storage, repoPath string) string {
	if repoPath == "" || storage == nil {
		return ""
	}
	info, err := DetectWorktree(repoPath)
	if err != nil {
		log.Warn("WorkspacePeersBlockForPath: failed to detect worktree info", "repo_path", repoPath, "err", err)
		return ""
	}
	mainRepoPath := repoPath
	if info.IsWorktree && info.MainRepoRoot != "" {
		mainRepoPath = info.MainRepoRoot
	}
	workspaceKey := WorkspaceKey(info.GitHubOwner, info.GitHubRepo, mainRepoPath, repoPath)
	if workspaceKey == "" {
		return ""
	}
	peers, err := storage.ListWorkspacePeers(ctx, workspaceKey, "")
	if err != nil {
		log.Warn("WorkspacePeersBlockForPath: failed to list workspace peers", "workspace_key", workspaceKey, "err", err)
		return ""
	}
	ApplyTmuxLiveness(peers, LiveTmuxSessionUUIDs(ctx))
	return BuildWorkspacePeersBlock(peers)
}

// ApplyTmuxLiveness overrides InstanceLive on each peer using an authoritative set of live
// session UUIDs (from LiveTmuxSessionUUIDs), and recomputes StaleGoal since it depends on
// InstanceLive. A peer whose UUID isn't in liveUUIDs is confirmed dead ("gone") even if its
// Status still says Active — this is exactly the crash case liveUUIDs exists to catch.
func ApplyTmuxLiveness(peers []WorkspacePeer, liveUUIDs map[string]struct{}) {
	for i := range peers {
		_, alive := liveUUIDs[peers[i].SessionUUID]
		peers[i].InstanceLive = alive
		if !alive {
			peers[i].StaleGoal = false
			continue
		}
		if peers[i].Goal != nil {
			elapsed := time.Since(peers[i].Goal.UpdatedAt)
			if elapsed < 0 {
				elapsed = 0
			}
			peers[i].StaleGoal = elapsed > goalStaleThreshold
		}
	}
}
