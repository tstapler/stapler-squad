package sessioncreate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// resolveSessionDefaults resolves session defaults (global -> directory ->
// profile, or the alias-specific equivalent when msg.AliasName is set),
// applies explicit request fields (env_vars/cli_flags) on top, and folds in
// any custom-program env vars. skip_defaults bypasses the alias/profile
// resolution entirely (for scripted or explicit-empty sessions), but the
// explicit-field merge and custom-program step still run. Returns the
// possibly-alias-resolved path (unchanged when msg.AliasName's Path is empty
// or resolvedPath was already set) alongside the resolved program/autoYes/
// env/CLI-flags and the alias's own session-type override (domain.SessionTypeDefault
// when none applies).
func (h *Handler) resolveAliasSessionDefaults(
	cfg *config.Config,
	msg *sessionv1.CreateSessionRequest,
	resolvedPath string,
	remoteRequested bool,
	program string,
	autoYes bool,
	instanceEnvVars map[string]string,
) (outProgram string, outAutoYes bool, outCLIFlags string, outResolvedPath string, aliasSessionType domain.SessionType, err error) {
	aliasSessionType = domain.SessionTypeDefault
	resolved, aliasErr := config.ResolveAlias(cfg, msg.AliasName, msg.Branch, msg.Title, "")
	if aliasErr != nil {
		if errors.Is(aliasErr, config.ErrAliasNotFound) {
			return "", false, "", "", "", connect.NewError(connect.CodeNotFound, fmt.Errorf("alias %q not found: %w", msg.AliasName, aliasErr))
		}
		return "", false, "", "", "", connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve alias %q: %w", msg.AliasName, aliasErr))
	}
	if program == "" {
		program = resolved.Program
	}
	if !autoYes && resolved.AutoYes {
		autoYes = true
	}
	for k, v := range resolved.EnvVars {
		instanceEnvVars[k] = v
	}
	if resolvedPath == "" && resolved.Path != "" {
		var aliasResolvedPath string
		var aliasTildeErr error
		if remoteRequested {
			aliasResolvedPath, aliasTildeErr = rejectRemoteTildePath(resolved.Path)
		} else {
			aliasResolvedPath, aliasTildeErr = expandLocalTildePath(resolved.Path)
		}
		if aliasTildeErr != nil {
			return "", false, "", "", "", connect.NewError(connect.CodeInvalidArgument, aliasTildeErr)
		}
		resolvedPath = aliasResolvedPath
	}
	// Read session type directly from the alias config — it is an alias-specific
	// property, not a cascading default, so it is not part of ResolvedDefaults.
	if alias := config.FindAlias(cfg, msg.AliasName); alias != nil {
		aliasSessionType = alias.SessionType
	}
	return program, autoYes, resolved.CLIFlags, resolvedPath, aliasSessionType, nil
}

