// Package claudectltest provides an in-memory InstanceContext and a controller
// builder for tests of code that consumes claudectl.ClaudeController.
package claudectltest

import (
	"fmt"
	"os"
	"time"

	"github.com/tstapler/stapler-squad/session/claudectl"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/ptyio"
)

// Instance is a minimal claudectl.InstanceContext whose Preview is fixed.
type Instance struct {
	Title   string
	Content string
}

func (m *Instance) GetTitle() string                    { return m.Title }
func (m *Instance) GetStableID() string                 { return "UNSET-STABLE-ID" }
func (m *Instance) GetPTYReader() (*os.File, error)     { return nil, fmt.Errorf("no PTY in mock") }
func (m *Instance) Preview() (string, error)            { return m.Content, nil }
func (m *Instance) LastMeaningfulOutputTime() time.Time { return time.Time{} }
func (m *Instance) GetCreatedAt() time.Time             { return time.Time{} }
func (m *Instance) SetLastMeaningfulOutput(time.Time)   {}
func (m *Instance) GetStatus() int                      { return 0 }
func (m *Instance) WriteToPTY([]byte) (int, error)      { return 0, nil }
func (m *Instance) GetProgram() string                  { return "" }

// NewController returns an unstarted controller over a pane pre-filled with
// content, plus the pane's buffer so a test can append output later.
func NewController(content string) (*claudectl.ClaudeController, *ptyio.CircularBuffer) {
	cc := claudectl.NewBareForTest("test", &Instance{Title: "test", Content: content})
	cc.SetDetectorsForTest(detection.NewStatusDetector(), detection.NewIdleDetector("test", nil))
	buf := ptyio.NewCircularBuffer(256 * 1024)
	if content != "" {
		_, _ = buf.Write([]byte(content))
	}
	cc.SetPTYAccessForTest(ptyio.NewPTYAccess("test", nil, buf))
	return cc, buf
}
