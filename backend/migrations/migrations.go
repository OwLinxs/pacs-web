// Package migrations embute os arquivos SQL de migration no binário, para que o
// deploy não dependa de arquivos soltos no servidor.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
