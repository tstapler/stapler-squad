package main

import (
	_ "embed"

	"github.com/tstapler/stapler-squad/pkg/changelog"
)

//go:embed CHANGELOG.md
var changelogMD []byte

func init() { changelog.SetEmbedded(changelogMD) }
