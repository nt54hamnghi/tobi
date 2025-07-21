/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Work with tags in your Obsidian vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	addCommands(cmd)

	return cmd
}

func addCommands(cmd *cobra.Command) {
	cmd.AddCommand(
		NewListCmd(),
		NewStatsCmd(),
		NewSyncCmd(),
	)
}
