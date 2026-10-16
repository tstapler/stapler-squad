// Package hookipc defines Stapler Squad's transport-neutral hook classification
// protocol and its local Unix-socket adapter.
package hookipc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/tstapler/stapler-squad/pkg/classifier"
)

type ProtocolVersion uint16

const CurrentProtocolVersion ProtocolVersion = 1

type InstanceFingerprint string
type HookRequestID string
type ToolUseID string
type RuleSnapshotVersion string

type DecisionSource string

const (
	DecisionSourcePrimary DecisionSource = "primary"
	DecisionSourceCache   DecisionSource = "cache"
	DecisionSourceDefer   DecisionSource = "defer"
)

var (
	ErrProtocolMismatch = errors.New("hookipc: protocol mismatch")
	ErrInstanceMismatch = errors.New("hookipc: instance mismatch")
	ErrInvalidEnvelope  = errors.New("hookipc: invalid envelope")
)

// InvocationContext contains only context supplied by the invoking hook. Env
// must contain referenced variables only, never a copy of the whole process
// environment.
type InvocationContext struct {
	Cwd string            `json:"cwd,omitempty"`
	Env map[string]string `json:"env,omitempty"`
}

// ClassificationEnvelope is the transport-neutral request parsed at the server
// boundary before any policy code runs.
type ClassificationEnvelope struct {
	ProtocolVersion     ProtocolVersion                     `json:"protocol_version"`
	InstanceFingerprint InstanceFingerprint                 `json:"instance_fingerprint"`
	RequestID           HookRequestID                       `json:"request_id"`
	Payload             classifier.PermissionRequestPayload `json:"payload"`
	Context             InvocationContext                   `json:"context,omitempty"`
}

// HookSpecificOutput is Claude Code's PreToolUse response body. A nil output in
// ClassificationReply represents defer and is emitted as empty stdout by the
// CLI adapter.
type HookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
}

type HookOutput struct {
	HookSpecificOutput HookSpecificOutput `json:"hookSpecificOutput"`
}

type ClassificationReply struct {
	ProtocolVersion     ProtocolVersion     `json:"protocol_version"`
	InstanceFingerprint InstanceFingerprint `json:"instance_fingerprint"`
	RequestID           HookRequestID       `json:"request_id"`
	RuleSnapshotVersion RuleSnapshotVersion `json:"rule_snapshot_version,omitempty"`
	Source              DecisionSource      `json:"source"`
	Output              *HookOutput         `json:"output,omitempty"`
}

func Fingerprint(configDir string) InstanceFingerprint {
	digest := sha256.Sum256([]byte(configDir))
	return InstanceFingerprint(hex.EncodeToString(digest[:]))
}

// NewRequestID returns a stable ID when Claude supplies toolUseID. Older
// payloads receive a cryptographically random request ID and stable=false, so
// they can never be deduplicated by payload similarity.
func NewRequestID(sessionID string, toolUseID ToolUseID) (id HookRequestID, stable bool, err error) {
	if toolUseID != "" {
		digest := sha256.Sum256([]byte(sessionID + "\x00" + string(toolUseID)))
		return HookRequestID(hex.EncodeToString(digest[:])), true, nil
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", false, fmt.Errorf("hookipc: generate request id: %w", err)
	}
	return HookRequestID(hex.EncodeToString(random[:])), false, nil
}

func (e ClassificationEnvelope) Validate(expected HookEndpoint) error {
	if e.ProtocolVersion != expected.ProtocolVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrProtocolMismatch, e.ProtocolVersion, expected.ProtocolVersion)
	}
	if e.InstanceFingerprint != expected.InstanceFingerprint {
		return fmt.Errorf("%w: request does not belong to this instance", ErrInstanceMismatch)
	}
	if e.RequestID == "" || e.Payload.HookEventName == "" || e.Payload.ToolName == "" {
		return ErrInvalidEnvelope
	}
	return nil
}

func (r ClassificationReply) Validate(expected HookEndpoint, requestID HookRequestID) error {
	if r.ProtocolVersion != expected.ProtocolVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrProtocolMismatch, r.ProtocolVersion, expected.ProtocolVersion)
	}
	if r.InstanceFingerprint != expected.InstanceFingerprint {
		return fmt.Errorf("%w: response does not belong to intended instance", ErrInstanceMismatch)
	}
	if r.RequestID != requestID {
		return fmt.Errorf("%w: response request id mismatch", ErrInvalidEnvelope)
	}
	switch r.Source {
	case DecisionSourcePrimary, DecisionSourceCache, DecisionSourceDefer:
		return nil
	default:
		return fmt.Errorf("%w: unknown decision source", ErrInvalidEnvelope)
	}
}
