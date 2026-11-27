// Command ssq is a command line client for the stapler-squad API. Its commands
// are derived from the proto service descriptors; see internal/ssqcli.
package main

import (
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/internal/ssqcli"
)

func main() {
	root := ssqcli.NewRootCommand(ssqcli.DefaultEnv(config.GetConfigDir))
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ssq:", err)
		os.Exit(1)
	}
}