func (h *Handler) resolveSessionDefaults(cfg *config.Config, msg *sessionv1.CreateSessionRequest, resolvedPath string, remoteRequested bool) (
	program string, autoYes bool, instanceEnvVars map[string]string, instanceCLIFlags string, aliasSessionType domain.SessionType, outPath string, err error,
) {
	program = msg.Program
	autoYes = msg.AutoYes
	instanceEnvVars = make(map[string]string)
	aliasSessionType = domain.SessionTypeDefault // session type from alias config (empty = no override)
	if !msg.SkipDefaults {
		if msg.AliasName != "" {
			var aliasErr error
			program, autoYes, instanceCLIFlags, resolvedPath, aliasSessionType, aliasErr =
				h.resolveAliasSessionDefaults(cfg, msg, resolvedPath, remoteRequested, program, autoYes, instanceEnvVars)
			if aliasErr != nil {
				return "", false, nil, "", "", "", aliasErr
			}
		} else {
			workingDir := msg.WorkingDir
			if workingDir == "" {
				workingDir = resolvedPath
			}
			resolved := config.ResolveDefaults(cfg, workingDir, msg.Profile)
			if program == "" {
				program = resolved.Program
			}
			if !autoYes && resolved.AutoYes {
				autoYes = true
			}
			for k, v := range resolved.EnvVars {
				instanceEnvVars[k] = v
			}
			instanceCLIFlags = resolved.CLIFlags
		}
	}

	// Merge explicit request env_vars on top of resolved defaults.
	for k, v := range msg.EnvVars {
		instanceEnvVars[k] = v
	}
	// Append explicit request cli_flags on top of resolved defaults.
	if msg.CliFlags != "" {
		if instanceCLIFlags != "" {
			instanceCLIFlags += " " + msg.CliFlags
		} else {
			instanceCLIFlags = msg.CliFlags
		}
	}

	// If program refers to a custom program ID, resolve its underlying command, CLI flags, and env vars.
	if program != "" {
		resolvedProg := config.ResolveProgramConfig(cfg, program)
		if resolvedProg.IsCustom {
			for k, v := range resolvedProg.EnvVars {
				if _, exists := instanceEnvVars[k]; !exists {
					instanceEnvVars[k] = v
				}
			}
			// Program CLIFlags are NOT prepended here: buildLaunchCommand resolves them
			// from the stored custom program ID at launch, so doing it here doubles them.
		}
	}

	return program, autoYes, instanceEnvVars, instanceCLIFlags, aliasSessionType, resolvedPath, nil
}

// remapSessionTypeForSpecialCases determines the effective session type --
// explicit msg.SessionType if provided, otherwise inferred from branch via
// resolveSessionType -- then applies, in order: the alias fallback (when the
// alias specified a session type and the request itself didn't), the
// one-off-runs-as-directory collapse, the deferred-GitHub-URL override (force
// NewWorktree since resolveSessionType's empty-branch fallback would
// otherwise misclassify an unresolved clone target as a plain directory), and
// the resume-forces-directory guard (must not create a new worktree that
// would break the --resume lookup). requestedNewWorktree is captured before
// any of these remaps run, so it names the wire-level request itself -- feeds
// startLocked's Epic 1.5 invariant check.
func remapSessionTypeForSpecialCases(msg *sessionv1.CreateSessionRequest, branch string, aliasSessionType domain.SessionType, deferredGitHubURL bool) (domain.SessionType, bool, error) {
	sessionType, err := resolveSessionType(msg, branch)
	if err != nil {
		return "", false, connect.NewError(connect.CodeInvalidArgument, err)
	}
	requestedNewWorktree := msg.SessionType == sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE
	if aliasSessionType != domain.SessionTypeDefault && msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		sessionType = aliasSessionType
	}

	// One-off sessions run as directory sessions — the path was already generated above.
	if sessionType == domain.SessionTypeOneOff {
		sessionType = domain.SessionTypeDirectory
	}

	if deferredGitHubURL && msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED &&
		msg.ExistingWorktree == "" {
		sessionType = domain.SessionTypeNewWorktree
	}

	// For resume sessions, force DIRECTORY type — we must not create a new worktree
	// that would produce a different project path and break the --resume lookup.
	if msg.ResumeId != "" && msg.ForkSourceId == "" &&
		msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		sessionType = domain.SessionTypeDirectory
	}

	return sessionType, requestedNewWorktree, nil
}

