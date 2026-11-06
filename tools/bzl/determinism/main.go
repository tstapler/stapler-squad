package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run exits 0 when every cacheable action matched (or mismatched only as an accepted downgrade), 1 on an
// unaccepted mismatch, 2 on unreadable input.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("determinism", flag.ContinueOnError)
	fs.SetOutput(stderr)
	logA := fs.String("a", "", "execution log (JSON lines) of the first clean build")
	logB := fs.String("b", "", "execution log (JSON lines) of the second clean build")
	downgrades := fs.String("downgrades", "tools/bzl/hermetic-downgrades.txt", "accepted hermeticity downgrades")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *logA == "" || *logB == "" {
		fmt.Fprintln(stderr, "determinism: -a and -b are required")
		return 2
	}
	a, err := readLogFile(*logA)
	if err != nil {
		fmt.Fprintln(stderr, "determinism:", err)
		return 2
	}
	b, err := readLogFile(*logB)
	if err != nil {
		fmt.Fprintln(stderr, "determinism:", err)
		return 2
	}
	accepted, err := readDowngrades(*downgrades)
	if err != nil {
		fmt.Fprintln(stderr, "determinism:", err)
		return 2
	}
	rep := Compare(a, b, accepted)
	for _, m := range rep.Mismatches {
		kind := "MISMATCH"
		if m.Accepted {
			kind = "MISMATCH (accepted downgrade)"
		}
		fmt.Fprintf(stdout, "%s %s %s %s: %s != %s\n", kind, m.Mnemonic, m.Label, m.Path, m.DigestA, m.DigestB)
	}
	for _, m := range rep.NowMatching {
		fmt.Fprintf(stdout, "NOTE %s matched; its no-remote-cache tag and hermetic-downgrades.txt row may be removed\n", m)
	}
	fmt.Fprintf(stdout, "compared=%d only_a=%d only_b=%d mismatches=%d\n", rep.Compared, rep.OnlyA, rep.OnlyB, len(rep.Mismatches))
	if rep.Failed() {
		return 1
	}
	return 0
}

func readLogFile(p string) ([]Spawn, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadLog(f)
}

func readDowngrades(p string) (map[string]bool, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := ParseDowngrades(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	m := map[string]bool{}
	for _, r := range rows {
		m[r.Action] = true
	}
	return m, nil
}
