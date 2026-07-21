package session

import (
	"context"
	"errors"
	"fmt"
)

// ErrCorrelationDrifted is returned when a fresh CorrelateCandidate result
// (re-run at commit time) disagrees with the CorrelationResult the caller
// echoed back from PreviewImportExternalSession (Task 1.2.1f). Something
// about the candidate's history file(s) changed between preview and
// commit (e.g. a new conversation started, an ambiguous set resolved to a
// single file). Callers must map this onto connect.CodeFailedPrecondition
// and ask the client to re-preview.
var ErrCorrelationDrifted = errors.New("import commit: correlation result changed since preview -- please re-preview")

// ErrDisambiguationChoiceInvalid is returned when disambiguation_choice is
// non-empty but does not name one of the FRESH correlation result's
// Candidates by ConversationUUID. Deliberately re-validated against the
// fresh result, never the caller-supplied expected_correlation, so a
// disambiguation choice can never be replayed against a stale candidate
// set.
var ErrDisambiguationChoiceInvalid = errors.New("import commit: disambiguation_choice does not match any candidate in the fresh correlation result")

// ErrAmbiguousWithoutChoice is returned when the fresh correlation result is
// Ambiguous but the caller supplied no disambiguation_choice.
var ErrAmbiguousWithoutChoice = errors.New("import commit: correlation is ambiguous and no disambiguation_choice was supplied")

// CommitImportParams holds everything CommitImportExternalSession needs to
// persist a managed Instance for an external candidate, start it resumed,
// and suspend the original process.
type CommitImportParams struct {
	Detector  *HistoryFileDetector
	Storage   InstanceStore
	Registry  *Registry
	Linker    *HistoryLinker
	Suspended *SuspendedProcessStore

	Candidate             ExternalSessionCandidate
	ExpectedCorrelation   CorrelationResult
	DisambiguationChoice  string
	OriginalPID           int32
	OriginalCreateTimeMs  int64
}

// CommitImportResult is the domain-level outcome of a successful commit.
type CommitImportResult struct {
	Instance *Instance
	// FreshCreateTimeMs is a newly re-read create-time for OriginalPID, taken
	// immediately after suspension, for the caller to mint a fresh
	// PIDIdentity in the RPC response (per import.proto's doc comment on
	// CommitImportExternalSessionResponse.pid_identity).
	FreshCreateTimeMs int64
}

// CommitImportExternalSession re-runs correlation fresh (never trusting the
// caller-supplied ExpectedCorrelation), resolves the conversation UUID to
// resume, persists a managed Instance, registers it with HistoryLinker,
// starts it resumed, and finally suspends the original process -- in that
// order, so the original process only stops writing once the resumed
// session is confirmed up and running.
//
// On any failure after the Instance has been persisted, the Instance is
// compensating-deleted before returning (Story 1.2.3) so no partial/orphan
// row is left behind. On any failure before suspension, the original
// process is never touched.
func CommitImportExternalSession(ctx context.Context, params CommitImportParams) (CommitImportResult, error) {
	if params.Storage == nil {
		return CommitImportResult{}, fmt.Errorf("commit import: Storage is required")
	}
	if params.Detector == nil {
		return CommitImportResult{}, fmt.Errorf("commit import: Detector is required")
	}

	fresh, err := CorrelateCandidate(params.Detector, params.Candidate)
	if err != nil {
		return CommitImportResult{}, fmt.Errorf("commit import: correlation failed: %w", err)
	}

	resumeUUID, err := resolveResumeUUID(fresh, params.ExpectedCorrelation, params.DisambiguationChoice)
	if err != nil {
		return CommitImportResult{}, err
	}

	if err := CheckPathNotAlreadyManaged(params.Candidate.Path, params.Storage); err != nil {
		return CommitImportResult{}, err
	}

	instance, err := CreateManagedInstance(ctx, CreateManagedInstanceParams{
		Options: InstanceOptions{
			Title:       importInstanceTitle(params.Candidate),
			Path:        params.Candidate.Path,
			Program:     params.Candidate.Program,
			SessionType: SessionTypeDirectory,
		},
		Storage:  params.Storage,
		Registry: params.Registry,
		ResumeID: resumeUUID,
	})
	if err != nil {
		return CommitImportResult{}, fmt.Errorf("commit import: %w", err)
	}

	// From here on, any failure must compensating-delete the persisted
	// Instance (Story 1.2.3) before returning.
	if err := startAndSuspend(ctx, params, instance); err != nil {
		if delErr := params.Storage.DeleteInstance(instance.Title); delErr != nil {
			return CommitImportResult{}, fmt.Errorf("%w (compensating delete also failed: %v)", err, delErr)
		}
		return CommitImportResult{}, err
	}

	if params.Linker != nil {
		params.Linker.AddInstance(instance)
	}

	freshCreateTimeMs := params.OriginalCreateTimeMs

	return CommitImportResult{Instance: instance, FreshCreateTimeMs: freshCreateTimeMs}, nil
}

