// Command fresh-clone implements P3's second half (Task 7.5a): `bazel build //...` must succeed in an empty
// clone of HEAD with no preconditions. The clone goes under a short directory so Unix-socket path limits in
// the build never matter. Arguments after `--` are passed to bazel build.
//
//	fresh-clone [--dry-run] [--keep] [--dir DIR] [-- bazel-build-flags...]
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	fs := flag.NewFlagSet("fresh-clone", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "print the commands without running them")
	keep := fs.Bool("keep", false, "keep the clone afterwards")
	dir := fs.String("dir", "", "clone directory (default: a new short dir under $TMPDIR)")
	repo := fs.String("repo", ".", "repository to clone")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	bazel := os.Getenv("BAZEL")
	if bazel == "" {
		bazel = "bazel"
	}
	target := *dir
	if target == "" {
		base, err := os.MkdirTemp(os.Getenv("TMPDIR"), "fc")
		if err != nil {
			fmt.Fprintln(os.Stderr, "fresh-clone:", err)
			return 2
		}
		target = filepath.Join(base, "c")
	}
	clone := []string{"git", "clone", "--quiet", "--no-hardlinks", *repo, target}
	build := append([]string{bazel, "build", "//..."}, fs.Args()...)
	if *dry {
		fmt.Println("+", clone)
		fmt.Printf("+ (cd %s && %v)\n", target, build)
		return 0
	}
	if !*keep {
		defer os.RemoveAll(filepath.Dir(target))
	}
	for _, c := range []struct {
		argv []string
		dir  string
	}{{clone, ""}, {build, target}} {
		cmd := exec.Command(c.argv[0], c.argv[1:]...)
		cmd.Dir = c.dir
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "fresh-clone: FAIL:", c.argv, err)
			return 1
		}
	}
	fmt.Println("fresh-clone: OK")
	return 0
}
