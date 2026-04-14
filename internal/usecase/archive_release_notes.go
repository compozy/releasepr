package usecase

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/compozy/releasepr/internal/repository"
	"github.com/spf13/afero"
)

// ArchivedReleaseNoteMove stores one source and destination pair for rollback.
type ArchivedReleaseNoteMove struct {
	From string
	To   string
}

// ArchiveReleaseNotesResult contains the serialized rollback state for archived notes.
type ArchiveReleaseNotesResult struct {
	Moves          []ArchivedReleaseNoteMove
	GitKeepCreated bool
}

// ArchiveReleaseNotesUseCase moves active release notes into the versioned archive directory.
type ArchiveReleaseNotesUseCase struct {
	FSRepo  repository.FileSystemRepository
	GitRepo repository.GitExtendedRepository
}

// Execute archives all active release notes for the provided version.
func (uc *ArchiveReleaseNotesUseCase) Execute(ctx context.Context, version string) (*ArchiveReleaseNotesResult, error) {
	trimmedVersion := strings.TrimSpace(version)
	if trimmedVersion == "" {
		return nil, fmt.Errorf("version cannot be empty")
	}
	exists, err := afero.DirExists(uc.fsRepo(), releaseNotesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect release notes directory: %w", err)
	}
	if !exists {
		return &ArchiveReleaseNotesResult{}, nil
	}
	files, err := uc.activeReleaseNoteFiles()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return &ArchiveReleaseNotesResult{}, nil
	}
	archiveDir := releaseNotesArchiveDir(trimmedVersion)
	if err := uc.fsRepo().MkdirAll(archiveDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create release notes archive directory: %w", err)
	}
	result := &ArchiveReleaseNotesResult{}
	for _, file := range files {
		target := filepath.Join(archiveDir, filepath.Base(file))
		if err := uc.GitRepo.MoveFile(ctx, file, target); err != nil {
			if rollbackErr := uc.rollbackMoves(ctx, result.Moves); rollbackErr != nil {
				return nil, errors.Join(
					fmt.Errorf("failed to archive release note %s: %w", file, err),
					fmt.Errorf("failed to roll back archived release notes: %w", rollbackErr),
				)
			}
			return nil, fmt.Errorf("failed to archive release note %s: %w", file, err)
		}
		result.Moves = append(result.Moves, ArchivedReleaseNoteMove{
			From: file,
			To:   target,
		})
	}
	created, err := uc.ensureGitKeep()
	if err != nil {
		if rollbackErr := uc.rollbackMoves(ctx, result.Moves); rollbackErr != nil {
			return nil, errors.Join(err, fmt.Errorf("failed to roll back archived release notes: %w", rollbackErr))
		}
		return nil, err
	}
	result.GitKeepCreated = created
	return result, nil
}

// ToRollbackData converts the archive result into JSON-friendly saga data.
func (r ArchiveReleaseNotesResult) ToRollbackData() map[string]any {
	moves := make([]map[string]any, 0, len(r.Moves))
	for _, move := range r.Moves {
		moves = append(moves, map[string]any{
			"from": move.From,
			"to":   move.To,
		})
	}
	return map[string]any{
		"moves":           moves,
		"gitkeep_created": r.GitKeepCreated,
	}
}

// ParseArchiveReleaseNotesResult reconstructs rollback data loaded from persisted saga state.
func ParseArchiveReleaseNotesResult(data map[string]any) (*ArchiveReleaseNotesResult, error) {
	result := &ArchiveReleaseNotesResult{}
	if gitKeepCreated, ok := data["gitkeep_created"].(bool); ok {
		result.GitKeepCreated = gitKeepCreated
	}
	rawMoves, ok := data["moves"]
	if !ok {
		return result, nil
	}
	moveItems, ok := rawMoves.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid moves rollback data")
	}
	for _, item := range moveItems {
		moveMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid move entry rollback data")
		}
		from, fromOK := moveMap["from"].(string)
		to, toOK := moveMap["to"].(string)
		if !fromOK || !toOK {
			return nil, fmt.Errorf("invalid move path rollback data")
		}
		result.Moves = append(result.Moves, ArchivedReleaseNoteMove{
			From: from,
			To:   to,
		})
	}
	return result, nil
}

func (uc *ArchiveReleaseNotesUseCase) activeReleaseNoteFiles() ([]string, error) {
	entries, err := afero.ReadDir(uc.fsRepo(), releaseNotesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read release notes directory: %w", err)
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		files = append(files, filepath.Join(releaseNotesDir, entry.Name()))
	}
	return files, nil
}

func (uc *ArchiveReleaseNotesUseCase) ensureGitKeep() (bool, error) {
	gitKeepPath := releaseNotesGitKeepPath()
	exists, err := afero.Exists(uc.fsRepo(), gitKeepPath)
	if err != nil {
		return false, fmt.Errorf("failed to inspect release notes gitkeep: %w", err)
	}
	if exists {
		return false, nil
	}
	if err := afero.WriteFile(uc.fsRepo(), gitKeepPath, []byte(""), 0644); err != nil {
		return false, fmt.Errorf("failed to create release notes gitkeep: %w", err)
	}
	return true, nil
}

func (uc *ArchiveReleaseNotesUseCase) rollbackMoves(ctx context.Context, moves []ArchivedReleaseNoteMove) error {
	rollbackErrors := make([]error, 0)
	for index := len(moves) - 1; index >= 0; index-- {
		move := moves[index]
		exists, err := afero.Exists(uc.fsRepo(), move.To)
		if err != nil {
			rollbackErrors = append(
				rollbackErrors,
				fmt.Errorf("failed to inspect archived release note %s: %w", move.To, err),
			)
			continue
		}
		if !exists {
			continue
		}
		if err := uc.GitRepo.MoveFile(ctx, move.To, move.From); err != nil {
			rollbackErrors = append(
				rollbackErrors,
				fmt.Errorf("failed to restore archived release note %s: %w", move.To, err),
			)
		}
	}
	if len(rollbackErrors) == 0 {
		return nil
	}
	return errors.Join(rollbackErrors...)
}

func (uc *ArchiveReleaseNotesUseCase) fsRepo() repository.FileSystemRepository {
	if uc.FSRepo != nil {
		return uc.FSRepo
	}
	return afero.NewOsFs()
}
