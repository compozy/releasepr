package usecase

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type archiveGitRepoStub struct {
	moves      []ArchivedReleaseNoteMove
	fsRepo     afero.Fs
	failOnCall int
	moveCalls  int
}

func (s *archiveGitRepoStub) LatestTag(context.Context) (string, error) {
	return "", nil
}

func (s *archiveGitRepoStub) CommitsSinceTag(context.Context, string) (int, error) {
	return 0, nil
}

func (s *archiveGitRepoStub) TagExists(context.Context, string) (bool, error) {
	return false, nil
}

func (s *archiveGitRepoStub) CreateBranch(context.Context, string) error {
	return nil
}

func (s *archiveGitRepoStub) CreateTag(context.Context, string, string) error {
	return nil
}

func (s *archiveGitRepoStub) PushTag(context.Context, string) error {
	return nil
}

func (s *archiveGitRepoStub) PushBranch(context.Context, string) error {
	return nil
}

func (s *archiveGitRepoStub) CheckoutBranch(context.Context, string) error {
	return nil
}

func (s *archiveGitRepoStub) ConfigureUser(context.Context, string, string) error {
	return nil
}

func (s *archiveGitRepoStub) AddFiles(context.Context, string) error {
	return nil
}

func (s *archiveGitRepoStub) Commit(context.Context, string) error {
	return nil
}

func (s *archiveGitRepoStub) GetHeadCommit(context.Context) (string, error) {
	return "", nil
}

func (s *archiveGitRepoStub) GetCurrentBranch(context.Context) (string, error) {
	return "main", nil
}

func (s *archiveGitRepoStub) PushBranchForce(context.Context, string) error {
	return nil
}

func (s *archiveGitRepoStub) DeleteBranch(context.Context, string) error {
	return nil
}

func (s *archiveGitRepoStub) DeleteRemoteBranch(context.Context, string) error {
	return nil
}

func (s *archiveGitRepoStub) ListLocalBranches(context.Context) ([]string, error) {
	return []string{"main"}, nil
}

func (s *archiveGitRepoStub) ListRemoteBranches(context.Context) ([]string, error) {
	return []string{"origin/main"}, nil
}

func (s *archiveGitRepoStub) RemoteBranchExists(context.Context, string) (bool, error) {
	return false, nil
}

func (s *archiveGitRepoStub) MoveFile(_ context.Context, from, to string) error {
	s.moveCalls++
	if s.failOnCall != 0 && s.moveCalls == s.failOnCall {
		return fmt.Errorf("move failed")
	}
	s.moves = append(s.moves, ArchivedReleaseNoteMove{
		From: from,
		To:   to,
	})
	return s.fsRepo.Rename(from, to)
}

func (s *archiveGitRepoStub) RestoreFile(context.Context, string) error {
	return nil
}

func (s *archiveGitRepoStub) ResetHard(context.Context, string) error {
	return nil
}

func (s *archiveGitRepoStub) GetFileStatus(context.Context, string) (string, error) {
	return "", nil
}

func TestArchiveReleaseNotesUseCase_Execute(t *testing.T) {
	t.Run("Should archive active release notes and create gitkeep", func(t *testing.T) {
		fsRepo := afero.NewMemMapFs()
		require.NoError(t, fsRepo.MkdirAll(releaseNotesDir, 0755))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/a.md", []byte("a"), 0644))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/b.md", []byte("b"), 0644))
		gitRepo := &archiveGitRepoStub{fsRepo: fsRepo}
		uc := &ArchiveReleaseNotesUseCase{
			FSRepo:  fsRepo,
			GitRepo: gitRepo,
		}
		result, err := uc.Execute(t.Context(), "v1.2.3")
		require.NoError(t, err)
		require.Len(t, result.Moves, 2)
		assert.True(t, result.GitKeepCreated)
		exists, existsErr := afero.Exists(fsRepo, ".release-notes/archive/v1.2.3/a.md")
		require.NoError(t, existsErr)
		assert.True(t, exists)
		exists, existsErr = afero.Exists(fsRepo, ".release-notes/.gitkeep")
		require.NoError(t, existsErr)
		assert.True(t, exists)
	})
	t.Run("Should return empty result when directory does not exist", func(t *testing.T) {
		fsRepo := afero.NewMemMapFs()
		gitRepo := &archiveGitRepoStub{fsRepo: fsRepo}
		uc := &ArchiveReleaseNotesUseCase{
			FSRepo:  fsRepo,
			GitRepo: gitRepo,
		}
		result, err := uc.Execute(t.Context(), "v1.2.3")
		require.NoError(t, err)
		assert.Empty(t, result.Moves)
		assert.False(t, result.GitKeepCreated)
	})
	t.Run("Should roll back already moved notes when a later archive move fails", func(t *testing.T) {
		fsRepo := afero.NewMemMapFs()
		require.NoError(t, fsRepo.MkdirAll(releaseNotesDir, 0755))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/a.md", []byte("a"), 0644))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/b.md", []byte("b"), 0644))
		gitRepo := &archiveGitRepoStub{
			fsRepo:     fsRepo,
			failOnCall: 2,
		}
		uc := &ArchiveReleaseNotesUseCase{
			FSRepo:  fsRepo,
			GitRepo: gitRepo,
		}
		result, err := uc.Execute(t.Context(), "v1.2.3")
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "failed to archive release note")
		activeAExists, activeAErr := afero.Exists(fsRepo, ".release-notes/a.md")
		require.NoError(t, activeAErr)
		assert.True(t, activeAExists)
		activeBExists, activeBErr := afero.Exists(fsRepo, ".release-notes/b.md")
		require.NoError(t, activeBErr)
		assert.True(t, activeBExists)
		archivedAExists, archivedAErr := afero.Exists(fsRepo, ".release-notes/archive/v1.2.3/a.md")
		require.NoError(t, archivedAErr)
		assert.False(t, archivedAExists)
		gitKeepExists, gitKeepErr := afero.Exists(fsRepo, ".release-notes/.gitkeep")
		require.NoError(t, gitKeepErr)
		assert.False(t, gitKeepExists)
	})
}

func TestParseArchiveReleaseNotesResult(t *testing.T) {
	t.Run("Should rebuild archive rollback data from persisted map", func(t *testing.T) {
		result, err := ParseArchiveReleaseNotesResult(map[string]any{
			"gitkeep_created": true,
			"moves": []any{
				map[string]any{
					"from": ".release-notes/a.md",
					"to":   ".release-notes/archive/v1.2.3/a.md",
				},
			},
		})
		require.NoError(t, err)
		require.Len(t, result.Moves, 1)
		assert.True(t, result.GitKeepCreated)
		assert.Equal(t, ".release-notes/a.md", result.Moves[0].From)
		assert.Equal(t, ".release-notes/archive/v1.2.3/a.md", result.Moves[0].To)
	})
}
