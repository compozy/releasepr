package usecase

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const githubPullRequestBodyMaxCharacters = 65_536

const pullRequestBodyTruncationMarker = "\n\n_Changelog shortened here; read the complete text in `RELEASE_BODY.md`._"

func boundPullRequestBody(body, version, changelog, releaseNotes string) string {
	if utf8.RuneCountInString(body) <= githubPullRequestBodyMaxCharacters {
		return body
	}
	header := fmt.Sprintf(`
## Release %s

This PR prepares the release of version %s.

### Changelog

`, version, version)
	footerBudget := githubPullRequestBodyMaxCharacters - utf8.RuneCountInString(header)
	footer := boundedPullRequestFooter(releaseNotes, footerBudget)
	changelogBudget := githubPullRequestBodyMaxCharacters - utf8.RuneCountInString(header+footer)
	return header + truncateMarkdown(strings.TrimSpace(changelog), changelogBudget) + footer
}

func boundedPullRequestFooter(releaseNotes string, characterBudget int) string {
	titles := releaseNoteTitles(releaseNotes)
	details := `

### Full release details

The complete changelog and unabridged release notes are in ` +
		"[`RELEASE_BODY.md`](./RELEASE_BODY.md) and [`RELEASE_NOTES.md`](./RELEASE_NOTES.md) in this pull request."
	summary := ""
	if strings.TrimSpace(releaseNotes) != "" {
		summary = releaseNoteTitleSummary(titles)
	}
	if utf8.RuneCountInString(summary+details) <= characterBudget {
		return summary + details + "\n"
	}
	return fmt.Sprintf(
		"\n\n### Release notes included\n\n%d release notes; see the complete files below.%s\n",
		len(titles),
		details,
	)
}

func releaseNoteTitleSummary(titles []string) string {
	if len(titles) == 0 {
		return "\n\n### Release notes included\n\nRelease notes are included in the complete files below."
	}
	var summary strings.Builder
	summary.WriteString("\n\n### Release notes included\n")
	for _, title := range titles {
		summary.WriteString("\n- ")
		summary.WriteString(title)
	}
	return summary.String()
}

func releaseNoteTitles(markdown string) []string {
	lines := strings.Split(markdown, "\n")
	titles := make([]string, 0)
	inCodeFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inCodeFence = !inCodeFence
			continue
		}
		if inCodeFence || !strings.HasPrefix(trimmed, "##### ") {
			continue
		}
		titles = append(titles, strings.TrimSpace(strings.TrimPrefix(trimmed, "##### ")))
	}
	return titles
}

func truncateMarkdown(markdown string, characterBudget int) string {
	if characterBudget <= 0 {
		return ""
	}
	runes := []rune(markdown)
	if len(runes) <= characterBudget {
		return markdown
	}
	marker := []rune(pullRequestBodyTruncationMarker)
	if len(marker) >= characterBudget {
		return string(marker[:characterBudget])
	}
	return string(runes[:characterBudget-len(marker)]) + pullRequestBodyTruncationMarker
}
