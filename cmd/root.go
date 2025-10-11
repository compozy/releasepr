package cmd

import (
	"errors"

	"github.com/compozy/releasepr/internal/logger"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "pr-release",
	Short: "Create release pull requests for any repository",
	Long:  `pr-release automates tagging, changelog generation, and release pull request orchestration for GitHub repositories.`,
}

func Execute() error {
	execErr := rootCmd.Execute()
	syncErr := logger.Sync(logger.FromContext(rootCmd.Context()))
	if execErr != nil {
		if syncErr != nil {
			return errors.Join(execErr, syncErr)
		}
		return execErr
	}
	return syncErr
}
