package session

import "github.com/tstapler/stapler-squad/session/instance"

// sessionDriverControl is the production DriverControl. It lives outside
// session_driver.go so the forwarding RestartForRetry is not an unguarded revival
// site for noarchivedrevival: the automated callers stay in session_driver.go with
// their IsArchived checks, and the only other caller is RetryNow, an explicit
// user action.
type sessionDriverControl struct{}

func (sessionDriverControl) Start(inst *instance.Instance, allowedPath string) {
	StartSessionDriver(inst, allowedPath)
}

func (sessionDriverControl) RestartForRetry(inst *instance.Instance, allowedPath, continuationPrompt string, policy instance.RetryPolicy, stop <-chan struct{}) error {
	return restartForRetry(inst, allowedPath, continuationPrompt, policy, stop)
}

func init() { instance.DriverControlHook = sessionDriverControl{} }
