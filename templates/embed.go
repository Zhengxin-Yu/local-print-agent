// Package templates embeds the self-contained document templates.
package templates

import "embed"

// Assets contains every template needed to render print-ready HTML.
//
//go:embed balloon_ticket.html.tmpl source_code.html.tmpl
var Assets embed.FS