// resolveSessionType maps a CreateSessionRequest + resolved branch to a session.SessionType.
// Priority: explicit session_type > inference from branch/existing_worktree.
// ONE_OFF is returned as SessionTypeOneOff; callers are responsible for converting it to
// SessionTypeDirectory after the one-off directory has been generated.
func resolveSessionType(msg *sessionv1.CreateSessionRequest, branch string) (domain.SessionType, error) {
	if msg.SessionType != sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED {
		switch msg.SessionType {
		case sessionv1.SessionType_SESSION_TYPE_DIRECTORY:
			return domain.SessionTypeDirectory, nil
		case sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE:
			return domain.SessionTypeNewWorktree, nil
		case sessionv1.SessionType_SESSION_TYPE_EXISTING_WORKTREE:
			return domain.SessionTypeExistingWorktree, nil
		case sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT:
			return domain.SessionTypeNewProject, nil
		case sessionv1.SessionType_SESSION_TYPE_ONE_OFF:
			return domain.SessionTypeOneOff, nil
		default:
			// Fail loudly instead of silently downgrading an unrecognized enum value
			// to SessionTypeDirectory -- this exact silent-substitution shape has
			// already caused prior incidents in this repo (see plan's Pattern
			// Decisions table / research/pitfalls.md §1).
			return "", fmt.Errorf("unrecognized session_type %v", msg.SessionType)
		}
	}
	if msg.ExistingWorktree != "" {
		return domain.SessionTypeExistingWorktree, nil
	}
	if branch != "" {
		return domain.SessionTypeNewWorktree, nil
	}
	return domain.SessionTypeDirectory, nil
}

// enterpriseHosts unions statically-configured GitHub Enterprise hosts with hosts
// from dynamically-added accounts (gh CLI import, device auth) — mirrors
// ListGitHubAccounts' host union in githubrpc/github_user_service.go so every caller
// recognizes the same enterprise URLs the omnibar's detector does. CreateSession
// and PreviewDestinationPath both call this so the two never diverge.
func (h *Handler) EnterpriseHosts(cfg *config.Config) []string {
	configuredHosts := cfg.GetGitHubEnterpriseHosts()
	var cachedAccounts []githubpkg.CachedAccount
	if userPRCache := h.d.UserPRCache(); userPRCache != nil {
		cachedAccounts = userPRCache.GetCachedAccounts()
	}
	seenHosts := make(map[string]bool, len(configuredHosts)+len(cachedAccounts))
	hosts := make([]string, 0, len(configuredHosts)+len(cachedAccounts))
	addHost := func(host string) {
		host = domain.NormalizeHost(host)
		if host == "" || domain.IsGitHubCom(host) || seenHosts[host] {
			return
		}
		seenHosts[host] = true
		hosts = append(hosts, host)
	}
	for _, h := range configuredHosts {
		addHost(h.Host)
	}
	for _, a := range cachedAccounts {
		addHost(a.Host)
	}
	return hosts
}

// rejectRemoteTildePath rejects a `~`-prefixed path for a remote-host
// location (req.Msg.Remote set) instead of expanding it against
// os.UserHomeDir(), which would resolve to this server process's own home
// directory, not the remote host's -- found in pre-ship review. The remote
// host's real home directory isn't resolvable here without an extra SSH
// round trip, so callers targeting a remote host should pass an absolute
// path instead.
//
// Split from expandLocalTildePath below (Fowler's Remove Flag Argument):
// callers pick the function that matches their target instead of passing a
// remote bool flag.
func rejectRemoteTildePath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		return "", fmt.Errorf("path %q: ~ is not supported for remote sessions -- use an absolute path on the remote host", path)
	}
	return path, nil
}

// expandLocalTildePath expands a leading "~" or "~/" in path against this
// process's local home directory. Falls back to returning path unchanged if
// the home directory can't be resolved, or if the expanded result would
// escape the home directory via path traversal.
func expandLocalTildePath(path string) (string, error) {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home, nil
		} else {
			log.Warn("expandLocalTildePath: failed to resolve home directory", "err", err)
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded := filepath.Join(home, path[2:])
			// Guard against path traversal: reject any result that escapes the home directory.
			if !strings.HasPrefix(expanded, home+string(filepath.Separator)) && expanded != home {
				log.Warn("expandLocalTildePath: path traversal rejected", "input", path)
				return path, nil
			}
			return expanded, nil
		} else {
			log.Warn("expandLocalTildePath: failed to resolve home directory", "err", err)
		}
	}
	return path, nil
}
