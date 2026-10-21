// Package migrations embeds the SQLite migration files so they need not
// exist on disk at runtime.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
