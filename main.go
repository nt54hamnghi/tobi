package main

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/nt54hamnghi/tobi/cmd"
)

const VERSION = "0.1.3"

func main() {
	c := cmd.NewRootCmd()
	if err := fang.Execute(context.Background(), c,
		fang.WithVersion(VERSION),
	); err != nil {
		os.Exit(1)
	}
}
