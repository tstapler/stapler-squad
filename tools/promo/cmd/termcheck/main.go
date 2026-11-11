// Command termcheck fails on banned trademark terms in tracked source and published content.
package main

import (
	"os"

	"github.com/tstapler/stapler-squad/tools/promo/internal/termcheck"
)

func main() { os.Exit(termcheck.Run(os.Args[1:], os.Stdout, os.Stderr)) }
