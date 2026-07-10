// Package migrations embeds the SQL schema migrations into the binary.
//
// They used to be read from a `migrations/` directory relative to the working
// directory, so a binary started from anywhere else found no files, applied nothing,
// and served every request against an empty database — while /health answered 200.
package migrations

import "embed"

//go:embed *.up.sql *.down.sql
var FS embed.FS
