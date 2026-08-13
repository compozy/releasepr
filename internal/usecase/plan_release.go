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
	ReleaseTagCommit(ctx context.Context, tag string) (commit string, annotated bool, err error)
	PreviousReleaseTag(ctx context.Context, commit string, includePrereleases bool, excludeTag string) (string, error)
}

// PlanReleaseInput contains the explicit operator-provided release identity.
type PlanReleaseInput struct {
	Ref              string
	Version          string
	Channel          domain.ReleaseChannel
	AllowExistingTag bool
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
	candidate, err := domain.NewReleasePlan(input.Ref, resolvedCommit, input.Version, "", input.Channel)
	if err != nil {
		return nil, err
	}
	tagCommit, annotated, err := uc.gitRepo.ReleaseTagCommit(ctx, candidate.Tag)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect release tag %q: %w", candidate.Tag, err)
	}
	if err := validateExistingReleaseTag(
		candidate.Tag,
		resolvedCommit,
		tagCommit,
		annotated,
		input.AllowExistingTag,
	); err != nil {
		return nil, err
	}
	includePrereleases := input.Channel == domain.ReleaseChannelBeta
	previousTag, err := uc.gitRepo.PreviousReleaseTag(ctx, resolvedCommit, includePrereleases, candidate.Tag)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve previous release tag: %w", err)
	}
	return domain.NewReleasePlan(input.Ref, resolvedCommit, input.Version, previousTag, input.Channel)
}

func validateExistingReleaseTag(tag, releaseCommit, tagCommit string, annotated, allowExisting bool) error {
	if tagCommit == "" {
		return nil
	}
	if !allowExisting {
		return fmt.Errorf("release tag %q already exists", tag)
	}
	if !annotated {
		return fmt.Errorf("release tag %q exists but is not annotated", tag)
	}
	if tagCommit != releaseCommit {
		return fmt.Errorf("release tag %q resolves to %s, expected %s", tag, tagCommit, releaseCommit)
	}
	return nil
}
