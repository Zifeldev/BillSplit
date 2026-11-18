// Package migrations embeds per-service SQL migrations. Each service applies
// only its own folder into its own Postgres schema.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed auth social split finance notify
var all embed.FS

func For(service string) fs.FS {
	sub, err := fs.Sub(all, service)
	if err != nil {
		panic(err)
	}
	return sub
}
