// Command mediaguard fails PRs that add binary media outside the allow-list.
package main

import (
	"os"

	"github.com/tstapler/stapler-squad/tools/promo/internal/mediaguard"
)

func main() { os.Exit(mediaguard.Run(os.Args[1:], os.Stdout, os.Stderr)) }
