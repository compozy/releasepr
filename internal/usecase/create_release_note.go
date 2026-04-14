package usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/compozy/releasepr/internal/domain"
	"github.com/compozy/releasepr/internal/repository"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

var releaseNoteSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// CreateReleaseNoteInput contains the inputs required to create a release note file.
type CreateReleaseNoteInput struct {
	Title string
	Type  string
	Body  string
}

// CreateReleaseNoteUseCase creates a new release note markdown file on disk.
type CreateReleaseNoteUseCase struct {
	FSRepo repository.FileSystemRepository
	Now    func() time.Time
}

// Execute creates the release note file and returns its relative path.
func (uc *CreateReleaseNoteUseCase) Execute(_ context.Context, input CreateReleaseNoteInput) (string, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return "", fmt.Errorf("title cannot be empty")
	}
	noteType, err := domain.ParseReleaseNoteType(input.Type)
	if err != nil {
		return "", err
	}
	if err := uc.ensureReleaseNotesDir(); err != nil {
		return "", err
	}
	filename := uc.releaseNoteFilename(title)
	path := filepath.Join(releaseNotesDir, filename)
	content, err := buildReleaseNoteFileContent(title, noteType, input.Body)
	if err != nil {
		return "", fmt.Errorf("failed to build release note content: %w", err)
	}
	if err := afero.WriteFile(uc.fsRepo(), path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write release note file: %w", err)
	}
	return path, nil
}

func (uc *CreateReleaseNoteUseCase) ensureReleaseNotesDir() error {
	if err := uc.fsRepo().MkdirAll(releaseNotesDir, 0755); err != nil {
		return fmt.Errorf("failed to create release notes directory: %w", err)
	}
	return nil
}

func (uc *CreateReleaseNoteUseCase) releaseNoteFilename(title string) string {
	timestamp := uc.now().Unix()
	return fmt.Sprintf("%s-%d.md", slugifyReleaseNoteTitle(title), timestamp)
}

func (uc *CreateReleaseNoteUseCase) now() time.Time {
	if uc.Now != nil {
		return uc.Now()
	}
	return time.Now()
}

func (uc *CreateReleaseNoteUseCase) fsRepo() repository.FileSystemRepository {
	if uc.FSRepo != nil {
		return uc.FSRepo
	}
	return afero.NewOsFs()
}

func slugifyReleaseNoteTitle(title string) string {
	slug := releaseNoteSlugPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(title)), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return defaultReleaseNoteSlug
	}
	return slug
}

func buildReleaseNoteFileContent(title string, noteType domain.ReleaseNoteType, body string) (string, error) {
	renderedBody := strings.TrimSpace(body)
	if renderedBody == "" {
		renderedBody = releaseNotesTemplateBody
	}
	frontmatter, err := yaml.Marshal(releaseNoteFrontmatter{
		Title: title,
		Type:  string(noteType),
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal release note frontmatter: %w", err)
	}
	return fmt.Sprintf("---\n%s---\n\n%s\n", frontmatter, renderedBody), nil
}
