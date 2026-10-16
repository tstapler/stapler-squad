//go:build !windows

package clihelp

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"time"
)

const helperEnv = "CLIHELP_TEST_HELPER"

// TestMain re-execs the test binary as a fake CLI when CLIHELP_TEST_HELPER is set
// (same convention as executor/safeexec/safeexec_sigkill_helper_test.go).
func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		runHelperMode(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runHelperMode(mode string) {
	switch mode {
	case "bigout":
		chunk := strings.Repeat("x", 1<<10)
		for i := 0; i < 1024; i++ {
			fmt.Fprint(os.Stdout, chunk)
			fmt.Fprint(os.Stderr, chunk)
		}
		os.Exit(3)
	case "flood":
		chunk := []byte(strings.Repeat("y", 64<<10))
		for {
			if _, err := os.Stdout.Write(chunk); err != nil {
				os.Exit(0)
			}
		}
	case "hang":
		signal.Ignore(syscall.SIGTERM)
		fmt.Printf("pid=%d\n", os.Getpid())
		_ = os.Stdout.Sync()
		time.Sleep(time.Hour) //nolint:notimesleeptest helper subprocess must block until killed by the test; not a timing wait
	case "orphan":
		cmd := safeexec.CommandContext(context.Background(), os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), helperEnv+"=hang")
		cmd.Stdout = os.Stdout
		if err := cmd.Start(); err != nil {
			os.Exit(4)
		}
		time.Sleep(200 * time.Millisecond) //nolint:notimesleeptest helper subprocess: grandchild's stdout is the parent's own fd, so there is no pipe to read a ready signal from
	case "printenv":
		env := os.Environ()
		sort.Strings(env)
		fmt.Println(strings.Join(env, "\n"))
		wd, _ := os.Getwd()
		fmt.Printf("cwd=%s\n", wd)
		entries, _ := os.ReadDir(wd)
		fmt.Printf("cwdentries=%d\n", len(entries))
	case "sid":
		sid, _ := unix.Getsid(0)
		fmt.Printf("sid=%d pid=%d\n", sid, os.Getpid())
	}
}
