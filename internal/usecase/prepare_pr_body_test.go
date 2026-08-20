package usecase

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/compozy/releasepr/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparePRBodyUseCase_Execute(t *testing.T) {
	t.Run("Should preserve the exact body when it fits", func(t *testing.T) {
		t.Parallel()
		uc := &PreparePRBodyUseCase{}
		version, err := domain.NewVersion("v1.0.0")
		require.NoError(t, err)
		release := &domain.Release{
			Version:      version,
			Changelog:    "### Features\n- New feature",
			ReleaseNotes: "### Release Notes\n\n#### Highlights\n\n##### Shared layout package\nMore details here.",
		}
		body, err := uc.Execute(t.Context(), release)
		require.NoError(t, err)
		assert.Equal(t, `
## Release v1.0.0

This PR prepares the release of version v1.0.0.

### Changelog

### Features
- New feature

### Release Notes

#### Highlights

##### Shared layout package
More details here.
`, body)
	})
	t.Run("Should bound an oversized body while identifying every release note", func(t *testing.T) {
		t.Parallel()
		uc := &PreparePRBodyUseCase{}
		version, err := domain.NewVersion("v0.3.0")
		require.NoError(t, err)
		var releaseNotes strings.Builder
		titles := make([]string, 0, 51)
		for index := 1; index <= 51; index++ {
			title := fmt.Sprintf("Release note %02d", index)
			titles = append(titles, title)
			fmt.Fprintf(&releaseNotes, "##### %s\n%s\n\n",
				title,
				strings.Repeat("Detailed release note content. ", 55))
		}
		release := &domain.Release{
			Version:      version,
			Changelog:    strings.Repeat("- fix: release change\n", 250),
			ReleaseNotes: releaseNotes.String(),
		}
		body, err := uc.Execute(t.Context(), release)
		require.NoError(t, err)
		assert.LessOrEqual(t, utf8.RuneCountInString(body), 65_536)
		assert.Contains(t, body, "RELEASE_BODY.md")
		assert.Contains(t, body, "RELEASE_NOTES.md")
		for _, title := range titles {
			assert.Contains(t, body, title)
		}
	})
	t.Run("Should bound a changelog that exceeds the pull request limit", func(t *testing.T) {
		t.Parallel()
		uc := &PreparePRBodyUseCase{}
		version, err := domain.NewVersion("v1.0.0")
		require.NoError(t, err)
		release := &domain.Release{
			Version:   version,
			Changelog: strings.Repeat("界", 65_537),
		}
		body, err := uc.Execute(t.Context(), release)
		require.NoError(t, err)
		assert.LessOrEqual(t, utf8.RuneCountInString(body), 65_536)
		assert.Contains(t, body, "RELEASE_BODY.md")
	})
	t.Run("Should prepare PR body with release information", func(t *testing.T) {
		uc := &PreparePRBodyUseCase{}
		version, _ := domain.NewVersion("v1.0.0")
		release := &domain.Release{
			Version:      version,
			Changelog:    "### Features\n- New feature\n### Bug Fixes\n- Fixed bug",
			ReleaseNotes: "### Release Notes\n\n#### Highlights\n\n##### Shared layout package\nMore details here.",
		}
		body, err := uc.Execute(t.Context(), release)
		require.NoError(t, err)
		assert.Contains(t, body, "Release v1.0.0")
		assert.Contains(t, body, "This PR prepares the release of version v1.0.0")
		assert.Contains(t, body, "### Features")
		assert.Contains(t, body, "- New feature")
		assert.Contains(t, body, "### Bug Fixes")
		assert.Contains(t, body, "- Fixed bug")
		assert.Contains(t, body, "### Release Notes")
		assert.Contains(t, body, "##### Shared layout package")
	})
	t.Run("Should handle empty changelog", func(t *testing.T) {
		uc := &PreparePRBodyUseCase{}
		version, _ := domain.NewVersion("v0.1.0")
		release := &domain.Release{
			Version:      version,
			Changelog:    "",
			ReleaseNotes: "",
		}
		body, err := uc.Execute(t.Context(), release)
		require.NoError(t, err)
		assert.Contains(t, body, "Release v0.1.0")
		assert.Contains(t, body, "### Changelog")
		// Check that after Changelog header there's no content
		lines := strings.Split(body, "\n")
		changelogIndex := -1
		for i, line := range lines {
			if strings.Contains(line, "### Changelog") {
				changelogIndex = i
				break
			}
		}
		assert.NotEqual(t, -1, changelogIndex)
		// Next line after changelog should be empty or end of string
		if changelogIndex+1 < len(lines) {
			assert.Equal(t, "", strings.TrimSpace(lines[changelogIndex+1]))
		}
	})
	t.Run("Should format multi-line changelog correctly", func(t *testing.T) {
		uc := &PreparePRBodyUseCase{}
		version, _ := domain.NewVersion("v2.0.0")
		release := &domain.Release{
			Version: version,
			Changelog: `## [2.0.0] - 2024-01-01

### Added
- New API endpoints
- Documentation improvements

### Changed
- Updated dependencies
- Refactored core module

### Fixed
- Memory leak in worker process
- Race condition in cache`,
			ReleaseNotes: "### Release Notes\n\n#### Features\n\n##### Templated example\nLiteral braces: {{ .Value }}",
		}
		body, err := uc.Execute(t.Context(), release)
		require.NoError(t, err)
		assert.Contains(t, body, "Release v2.0.0")
		assert.Contains(t, body, "### Added")
		assert.Contains(t, body, "- New API endpoints")
		assert.Contains(t, body, "### Changed")
		assert.Contains(t, body, "- Updated dependencies")
		assert.Contains(t, body, "### Fixed")
		assert.Contains(t, body, "- Memory leak in worker process")
		assert.Contains(t, body, "{{ .Value }}")
	})
	t.Run("Should preserve literal script and javascript text in markdown", func(t *testing.T) {
		uc := &PreparePRBodyUseCase{}
		version, _ := domain.NewVersion("v2.0.0")
		release := &domain.Release{
			Version: version,
			Changelog: `### Fixes
- fix: block javascript: URLs in redirect params`,
			ReleaseNotes: "### Release Notes\n\n```html\n<script>alert('xss')</script>\n```\n",
		}
		body, err := uc.Execute(t.Context(), release)
		require.NoError(t, err)
		assert.Contains(t, body, "javascript: URLs")
		assert.Contains(t, body, "<script>alert('xss')</script>")
	})
	t.Run("Should reject markdown content with null bytes", func(t *testing.T) {
		uc := &PreparePRBodyUseCase{}
		version, _ := domain.NewVersion("v2.0.0")
		release := &domain.Release{
			Version:      version,
			Changelog:    "### Features\nbad\x00value",
			ReleaseNotes: "",
		}
		body, err := uc.Execute(t.Context(), release)
		require.Error(t, err)
		assert.Empty(t, body)
		assert.ErrorContains(t, err, "changelog contains invalid null byte")
	})
}
