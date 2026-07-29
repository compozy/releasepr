package service

import (
	"context"

	"github.com/compozy/releasepr/internal/domain"
)

// CliffService defines the interface for interacting with git-cliff.

type CliffService interface {
	CalculateNextVersion(ctx context.Context, latestTag string) (*domain.Version, error)
	GenerateChangelog(ctx context.Context, version, mode string) (string, error)
	GenerateFullChangelog(ctx context.Context, version string) (string, error)
}

// ReleaseBodyCliffService renders a changelog for an explicit release range.
type ReleaseBodyCliffService interface {
	GenerateReleaseBodyChangelog(ctx context.Context, tag, gitRange string, initialRelease bool) (string, error)
}

// FullCliffService exposes both release-PR and explicit-release changelog operations.
type FullCliffService interface {
	CliffService
	ReleaseBodyCliffService
}
