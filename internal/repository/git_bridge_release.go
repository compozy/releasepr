package repository

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Masterminds/semver/v3"
)

func (r *gitRepository) bridgeReleaseTag(ctx context.Context, commit, excludeTag, previous string) (string, error) {
	line, err := r.runReadOnlyGit(ctx, "rev-list", "--first-parent", commit)
	if err != nil {
		return "", fmt.Errorf("failed to read release line: %w", err)
	}
	positions := make(map[string]int)
	for index, hash := range strings.Fields(line) {
		positions[hash] = index
	}
	boundary := len(positions)
	if previous != "" {
		hash, resolveErr := r.runReadOnlyGit(ctx, "rev-parse", previous+"^{commit}")
		if resolveErr != nil {
			return "", fmt.Errorf("failed to resolve predecessor: %w", resolveErr)
		}
		boundary = positions[strings.TrimSpace(hash)]
	}
	return r.selectBridgeReleaseTag(ctx, commit, excludeTag, previous, positions, boundary)
}

func (r *gitRepository) selectBridgeReleaseTag(
	ctx context.Context,
	commit, excludeTag, previous string,
	positions map[string]int,
	boundary int,
) (string, error) {
	output, err := r.runReadOnlyGit(ctx, "tag", "--list", "v*")
	if err != nil {
		return "", fmt.Errorf("failed to list bridge release tags: %w", err)
	}
	best, bestPosition := previous, boundary
	for _, tag := range strings.Fields(output) {
		version, parseErr := semver.StrictNewVersion(strings.TrimPrefix(tag, "v"))
		if tag == excludeTag || parseErr != nil {
			continue
		}
		position, eligible, candidateErr := r.bridgeReleasePosition(ctx, commit, tag, positions)
		if candidateErr != nil {
			return "", candidateErr
		}
		if !eligible || position >= boundary || position > bestPosition {
			continue
		}
		if position == bestPosition && best != "" {
			bestVersion, versionErr := semver.StrictNewVersion(strings.TrimPrefix(best, "v"))
			if versionErr != nil || !version.GreaterThan(bestVersion) {
				continue
			}
		}
		best, bestPosition = tag, position
	}
	return best, nil
}

func (r *gitRepository) bridgeReleasePosition(
	ctx context.Context,
	commit, tag string,
	positions map[string]int,
) (int, bool, error) {
	hash, err := r.runReadOnlyGit(ctx, "rev-parse", tag+"^{commit}")
	if err != nil {
		return 0, false, fmt.Errorf("failed to resolve bridge tag %s: %w", tag, err)
	}
	if _, onLine := positions[strings.TrimSpace(hash)]; onLine {
		return 0, false, nil
	}
	base, err := r.runReadOnlyGit(ctx, "merge-base", "--all", commit, strings.TrimSpace(hash))
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to find bridge merge base for %s: %w", tag, err)
	}
	position, onLine := positions[strings.TrimSpace(base)]
	return position, onLine, nil
}
