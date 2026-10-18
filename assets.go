package main

import "embed"

// Embedded assets. //go:embed cannot reach parent directories, so the
// embeds live in the root package next to webui/ and resources/.

//go:embed resources/icon.png
var appIconPNG []byte

//go:embed webui
var webUIFS embed.FS
