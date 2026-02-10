package web

import "embed"

//go:embed frontend/dist
var FrontendFS embed.FS
