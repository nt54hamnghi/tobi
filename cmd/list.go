package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all tags",
		Long:  `List all tags in your Obsidian vault`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("list called")
			return nil
		},
	}

	return cmd
}
