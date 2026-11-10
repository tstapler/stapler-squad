package main

import (
	"fmt"
	"strings"

	tags "github.com/tstapler/stapler-squad/tools/bzl/spike-tags"
)

func main() { fmt.Println(strings.Join(tags.Active(), " ")) }
