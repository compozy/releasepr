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
	ReleaseTagCommit(ctx context.Context, tag string) (commit string, annotated bool, err error)
	PreviousReleaseTag(ctx context.Context, commit string, includePrereleases bool, excludeTag string) (string, error)
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

type releaseTagState struct {
	commit    string
	annotated bool
}

// ReleaseTagCommit returns the target commit and annotation status of a local or origin tag.
func (r *gitRepository) ReleaseTagCommit(ctx context.Context, tag string) (string, bool, error) {
	local, err := r.localReleaseTagState(tag)
	if err != nil {
		return "", false, err
	}
	remote, err := r.remoteReleaseTagState(ctx, tag)
	if err != nil {
		return "", false, err
	}
	if local.commit != "" && remote.commit != "" && local != remote {
		return "", false, fmt.Errorf("release tag %s differs between the worktree and origin", tag)
	}
	if remote.commit != "" {
		return remote.commit, remote.annotated, nil
	}
	return local.commit, local.annotated, nil
}

func (r *gitRepository) localReleaseTagState(tag string) (releaseTagState, error) {
	tagRef, err := r.repo.Reference(plumbing.NewTagReferenceName(tag), true)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return releaseTagState{}, nil
	}
	if err != nil {
		return releaseTagState{}, fmt.Errorf("failed to inspect local release tag %s: %w", tag, err)
	}
	commit, err := r.resolveTagCommit(tagRef)
	if err != nil {
		return releaseTagState{}, fmt.Errorf("failed to resolve local release tag %s: %w", tag, err)
	}
	_, tagObjectErr := r.repo.TagObject(tagRef.Hash())
	if tagObjectErr != nil && !errors.Is(tagObjectErr, plumbing.ErrObjectNotFound) {
		return releaseTagState{}, fmt.Errorf("failed to inspect local release tag object %s: %w", tag, tagObjectErr)
	}
	return releaseTagState{commit: commit.String(), annotated: tagObjectErr == nil}, nil
}

func (r *gitRepository) remoteReleaseTagState(ctx context.Context, tag string) (releaseTagState, error) {
	if _, err := r.repo.Remote(gitOriginRemoteName); err != nil {
		return releaseTagState{}, fmt.Errorf("failed to get remote origin: %w", err)
	}
	tagRef := plumbing.NewTagReferenceName(tag)
	output, err := r.runReadOnlyGit(
		ctx,
		"ls-remote",
		"--tags",
		gitOriginRemoteName,
		tagRef.String(),
		tagRef.String()+"^{}",
	)
	if err != nil {
		return releaseTagState{}, fmt.Errorf("failed to inspect remote release tag %s: %w", tag, err)
	}
	return parseRemoteReleaseTagState(output, tagRef.String())
}

func parseRemoteReleaseTagState(output, tagRef string) (releaseTagState, error) {
	var directHash, peeledHash string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return releaseTagState{}, fmt.Errorf("remote tag output contains a malformed line")
		}
		switch fields[1] {
		case tagRef:
			directHash = fields[0]
		case tagRef + "^{}":
			peeledHash = fields[0]
		}
	}
	if directHash == "" {
		return releaseTagState{}, nil
	}
	if peeledHash != "" {
		return releaseTagState{commit: peeledHash, annotated: true}, nil
	}
	return releaseTagState{commit: directHash}, nil
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
	excludeTag string,
) (string, error) {
	if _, err := r.ResolveRevision(ctx, commit); err != nil {
		return "", err
	}
	tags, err := r.mergedReleaseTags(ctx, commit, includePrereleases, excludeTag)
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
	excludeTag string,
) ([]string, error) {
	output, err := r.runReadOnlyGit(ctx, "tag", "--merged", commit, "--list", "v*")
	if err != nil {
		return nil, fmt.Errorf("failed to list reachable release tags: %w", err)
	}
	tags := make([]string, 0)
	for _, tag := range strings.Fields(output) {
		if tag == excludeTag {
			continue
		}
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
