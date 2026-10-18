package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
)

func TestFanOutSession_RejectsCountOutsideCap(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	for _, count := range []int32{-1, 0, 1, 6, 100} {
		_, err := svc.FanOutSession(context.Background(), connect.NewRequest(&sessionv1.FanOutSessionRequest{
			Base:  &sessionv1.CreateSessionRequest{Title: "t", Path: "/tmp"},
			Count: count,
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("count=%d: want InvalidArgument, got %v", count, err)
		}
	}
}

func TestFanOutSession_RejectsMissingTitleAndExcessOverrides(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	_, err := svc.FanOutSession(context.Background(), connect.NewRequest(&sessionv1.FanOutSessionRequest{
		Base: &sessionv1.CreateSessionRequest{}, Count: 2,
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("missing title: want InvalidArgument, got %v", err)
	}
	_, err = svc.FanOutSession(context.Background(), connect.NewRequest(&sessionv1.FanOutSessionRequest{
		Base: &sessionv1.CreateSessionRequest{Title: "t"}, Count: 2, ProgramOverrides: []string{"a", "b", "c"},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("too many overrides: want InvalidArgument, got %v", err)
	}
}

func TestFanoutWinnerEligibility(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status session.Status
		claude detection.DetectedStatus
		wantOK bool
	}{
		{"stopped", session.Stopped, detection.StatusUnknown, true},
		{"paused", session.Paused, detection.StatusUnknown, true},
		{"active idle", session.Active, detection.StatusIdle, true},
		{"active ready", session.Active, detection.StatusReady, true},
		{"active still working", session.Active, detection.StatusProcessing, false},
		{"creating", session.Creating, detection.StatusUnknown, false},
	}
	for _, c := range cases {
		err := fanoutWinnerEligibility(c.status, c.claude)
		if (err == nil) != c.wantOK {
			t.Errorf("%s: eligible=%v, err=%v", c.name, err == nil, err)
		}
	}
}

func TestPickFanoutWinner_UnknownGroupIsNotFound(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	_, err := svc.PickFanoutWinner(context.Background(), connect.NewRequest(&sessionv1.PickFanoutWinnerRequest{
		GroupId: "nope", WinnerSessionId: "x",
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("want NotFound, got %v", err)
	}
}
