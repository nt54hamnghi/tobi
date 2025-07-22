package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Calculate statistics for tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("stats called")
			return nil
		},
	}

	return cmd
}
