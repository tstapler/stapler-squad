package mcp

import (
	stdlog "log"
	"os"

	"github.com/tstapler/stapler-squad/log"
)

// InitMCPLogging redirects all application loggers to stderr so that log lines
// do not pollute the MCP stdio channel (stdout). Must be called before RunServer.
func InitMCPLogging() {
	redirectToStderr(log.InfoLog)
	redirectToStderr(log.WarningLog)
	redirectToStderr(log.ErrorLog)
	redirectToStderr(log.DebugLog)
}

func redirectToStderr(l *stdlog.Logger) {
	if l == nil {
		return
	}
	l.SetOutput(os.Stderr)
}
