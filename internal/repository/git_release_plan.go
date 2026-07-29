package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/go-git/go-git/v5/plumbing"
)

// GitReleasePlanRepository exposes read-only Git facts used by explicit release planning.
type GitReleasePlanRepository interface {
	GetHeadCommit(ctx context.Context) (string, error)
	ResolveRevision(ctx context.Context, ref string) (string, error)
	ReleaseTagExists(ctx context.Context, tag string) (bool, error)
	PreviousReleaseTag(ctx context.Context, commit string, includePrereleases bool) (string, error)
}

// GitReleaseBodyRepository exposes read-only Git facts used to render an explicit release body.
type GitReleaseBodyRepository interface {
	AddedFiles(ctx context.Context, gitRange, pathspec string) ([]string, error)
}

// GitOrchestrationRepository combines mutable release-PR operations with explicit release planning.
type GitOrchestrationRepository interface {
	GitExtendedRepository
	GitReleasePlanRepository
	GitReleaseBodyRepository
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

// PreviousReleaseTag returns the nearest reachable strict semantic-version tag.
func (r *gitRepository) PreviousReleaseTag(
	ctx context.Context,
	commit string,
	includePrereleases bool,
) (string, error) {
	if _, err := r.ResolveRevision(ctx, commit); err != nil {
		return "", err
	}
	tags, err := r.mergedReleaseTags(ctx, commit, includePrereleases)
	if err != nil || len(tags) == 0 {
		return "", err
	}
	args := []string{
		"describe",
		"--tags",
		"--abbrev=0",
		"--first-parent",
		"--candidates",
		strconv.Itoa(len(tags)),
	}
	for _, tag := range tags {
		args = append(args, "--match", tag)
	}
	args = append(args, commit)
	output, err := r.runReadOnlyGit(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to describe previous release tag: %w", err)
	}
	return strings.TrimSpace(output), nil
}

func (r *gitRepository) mergedReleaseTags(
	ctx context.Context,
	commit string,
	includePrereleases bool,
) ([]string, error) {
	output, err := r.runReadOnlyGit(ctx, "tag", "--merged", commit, "--list", "v*")
	if err != nil {
		return nil, fmt.Errorf("failed to list reachable release tags: %w", err)
	}
	tags := make([]string, 0)
	for _, tag := range strings.Fields(output) {
		version, parseErr := semver.StrictNewVersion(strings.TrimPrefix(tag, "v"))
		if parseErr != nil || !strings.HasPrefix(tag, "v") {
			continue
		}
		if !includePrereleases && version.Prerelease() != "" {
			continue
		}
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags, nil
}

// AddedFiles lists files introduced by a Git range under one pathspec.
func (r *gitRepository) AddedFiles(ctx context.Context, gitRange, pathspec string) ([]string, error) {
	output, err := r.runReadOnlyGit(
		ctx,
		"diff",
		"--name-only",
		"-z",
		"--diff-filter=A",
		"--find-renames",
		gitRange,
		"--",
		pathspec,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list added files: %w", err)
	}
	paths := strings.Split(strings.TrimSuffix(output, "\x00"), "\x00")
	if len(paths) == 1 && paths[0] == "" {
		return []string{}, nil
	}
	sort.Strings(paths)
	return paths, nil
}

func (r *gitRepository) runReadOnlyGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.getWorkingDirectory()
	cmd.Env = append(os.Environ(), r.getGitEnv()...)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf(
				"git %s failed: %w (stderr: %s)",
				args[0],
				err,
				strings.TrimSpace(string(exitErr.Stderr)),
			)
		}
		return "", fmt.Errorf("git %s failed: %w", args[0], err)
	}
	return string(output), nil
}
