package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/go-git/go-git/v5/plumbing"
)

// GitReleasePlanRepository exposes read-only Git facts used by explicit release planning.
type GitReleasePlanRepository interface {
	GetHeadCommit(ctx context.Context) (string, error)
	ResolveRevision(ctx context.Context, ref string) (string, error)
	ReleaseTagExists(ctx context.Context, tag string) (bool, error)
}

// GitOrchestrationRepository combines mutable release-PR operations with explicit release planning.
type GitOrchestrationRepository interface {
	GitExtendedRepository
	GitReleasePlanRepository
}

var _ GitOrchestrationRepository = (*gitRepository)(nil)

// ReleaseTagExists checks if a release tag exists locally or on origin.
func (r *gitRepository) ReleaseTagExists(ctx context.Context, tag string) (bool, error) {
	localExists, err := r.TagExists(ctx, tag)
	if err != nil {
		return false, err
	}
	if localExists {
		return true, nil
	}
	if _, err := r.repo.Remote(gitOriginRemoteName); err != nil {
		return false, fmt.Errorf("failed to get remote origin: %w", err)
	}
	tagRef := plumbing.NewTagReferenceName(tag)
	//nolint:gosec // Git is invoked without a shell; the tag is one argument.
	cmd := exec.CommandContext(
		ctx,
		"git",
		"ls-remote",
		"--exit-code",
		"--tags",
		gitOriginRemoteName,
		tagRef.String(),
	)
	cmd.Dir = r.getWorkingDirectory()
	cmd.Env = append(os.Environ(), r.getGitEnv()...)
	err = cmd.Run()
	if err == nil {
		return true, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, fmt.Errorf("remote tag check canceled: %w", ctxErr)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
		return false, nil
	}
	return false, fmt.Errorf("failed to check remote tag %s: %w", tag, err)
}

// ResolveRevision resolves a Git revision to its commit SHA.
func (r *gitRepository) ResolveRevision(ctx context.Context, ref string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("revision resolution canceled: %w", err)
	}
	hash, err := r.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return "", fmt.Errorf("failed to resolve revision %q: %w", ref, err)
	}
	return hash.String(), nil
}
