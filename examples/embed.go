// Package examples embeds the pre-built demo applications into the AYB binary.
// Demo dist/ directories are built at build time (make demos) and served
// directly by `ayb demo <name>` — no Node.js required at runtime.
package examples

import (
	"embed"
	"io/fs"
)

// FS contains the pre-built static assets (dist/) and schema files for all demos.
//
//go:embed kanban/dist live-polls/dist
//go:embed kanban/schema.sql live-polls/schema.sql
var FS embed.FS

// DemoDist returns a sub-filesystem rooted at the given demo's dist/ directory.
func DemoDist(name string) (fs.FS, error) {
	return fs.Sub(FS, name+"/dist")
}
