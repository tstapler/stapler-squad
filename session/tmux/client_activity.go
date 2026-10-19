package tmux

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// ClientActivity is one attached tmux client: which session it shows, whether it
// is a control-mode client (always the case for ssq's own) and when it last had input.
type ClientActivity struct {
	Session     string
	ControlMode bool
	Activity    time.Time
	// PID is the tmux client process; 0 when unknown.
	PID int32
}

// IsOurs reports whether the client is a child of this process: ssq attaches a
// pty client to every session it starts, and that client is not a person.
func (c ClientActivity) IsOurs() bool {
	if c.PID <= 0 {
		return false
	}
	proc, err := process.NewProcess(c.PID)
	if err != nil {
		return false
	}
	ppid, err := proc.Ppid()
	return err == nil && int(ppid) == os.Getpid()
}

// clientActivityFormat puts the session name last so it may hold any character but a newline.
const clientActivityFormat = "#{client_control_mode} #{client_activity} #{client_pid} #{client_session}"

// ListClientActivity lists the clients attached to serverSocket's tmux server.
func ListClientActivity(ctx context.Context, serverSocket string) ([]ClientActivity, error) {
	args := prependSocket(serverSocket, []string{"list-clients", "-F", clientActivityFormat})
	out, err := (LocalRunner{}).Run(ctx, "", ResolveClientForSocket(serverSocket), args...)
	if err != nil {
		return nil, fmt.Errorf("tmux list-clients: %w", err)
	}
	return ParseClientActivity(string(out)), nil
}

// ParseClientActivity parses clientActivityFormat output, skipping malformed lines.
func ParseClientActivity(out string) []ClientActivity {
	var clients []ClientActivity
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 4)
		if len(fields) != 4 {
			continue
		}
		secs, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		pid, _ := strconv.ParseInt(fields[2], 10, 32)
		clients = append(clients, ClientActivity{
			Session: fields[3], ControlMode: fields[0] == "1", Activity: time.Unix(secs, 0), PID: int32(pid), //nolint:gosec // a pid fits in int32
		})
	}
	return clients
}
