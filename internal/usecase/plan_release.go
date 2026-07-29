package usecase

import (
	"context"
	"fmt"

	"github.com/compozy/releasepr/internal/domain"
)

// ReleasePlanGitRepository exposes the repository facts required to prove a release plan.
type ReleasePlanGitRepository interface {
	ResolveRevision(ctx context.Context, ref string) (string, error)
	GetHeadCommit(ctx context.Context) (string, error)
	ReleaseTagExists(ctx context.Context, tag string) (bool, error)
	PreviousReleaseTag(ctx context.Context, commit string, includePrereleases bool) (string, error)
}

// PlanReleaseInput contains the explicit operator-provided release identity.
type PlanReleaseInput struct {
	Ref     string
	Version string
	Channel domain.ReleaseChannel
}

// PlanReleaseUseCase proves repository state and returns the canonical release plan.
type PlanReleaseUseCase struct {
	gitRepo ReleasePlanGitRepository
}

// NewPlanReleaseUseCase creates a release planning use case.
func NewPlanReleaseUseCase(gitRepo ReleasePlanGitRepository) *PlanReleaseUseCase {
	return &PlanReleaseUseCase{gitRepo: gitRepo}
}

// Execute validates the release identity against the checked-out repository without mutating it.
func (uc *PlanReleaseUseCase) Execute(ctx context.Context, input PlanReleaseInput) (*domain.ReleasePlan, error) {
	resolvedCommit, err := uc.gitRepo.ResolveRevision(ctx, input.Ref)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve release ref %q: %w", input.Ref, err)
	}
	headCommit, err := uc.gitRepo.GetHeadCommit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve HEAD commit: %w", err)
	}
	if resolvedCommit != headCommit {
		return nil, fmt.Errorf(
			"release ref %q resolves to %s and does not match HEAD %s",
			input.Ref,
			resolvedCommit,
			headCommit,
		)
	}
	includePrereleases := input.Channel == domain.ReleaseChannelBeta
	previousTag, err := uc.gitRepo.PreviousReleaseTag(ctx, resolvedCommit, includePrereleases)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve previous release tag: %w", err)
	}
	plan, err := domain.NewReleasePlan(input.Ref, resolvedCommit, input.Version, previousTag, input.Channel)
	if err != nil {
		return nil, err
	}
	tagExists, err := uc.gitRepo.ReleaseTagExists(ctx, plan.Tag)
	if err != nil {
		return nil, fmt.Errorf("failed to prove release tag %q is absent: %w", plan.Tag, err)
	}
	if tagExists {
		return nil, fmt.Errorf("release tag %q already exists", plan.Tag)
	}
	return plan, nil
}
