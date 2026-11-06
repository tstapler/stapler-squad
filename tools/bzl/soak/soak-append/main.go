// Command soak-append appends one run row to soak-ledger.yaml, keeping the file's comments, and validates the
// result with the same schema soak-check uses. The nightly flake job (Task 4.9c) calls it with the exit code of
// `bazel test`; a failure it cannot classify is left unattributed so Story 7.6 forces a human to attribute it.
// Exit 0 appended (or the identical row already exists), 2 on any error.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/tstapler/stapler-squad/tools/bzl/soak"
)

func main() {
	file := flag.String("file", "project_plans/bazel-conversion/implementation/soak-ledger.yaml", "ledger file")
	runID := flag.String("run-id", "", "run_id of the new row")
	exit := flag.Int("exit-code", -1, "exit code of `bazel test`")
	flag.Parse()
	if err := appendFile(*file, *runID, *exit, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "soak-append:", err)
		os.Exit(2)
	}
}

// RowFor maps a `bazel test` exit code to a ledger row. 32, 34 and 36 are Bazel's remote-environment, remote-error
// and local-environment failures (infra); every other nonzero code stays unattributed on purpose.
func RowFor(runID string, exitCode int) soak.Run {
	switch exitCode {
	case 0:
		return soak.Run{RunID: runID, Result: "pass"}
	case 32, 34, 36:
		return soak.Run{RunID: runID, Result: "fail", Attribution: soak.Infra}
	}
	return soak.Run{RunID: runID, Result: "fail"}
}

func appendFile(path, runID string, exitCode int, out io.Writer) error {
	if runID == "" || exitCode < 0 {
		return fmt.Errorf("-run-id and -exit-code are required")
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	res, added, err := Append(src, RowFor(runID, exitCode))
	if err != nil {
		return err
	}
	if !added {
		fmt.Fprintf(out, "run %s already recorded\n", runID)
		return nil
	}
	if err := os.WriteFile(path, res, 0o644); err != nil { //nolint:gosec // repo file
		return err
	}
	fmt.Fprintf(out, "appended %s\n", runID)
	return nil
}

// Append adds row to the ledger's runs list. added is false when an identical row is already there; a different
// row with the same run_id is an error.
func Append(src []byte, row soak.Run) (res []byte, added bool, err error) {
	l, err := soak.Parse(bytes.NewReader(src))
	if err != nil {
		return nil, false, err
	}
	for _, r := range l.Runs {
		if r.RunID == row.RunID {
			if r == row {
				return src, false, nil
			}
			return nil, false, fmt.Errorf("run_id %s already recorded with a different result", row.RunID)
		}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, false, err
	}
	runs := mapValue(doc.Content[0], "runs")
	if runs == nil || runs.Kind != yaml.SequenceNode {
		return nil, false, fmt.Errorf("ledger has no runs list")
	}
	var node yaml.Node
	if err := node.Encode(row); err != nil {
		return nil, false, err
	}
	runs.Style = 0 // an empty `runs: []` is flow style; block style once rows exist
	runs.Content = append(runs.Content, &node)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, false, err
	}
	if errs := reparse(buf.Bytes()); len(errs) > 0 {
		return nil, false, fmt.Errorf("result fails ledger validation: %v", errs)
	}
	return buf.Bytes(), true, nil
}

func reparse(b []byte) []string {
	l, err := soak.Parse(bytes.NewReader(b))
	if err != nil {
		return []string{err.Error()}
	}
	return l.Validate()
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}
