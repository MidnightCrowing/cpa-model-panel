package web

import "embed"

// Dist holds the built frontend. Place files under web/dist.
// A placeholder index is included so `go build` works before npm build.
//
//go:embed all:dist
var Dist embed.FS
