// Command merge-bazel-regen is the git merge driver for conflicting **/BUILD.bazel files (Task 7.8b).
//
//	merge-bazel-regen %O %A %B %P      driver: keep our side, record that Gazelle must run, exit 0
//	merge-bazel-regen --finish         after the merge: if a driver run recorded a pending regen, run Gazelle
//
// Either side is acceptable because Gazelle rewrites generated BUILD files from the sources. Hand-written
// files (tools/bzl/hand-build-files.txt) are the exception: Gazelle never rewrites them, so the driver refuses
// and leaves the conflict for a human.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const pendingMarker = "bazel-regen-pending"

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 1 && args[0] == "--finish" {
		return finish()
	}
	if len(args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: merge-bazel-regen %O %A %B %P | --finish")
		return 2
	}
	path := args[3]
	hand, err := handWritten("tools/bzl/hand-build-files.txt", path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "merge-bazel-regen: cannot read hand-build-files.txt, refusing:", err)
		return 1
	}
	if hand {
		fmt.Fprintf(os.Stderr, "merge-bazel-regen: %s is hand-written (tools/bzl/hand-build-files.txt); resolve the conflict by hand\n", path)
		return 1
	}
	if err := markPending(); err != nil {
		fmt.Fprintln(os.Stderr, "merge-bazel-regen:", err)
		return 1
	}
	return 0 // %A already holds our side
}

func handWritten(list, path string) (bool, error) {
	f, err := os.Open(list)
	if err != nil {
		return false, err
	}
	defer f.Close()
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if l := strings.TrimSpace(strings.SplitN(sc.Text(), "#", 2)[0]); l == path {
			return true, nil
		}
	}
	return false, sc.Err()
}

func gitDir() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	return strings.TrimSpace(string(out)), err
}

func markPending() error {
	d, err := gitDir()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, pendingMarker), []byte("1\n"), 0o644)
}

func finish() int {
	d, err := gitDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "merge-bazel-regen:", err)
		return 1
	}
	marker := filepath.Join(d, pendingMarker)
	if _, err := os.Stat(marker); err != nil {
		return 0
	}
	bazel := os.Getenv("BAZEL")
	if bazel == "" {
		bazel = "bazel"
	}
	c := exec.Command(bazel, "run", "//:gazelle")
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "merge-bazel-regen: gazelle failed:", err, "\nRun: make gazelle")
		return 1
	}
	if err := os.Remove(marker); err != nil {
		fmt.Fprintln(os.Stderr, "merge-bazel-regen:", err)
		return 1
	}
	return 0
}
