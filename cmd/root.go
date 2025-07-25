package cmd

import (
	"os"

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
	)
}

// defaultVaultPath returns the Obsidian vault path by checking the OBSIDIAN_VAULT_PATH
// environment variable first, falling back to the current working directory.
// Returns an error if unable to determine the current working directory.
func defaultVaultPath() (string, error) {
	path, exist := os.LookupEnv("OBSIDIAN_VAULT_PATH")
	if !exist {
		return os.Getwd()
	}
	return path, nil
}
