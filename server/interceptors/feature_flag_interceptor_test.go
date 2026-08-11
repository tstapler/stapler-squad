package interceptors

import (
	"context"
	"testing"

	"connectrpc.com/connect"
)

func alwaysNext(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
	return connect.NewResponse(&struct{}{}), nil
}

func TestNewFeatureFlagInterceptor_FlagEnabled(t *testing.T) {
	interceptor := NewFeatureFlagInterceptor("test-flag", func() bool { return true })
	handler := interceptor(alwaysNext)
	_, err := handler(context.Background(), connect.NewRequest(&struct{}{}))
	if err != nil {
		t.Fatalf("expected nil error when flag is enabled, got: %v", err)
	}
}

func TestNewFeatureFlagInterceptor_FlagDisabled(t *testing.T) {
	interceptor := NewFeatureFlagInterceptor("test-flag", func() bool { return false })
	handler := interceptor(alwaysNext)
	_, err := handler(context.Background(), connect.NewRequest(&struct{}{}))
	if err == nil {
		t.Fatal("expected error when flag is disabled, got nil")
	}
	connectErr, ok := err.(*connect.Error)
	if !ok {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound (%d), got %v", connect.CodeNotFound, connectErr.Code())
	}
}
