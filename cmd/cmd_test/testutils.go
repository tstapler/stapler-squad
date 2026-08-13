package cmd_test

import (
	"os/exec"
)

type MockCmdExec struct {
	RunFunc            func(cmd *exec.Cmd) error
	OutputFunc         func(cmd *exec.Cmd) ([]byte, error)
	CombinedOutputFunc func(cmd *exec.Cmd) ([]byte, error)
}

func (e MockCmdExec) Run(cmd *exec.Cmd) error {
	return e.RunFunc(cmd)
}

func (e MockCmdExec) Output(cmd *exec.Cmd) ([]byte, error) {
	return e.OutputFunc(cmd)
}

func (e MockCmdExec) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	if e.CombinedOutputFunc != nil {
		return e.CombinedOutputFunc(cmd)
	}
	return e.OutputFunc(cmd)
}
