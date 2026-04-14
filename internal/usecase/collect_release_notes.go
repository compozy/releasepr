package usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/compozy/releasepr/internal/domain"
	"github.com/compozy/releasepr/internal/logger"
	"github.com/compozy/releasepr/internal/repository"
	"github.com/spf13/afero"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type releaseNoteFrontmatter struct {
	Title string `yaml:"title"`
	Type  string `yaml:"type"`
}

// CollectReleaseNotesUseCase loads and renders the release notes relevant to one release version.
type CollectReleaseNotesUseCase struct {
	FSRepo repository.FileSystemRepository
}

// Execute returns the valid release notes found in `.release-notes/` and the matching version archive.
func (uc *CollectReleaseNotesUseCase) Execute(
	ctx context.Context,
	version string,
) (*domain.ReleaseNotesCollection, error) {
	paths, err := uc.releaseNotePaths(version)
	if err != nil {
		return nil, err
	}
	notesByType := make(map[domain.ReleaseNoteType][]domain.ReleaseNote)
	warnings := []string{}
	log := logger.FromContext(ctx).Named("usecase.collect_release_notes")
	for _, path := range paths {
		note, parseErr := uc.parseReleaseNote(path)
		if parseErr != nil {
			warning := fmt.Sprintf("%s: %v", path, parseErr)
			warnings = append(warnings, warning)
			log.Warn("Skipping invalid release note", zap.String("path", path), zap.Error(parseErr))
			continue
		}
		notesByType[note.Type] = append(notesByType[note.Type], *note)
	}
	orderedNotes := make([]domain.ReleaseNote, 0)
	for _, noteType := range domain.OrderedReleaseNoteTypes() {
		notes := append([]domain.ReleaseNote(nil), notesByType[noteType]...)
		sort.Slice(notes, func(i, j int) bool {
			leftName := filepath.Base(notes[i].SourcePath)
			rightName := filepath.Base(notes[j].SourcePath)
			if leftName == rightName {
				return notes[i].SourcePath < notes[j].SourcePath
			}
			return leftName < rightName
		})
		orderedNotes = append(orderedNotes, notes...)
	}
	return &domain.ReleaseNotesCollection{
		Notes:    orderedNotes,
		Warnings: warnings,
	}, nil
}

func (uc *CollectReleaseNotesUseCase) releaseNotePaths(version string) ([]string, error) {
	trimmedVersion := strings.TrimSpace(version)
	directories := []string{releaseNotesDir}
	if trimmedVersion != "" {
		directories = append(directories, releaseNotesArchiveDir(trimmedVersion))
	}
	paths := make([]string, 0)
	for _, directory := range directories {
		exists, err := afero.DirExists(uc.fsRepo(), directory)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect release notes directory %s: %w", directory, err)
		}
		if !exists {
			continue
		}
		entries, err := afero.ReadDir(uc.fsRepo(), directory)
		if err != nil {
			return nil, fmt.Errorf("failed to read release notes directory %s: %w", directory, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	return paths, nil
}

func (uc *CollectReleaseNotesUseCase) parseReleaseNote(path string) (*domain.ReleaseNote, error) {
	data, err := afero.ReadFile(uc.fsRepo(), path)
	if err != nil {
		return nil, fmt.Errorf("failed to read release note: %w", err)
	}
	frontmatter, body, err := splitReleaseNoteFrontmatter(string(data))
	if err != nil {
		return nil, err
	}
	var metadata releaseNoteFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}
	title := strings.TrimSpace(metadata.Title)
	if title == "" {
		return nil, fmt.Errorf("title cannot be empty")
	}
	noteType, err := domain.ParseReleaseNoteType(metadata.Type)
	if err != nil {
		return nil, err
	}
	renderedBody := strings.TrimSpace(body)
	if renderedBody == "" {
		return nil, fmt.Errorf("body cannot be empty")
	}
	return &domain.ReleaseNote{
		Title:      title,
		Type:       noteType,
		Body:       renderedBody,
		SourcePath: path,
	}, nil
}

func (uc *CollectReleaseNotesUseCase) fsRepo() repository.FileSystemRepository {
	if uc.FSRepo != nil {
		return uc.FSRepo
	}
	return afero.NewOsFs()
}

func splitReleaseNoteFrontmatter(content string) (string, string, error) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", "", fmt.Errorf("missing frontmatter header")
	}
	rest := strings.TrimPrefix(normalized, "---\n")
	endIndex := strings.Index(rest, "\n---\n")
	if endIndex == -1 {
		return "", "", fmt.Errorf("missing frontmatter footer")
	}
	frontmatter := strings.TrimSpace(rest[:endIndex])
	body := rest[endIndex+5:]
	return frontmatter, body, nil
}
