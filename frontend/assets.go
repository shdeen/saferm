// Package frontend embeds the built desktop interface.
package frontend

import "embed"

// Assets contains the built frontend files of the dist directory.
//
//go:embed all:dist
var Assets embed.FS
