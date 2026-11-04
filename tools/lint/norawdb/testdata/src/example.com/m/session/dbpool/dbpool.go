package dbpool

import "database/sql"

func Open() { _, _ = sql.Open("sqlite", "x.db") } // the registry itself is exempt
