package cmd

import (
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/thediveo/enumflag/v2"
)

// https://github.com/thediveo/enumflag?tab=readme-ov-file#cli-flag-with-default
type displayMode enumflag.Flag

const (
	name displayMode = iota
	count
	relative
)

var displayModeIDs = map[displayMode][]string{
	name:     {"name", "n"},
	count:    {"count", "c"},
	relative: {"relative", "r"},
}

// displayModeVariants returns an iterator that yields the canonical variant
// string representation for each display mode
func displayModeVariants() iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, v := range displayModeIDs {
			if !yield(v[0]) {
				return
			}
		}
	}
}

// displayModeAliases returns an iterator that yields all variant string
// representations (canonical and aliases) for every display mode.
func displayModeAliases() iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, v := range displayModeIDs {
			for _, a := range v {
				if !yield(a) {
					return
				}
			}
		}
	}
}

func displayModeUsage() string {
	v := slices.Collect(displayModeVariants())
	return fmt.Sprintf("display mode (%s)", strings.Join(v, "|"))
}

func completeDisplayModeFlag(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return slices.Collect(displayModeAliases()), cobra.ShellCompDirectiveDefault
}
