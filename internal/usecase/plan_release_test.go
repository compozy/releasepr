// Contract: a release plan targets the checked-out commit and a globally absent tag.
// Owning layer: release planning use case.
// Canonical suite: this file; no existing use case owns explicit release identity.
// Rationale: repository facts are behavioral preconditions of every publication.
package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/compozy/releasepr/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type releasePlanGitRepositoryStub struct {
	resolvedCommit string
	headCommit     string
	tagExists      bool
	previousTag    string
	resolveErr     error
	headErr        error
	tagErr         error
	previousTagErr error
	includePreview bool
}

func (s *releasePlanGitRepositoryStub) ResolveRevision(context.Context, string) (string, error) {
	return s.resolvedCommit, s.resolveErr
}

func (s *releasePlanGitRepositoryStub) GetHeadCommit(context.Context) (string, error) {
	return s.headCommit, s.headErr
}

func (s *releasePlanGitRepositoryStub) ReleaseTagExists(context.Context, string) (bool, error) {
	return s.tagExists, s.tagErr
}

func (s *releasePlanGitRepositoryStub) PreviousReleaseTag(
	_ context.Context,
	_ string,
	includePrereleases bool,
) (string, error) {
	s.includePreview = includePrereleases
	return s.previousTag, s.previousTagErr
}

func TestPlanReleaseUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("Should return the plan when ref matches HEAD and tag is absent", func(t *testing.T) {
		t.Parallel()

		repo := &releasePlanGitRepositoryStub{
			resolvedCommit: "0123456789abcdef",
			headCommit:     "0123456789abcdef",
			previousTag:    "v0.3.0-beta.0",
		}
		uc := NewPlanReleaseUseCase(repo)
		plan, err := uc.Execute(t.Context(), PlanReleaseInput{
			Ref:     "refs/heads/main",
			Version: "0.3.0-beta.1",
			Channel: domain.ReleaseChannelBeta,
		})
		require.NoError(t, err)
		assert.Equal(t, "0123456789abcdef", plan.Commit)
		assert.Equal(t, "v0.3.0-beta.1", plan.Tag)
		assert.Equal(t, "v0.3.0-beta.0", plan.PreviousTag)
		assert.Equal(t, "v0.3.0-beta.0..0123456789abcdef", plan.GitRange)
		assert.True(t, repo.includePreview)
	})

	t.Run("Should exclude prereleases when resolving a stable predecessor", func(t *testing.T) {
		t.Parallel()

		repo := &releasePlanGitRepositoryStub{
			resolvedCommit: "0123456789abcdef",
			headCommit:     "0123456789abcdef",
			previousTag:    "v0.2.15",
		}
		uc := NewPlanReleaseUseCase(repo)
		plan, err := uc.Execute(t.Context(), PlanReleaseInput{
			Ref:     "main",
			Version: "0.3.0",
			Channel: domain.ReleaseChannelStable,
		})
		require.NoError(t, err)
		assert.Equal(t, "v0.2.15", plan.PreviousTag)
		assert.False(t, repo.includePreview)
	})

	t.Run("Should reject a ref that does not resolve to HEAD", func(t *testing.T) {
		t.Parallel()

		repo := &releasePlanGitRepositoryStub{
			resolvedCommit: "aaaaaaaa",
			headCommit:     "bbbbbbbb",
		}
		uc := NewPlanReleaseUseCase(repo)
		plan, err := uc.Execute(t.Context(), PlanReleaseInput{
			Ref:     "legacy/v0.2",
			Version: "0.2.16",
			Channel: domain.ReleaseChannelLegacy,
		})
		require.Error(t, err)
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "does not match HEAD")
	})

	t.Run("Should reject a tag that already exists", func(t *testing.T) {
		t.Parallel()

		repo := &releasePlanGitRepositoryStub{
			resolvedCommit: "0123456789abcdef",
			headCommit:     "0123456789abcdef",
			tagExists:      true,
		}
		uc := NewPlanReleaseUseCase(repo)
		plan, err := uc.Execute(t.Context(), PlanReleaseInput{
			Ref:     "main",
			Version: "0.3.0",
			Channel: domain.ReleaseChannelStable,
		})
		require.Error(t, err)
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "already exists")
	})

	t.Run("Should preserve repository errors", func(t *testing.T) {
		t.Parallel()

		repo := &releasePlanGitRepositoryStub{resolveErr: errors.New("revision unavailable")}
		uc := NewPlanReleaseUseCase(repo)
		plan, err := uc.Execute(t.Context(), PlanReleaseInput{
			Ref:     "main",
			Version: "0.3.0",
			Channel: domain.ReleaseChannelStable,
		})
		require.Error(t, err)
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "revision unavailable")
	})

	t.Run("Should preserve HEAD resolution errors", func(t *testing.T) {
		t.Parallel()

		repo := &releasePlanGitRepositoryStub{
			resolvedCommit: "0123456789abcdef",
			headErr:        errors.New("HEAD unavailable"),
		}
		uc := NewPlanReleaseUseCase(repo)
		plan, err := uc.Execute(t.Context(), PlanReleaseInput{
			Ref:     "main",
			Version: "0.3.0",
			Channel: domain.ReleaseChannelStable,
		})
		require.Error(t, err)
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "HEAD unavailable")
	})

	t.Run("Should fail closed when remote tag state cannot be proved", func(t *testing.T) {
		t.Parallel()

		repo := &releasePlanGitRepositoryStub{
			resolvedCommit: "0123456789abcdef",
			headCommit:     "0123456789abcdef",
			tagErr:         errors.New("origin unavailable"),
		}
		uc := NewPlanReleaseUseCase(repo)
		plan, err := uc.Execute(t.Context(), PlanReleaseInput{
			Ref:     "main",
			Version: "0.3.0",
			Channel: domain.ReleaseChannelStable,
		})
		require.Error(t, err)
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "failed to prove release tag")
		assert.ErrorContains(t, err, "origin unavailable")
	})

	t.Run("Should fail closed when the predecessor cannot be resolved", func(t *testing.T) {
		t.Parallel()

		repo := &releasePlanGitRepositoryStub{
			resolvedCommit: "0123456789abcdef",
			headCommit:     "0123456789abcdef",
			previousTagErr: errors.New("history unavailable"),
		}
		uc := NewPlanReleaseUseCase(repo)
		plan, err := uc.Execute(t.Context(), PlanReleaseInput{
			Ref:     "main",
			Version: "0.3.0-beta.1",
			Channel: domain.ReleaseChannelBeta,
		})
		require.Error(t, err)
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "failed to resolve previous release tag")
		assert.ErrorContains(t, err, "history unavailable")
	})
}
