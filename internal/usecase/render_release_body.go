package usecase

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/compozy/releasepr/internal/domain"
	"github.com/compozy/releasepr/internal/repository"
)

// ReleaseBodyGitRepository exposes the Git diff needed to scope custom notes.
type ReleaseBodyGitRepository interface {
	AddedFiles(ctx context.Context, gitRange, pathspec string) ([]string, error)
}

// RenderReleaseBodyInput identifies the release body to render.
type RenderReleaseBodyInput struct {
	Tag      string
	GitRange string
	Initial  bool
}

// RenderReleaseBodyUseCase renders one explicit release body from a canonical range.
type RenderReleaseBodyUseCase struct {
	gitRepo  ReleaseBodyGitRepository
	fsRepo   repository.FileSystemRepository
	cliffSvc interface {
		GenerateReleaseBodyChangelog(ctx context.Context, tag, gitRange string, initialRelease bool) (string, error)
	}
}

// NewRenderReleaseBodyUseCase creates an explicit release-body renderer.
func NewRenderReleaseBodyUseCase(
	gitRepo ReleaseBodyGitRepository,
	fsRepo repository.FileSystemRepository,
	cliffSvc interface {
		GenerateReleaseBodyChangelog(ctx context.Context, tag, gitRange string, initialRelease bool) (string, error)
	},
) *RenderReleaseBodyUseCase {
	return &RenderReleaseBodyUseCase{
		gitRepo:  gitRepo,
		fsRepo:   fsRepo,
		cliffSvc: cliffSvc,
	}
}

// Execute renders the changelog and only the custom notes introduced by the selected range.
func (uc *RenderReleaseBodyUseCase) Execute(
	ctx context.Context,
	input RenderReleaseBodyInput,
) (string, error) {
	coreTag, err := domain.CoreReleaseTag(input.Tag)
	if err != nil {
		return "", err
	}
	if err := domain.ValidateReleaseSelector(input.GitRange, input.Initial); err != nil {
		return "", err
	}
	changelog, err := uc.cliffSvc.GenerateReleaseBodyChangelog(
		ctx,
		input.Tag,
		input.GitRange,
		input.Initial,
	)
	if err != nil {
		return "", fmt.Errorf("failed to render release changelog: %w", err)
	}
	collection, err := uc.collectNotes(ctx, coreTag, input.GitRange)
	if err != nil {
		return "", err
	}
	return domain.BuildReleaseBody(changelog, collection.RenderMarkdown()), nil
}

func (uc *RenderReleaseBodyUseCase) collectNotes(
	ctx context.Context,
	coreTag string,
	gitRange string,
) (*domain.ReleaseNotesCollection, error) {
	collector := &CollectReleaseNotesUseCase{FSRepo: uc.fsRepo}
	if gitRange == "" {
		return collector.Execute(ctx, coreTag)
	}
	paths, err := uc.gitRepo.AddedFiles(ctx, gitRange, releaseNotesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve release-note range: %w", err)
	}
	return collector.collectPaths(ctx, releaseNotePathsForCore(paths, coreTag))
}

func releaseNotePathsForCore(paths []string, coreTag string) []string {
	activeDir := filepath.Clean(releaseNotesDir)
	archiveDir := filepath.Clean(releaseNotesArchiveDir(coreTag))
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		cleanPath := filepath.Clean(path)
		if filepath.Ext(cleanPath) != releaseNoteMarkdownExt {
			continue
		}
		parent := filepath.Dir(cleanPath)
		if parent == activeDir || parent == archiveDir {
			filtered = append(filtered, cleanPath)
		}
	}
	return filtered
}
