// Package ui provides the embedded React SPA filesystem.
// Embed can only access files in or below the package directory,
// so this package lives next to the dist/ folder it embeds.
package ui

import "embed"

// DistFS contains the built React UI assets (index.html, JS, CSS).
// The "all:" prefix includes files starting with . or _ (like .vite/).
//
//go:embed all:dist
var DistFS embed.FS
