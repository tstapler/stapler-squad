package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const portOwnerLookupTimeout = 2 * time.Second

func newRemoteBindError(addr string, bindErr error) error {
	if !errors.Is(bindErr, syscall.EADDRINUSE) {
		return fmt.Errorf("bind remote server on %s: %w", addr, bindErr)
	}

	_, port, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		return fmt.Errorf("bind remote server on %s: %w; could not determine port owner: %v", addr, bindErr, splitErr)
	}

	return fmt.Errorf(
		"bind remote server on %s: %w; process listening on port %s:\n%s",
		addr,
		bindErr,
		port,
		lookupPortOwner(port),
	)
}

func lookupPortOwner(port string) string {
	commands := []struct {
		name string
		args []string
	}{
		{name: "lsof", args: []string{"-nP", "-iTCP:" + port, "-sTCP:LISTEN"}},
		{name: "ss", args: []string{"-ltnp", "sport = :" + port}},
	}

	var failures []string
	for _, command := range commands {
		if _, err := exec.LookPath(command.name); err != nil {
			failures = append(failures, command.name+" not found")
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), portOwnerLookupTimeout)
		output, err := exec.CommandContext(ctx, command.name, command.args...).CombinedOutput()
		lookupErr := ctx.Err()
		cancel()

		if diagnostic := strings.TrimSpace(string(output)); diagnostic != "" {
			return diagnostic
		}
		if errors.Is(lookupErr, context.DeadlineExceeded) {
			failures = append(failures, command.name+" timed out")
		} else if err != nil {
			failures = append(failures, fmt.Sprintf("%s failed: %v", command.name, err))
		} else {
			failures = append(failures, command.name+" found no listener")
		}
	}

	return "port-owner lookup unavailable (" + strings.Join(failures, "; ") + ")"
}
