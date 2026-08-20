// Package web provides the embedded single-page local control console.
package web

import "embed"

// Assets contains precisely the public assets served by the HTTP router.
//
//go:embed index.html app.js styles.css
var Assets embed.FS
