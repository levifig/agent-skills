package main

import (
	"os"

	"github.com/levifig/loaf/vnext/internal/command"
)

func main() {
	os.Exit(command.New(os.Stdout, os.Stderr).Run(os.Args[1:]))
}
