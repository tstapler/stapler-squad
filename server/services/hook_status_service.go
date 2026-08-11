package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/internal/claudehooks"
	"github.com/tstapler/stapler-squad/log"
)

// resolveSsqHooksBin returns the path to the ssq-hooks binary and whether it exists.
// Preference: ~/.local/bin/ssq-hooks (where `make install` puts it), then $PATH.
func resolveSsqHooksBin() (string, bool) {
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".local", "bin", "ssq-hooks")
		if isExecutableFile(p) {
			return p, true
		}
	}
	if p, err := exec.LookPath("ssq-hooks"); err == nil {
		return p, true
	}
	return "", false
}

// resolveSsqHookHandler returns the path to the ssq-hook-handler script and whether
// it exists. Preference: ~/.local/bin, then $PATH, then a scripts/ dir next to the
// running server binary (repo/dev layout).
func resolveSsqHookHandler() (string, bool) {
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".local", "bin", "ssq-hook-handler")
		if isExecutableFile(p) {
			return p, true
		}
	}
	if p, err := exec.LookPath("ssq-hook-handler"); err == nil {
		return p, true
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "scripts", "ssq-hook-handler")
		if isExecutableFile(p) {
			return p, true
		}
	}
	return "", false
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// hookStatus builds a GetHookStatusResponse from the global settings file and the
// availability of the underlying binaries.
func hookStatus() (*sessionv1.GetHookStatusResponse, error) {
	settingsPath, err := claudehooks.DefaultGlobalSettingsPath()
	if err != nil {
		return nil, err
	}
	st, err := claudehooks.DetectStatus(settingsPath)
	if err != nil {
		return nil, err
	}
	_, rulesAvail := resolveSsqHooksBin()
	_, notifyAvail := resolveSsqHookHandler()
	return &sessionv1.GetHookStatusResponse{
		RulesInstalled:         st.RulesInstalled,
		NotificationsInstalled: st.NotificationsInstalled,
		RulesAvailable:         rulesAvail,
		NotificationsAvailable: notifyAvail,
	}, nil
}

// +api: hooks:status
// GetHookStatus reports whether the global Claude Code hooks are installed and
// whether the binaries needed to install them are available.
func (s *SessionService) GetHookStatus(
	_ context.Context,
	_ *connect.Request[sessionv1.GetHookStatusRequest],
) (*connect.Response[sessionv1.GetHookStatusResponse], error) {
	status, err := hookStatus()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(status), nil
}

// +api: hooks:install
// InstallHooks installs the requested global hooks into ~/.claude/settings.json.
// A requested hook whose binary is unavailable is reported in messages with a
// manual fallback rather than failing the whole call.
func (s *SessionService) InstallHooks(
	_ context.Context,
	req *connect.Request[sessionv1.InstallHooksRequest],
) (*connect.Response[sessionv1.InstallHooksResponse], error) {
	settingsPath, err := claudehooks.DefaultGlobalSettingsPath()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var messages []string

	if req.Msg.InstallRules {
		if bin, ok := resolveSsqHooksBin(); ok {
			if err := claudehooks.InstallRules(settingsPath, bin); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("install rules hook: %w", err))
			}
			log.Info("[InstallHooks] installed rules hook", "bin", bin)
			messages = append(messages, "Rule enforcement hook installed.")
		} else {
			messages = append(messages, "ssq-hooks binary not found — run `make install` first, then `ssq-hooks install claude`.")
		}
	}

	if req.Msg.InstallNotifications {
		if handler, ok := resolveSsqHookHandler(); ok {
			if err := claudehooks.InstallNotifications(settingsPath, handler); err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("install notification hooks: %w", err))
			}
			log.Info("[InstallHooks] installed notification hooks", "handler", handler)
			messages = append(messages, "Notification hooks installed.")
		} else {
			messages = append(messages, "ssq-hook-handler not found — run `./scripts/ssq-hooks-install` from the repo.")
		}
	}

	status, err := hookStatus()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&sessionv1.InstallHooksResponse{
		Status:   status,
		Messages: messages,
	}), nil
}
