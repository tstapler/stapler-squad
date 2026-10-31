package interceptors

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestMethodName_should_StripServicePrefix(t *testing.T) {
	assert.Equal(t, "ListShells", methodName("/session.v1.SessionService/ListShells"))
	assert.Equal(t, "bare", methodName("bare"))
}

func TestDBQueryCountInterceptor_should_PassThroughAndPropagateErrors(t *testing.T) {
	want := connect.NewError(connect.CodeInternal, assert.AnError)
	called := false
	next := NewDBQueryCountInterceptor().WrapUnary(func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		called = true
		return nil, want
	})
	_, err := next(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.ErrorIs(t, err, want)
	assert.True(t, called)
}
