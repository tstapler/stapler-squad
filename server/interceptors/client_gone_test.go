package interceptors

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"testing"

	"connectrpc.com/connect"
)

func TestAsCanceled(t *testing.T) {
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	live := context.Background()
	write := fmt.Errorf("write envelope: write tcp [::1]:8543->[::1]:1: %w", syscall.EPIPE)
	backend := fmt.Errorf("dial tymuxd: %w", syscall.ECONNRESET)
	unknownWrap := connect.NewError(connect.CodeUnknown, write)
	internalCanceled := connect.NewError(connect.CodeInternal, context.Canceled)

	for name, tc := range map[string]struct {
		ctx  context.Context
		err  error
		want connect.Code
	}{
		"connect write broken pipe, live ctx":  {live, write, connect.CodeCanceled},
		"connect Unknown wrapping write reset": {live, unknownWrap, connect.CodeCanceled},
		"ctx canceled, dead ctx":               {dead, context.Canceled, connect.CodeCanceled},
		"backend reset, live ctx stays real":   {live, backend, connect.CodeUnknown},
		"server-side canceled, live ctx":       {live, context.Canceled, connect.CodeUnknown},
		"internal wrapping canceled kept":      {dead, internalCanceled, connect.CodeInternal},
		"plain error":                          {live, errors.New("boom"), connect.CodeUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			if got := connect.CodeOf(asCanceled(tc.ctx, tc.err)); got != tc.want {
				t.Fatalf("code = %v, want %v", got, tc.want)
			}
		})
	}
	if asCanceled(live, nil) != nil {
		t.Fatal("nil must stay nil")
	}
}

type recordingRegistry struct{ calls int }

func (r *recordingRegistry) Record(ctx context.Context, _ error, _ string) {
	if ctx.Err() == nil {
		r.calls++
	}
}

func TestErrorRecorder_SkipsClientCancelAndDetachesCtx(t *testing.T) {
	reg := &recordingRegistry{}
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	run := func(err error) {
		fn := NewErrorRecorderInterceptor(reg)(func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) { return nil, err })
		_, _ = fn(dead, connect.NewRequest(&struct{}{}))
	}
	run(connect.NewError(connect.CodeCanceled, context.Canceled))
	if reg.calls != 0 {
		t.Fatalf("client cancel must not be recorded, got %d", reg.calls)
	}
	run(connect.NewError(connect.CodeInternal, errors.New("boom")))
	if reg.calls != 1 {
		t.Fatalf("real error must be recorded with a live (detached) ctx, got %d", reg.calls)
	}
}
