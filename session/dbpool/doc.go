// Package dbpool is the one place that opens SQLite files for ent-backed
// databases (ADR-036). It owns the writer/reader pool shape, the pragma
// profile per durability class, WAL enforcement, instrumentation wiring
// (session/dbobs, session/dbsplit) and a process-wide snapshot for
// /actuator/db and the db.journal_mode gauge.
//
// A database is opened through a typed constructor (OpenCore, OpenEscape), so
// the use case, its pool label and its profile cannot be mixed up by a caller.
// Paths are plain file paths with no query string: the DSN is built here, and
// the journal mode is read back after open instead of trusting the DSN.
package dbpool
