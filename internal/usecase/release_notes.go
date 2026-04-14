package usecase

import "path/filepath"

const (
	releaseNotesDir            = ".release-notes"
	releaseNotesArchiveDirName = "archive"
	releaseNotesOutputFile     = "RELEASE_NOTES.md"
	releaseNotesGitKeepFile    = ".gitkeep"
	defaultReleaseNoteSlug     = "release-note"
	releaseNotesTemplateBody   = "<!-- Write your release note here. Supports full markdown including code blocks. -->"
)

func releaseNotesArchiveDir(version string) string {
	return filepath.Join(releaseNotesDir, releaseNotesArchiveDirName, version)
}

func releaseNotesGitKeepPath() string {
	return filepath.Join(releaseNotesDir, releaseNotesGitKeepFile)
}
