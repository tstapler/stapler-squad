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

// TestProcedureMethodName_ExtractsBareMethod_When_GivenFullyQualifiedProcedure
// covers the helper NewScopedFeatureFlagInterceptor uses to match gated
// method names against connect.Spec().Procedure ("/pkg.Service/Method").
// Full end-to-end gating behavior (which requires a real *connect.Request
// with a populated Spec -- not constructible outside the connect package) is
// covered by the ImportService-level tests in
// server/services/import_service_test.go
// (TestImportService_ThreeMutatingRPCs_ReturnUnimplemented_When_FeatureFlagUnset
// and friends), which spin up a real handler+client pair over httptest.
func TestProcedureMethodName_ExtractsBareMethod_When_GivenFullyQualifiedProcedure(t *testing.T) {
	cases := map[string]string{
		"/session.v1.ImportService/CommitImportExternalSession":  "CommitImportExternalSession",
		"/session.v1.ImportService/PreviewImportExternalSession": "PreviewImportExternalSession",
		"NoSlashes": "NoSlashes",
		"":          "",
	}
	for procedure, want := range cases {
		if got := procedureMethodName(procedure); got != want {
			t.Errorf("procedureMethodName(%q) = %q, want %q", procedure, got, want)
		}
	}
}
