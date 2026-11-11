package local

import "embed"

// The release build prepares these archives from pinned upstream downloads.
// Keep them compressed in the EXE: first launch expands them into app data;
// users never need a database installation or a development environment.
//
//go:embed all:runtimes
var runtimeAssets embed.FS
