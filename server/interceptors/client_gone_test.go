package interceptors

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"

	"connectrpc.com/connect"
)

func TestAsCanceled(t *testing.T) {
	broken := &net.OpError{Op: "write", Err: os.NewSyscallError("write", syscall.EPIPE)}
	for name, tc := range map[string]struct {
		err  error
		want connect.Code
		conv bool
	}{
		"broken pipe":   {fmt.Errorf("write envelope: %w", broken), connect.CodeCanceled, true},
		"conn reset":    {fmt.Errorf("x: %w", syscall.ECONNRESET), connect.CodeCanceled, true},
		"ctx canceled":  {context.Canceled, connect.CodeCanceled, true},
		"real internal": {connect.NewError(connect.CodeInternal, errors.New("boom")), connect.CodeInternal, false},
		"plain error":   {errors.New("boom"), connect.CodeUnknown, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := asCanceled(tc.err)
			if tc.conv && connect.CodeOf(got) != tc.want {
				t.Fatalf("code = %v, want %v", connect.CodeOf(got), tc.want)
			}
			if !tc.conv && connect.CodeOf(got) != tc.want {
				t.Fatalf("code = %v, want %v", connect.CodeOf(got), tc.want)
			}
		})
	}
	if asCanceled(nil) != nil {
		t.Fatal("nil must stay nil")
	}
}
