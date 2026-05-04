package services

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// TestGetSessionDefaults_ReturnsDefaults verifies that GetSessionDefaults returns a
// non-nil defaults payload without error on a fresh (empty) configuration.
func TestGetSessionDefaults_ReturnsDefaults(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.GetSessionDefaultsRequest{})
	resp, err := svc.GetSessionDefaults(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	assert.NotNil(t, resp.Msg.Defaults)
}

// TestResolveDefaults_NoPath verifies that ResolveDefaults with an empty working
// directory succeeds and returns a response (falls back to global defaults).
func TestResolveDefaults_NoPath(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.ResolveDefaultsRequest{
		WorkingDir:  "",
		ProfileName: "",
	})
	resp, err := svc.ResolveDefaults(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	// EnvVars must never be nil so the proto response is valid.
	assert.NotNil(t, resp.Msg.EnvVars)
}

// TestUpdateGlobalDefaults_UpdatesProgram verifies that calling UpdateGlobalDefaults
// with a program name persists it and returns the updated defaults.
func TestUpdateGlobalDefaults_UpdatesProgram(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.UpdateGlobalDefaultsRequest{
		Program: "aider",
	})
	resp, err := svc.UpdateGlobalDefaults(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Defaults)
	assert.Equal(t, "aider", resp.Msg.Defaults.Program)
}

// TestUpsertProfile_EmptyName verifies that UpsertProfile with an empty profile name
// returns CodeInvalidArgument.
func TestUpsertProfile_EmptyName(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.UpsertProfileRequest{
		Profile: &sessionv1.ProfileDefaultsProto{
			Name:    "",
			Program: "claude",
		},
	})
	_, err := svc.UpsertProfile(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertProfile_NilProfile verifies that UpsertProfile with a nil profile
// returns CodeInvalidArgument.
func TestUpsertProfile_NilProfile(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.UpsertProfileRequest{
		Profile: nil,
	})
	_, err := svc.UpsertProfile(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertProfile_CreatesProfile verifies that a valid name + program succeeds and
// the response echoes back the created profile.
func TestUpsertProfile_CreatesProfile(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.UpsertProfileRequest{
		Profile: &sessionv1.ProfileDefaultsProto{
			Name:    "work",
			Program: "claude",
		},
	})
	resp, err := svc.UpsertProfile(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Profile)
	assert.Equal(t, "work", resp.Msg.Profile.Name)
	assert.Equal(t, "claude", resp.Msg.Profile.Program)
}

// TestDeleteProfile_NotFound verifies that deleting a non-existent profile returns
// CodeNotFound.
func TestDeleteProfile_NotFound(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.DeleteProfileRequest{
		Name: "no-such-profile",
	})
	_, err := svc.DeleteProfile(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestDeleteProfile_EmptyName verifies that deleting with an empty name returns
// CodeInvalidArgument.
func TestDeleteProfile_EmptyName(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.DeleteProfileRequest{
		Name: "",
	})
	_, err := svc.DeleteProfile(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestDeleteProfile_Success verifies that upserting a profile and then deleting it
// succeeds.
func TestDeleteProfile_Success(t *testing.T) {
	svc := NewDefaultsService()

	// First create the profile.
	upsertReq := connect.NewRequest(&sessionv1.UpsertProfileRequest{
		Profile: &sessionv1.ProfileDefaultsProto{
			Name:    "temp-profile",
			Program: "aider",
		},
	})
	_, err := svc.UpsertProfile(context.Background(), upsertReq)
	require.NoError(t, err)

	// Now delete it.
	deleteReq := connect.NewRequest(&sessionv1.DeleteProfileRequest{
		Name: "temp-profile",
	})
	_, err = svc.DeleteProfile(context.Background(), deleteReq)
	require.NoError(t, err)
}

// TestUpsertDirectoryRule_EmptyPath verifies that UpsertDirectoryRule with an empty
// path returns CodeInvalidArgument.
func TestUpsertDirectoryRule_EmptyPath(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.UpsertDirectoryRuleRequest{
		Rule: &sessionv1.DirectoryRuleProto{
			Path: "",
		},
	})
	_, err := svc.UpsertDirectoryRule(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertDirectoryRule_NilRule verifies that UpsertDirectoryRule with a nil rule
// returns CodeInvalidArgument.
func TestUpsertDirectoryRule_NilRule(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.UpsertDirectoryRuleRequest{
		Rule: nil,
	})
	_, err := svc.UpsertDirectoryRule(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpsertDirectoryRule_ValidPath verifies that a rule with a valid path is created
// successfully and the response echoes back the path.
func TestUpsertDirectoryRule_ValidPath(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.UpsertDirectoryRuleRequest{
		Rule: &sessionv1.DirectoryRuleProto{
			Path:    "/home/user/projects",
			Profile: "work",
		},
	})
	resp, err := svc.UpsertDirectoryRule(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	require.NotNil(t, resp.Msg.Rule)
	assert.Equal(t, "/home/user/projects", resp.Msg.Rule.Path)
	assert.Equal(t, "work", resp.Msg.Rule.Profile)
}

// TestDeleteDirectoryRule_NotFound verifies that deleting a non-existent directory
// rule returns CodeNotFound.
func TestDeleteDirectoryRule_NotFound(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.DeleteDirectoryRuleRequest{
		Path: "/nonexistent/path",
	})
	_, err := svc.DeleteDirectoryRule(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeNotFound, connectErr.Code())
}

// TestDeleteDirectoryRule_EmptyPath verifies that deleting with an empty path returns
// CodeInvalidArgument.
func TestDeleteDirectoryRule_EmptyPath(t *testing.T) {
	svc := NewDefaultsService()

	req := connect.NewRequest(&sessionv1.DeleteDirectoryRuleRequest{
		Path: "",
	})
	_, err := svc.DeleteDirectoryRule(context.Background(), req)

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestDeleteDirectoryRule_Success verifies that upserting a directory rule and then
// deleting it succeeds.
func TestDeleteDirectoryRule_Success(t *testing.T) {
	svc := NewDefaultsService()

	// Create the rule first.
	upsertReq := connect.NewRequest(&sessionv1.UpsertDirectoryRuleRequest{
		Rule: &sessionv1.DirectoryRuleProto{
			Path:    "/tmp/test-project",
			Profile: "default",
		},
	})
	_, err := svc.UpsertDirectoryRule(context.Background(), upsertReq)
	require.NoError(t, err)

	// Delete it.
	deleteReq := connect.NewRequest(&sessionv1.DeleteDirectoryRuleRequest{
		Path: "/tmp/test-project",
	})
	_, err = svc.DeleteDirectoryRule(context.Background(), deleteReq)
	require.NoError(t, err)
}
