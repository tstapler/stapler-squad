package sessiontest

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/testutil/tmuxreap"
	"github.com/zalando/go-keyring"
)

// Main is the hermetic TestMain body shared by the session test binaries; it returns the exit code.
func Main(m *testing.M) int {
	log.InitializeForTests(log.ERROR, log.ERROR)
	defer log.Close()

	// Switch go-keyring to its in-memory mock provider so backlog_plugin_github*
	// tests (which call github.GetKeychainTokenForHost through Fetch/FetchAll/
	// CloseIssue/PostIssueComment) never touch the real OS keychain. Without
	// this, a machine with real GitHub accounts already connected (e.g. via
	// Settings > GitHub Accounts) returns a real token instead of "", making
	// those tests fail non-deterministically depending on local machine state.
	keyring.MockInit()

	// Fail the run on any non-loopback dial through http.DefaultTransport.
	netGuard := envtest.DenyNonLoopbackNetwork()

	// Tests that launch the real `claude` binary (restart/resume, MCP-config)
	// must not use the developer's credentials or MCP servers.
	restoreClaudeCLI, err := envtest.IsolateClaudeCLI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: %v\n", err)
		return 1
	}
	defer restoreClaudeCLI()

	// See envtest.ClearAmbientStaplerSquadStateEnv's doc comment: an ambient
	// STAPLER_SQUAD_TEST_DIR/STAPLER_SQUAD_INSTANCE left set in the shell (e.g.
	// by an earlier e2e run) silently wins over this package's own per-PID test
	// isolation, so tests that call config.LoadConfig() (e.g. SwitchProgram's
	// empty-string default resolution) read someone else's shared config.json
	// instead of a fresh default. Previously worked around ad hoc per-test via
	// t.Setenv (see TestSwitchProgram_EmptyString_ResolvesToConfigDefault's
	// comment) — this closes the gap for every other test in the package that
	// never got that treatment.
	restoreStaplerSquadEnv := envtest.ClearAmbientStaplerSquadStateEnv()
	defer restoreStaplerSquadEnv()

	tmuxreap.ReapLeakedTestServers()
	tmuxreap.StartTestServerWatchdog(os.Getpid())

	// Periodic goroutine dump so hangs produce visible output rather than silence.
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				DumpGoroutines("periodic")
			case <-stop:
				return
			}
		}
	}()

	// Also dump on interrupt so manual Ctrl-C reveals the hang site.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		DumpGoroutines("signal")
	}()

	code := m.Run()
	close(stop)
	RemoveGitTemplateRepos()
	if report := netGuard.Report(); report != "" {
		fmt.Fprint(os.Stderr, report)
		code = 1
	}
	return code
}

func DumpGoroutines(reason string) {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	fmt.Fprintf(os.Stderr, "\n=== goroutine dump (%s) ===\n%s\n", reason, buf[:n])
}
