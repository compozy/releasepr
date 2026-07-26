// Contract: the plan command emits deterministic machine-readable release identity.
// Owning layer: Cobra command boundary.
// Canonical suite: this file; cmd had no release-plan command suite.
// Rationale: workflow outputs are a public automation contract.
package cmd

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/compozy/releasepr/internal/domain"
	"github.com/compozy/releasepr/internal/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type releasePlannerStub struct {
	input usecase.PlanReleaseInput
	plan  *domain.ReleasePlan
	err   error
}

func (s *releasePlannerStub) Execute(_ context.Context, input usecase.PlanReleaseInput) (*domain.ReleasePlan, error) {
	s.input = input
	return s.plan, s.err
}

func TestNewPlanCmd(t *testing.T) {
	t.Parallel()

	plan, err := domain.NewReleasePlan("main", "0123456789abcdef", "0.3.0-beta.1", domain.ReleaseChannelBeta)
	require.NoError(t, err)

	t.Run("Should emit the release plan as JSON", func(t *testing.T) {
		t.Parallel()

		planner := &releasePlannerStub{plan: plan}
		output := new(bytes.Buffer)
		command := NewPlanCmd(planner)
		command.SetOut(output)
		command.SetArgs([]string{"--ref", "main", "--version", "0.3.0-beta.1", "--channel", "beta"})
		require.NoError(t, command.Execute())
		assert.Equal(t, usecase.PlanReleaseInput{
			Ref:     "main",
			Version: "0.3.0-beta.1",
			Channel: domain.ReleaseChannelBeta,
		}, planner.input)
		assert.JSONEq(t, `{
			"ref":"main",
			"commit":"0123456789abcdef",
			"version":"0.3.0-beta.1",
			"tag":"v0.3.0-beta.1",
			"channel":"beta",
			"github_prerelease":true,
			"github_make_latest":false,
			"npm_tag":"beta",
			"homebrew_skip_upload":true
		}`, output.String())
	})

	t.Run("Should emit deterministic GitHub outputs", func(t *testing.T) {
		t.Parallel()

		planner := &releasePlannerStub{plan: plan}
		output := new(bytes.Buffer)
		command := NewPlanCmd(planner)
		command.SetOut(output)
		command.SetArgs([]string{
			"--ref", "main",
			"--version", "0.3.0-beta.1",
			"--channel", "beta",
			"--format", "github",
		})
		require.NoError(t, command.Execute())
		assert.Equal(t, "release_ref=main\n"+
			"release_commit=0123456789abcdef\n"+
			"release_version=0.3.0-beta.1\n"+
			"release_tag=v0.3.0-beta.1\n"+
			"release_channel=beta\n"+
			"github_prerelease=true\n"+
			"github_make_latest=false\n"+
			"npm_tag=beta\n"+
			"homebrew_skip_upload=true\n", output.String())
	})

	t.Run("Should require every explicit release input", func(t *testing.T) {
		t.Parallel()

		command := NewPlanCmd(&releasePlannerStub{})
		command.SetArgs([]string{"--ref", "main", "--channel", "beta"})
		err := command.Execute()
		require.Error(t, err)
		assert.ErrorContains(t, err, "required flag")
	})

	t.Run("Should preserve planning failures", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("release ref mismatch")
		command := NewPlanCmd(&releasePlannerStub{err: expectedErr})
		command.SetArgs([]string{"--ref", "main", "--version", "0.3.0-beta.1", "--channel", "beta"})
		err := command.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
	})
}
