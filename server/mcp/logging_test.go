package mcp

import (
	stdlog "log"
	"os"
	"testing"

	"github.com/tstapler/stapler-squad/log"
)

func TestMCPLoggerStderrOnly(t *testing.T) {
	InitMCPLogging()
	loggers := []*stdlog.Logger{log.InfoLog, log.WarningLog, log.ErrorLog, log.DebugLog}
	for _, l := range loggers {
		if l == nil {
			continue
		}
		if l.Writer() != os.Stderr {
			t.Errorf("logger %v writes to %v, want os.Stderr", l, l.Writer())
		}
	}
}
