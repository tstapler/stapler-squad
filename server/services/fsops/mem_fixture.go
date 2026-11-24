package fsops

// EnsureTree makes dir an openable (possibly empty) root.
func (m *MemBackend) EnsureTree(dir string) { m.tree(dir) }

// MkParents creates dir/rel as a directory, with its parents.
func (m *MemBackend) MkParents(dir, rel string) { m.tree(dir).mkParents(rel + "/x") }
