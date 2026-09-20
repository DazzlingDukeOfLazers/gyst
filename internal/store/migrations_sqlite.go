package store

import _ "embed"

//go:embed schema_sqlite.sql
var sqliteSchema string

// sqliteMigrations are applied in order on open, once each.
var sqliteMigrations = []struct{ name, body string }{
	{"0001_schema", sqliteSchema},
}
