package cmd

import (
	"fmt"
	"strings"

	"github.com/compozy/releasepr/pkg/version"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Version:\t%s\n", safeValue(version.Version, "dev"))
			fmt.Fprintf(out, "Commit:\t%s\n", safeValue(version.CommitHash, "unknown"))
			fmt.Fprintf(out, "Built:\t%s\n", safeValue(version.BuildDate, "unknown"))
			return nil
		},
	}
}

func safeValue(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
