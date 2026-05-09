package app

import (
	"fmt"
	"io"
)

type app struct {
	version string
	stdout  io.Writer
	stderr  io.Writer
}

func Run(args []string, stdout, stderr io.Writer, version string) int {
	a := &app{
		version: version,
		stdout:  stdout,
		stderr:  stderr,
	}

	root := a.rootCmd()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
