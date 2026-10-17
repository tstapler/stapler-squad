package services

// osRootBackend serves files through os.Root, wrapping WorktreeRoot by composition.
type osRootBackend struct{}

func (osRootBackend) OpenRoot(dir string) (FileRoot, error) {
	root, err := OpenWorktreeRoot(dir)
	if err != nil {
		return nil, err
	}
	return osFileRoot{root}, nil
}

type osFileRoot struct{ *WorktreeRoot }

// Open shadows WorktreeRoot.Open to return the FileHandle interface (never a typed-nil *os.File).
func (r osFileRoot) Open(p ResolvedPath) (FileHandle, error) {
	f, err := r.WorktreeRoot.Open(p)
	if err != nil {
		return nil, err
	}
	return f, nil
}
