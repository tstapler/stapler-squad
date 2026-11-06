// Package clix is a near-miss of the sanctioned cli path and must still be flagged.
package clix

import "os/exec"

func bad() { _ = exec.Command("git", "status") } // want `exec\.Command invokes the git CLI directly`
