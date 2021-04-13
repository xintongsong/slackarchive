package web

import "embed"

//go:embed dist/*
var AssetFS embed.FS

var Prefix = "dist"
