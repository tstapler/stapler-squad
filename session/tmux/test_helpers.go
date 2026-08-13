package tmux

import (
	"os/exec"
)

// MockCmdExec provides mock functionality for executor.Executor interface
type MockCmdExec struct {
	RunFunc            func(cmd *exec.Cmd) error
	OutputFunc         func(cmd *exec.Cmd) ([]byte, error)
	CombinedOutputFunc func(cmd *exec.Cmd) ([]byte, error)
}

func (m MockCmdExec) Run(cmd *exec.Cmd) error {
	if m.RunFunc != nil {
		return m.RunFunc(cmd)
	}
	return nil
}

func (m MockCmdExec) Output(cmd *exec.Cmd) ([]byte, error) {
	if m.OutputFunc != nil {
		return m.OutputFunc(cmd)
	}
	return []byte(""), nil
}

func (m MockCmdExec) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	if m.CombinedOutputFunc != nil {
		return m.CombinedOutputFunc(cmd)
	}
	// Fall back to OutputFunc if CombinedOutputFunc is not set
	if m.OutputFunc != nil {
		return m.OutputFunc(cmd)
	}
	return []byte(""), nil
}
