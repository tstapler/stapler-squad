package prs

import (
	"errors"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// PRRefFromProto parses a request's PR selector; its errors are Connect errors.
func PRRefFromProto(p *sessionv1.PullRequestRef) (domain.PRRef, error) {
	if p == nil || p.GetNumber() <= 0 {
		return domain.PRRef{}, connect.NewError(connect.CodeInvalidArgument, errors.New("a pull request is required"))
	}
	ref, err := domain.NewPRRef(p.GetHost(), p.GetOwner(), p.GetRepo(), int(p.GetNumber()))
	if err != nil {
		return domain.PRRef{}, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ref, nil
}

// ListPRFilter turns an RPC's optional pr selector into a ref. Unset is "no filter"; set
// but invalid is InvalidArgument, never an unfiltered result.
func ListPRFilter(p *sessionv1.PullRequestRef) (ref domain.PRRef, set bool, err error) {
	if p == nil {
		return domain.PRRef{}, false, nil
	}
	ref, err = PRRefFromProto(p)
	return ref, err == nil, err
}

// ResolveTargetPR is the one place a PR action's selector becomes a PR. An unset
// selector means the session's active PR; a set one must belong to the session.
// The returned ref is never the zero ref: a session with no PR gets
// FailedPrecondition "no pull request", never a call for number 0.
func ResolveTargetPR(inst *instance.Instance, sel *sessionv1.PullRequestRef) (*domain.PRRef, error) {
	var requested *domain.PRRef
	if sel != nil {
		ref, err := PRRefFromProto(sel)
		if err != nil {
			return nil, err
		}
		requested = &ref
	}
	resolved, err := inst.ResolveTargetPR(requested)
	switch {
	case errors.Is(err, instance.ErrNoPullRequest):
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("no pull request"))
	case errors.Is(err, instance.ErrPRNotAttached):
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	case err != nil:
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return &resolved, nil
}
