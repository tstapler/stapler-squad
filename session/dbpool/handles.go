package dbpool

// CoreDB is the typed handle for sessions.db. It is a distinct type from
// EscapeDB so a repository cannot be handed the wrong file.
type CoreDB struct{ *Database }

// EscapeDB is the typed handle for analytics.db.
type EscapeDB struct{ *Database }

// OpenCore opens sessions.db with a read pool of readers connections
// (0 keeps one pool for reads and writes).
func OpenCore(path string, readers int) (CoreDB, error) {
	db, err := Open(CoreSpec(path, readers))
	return CoreDB{db}, err
}

// OpenEscape opens analytics.db.
func OpenEscape(path string) (EscapeDB, error) {
	db, err := Open(EscapeSpec(path))
	return EscapeDB{db}, err
}
