package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize tags",
		Long:  `Synchronize tags from your Obsidian vault with a storage file`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("sync called")
			return nil
		},
	}

	return cmd
}
