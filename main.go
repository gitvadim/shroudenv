package main

import (
	"embed"
	"shroudenv/cmd"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

func main() {
	cmd.FrontendFS = frontendFS
	cmd.Execute()
}

