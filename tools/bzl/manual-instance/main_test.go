package main

import (
	"bytes"
	"net"
	"strings"
	"testing"
)

func TestPlanIsolation(t *testing.T) {
	p := NewPlan("/home/x", "bazel")
	env := strings.Join(p.StartEnv, " ")
	for _, want := range []string{"PORT=62871", "STAPLER_SQUAD_INSTANCE=claude-manual-test", "STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1"} {
		if !strings.Contains(env, want) {
			t.Errorf("missing %s", want)
		}
	}
	all := env + strings.Join(p.Start, " ") + strings.Join(p.Stop[0], " ")
	for _, bad := range []string{"8543", "8444", "stapler-squad.service"} {
		if strings.Contains(all, bad) {
			t.Errorf("plan must not mention the live instance: %s", bad)
		}
	}
	if !strings.HasSuffix(p.BinDst, "manual-builds/manual-1/stapler-squad") || strings.Join(p.Stop[0], " ") != "tmux -L ssq-manual-1 kill-server" {
		t.Fatalf("%+v", p)
	}
	if !strings.Contains(strings.Join(p.Start, " "), "--tmux-keep-server") {
		t.Fatal("--tmux-keep-server required")
	}
}

func TestDryRunPrintsCommandsAndExitsZero(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"-dry-run"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "PORT=62871") {
		t.Fatalf("%d %s %s", code, out.String(), errb.String())
	}
}

func TestPortsFreeDetectsBusyPort(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	if busy := PortsFree(port); len(busy) != 1 {
		t.Fatalf("got %v", busy)
	}
}
