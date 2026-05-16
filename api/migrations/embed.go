// Package migrations bundles the goose-style SQL migrations into the
// server binary via embed.FS. The migrate subcommand in cmd/server
// initializes goose from this FS so a freshly built binary needs no
// filesystem state to run `migrate up`.
//
// To add a migration: drop a new `NNNN_*.sql` file alongside this
// embed.go. The embed pattern below matches it automatically; no Go
// changes required.
package migrations

import "embed"

// FS exposes the embedded migration SQL files as an fs.FS. Goose's
// SetBaseFS understands this directly.
//
//go:embed *.sql
var FS embed.FS
