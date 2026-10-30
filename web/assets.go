//go:build !production

package frontend

import (
	"io/fs"
	"testing/fstest"
)

// Development serves the UI through Vite; production embeds the compiled app.
func Assets() fs.FS {
	return fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(`<!doctype html><html lang="en"><meta charset="utf-8"><title>opnsense-ip-invites</title><body><p>Start the frontend with <code>cd web &amp;&amp; npm run dev</code>, or build the complete application with <code>make build</code>.</p></body></html>`)}}
}
