package a

import "database/sql"

func opens() {
	_, _ = sql.Open("sqlite", "x.db")  // want `SQLite opened outside session/dbpool`
	_, _ = sql.Open("sqlite3", "x.db") // want `SQLite opened outside session/dbpool`
	_, _ = sql.Open("postgres", "dsn")

	_, _ = sql.Open("sqlite", "x.db") //nolint:norawdb writes to a file another tool owns

	//nolint:norawdb
	_, _ = sql.Open("sqlite", "x.db") // want `needs a one-line reason`
}

func dynamicDriver(driver string) {
	_, _ = sql.Open(driver, "x.db") // not a constant driver name: not judged
}
