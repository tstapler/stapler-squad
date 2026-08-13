package backend

import "os"

func openFileCreate(path string) (*os.File, error) {
	return os.Create(path)
}
