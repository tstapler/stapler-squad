// Command manual-instance builds the Bazel binary and runs it as the isolated manual instance used by parity
// row P9 (Task 7.5e): ports 62871/62872, instance claude-manual-test, private tmux socket ssq-manual-1.
// It never touches the live :8543 service or the default tmux socket.
//
//	manual-instance --dry-run   print the exact commands, run nothing
//	manual-instance             build, copy, check the ports, start, print the smoke list
//	manual-instance --smoke     as above, then check / and /metrics answer, then stop
//	manual-instance --stop      stop it and kill its private tmux server
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	port       = "62871"
	remotePort = "62872"
	instance   = "claude-manual-test"
	socket     = "ssq-manual-1"
	pidName    = "manual-1.pid"
)

// Plan is the fixed command set; computed so tests can assert the isolation properties.
type Plan struct {
	Build    []string
	BinSrc   string
	BinDst   string
	Start    []string
	StartEnv []string
	Stop     [][]string
}

// NewPlan builds the plan for a home directory and bazel binary.
func NewPlan(home, bazel string) Plan {
	dst := filepath.Join(home, ".stapler-squad", "manual-builds", "manual-1", "stapler-squad")
	return Plan{
		Build:    []string{bazel, "build", "//:stapler-squad"},
		BinSrc:   "bazel-bin/stapler-squad",
		BinDst:   dst,
		Start:    []string{dst, "--tmux-keep-server", "--remote-port", remotePort},
		StartEnv: []string{"PORT=" + port, "STAPLER_SQUAD_INSTANCE=" + instance, "STAPLER_SQUAD_TMUX_SOCKET=" + socket},
		Stop:     [][]string{{"tmux", "-L", socket, "kill-server"}},
	}
}

// PortsFree reports which of the isolated ports are already bound.
func PortsFree(ports ...string) (busy []string) {
	for _, p := range ports {
		l, err := net.Listen("tcp", "127.0.0.1:"+p)
		if err != nil {
			busy = append(busy, p)
			continue
		}
		l.Close()
	}
	return busy
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("manual-instance", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dry := fs.Bool("dry-run", false, "print the commands and exit")
	stop := fs.Bool("stop", false, "stop the manual instance")
	smoke := fs.Bool("smoke", false, "start, check the smoke list, stop")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, "manual-instance:", err)
		return 2
	}
	bazel := os.Getenv("BAZEL")
	if bazel == "" {
		bazel = "bazel"
	}
	p := NewPlan(home, bazel)
	pidFile := filepath.Join(filepath.Dir(p.BinDst), pidName)
	switch {
	case *dry:
		fmt.Fprintf(stdout, "build: %s\ncopy:  %s -> %s\nstart: %s %s\nstop:  %s then kill the pid in %s\n",
			strings.Join(p.Build, " "), p.BinSrc, p.BinDst, strings.Join(p.StartEnv, " "), strings.Join(p.Start, " "),
			strings.Join(p.Stop[0], " "), pidFile)
		return 0
	case *stop:
		return stopInstance(p, pidFile, stderr)
	}
	return start(p, pidFile, *smoke, stdout, stderr)
}

func start(p Plan, pidFile string, smoke bool, stdout, stderr io.Writer) int {
	if busy := PortsFree(port, remotePort); len(busy) > 0 {
		fmt.Fprintf(stderr, "manual-instance: ports in use: %v (a stale manual instance? run --stop)\n", busy)
		return 1
	}
	if err := runCmd(p.Build, nil); err != nil {
		fmt.Fprintln(stderr, "manual-instance: build failed:", err)
		return 1
	}
	if err := copyFile(p.BinSrc, p.BinDst); err != nil {
		fmt.Fprintln(stderr, "manual-instance:", err)
		return 1
	}
	cmd := exec.Command(p.Start[0], p.Start[1:]...)
	cmd.Env = append(os.Environ(), p.StartEnv...)
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(stderr, "manual-instance:", err)
		return 1
	}
	_ = os.WriteFile(pidFile, []byte(fmt.Sprint(cmd.Process.Pid)), 0o644)
	fmt.Fprintf(stdout, "smoke list: GET http://localhost:%s/ answers 200; GET /metrics lists metric names; stop with --stop\n", port)
	if !smoke {
		return 0
	}
	defer stopInstance(p, pidFile, stderr)
	for _, path := range []string{"/", "/metrics"} {
		if !waitOK("http://localhost:" + port + path) {
			fmt.Fprintln(stderr, "manual-instance: smoke FAIL", path)
			return 1
		}
	}
	fmt.Fprintln(stdout, "manual-instance: smoke OK")
	return 0
}

func waitOK(url string) bool {
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(time.Second) {
		if r, err := http.Get(url); err == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				return true
			}
		}
	}
	return false
}

func stopInstance(p Plan, pidFile string, stderr io.Writer) int {
	if raw, err := os.ReadFile(pidFile); err == nil {
		var pid int
		if _, err := fmt.Sscan(string(raw), &pid); err == nil && pid > 1 {
			if proc, err := os.FindProcess(pid); err == nil {
				_ = proc.Kill()
			}
		}
		_ = os.Remove(pidFile)
	}
	for _, c := range p.Stop {
		if err := runCmd(c, nil); err != nil {
			fmt.Fprintln(stderr, "manual-instance: stop step failed (may already be stopped):", err)
		}
	}
	return 0
}

func runCmd(argv []string, env []string) error {
	c := exec.Command(argv[0], argv[1:]...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = append(os.Environ(), env...)
	return c.Run()
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
