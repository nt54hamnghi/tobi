package cmd

import (
	"github.com/spf13/cobra"
	"github.com/thediveo/enumflag/v2"
)

// https://github.com/thediveo/enumflag?tab=readme-ov-file#cli-flag-with-default
type displayMode enumflag.Flag

const (
	name displayMode = iota
	count
)

var displayModeIDs = map[displayMode][]string{
	name:  {"name", "n"},
	count: {"count", "c"},
}

func displayModeVariants() []string {
	vs := []string{}
	for _, v := range displayModeIDs {
		vs = append(vs, v...)
	}
	return vs
}

func completeDisplayModeFlag(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return displayModeVariants(), cobra.ShellCompDirectiveDefault
}
