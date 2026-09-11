package commands

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tstapler/stapler-squad/pkg/portguard"
)

var (
	ensurePortsFreePattern    string
	ensurePortsFreeTimeout    time.Duration
	ensurePortsFreeExcludePID int32
)

// EnsurePortsFreeCmd is a hidden operational command used by
// scripts/install-service.sh and scripts/dev-restart-guard.sh to guarantee a
// set of ports are unbound before the next stapler-squad instance starts —
// actively terminating a stale holder rather than waiting a fixed duration
// and proceeding regardless. See pkg/portguard's package doc for the
// incident history this replaces.
var EnsurePortsFreeCmd = &cobra.Command{
	Use:    "ensure-ports-free PORT [PORT...]",
	Short:  "Ensure the given TCP ports are free, terminating a matching stale process if needed",
	Hidden: true,
	Args:   cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ports := make([]int, 0, len(args))
		for _, a := range args {
			p, err := strconv.Atoi(a)
			if err != nil {
				return fmt.Errorf("invalid port %q: %w", a, err)
			}
			ports = append(ports, p)
		}

		err := portguard.EnsureReleased(context.Background(), portguard.Options{
			Ports:               ports,
			ProcessNameContains: ensurePortsFreePattern,
			Timeout:             ensurePortsFreeTimeout,
			ExcludePID:          ensurePortsFreeExcludePID,
		})
		if err != nil {
			return err
		}
		fmt.Printf("ports free: %s\n", strings.Trim(fmt.Sprint(ports), "[]"))
		return nil
	},
}

func init() {
	EnsurePortsFreeCmd.Flags().StringVar(&ensurePortsFreePattern, "process-name", "stapler-squad",
		"only terminate processes whose executable name contains this substring")
	EnsurePortsFreeCmd.Flags().DurationVar(&ensurePortsFreeTimeout, "timeout", portguard.DefaultTimeout,
		"how long to wait for a graceful exit before escalating to SIGKILL")
	EnsurePortsFreeCmd.Flags().Int32Var(&ensurePortsFreeExcludePID, "exclude-pid", 0,
		"never signal this PID even if it holds one of the target ports (e.g. the live launchd-managed service)")
}
