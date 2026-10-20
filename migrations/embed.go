// Package migrations embeds the SQLite schema migrations.
package migrations

import "embed"

// FS contains the versioned SQL migrations.
//
//go:embed sqlite/*.sql
var FS embed.FS
