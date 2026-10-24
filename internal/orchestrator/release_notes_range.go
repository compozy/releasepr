package orchestrator

import (
	"context"
	"fmt"

	"github.com/compozy/releasepr/internal/domain"
	"github.com/compozy/releasepr/internal/usecase"
)

func (o *PRReleaseOrchestrator) renderReleaseNotes(ctx context.Context, version string) (string, string, error) {
	head, err := o.gitRepo.ResolveRevision(ctx, "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve release-note head: %w", err)
	}
	previous, err := o.gitRepo.PreviousReleaseTag(ctx, head, version)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve release-note predecessor: %w", err)
	}
	gitRange, err := domain.BuildReleaseGitRange(previous, head)
	if err != nil {
		return "", "", err
	}
	renderer := usecase.NewRenderReleaseBodyUseCase(o.gitRepo, o.fsRepo, o.cliffSvc)
	return renderer.RenderParts(ctx, usecase.RenderReleaseBodyInput{
		Tag:      version,
		GitRange: gitRange,
		Initial:  previous == "",
	})
}
