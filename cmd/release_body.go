package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/compozy/releasepr/internal/domain"
	"github.com/compozy/releasepr/internal/usecase"
	"github.com/spf13/cobra"
)

type releaseBodyRenderer interface {
	Execute(ctx context.Context, input usecase.RenderReleaseBodyInput) (string, error)
}

// NewReleaseBodyCmd creates the explicit release-body rendering command.
func NewReleaseBodyCmd(renderer releaseBodyRenderer) *cobra.Command {
	var tag, gitRange string
	var initialRelease bool
	cmd := &cobra.Command{
		Use:   "release-body",
		Short: "Render a release body from an explicit Git range",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requirePlanInput("tag", tag); err != nil {
				return err
			}
			if err := domain.ValidateReleaseSelector(gitRange, initialRelease); err != nil {
				return err
			}
			body, err := renderer.Execute(cmd.Context(), usecase.RenderReleaseBodyInput{
				Tag:      tag,
				GitRange: gitRange,
				Initial:  initialRelease,
			})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSpace(body)); err != nil {
				return fmt.Errorf("failed to write release body: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "Target semantic-version Git tag")
	cmd.Flags().StringVar(&gitRange, "range", "", "Canonical <previous>..<commit> Git range")
	cmd.Flags().BoolVar(&initialRelease, "initial", false, "Render the first release when no predecessor exists")
	return cmd
}
