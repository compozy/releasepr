package domain

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// ReleaseChannel identifies an explicit publication policy.
type ReleaseChannel string

const (
	ReleaseChannelBeta   ReleaseChannel = "beta"
	ReleaseChannelStable ReleaseChannel = "stable"
	ReleaseChannelLegacy ReleaseChannel = "legacy"
)

// ReleasePlan is the canonical, read-only identity and publication policy for one release.
type ReleasePlan struct {
	Ref                string         `json:"ref"`
	Commit             string         `json:"commit"`
	Version            string         `json:"version"`
	Tag                string         `json:"tag"`
	PreviousTag        string         `json:"previous_tag"`
	GitRange           string         `json:"git_range"`
	InitialRelease     bool           `json:"initial_release"`
	Channel            ReleaseChannel `json:"channel"`
	GitHubPrerelease   bool           `json:"github_prerelease"`
	GitHubMakeLatest   bool           `json:"github_make_latest"`
	NPMTag             string         `json:"npm_tag"`
	HomebrewSkipUpload bool           `json:"homebrew_skip_upload"`
}

type publicationPolicy struct {
	githubPrerelease   bool
	githubMakeLatest   bool
	npmTag             string
	homebrewSkipUpload bool
}

// ParseReleaseChannel parses one of the supported explicit release channels.
func ParseReleaseChannel(value string) (ReleaseChannel, error) {
	channel := ReleaseChannel(value)
	switch channel {
	case ReleaseChannelBeta, ReleaseChannelStable, ReleaseChannelLegacy:
		return channel, nil
	default:
		return "", fmt.Errorf("unsupported release channel %q: expected beta, stable, or legacy", value)
	}
}

// NewReleasePlan validates explicit release identity and derives its publication policy.
func NewReleasePlan(
	ref string,
	commit string,
	version string,
	previousTag string,
	channel ReleaseChannel,
) (*ReleasePlan, error) {
	if err := validateReleaseRef(ref); err != nil {
		return nil, err
	}
	if strings.TrimSpace(commit) == "" {
		return nil, fmt.Errorf("release commit is required")
	}
	if strings.HasPrefix(version, "v") {
		return nil, fmt.Errorf("release version must not start with v")
	}
	parsedVersion, err := semver.StrictNewVersion(version)
	if err != nil {
		return nil, fmt.Errorf("release version %q must be a strict semantic version: %w", version, err)
	}
	policy, err := policyForRelease(channel, parsedVersion)
	if err != nil {
		return nil, err
	}
	gitRange, err := BuildReleaseGitRange(previousTag, commit)
	if err != nil {
		return nil, err
	}
	return &ReleasePlan{
		Ref:                ref,
		Commit:             commit,
		Version:            version,
		Tag:                "v" + version,
		PreviousTag:        previousTag,
		GitRange:           gitRange,
		InitialRelease:     previousTag == "",
		Channel:            channel,
		GitHubPrerelease:   policy.githubPrerelease,
		GitHubMakeLatest:   policy.githubMakeLatest,
		NPMTag:             policy.npmTag,
		HomebrewSkipUpload: policy.homebrewSkipUpload,
	}, nil
}

func validateReleaseRef(ref string) error {
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("release ref is required")
	}
	if strings.TrimSpace(ref) != ref || strings.ContainsAny(ref, "\r\n") {
		return fmt.Errorf("release ref %q contains whitespace or control characters", ref)
	}
	return nil
}

func policyForRelease(channel ReleaseChannel, version *semver.Version) (publicationPolicy, error) {
	switch channel {
	case ReleaseChannelBeta:
		if prerelease := version.Prerelease(); prerelease == "" || strings.Split(prerelease, ".")[0] != "beta" {
			return publicationPolicy{}, fmt.Errorf("beta channel requires a beta semantic-version prerelease")
		}
		return publicationPolicy{
			githubPrerelease:   true,
			npmTag:             "beta",
			homebrewSkipUpload: true,
		}, nil
	case ReleaseChannelStable, ReleaseChannelLegacy:
		if version.Prerelease() != "" {
			return publicationPolicy{}, fmt.Errorf(
				"%s channel requires a semantic version without prerelease data",
				channel,
			)
		}
		return publicationPolicy{
			githubMakeLatest: true,
			npmTag:           "latest",
		}, nil
	default:
		return publicationPolicy{}, fmt.Errorf("unsupported release channel %q", channel)
	}
}
