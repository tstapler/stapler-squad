package tmux

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ClientActivity is one attached tmux client: which session it shows, whether it
// is a control-mode client (always the case for ssq's own) and when it last had input.
type ClientActivity struct {
	Session     string
	ControlMode bool
	Activity    time.Time
}

// clientActivityFormat puts the session name last so it may hold any character but a newline.
const clientActivityFormat = "#{client_control_mode} #{client_activity} #{client_session}"

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
		fields := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(fields) != 3 {
			continue
		}
		secs, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		clients = append(clients, ClientActivity{
			Session: fields[2], ControlMode: fields[0] == "1", Activity: time.Unix(secs, 0),
		})
	}
	return clients
}
