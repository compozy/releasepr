package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/compozy/releasepr/internal/domain"
	"github.com/compozy/releasepr/internal/usecase"
	"github.com/spf13/cobra"
)

const (
	planFormatJSON   = "json"
	planFormatGitHub = "github"
)

type releasePlanner interface {
	Execute(ctx context.Context, input usecase.PlanReleaseInput) (*domain.ReleasePlan, error)
}

// NewPlanCmd creates the explicit release planning command.
func NewPlanCmd(planner releasePlanner) *cobra.Command {
	var ref, version, channelValue, outputFormat string
	var allowExistingTag bool
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Validate and emit an explicit release plan",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requirePlanInput("ref", ref); err != nil {
				return err
			}
			if err := requirePlanInput("version", version); err != nil {
				return err
			}
			if err := requirePlanInput("channel", channelValue); err != nil {
				return err
			}
			channel, err := domain.ParseReleaseChannel(channelValue)
			if err != nil {
				return err
			}
			plan, err := planner.Execute(cmd.Context(), usecase.PlanReleaseInput{
				Ref:              ref,
				Version:          version,
				Channel:          channel,
				AllowExistingTag: allowExistingTag,
			})
			if err != nil {
				return err
			}
			return writeReleasePlan(cmd.OutOrStdout(), plan, outputFormat)
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "Git ref that must resolve to the checked-out HEAD")
	cmd.Flags().StringVar(&version, "version", "", "Authoritative unprefixed semantic version")
	cmd.Flags().StringVar(&channelValue, "channel", "", "Publication channel: beta, stable, or legacy")
	cmd.Flags().StringVar(&outputFormat, "format", planFormatJSON, "Output format: json or github")
	cmd.Flags().BoolVar(
		&allowExistingTag,
		"allow-existing-tag",
		false,
		"Resume only when the existing release tag is annotated and resolves to the planned commit",
	)
	return cmd
}

func requirePlanInput(name, value string) error {
	if value == "" {
		return fmt.Errorf("required flag %q not set", name)
	}
	return nil
}

func writeReleasePlan(output io.Writer, plan *domain.ReleasePlan, outputFormat string) error {
	switch outputFormat {
	case planFormatJSON:
		encoder := json.NewEncoder(output)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(plan); err != nil {
			return fmt.Errorf("failed to write JSON release plan: %w", err)
		}
		return nil
	case planFormatGitHub:
		lines := []string{
			"release_ref=" + plan.Ref,
			"release_commit=" + plan.Commit,
			"release_version=" + plan.Version,
			"release_tag=" + plan.Tag,
			"release_previous_tag=" + plan.PreviousTag,
			"release_git_range=" + plan.GitRange,
			"release_initial=" + strconv.FormatBool(plan.InitialRelease),
			"release_channel=" + string(plan.Channel),
			"github_prerelease=" + strconv.FormatBool(plan.GitHubPrerelease),
			"github_make_latest=" + strconv.FormatBool(plan.GitHubMakeLatest),
			"npm_tag=" + plan.NPMTag,
			"homebrew_skip_upload=" + strconv.FormatBool(plan.HomebrewSkipUpload),
		}
		if _, err := fmt.Fprintln(output, strings.Join(lines, "\n")); err != nil {
			return fmt.Errorf("failed to write GitHub release plan: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported plan output format %q: expected json or github", outputFormat)
	}
}
