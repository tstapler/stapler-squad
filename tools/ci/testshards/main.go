package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func main() { os.Exit(run(os.Args[1:], goList, os.Stdout, os.Stderr)) }

func goList() ([]string, error) {
	cmd := exec.Command("go", append([]string{"list"}, roots...)...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	return strings.Fields(string(out)), nil
}

func run(args []string, list func() ([]string, error), out, errw io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(errw, "usage: testshards <shard>")
		return 2
	}
	pkgs, err := list()
	if err != nil {
		fmt.Fprintln(errw, "testshards:", err)
		return 1
	}
	byShard, err := partition(pkgs)
	if err != nil {
		fmt.Fprintln(errw, "testshards:", err)
		return 1
	}
	selected, ok := byShard[args[0]]
	if !ok {
		fmt.Fprintf(errw, "testshards: unknown shard %q\n", args[0])
		return 2
	}
	fmt.Fprintln(out, strings.Join(selected, "\n"))
	return 0
}
