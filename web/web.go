package web

import "embed"

// FS embeds all static web assets (HTML, CSS, JS)
//
//go:embed *
var FS embed.FS
