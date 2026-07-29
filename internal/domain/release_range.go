package domain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

var gitRevisionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// BuildReleaseGitRange builds the canonical predecessor-to-commit range for a release.
func BuildReleaseGitRange(previousTag, commit string) (string, error) {
	if previousTag == "" {
		return "", nil
	}
	if !strings.HasPrefix(previousTag, "v") {
		return "", fmt.Errorf("previous release tag %q must start with v", previousTag)
	}
	if _, err := semver.StrictNewVersion(strings.TrimPrefix(previousTag, "v")); err != nil {
		return "", fmt.Errorf("previous release tag %q must be a strict semantic version: %w", previousTag, err)
	}
	gitRange := previousTag + ".." + commit
	if err := ValidateReleaseGitRange(gitRange); err != nil {
		return "", err
	}
	return gitRange, nil
}

// ValidateReleaseGitRange validates the two-dot Git range accepted by release rendering.
func ValidateReleaseGitRange(gitRange string) error {
	previous, commit, found := strings.Cut(gitRange, "..")
	if !found || strings.Contains(commit, "..") {
		return fmt.Errorf("invalid git range %q: expected <previous>..<commit>", gitRange)
	}
	if !gitRevisionPattern.MatchString(previous) || !gitRevisionPattern.MatchString(commit) {
		return fmt.Errorf("invalid git range %q: revisions contain unsupported characters", gitRange)
	}
	return nil
}

// ValidateReleaseSelector requires either one exact Git range or explicit first-release mode.
func ValidateReleaseSelector(gitRange string, initialRelease bool) error {
	if initialRelease && gitRange != "" {
		return fmt.Errorf("initial release cannot include a git range")
	}
	if !initialRelease && gitRange == "" {
		return fmt.Errorf("git range is required unless initial release mode is explicit")
	}
	if gitRange == "" {
		return nil
	}
	return ValidateReleaseGitRange(gitRange)
}

// CoreReleaseTag returns the stable core tag associated with a prerelease tag.
func CoreReleaseTag(tag string) (string, error) {
	if !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("release tag %q must start with v", tag)
	}
	version, err := semver.StrictNewVersion(strings.TrimPrefix(tag, "v"))
	if err != nil {
		return "", fmt.Errorf("release tag %q must be a strict semantic version: %w", tag, err)
	}
	return fmt.Sprintf("v%d.%d.%d", version.Major(), version.Minor(), version.Patch()), nil
}
