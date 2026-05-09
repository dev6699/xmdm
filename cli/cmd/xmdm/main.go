package main

import (
	"os"

	"xmdm/cli/internal/app"
	"xmdm/cli/internal/version"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdout, os.Stderr, version.Full()))
}
