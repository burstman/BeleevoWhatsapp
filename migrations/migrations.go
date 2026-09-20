package migrations

import "embed"

// FS embeds all SQL migration files into the binary so that no external
// migration tooling is required at deploy time.
//
//go:embed *.sql
var FS embed.FS
