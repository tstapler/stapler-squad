// Package norawdb is a deliberate norawdb violation; never import it.
package norawdb

import "database/sql"

// Violation opens SQLite outside session/dbpool.
func Violation() { _, _ = sql.Open("sqlite", "x.db") }
