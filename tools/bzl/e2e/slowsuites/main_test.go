package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const inv = `| Suite | p50 s | Classification | Status | Own CI job |
|---|---|---|---|---|
| mcp | 590 | rdeps | x | mcp.yml#mcp~go test |
| harness | - | full: external | x | - |
`

func TestParseInventory(t *testing.T) {
	t.Parallel()
	s, bad := parseInventory(inv)
	if len(bad) != 0 || len(s) != 1 || s[0].workflow != "mcp.yml" || s[0].job != "mcp" || s[0].marker != "go test" {
		t.Fatalf("got %+v %v", s, bad)
	}
	if _, bad := parseInventory("| a | 1 | rdeps | x | nohash |\n"); len(bad) != 1 {
		t.Fatalf("malformed cell must be reported: %v", bad)
	}
}

func TestCheckSuite(t *testing.T) {
	t.Parallel()
	s := suite{"mcp", "mcp.yml", "mcp", "go test"}
	if bad := checkSuite(s, "jobs:\n  mcp:\n    steps:\n      - run: go test ./...\n"); len(bad) != 0 {
		t.Fatalf("healthy: %v", bad)
	}
	if bad := checkSuite(s, "jobs:\n  other:\n    - run: go test\n"); len(bad) != 1 {
		t.Fatalf("missing job: %v", bad)
	}
	if bad := checkSuite(s, "jobs:\n  mcp:\n    - run: bazel test //...\n"); len(bad) != 2 {
		t.Fatalf("marker gone and bazel invoked: %v", bad)
	}
	if bad := checkSuite(s, "jobs:\n  mcp:\n    - run: go test\n    # bazel test later\n"); len(bad) != 0 {
		t.Fatalf("comments ignored: %v", bad)
	}
}

func TestCheckBazelWorkflows(t *testing.T) {
	t.Parallel()
	s := []suite{{"mcp", "mcp.yml", "mcp", ""}}
	if bad := checkBazelWorkflows(s, map[string]string{"bazel.yml": "# mcp.yml mentioned\nbody: mcp.yml stays\n"}); len(bad) != 0 {
		t.Fatalf("mentions are fine: %v", bad)
	}
	if bad := checkBazelWorkflows(s, map[string]string{"bazel.yml": "    uses: ./.github/workflows/mcp.yml\n"}); len(bad) != 1 {
		t.Fatalf("reuse must fail: %v", bad)
	}
}

func TestRunEndToEnd(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(inventoryPath, inv)
	write(".github/workflows/mcp.yml", "jobs:\n  mcp:\n    - run: go test\n")
	write(".github/workflows/bazel.yml", "jobs:\n  b:\n    - run: bazel build //x\n")
	var out bytes.Buffer
	if code := run(root, &out); code != 0 {
		t.Fatalf("code %d: %s", code, out.String())
	}
	write(".github/workflows/bazel.yml", "jobs:\n  b:\n    uses: ./.github/workflows/mcp.yml\n")
	out.Reset()
	if code := run(root, &out); code != 1 || !strings.Contains(out.String(), "reuses mcp.yml") {
		t.Fatalf("code %d: %s", code, out.String())
	}
}
