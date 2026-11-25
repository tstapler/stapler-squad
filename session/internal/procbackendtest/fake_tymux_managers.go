package procbackendtest

import (
	"time"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/launchcmd"
	"github.com/tstapler/stapler-squad/session/tymux"
)

// FakeTymuxManagerForStartRestore is a minimal tymux.TymuxManager fake for
// Story 2.1.3's Start/RestoreWithWorkDir tests: it embeds the interface (so
// any unused method panics if called — none should be for these tests) and
// overrides only Start/RestoreWithWorkDir, with optional hooks so a test can
// observe call order relative to the injected ensureDaemonRunningFn.
type FakeTymuxManagerForStartRestore struct {
	tymux.TymuxManager // deliberately nil; unused methods must not be called
	StartErr           error
	RestoreErr         error
	OnStart            func(dir string)
	OnRestore          func(workDir string)
}

func (f *FakeTymuxManagerForStartRestore) Start(dir string) error {
	if f.OnStart != nil {
		f.OnStart(dir)
	}
	return f.StartErr
}

func (f *FakeTymuxManagerForStartRestore) RestoreWithWorkDir(w string) error {
	if f.OnRestore != nil {
		f.OnRestore(w)
	}
	return f.RestoreErr
}

// FakeTymuxManagerForBackendRestarted is a minimal tymux.TymuxManager fake
// used only to prove BackendRestarted() is reachable through the interface
// (Phase 5 spec-compliance sweep, Story 2.5.3 gap): it embeds the interface
// (so any unused method panics if called — none should be for this test)
// and overrides only BackendRestarted with a canned value.
type FakeTymuxManagerForBackendRestarted struct {
	tymux.TymuxManager // deliberately nil; unused methods must not be called
	Restarted          bool
	Since              time.Time
}

func (f *FakeTymuxManagerForBackendRestarted) BackendRestarted() (bool, time.Time) {
	return f.Restarted, f.Since
}

// FakeTymuxManagerForSetCommand is a minimal tymux.TymuxManager fake that
// records SetCommand's argument and otherwise no-ops Start/HasSession, used
// both to test newTymuxBackendFromOpts's wiring and Restart's in-place
// tymux-respawn SetCommand branch.
type FakeTymuxManagerForSetCommand struct {
	tymux.TymuxManager        // deliberately nil; unused methods must not be called
	GotCommand         string // gotLaunch rendered as shell text
	GotLaunch          launchcmd.LaunchCommand
	StartErr           error
	GotEnv             []string
	GotLabel           string
	GotStoredID        string
	SessionID          string
	StartCalls         int
	RestoreCalls       int
}

func (f *FakeTymuxManagerForSetCommand) SetStoredSessionID(id string) { f.GotStoredID = id }

func (f *FakeTymuxManagerForSetCommand) GetSessionIdentifier() string { return f.SessionID }

func (f *FakeTymuxManagerForSetCommand) AppendEnv(env ...string) {
	f.GotEnv = append(f.GotEnv, env...)
}

func (f *FakeTymuxManagerForSetCommand) SetCommand(command launchcmd.LaunchCommand) {
	f.GotLaunch = command
	f.GotCommand = instance.ShellString(command)
}

func (f *FakeTymuxManagerForSetCommand) SetSessionLabel(label string) {
	f.GotLabel = label
}

func (f *FakeTymuxManagerForSetCommand) PersistedSessionID() string { return f.SessionID }

func (f *FakeTymuxManagerForSetCommand) RestoreWithWorkDir(string) error {
	f.RestoreCalls++
	return nil
}

func (f *FakeTymuxManagerForSetCommand) Start(string) error {
	f.StartCalls++
	return f.StartErr
}

func (f *FakeTymuxManagerForSetCommand) HasSession() bool {
	return false
}