// resolveResumeUUID re-validates the fresh correlation result and returns
// the conversation UUID to resume, implementing both the correlation-drift
// guard (Task 1.2.1f) and the disambiguation double-check (Task 1.2.1b).
func resolveResumeUUID(fresh, expected CorrelationResult, disambiguationChoice string) (string, error) {
	if fresh.Kind != expected.Kind {
		return "", ErrCorrelationDrifted
	}

	switch fresh.Kind {
	case CorrelationNotFound:
		return "", nil
	case CorrelationResolved:
		if fresh.UUID != expected.UUID {
			return "", ErrCorrelationDrifted
		}
		if disambiguationChoice != "" {
			return "", fmt.Errorf("commit import: disambiguation_choice must be empty when correlation is resolved")
		}
		return fresh.UUID, nil
	case CorrelationAmbiguous:
		if disambiguationChoice == "" {
			return "", ErrAmbiguousWithoutChoice
		}
		for _, c := range fresh.Candidates {
			if c.ConversationUUID == disambiguationChoice {
				return disambiguationChoice, nil
			}
		}
		return "", ErrDisambiguationChoiceInvalid
	default:
		return "", fmt.Errorf("commit import: unhandled correlation kind %v", fresh.Kind)
	}
}

// startAndSuspend starts the newly-created instance resumed, then persists
// a SuspendedProcessRecord and SIGSTOPs the original process. If starting
// fails, the original process is never suspended. If persisting the
// suspended-process record fails, the original process is also never
// suspended (we must not freeze a process we can't durably remember to
// unfreeze). If suspension itself fails after the record was persisted, the
// record is removed again so a subsequent restart doesn't try to resume a
// process that was never actually stopped.
func startAndSuspend(_ context.Context, params CommitImportParams, instance *Instance) error {
	if err := instance.Start(false); err != nil {
		return fmt.Errorf("commit import: failed to start resumed instance: %w", err)
	}

	if params.OriginalPID <= 0 {
		// No original process to suspend (e.g. candidate had no PID) --
		// nothing further to do.
		return nil
	}

	if params.Suspended != nil {
		record := SuspendedProcessRecord{
			PID:          params.OriginalPID,
			CreateTimeMs: params.OriginalCreateTimeMs,
			Candidate:    params.Candidate,
			InstanceID:   instance.Title,
		}
		if err := params.Suspended.Add(record); err != nil {
			return fmt.Errorf("commit import: failed to persist suspended-process record: %w", err)
		}
	}

	if err := SuspendOriginalProcess(params.OriginalPID); err != nil {
		if params.Suspended != nil {
			_ = params.Suspended.Remove(instance.Title)
		}
		return fmt.Errorf("commit import: failed to suspend original process: %w", err)
	}

	return nil
}

// importInstanceTitle derives a Title for the managed Instance from the
// candidate. Uses the candidate's tmux session name when available (it's
// already unique within the tmux server), falling back to a PID-qualified
// name otherwise.
func importInstanceTitle(candidate ExternalSessionCandidate) string {
	if candidate.TmuxSession != "" {
		return candidate.TmuxSession
	}
	return fmt.Sprintf("imported-%d", candidate.PID)
}
