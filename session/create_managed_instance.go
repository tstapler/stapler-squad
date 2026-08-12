package session

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// Sentinel errors returned by CreateManagedInstance, wrapped with
// caller-specific detail via %w so callers that need to map them onto
// specific RPC error codes (e.g. SessionService.CreateSession mapping
// ErrPathNotExist to connect.CodeNotFound) can do so with errors.Is rather
// than string-matching.
var (
	// ErrPathNotExist is returned when SessionType is Directory, the target
	// path does not exist, and CreateIfMissing was not set.
	ErrPathNotExist = errors.New("path does not exist")
	// ErrResumePathNotExist is ErrPathNotExist's variant for resume flows,
	// where the missing directory means the original project is gone rather
	// than "not created yet".
	ErrResumePathNotExist = errors.New("cannot resume: project directory no longer exists")
	// ErrInstanceConstructionFailed wraps a NewInstance failure (e.g. invalid
	// InstanceOptions).
	ErrInstanceConstructionFailed = errors.New("failed to create instance")
	// ErrInstanceRegistrationFailed wraps a Registry.Register failure.
	ErrInstanceRegistrationFailed = errors.New("failed to register instance")
	// ErrInstanceSaveFailed wraps a Storage.AddInstance failure.
	ErrInstanceSaveFailed = errors.New("failed to save instance")
)

// CreateManagedInstanceParams holds everything needed to construct and
// persist a managed Instance, independent of how the caller arrived at these
// values (an RPC request, an import commit, etc). Deliberately a plain
// struct with no connect.Request/connect.Response types in sight -- both
// SessionService.CreateSession's connect handler and
// CommitImportExternalSession (Story 1.2.1) build one of these and call
// CreateManagedInstance directly, so the dependency always points RPC layer
// -> domain function, never handler -> handler (see
// project_plans/import-external-session/implementation/plan.md Story
// 1.2.0a).
type CreateManagedInstanceParams struct {
	Options CreateInstanceOptions

	// Storage persists the constructed Instance (mirrors
	// SessionService.storage; typically a *session.Storage backed by
	// EntRepository). Required.
	Storage InstanceStore

	// Registry, if non-nil, registers the instance's live handle before
	// Storage.AddInstance runs, matching CreateSession's existing
	// register-before-persist ordering so there is never a window where the
	// session is findable via storage but has no live actor. Optional --
	// callers that don't use the live-handle registry (there are none today,
	// but the field is a plain pointer, not an interface, so this is not
	// speculative) may leave it nil.
	Registry *Registry

	// CreateIfMissing controls whether a Directory-mode session may be
	// created even though its target path does not yet exist on disk. This
	// mirrors CreateSessionRequest.CreateIfMissing but is surfaced separately
	// from CreateInstanceOptions since it drives a pre-flight check rather
	// than an InstanceOptions field.
	CreateIfMissing bool

	// ResumeID, when non-empty, is the conversation UUID this instance
	// should resume via `--resume`. Named ResumeID (not embedded in
	// InstanceOptions.ResumeId) to match the plan's
	// CreateManagedInstanceParams{SessionType, Path, ResumeID} example
	// verbatim; it is copied onto InstanceOptions.ResumeId internally.
	ResumeID string
}

// CreateInstanceOptions is a type alias for session.InstanceOptions, kept as
// a distinct name in this file so CreateManagedInstanceParams reads clearly
// at call sites (options for the instance to create) without forcing every
// caller to spell out the full InstanceOptions literal inline.
type CreateInstanceOptions = InstanceOptions

// CreateManagedInstance resolves path/session-type concerns, constructs a
// real *Instance via NewInstance, registers it in the live-handle registry
// (if provided), and persists it via Storage.AddInstance. It deliberately
// does NOT call instance.Start() -- starting tmux/the underlying process is
// left to the caller, which may want to do so asynchronously (as
// SessionService.CreateSession's handler does) or under additional
// preconditions (as CommitImportExternalSession does, e.g. after suspending
// the original process).
//
// On any failure after a successful Registry.Register, the registration is
// rolled back via Registry.ForceRelease before returning the error, so no
// phantom live-handle entry is left behind.
func CreateManagedInstance(ctx context.Context, params CreateManagedInstanceParams) (*Instance, error) {
	if params.Storage == nil {
		return nil, fmt.Errorf("create managed instance: Storage is required")
	}

	opts := params.Options
	opts.ResumeId = params.ResumeID

	// For Directory mode: if the path does not exist and CreateIfMissing is
	// not set, fail fast -- callers (e.g. the CreateSession RPC handler) use
	// this to show a "create it?" confirmation dialog instead of silently
	// creating a directory the user didn't ask for.
	if opts.SessionType == SessionTypeDirectory {
		if _, statErr := os.Stat(opts.Path); os.IsNotExist(statErr) {
			if !params.CreateIfMissing {
				if opts.ResumeId != "" {
					return nil, fmt.Errorf("%w: %s", ErrResumePathNotExist, opts.Path)
				}
				return nil, fmt.Errorf("%w: %s", ErrPathNotExist, opts.Path)
			}
			// CreateIfMissing=true: fall through; setupFirstTimeWorktree (invoked
			// during instance.Start()) handles creation.
		}
	}

	instance, err := NewInstance(opts)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInstanceConstructionFailed, err)
	}

	// Register in the live-handle map BEFORE persisting to storage so there is
	// never a window where the session is findable by
	// storage.FindInstanceDataByID but has no live actor. ForceRelease (not
	// the release closure) is used in the rollback path because a concurrent
	// Acquire racing between Register and AddInstance failure could bump
	// refcount to 2, making plain release() decrement 2->1 and leave a
	// phantom entry alive.
	if params.Registry != nil {
		live := NewLiveInstance(instance)
		if _, regErr := params.Registry.Register(live); regErr != nil {
			live.Stop()
			return nil, fmt.Errorf("%w: %w", ErrInstanceRegistrationFailed, regErr)
		}
	}

	if err := params.Storage.AddInstance(instance); err != nil {
		if params.Registry != nil {
			params.Registry.ForceRelease(instance.GetStableID())
		}
		return nil, fmt.Errorf("%w: %w", ErrInstanceSaveFailed, err)
	}

	return instance, nil
}
