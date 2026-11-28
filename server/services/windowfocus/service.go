// Package windowfocus serves the FocusWindow RPC: it raises an already-running developer app.
package windowfocus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services/notify"
)

// Service serves the FocusWindow RPC.
type Service struct {
	activator WindowActivator
}

// NewService creates a service that activates windows through Launch Services.
func NewService() *Service {
	return &Service{activator: newLaunchServicesActivator()}
}

// FocusWindow activates a window for the specified application.
// Uses Launch Services (lsappinfo + open) on macOS, which raises no per-app
// Automation (Apple Events) consent dialog.
func (s *Service) FocusWindow(
	ctx context.Context,
	req *connect.Request[sessionv1.FocusWindowRequest],
) (*connect.Response[sessionv1.FocusWindowResponse], error) {
	// Validate localhost-only origin
	if err := validateLocalhostOriginForFocus(ctx, req); err != nil {
		return nil, err
	}

	platform := detectPlatform()

	// Need at least bundle_id or app_name
	bundleID := ""
	if req.Msg.BundleId != nil {
		bundleID = *req.Msg.BundleId
	}
	appName := ""
	if req.Msg.AppName != nil {
		appName = *req.Msg.AppName
	}

	if bundleID == "" && appName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("bundle_id or app_name is required"))
	}

	// Only macOS is supported currently
	if platform != "darwin" {
		return connect.NewResponse(&sessionv1.FocusWindowResponse{
			Success:  false,
			Message:  fmt.Sprintf("window activation not supported on platform: %s", platform),
			Platform: platform,
		}), nil
	}

	if isBlockedTarget(bundleID, appName) {
		blockedTarget := bundleID
		if blockedTarget == "" {
			blockedTarget = appName
		}
		log.Info("FocusWindow skipped for non-developer app", "bundle", bundleID, "app", appName)
		return connect.NewResponse(&sessionv1.FocusWindowResponse{
			Success:  false,
			Message:  fmt.Sprintf("Focus Window is not supported for %q (not a developer app)", blockedTarget),
			Platform: platform,
		}), nil
	}

	if err := s.activator.Activate(ctx, bundleID, appName); err != nil {
		log.Warn("failed to activate window", "bundle", bundleID, "app", appName, "err", err)

		message := fmt.Sprintf("failed to activate window: %v", err)
		if errors.Is(err, ErrAppNotRunning) {
			targetApp := bundleID
			if targetApp == "" {
				targetApp = appName
			}
			message = fmt.Sprintf("Application '%s' is not running", targetApp)
		}

		return connect.NewResponse(&sessionv1.FocusWindowResponse{
			Success:  false,
			Message:  message,
			Platform: platform,
		}), nil
	}

	log.Info("window activated successfully", "bundle", bundleID, "app", appName)
	return connect.NewResponse(&sessionv1.FocusWindowResponse{
		Success:  true,
		Message:  "Window activated successfully",
		Platform: platform,
	}), nil
}

// validateLocalhostOriginForFocus ensures FocusWindow requests come from localhost.
func validateLocalhostOriginForFocus(ctx context.Context, req *connect.Request[sessionv1.FocusWindowRequest]) error {
	// Check X-Real-IP header first (if behind a proxy)
	realIP := req.Header().Get("X-Real-IP")
	if realIP != "" {
		if !notify.IsLocalhostIP(realIP) {
			return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("FocusWindow can only be called from localhost"))
		}
		return nil
	}

	// Check X-Forwarded-For header
	forwardedFor := req.Header().Get("X-Forwarded-For")
	if forwardedFor != "" {
		ips := strings.Split(forwardedFor, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if !notify.IsLocalhostIP(clientIP) {
				return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("FocusWindow can only be called from localhost"))
			}
			return nil
		}
	}

	// Direct connection mode - server binds to localhost
	return nil
}

// detectPlatform returns the current operating system.
func detectPlatform() string {
	switch osVal := os.Getenv("GOOS"); osVal {
	case "":
		// GOOS not set, use runtime detection
		return runtime.GOOS
	default:
		return osVal
	}
}
