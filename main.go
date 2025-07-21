package main

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/nt54hamnghi/tag/cmd"
)

const VERSION = "0.1.0"

func main() {
	c := cmd.NewRootCmd()
	if err := fang.Execute(context.Background(), c,
		fang.WithVersion(VERSION),
	); err != nil {
		os.Exit(1)
	}
}
