package migrations

import "embed"

// FS contains ordered SQL migrations applied by the SQLite store.
//
//go:embed *.sql
var FS embed.FS
