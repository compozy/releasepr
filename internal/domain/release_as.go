package domain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

var releaseAsFooterPattern = regexp.MustCompile(`(?mi)^[ \t]*Release-As:[ \t]*(\S+)[ \t]*$`)

// ReleaseAsVersion returns the version declared by Release-As footers, or nil when none is declared.
// Every footer must name the same strict semantic version.
func ReleaseAsVersion(messages []string) (*Version, error) {
	var declared *Version
	for _, message := range messages {
		for _, match := range releaseAsFooterPattern.FindAllStringSubmatch(message, -1) {
			raw := strings.TrimPrefix(match[1], "v")
			parsed, err := semver.StrictNewVersion(raw)
			if err != nil {
				return nil, fmt.Errorf("release-as footer %q must be a strict semantic version: %w", match[1], err)
			}
			version := &Version{parsed}
			if declared != nil && !declared.Equal(version.Version) {
				return nil, fmt.Errorf(
					"conflicting release-as footers declare %s and %s",
					declared.String(),
					version.String(),
				)
			}
			declared = version
		}
	}
	return declared, nil
}

// ResolveReleaseAs applies a declared version over the computed one, refusing to move backwards.
func ResolveReleaseAs(computed *Version, declared *Version) (*Version, error) {
	if declared == nil {
		return computed, nil
	}
	if computed != nil && declared.LessThan(computed.Version) {
		return nil, fmt.Errorf(
			"release-as version %s is lower than the computed version %s",
			declared.String(),
			computed.String(),
		)
	}
	return declared, nil
}
