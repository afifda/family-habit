package migrations

import "embed"

// Files contains the versioned SQL migrations shipped with the API and migration binary.
//
//go:embed *.sql
var Files embed.FS
