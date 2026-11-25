package services

import (
	"bytes"
	"errors"
)

var errDiskFull = errors.New("no space left on device")

func lines(b []byte) [][]byte {
	return bytes.Split(bytes.TrimSpace(b), []byte("\n"))
}
