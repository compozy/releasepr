package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "pr-release",
	Short: "Create release pull requests for any repository",
	Long:  `pr-release automates tagging, changelog generation, and release pull request orchestration for GitHub repositories.`,
}

func Execute() error {
	return rootCmd.Execute()
}
