package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepo(t *testing.T) (string, *git.Repository) {
	dir, err := os.MkdirTemp("", "git-test-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	// Create initial commit
	wt, err := repo.Worktree()
	require.NoError(t, err)
	testFile := filepath.Join(dir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)
	_, err = wt.Add("test.txt")
	require.NoError(t, err)
	_, err = wt.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)
	return dir, repo
}

func TestNewGitRepository(t *testing.T) {
	t.Run("Should create git repository for existing repo", func(t *testing.T) {
		dir, _ := setupTestRepo(t)
		oldPwd, _ := os.Getwd()
		err := os.Chdir(dir)
		require.NoError(t, err)
		defer os.Chdir(oldPwd)
		gitRepo, err := NewGitRepository()
		assert.NoError(t, err)
		assert.NotNil(t, gitRepo)
	})
	t.Run("Should return error for non-git directory", func(t *testing.T) {
		dir, err := os.MkdirTemp("", "non-git-*")
		require.NoError(t, err)
		defer os.RemoveAll(dir)
		oldPwd, _ := os.Getwd()
		err = os.Chdir(dir)
		require.NoError(t, err)
		defer os.Chdir(oldPwd)
		gitRepo, err := NewGitRepository()
		assert.Error(t, err)
		assert.Nil(t, gitRepo)
	})
}

func TestGitRepository_LatestTag(t *testing.T) {
	t.Run("Should return latest tag when tags exist", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		oldPwd, _ := os.Getwd()
		err := os.Chdir(dir)
		require.NoError(t, err)
		defer os.Chdir(oldPwd)
		// Create a tag
		head, err := repo.Head()
		require.NoError(t, err)
		_, err = repo.CreateTag("v1.0.0", head.Hash(), &git.CreateTagOptions{
			Message: "Release v1.0.0",
			Tagger: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)
		gitRepo := &gitRepository{repo: repo}
		tag, err := gitRepo.LatestTag(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "v1.0.0", tag)
	})
	t.Run("Should return empty string when no tags exist", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		oldPwd, _ := os.Getwd()
		err := os.Chdir(dir)
		require.NoError(t, err)
		defer os.Chdir(oldPwd)
		gitRepo := &gitRepository{repo: repo}
		tag, err := gitRepo.LatestTag(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "", tag)
	})
}

func TestGitRepository_TagExists(t *testing.T) {
	t.Run("Should return true when tag exists", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		oldPwd, _ := os.Getwd()
		err := os.Chdir(dir)
		require.NoError(t, err)
		defer os.Chdir(oldPwd)
		head, err := repo.Head()
		require.NoError(t, err)
		_, err = repo.CreateTag("v1.0.0", head.Hash(), nil)
		require.NoError(t, err)
		gitRepo := &gitRepository{repo: repo}
		exists, err := gitRepo.TagExists(context.Background(), "v1.0.0")
		assert.NoError(t, err)
		assert.True(t, exists)
	})
	t.Run("Should return false when tag does not exist", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		oldPwd, _ := os.Getwd()
		err := os.Chdir(dir)
		require.NoError(t, err)
		defer os.Chdir(oldPwd)
		gitRepo := &gitRepository{repo: repo}
		exists, err := gitRepo.TagExists(context.Background(), "v1.0.0")
		assert.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestGitRepository_ReleaseTagCommit(t *testing.T) {
	t.Run("Should return a lightweight tag target from origin", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		remoteDir := t.TempDir()
		remoteRepo, err := git.PlainInit(remoteDir, true)
		require.NoError(t, err)
		head, err := repo.Head()
		require.NoError(t, err)
		require.NoError(t, remoteRepo.Storer.SetReference(
			plumbing.NewHashReference(plumbing.NewTagReferenceName("v1.0.0"), head.Hash()),
		))
		_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remoteDir}})
		require.NoError(t, err)
		oldPwd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(dir))
		defer func() {
			require.NoError(t, os.Chdir(oldPwd))
		}()
		gitRepo := &gitRepository{repo: repo}
		commit, annotated, err := gitRepo.ReleaseTagCommit(t.Context(), "v1.0.0")
		require.NoError(t, err)
		assert.Equal(t, head.Hash().String(), commit)
		assert.False(t, annotated)
	})
	t.Run("Should return an annotated tag target from origin", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		remoteDir := t.TempDir()
		_, err := git.PlainInit(remoteDir, true)
		require.NoError(t, err)
		_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remoteDir}})
		require.NoError(t, err)
		head, err := repo.Head()
		require.NoError(t, err)
		_, err = repo.CreateTag("v1.0.0", head.Hash(), &git.CreateTagOptions{
			Message: "Release v1.0.0",
			Tagger:  &object.Signature{Name: "Test User", Email: "test@example.com"},
		})
		require.NoError(t, err)
		require.NoError(t, repo.Push(&git.PushOptions{
			RemoteName: "origin",
			RefSpecs: []config.RefSpec{
				config.RefSpec("refs/tags/v1.0.0:refs/tags/v1.0.0"),
			},
		}))
		oldPwd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(dir))
		defer func() {
			require.NoError(t, os.Chdir(oldPwd))
		}()
		gitRepo := &gitRepository{repo: repo}
		commit, annotated, err := gitRepo.ReleaseTagCommit(t.Context(), "v1.0.0")
		require.NoError(t, err)
		assert.Equal(t, head.Hash().String(), commit)
		assert.True(t, annotated)
	})
	t.Run("Should return an empty state when tag is absent locally and on origin", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		remoteDir := t.TempDir()
		_, err := git.PlainInit(remoteDir, true)
		require.NoError(t, err)
		_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remoteDir}})
		require.NoError(t, err)
		oldPwd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(dir))
		defer func() {
			require.NoError(t, os.Chdir(oldPwd))
		}()
		gitRepo := &gitRepository{repo: repo}
		commit, annotated, err := gitRepo.ReleaseTagCommit(t.Context(), "v1.0.0")
		require.NoError(t, err)
		assert.Empty(t, commit)
		assert.False(t, annotated)
	})
	t.Run("Should reject divergent local and origin tags", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		initial, err := repo.Head()
		require.NoError(t, err)
		wt, err := repo.Worktree()
		require.NoError(t, err)
		target := commitFixtureWithParents(t, dir, wt, "target", "fix: target", initial.Hash())
		_, err = repo.CreateTag("v1.0.0", target, &git.CreateTagOptions{
			Message: "Release v1.0.0",
			Tagger:  &object.Signature{Name: "Test User", Email: "test@example.com"},
		})
		require.NoError(t, err)
		remoteDir := t.TempDir()
		remoteRepo, err := git.PlainInit(remoteDir, true)
		require.NoError(t, err)
		require.NoError(t, remoteRepo.Storer.SetReference(
			plumbing.NewHashReference(plumbing.NewTagReferenceName("v1.0.0"), initial.Hash()),
		))
		_, err = repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{remoteDir}})
		require.NoError(t, err)
		oldPwd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(dir))
		defer func() {
			require.NoError(t, os.Chdir(oldPwd))
		}()
		gitRepo := &gitRepository{repo: repo}
		commit, annotated, err := gitRepo.ReleaseTagCommit(t.Context(), "v1.0.0")
		require.Error(t, err)
		assert.Empty(t, commit)
		assert.False(t, annotated)
		assert.ErrorContains(t, err, "differs between the worktree and origin")
	})
	t.Run("Should fail closed when origin is unavailable", func(t *testing.T) {
		_, repo := setupTestRepo(t)
		gitRepo := &gitRepository{repo: repo}
		commit, annotated, err := gitRepo.ReleaseTagCommit(t.Context(), "v1.0.0")
		require.Error(t, err)
		assert.Empty(t, commit)
		assert.False(t, annotated)
		assert.ErrorContains(t, err, "failed to get remote origin")
	})
}

func TestGitRepository_ResolveRevision(t *testing.T) {
	t.Run("Should resolve HEAD to its commit", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		oldPwd, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(dir))
		defer func() {
			require.NoError(t, os.Chdir(oldPwd))
		}()
		head, err := repo.Head()
		require.NoError(t, err)
		gitRepo := &gitRepository{repo: repo}
		commit, err := gitRepo.ResolveRevision(t.Context(), "HEAD")
		require.NoError(t, err)
		assert.Equal(t, head.Hash().String(), commit)
	})
}

func TestGitRepository_PreviousReleaseTag(t *testing.T) {
	t.Run("Should select the nearest reachable tag according to channel policy", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		initial, err := repo.Head()
		require.NoError(t, err)
		_, err = repo.CreateTag("v1.0.0", initial.Hash(), nil)
		require.NoError(t, err)
		wt, err := repo.Worktree()
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("beta one"), 0644))
		_, err = wt.Add("test.txt")
		require.NoError(t, err)
		betaCommit, err := wt.Commit("feat: beta one", &git.CommitOptions{
			Author: &object.Signature{Name: "Test User", Email: "test@example.com"},
		})
		require.NoError(t, err)
		_, err = repo.CreateTag("v1.1.0-beta.1", betaCommit, nil)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("beta two"), 0644))
		_, err = wt.Add("test.txt")
		require.NoError(t, err)
		targetCommit, err := wt.Commit("fix: beta two", &git.CommitOptions{
			Author: &object.Signature{Name: "Test User", Email: "test@example.com"},
		})
		require.NoError(t, err)
		_, err = repo.CreateTag("vpreview", targetCommit, nil)
		require.NoError(t, err)
		gitRepo := &gitRepository{repo: repo}
		betaTag, err := gitRepo.PreviousReleaseTag(t.Context(), targetCommit.String(), true, "")
		require.NoError(t, err)
		assert.Equal(t, "v1.1.0-beta.1", betaTag)
		stableTag, err := gitRepo.PreviousReleaseTag(t.Context(), targetCommit.String(), false, "")
		require.NoError(t, err)
		assert.Equal(t, "v1.0.0", stableTag)
	})

	t.Run("Should return an empty predecessor when no semantic release tag is reachable", func(t *testing.T) {
		_, repo := setupTestRepo(t)
		head, err := repo.Head()
		require.NoError(t, err)
		gitRepo := &gitRepository{repo: repo}
		tag, err := gitRepo.PreviousReleaseTag(t.Context(), head.Hash().String(), true, "")
		require.NoError(t, err)
		assert.Empty(t, tag)
	})
	t.Run("Should exclude the resumed release tag from predecessor selection", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		initial, err := repo.Head()
		require.NoError(t, err)
		_, err = repo.CreateTag("v1.0.0-beta.1", initial.Hash(), nil)
		require.NoError(t, err)
		wt, err := repo.Worktree()
		require.NoError(t, err)
		target := commitFixtureWithParents(t, dir, wt, "target", "fix: target", initial.Hash())
		_, err = repo.CreateTag("v1.0.0-beta.2", target, nil)
		require.NoError(t, err)
		gitRepo := &gitRepository{repo: repo}
		tag, err := gitRepo.PreviousReleaseTag(
			t.Context(),
			target.String(),
			true,
			"v1.0.0-beta.2",
		)
		require.NoError(t, err)
		assert.Equal(t, "v1.0.0-beta.1", tag)
	})
	t.Run("Should ignore a closer tag from a merged side branch", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		wt, err := repo.Worktree()
		require.NoError(t, err)
		base, err := repo.Head()
		require.NoError(t, err)
		mainCommit := commitFixtureWithParents(t, dir, wt, "main", "feat: main release", base.Hash())
		_, err = repo.CreateTag("v1.0.0", mainCommit, &git.CreateTagOptions{
			Message: "Release v1.0.0",
			Tagger:  &object.Signature{Name: "Test User", Email: "test@example.com"},
		})
		require.NoError(t, err)
		sideCommit := commitFixtureWithParents(t, dir, wt, "side", "feat: side release", mainCommit)
		_, err = repo.CreateTag("v9.9.9", sideCommit, &git.CreateTagOptions{
			Message: "Release v9.9.9",
			Tagger:  &object.Signature{Name: "Test User", Email: "test@example.com"},
		})
		require.NoError(t, err)
		mainNext := commitFixtureWithParents(t, dir, wt, "main next", "fix: main follow-up", mainCommit)
		mergeCommit, err := wt.Commit("build: merge side branch", &git.CommitOptions{
			AllowEmptyCommits: true,
			Author:            &object.Signature{Name: "Test User", Email: "test@example.com"},
			Parents:           []plumbing.Hash{mainNext, sideCommit},
		})
		require.NoError(t, err)
		gitRepo := &gitRepository{repo: repo}
		tag, err := gitRepo.PreviousReleaseTag(t.Context(), mergeCommit.String(), false, "")
		require.NoError(t, err)
		assert.Equal(t, "v1.0.0", tag)
	})
}

func commitFixtureWithParents(
	t *testing.T,
	dir string,
	wt *git.Worktree,
	content string,
	message string,
	parents ...plumbing.Hash,
) plumbing.Hash {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644))
	_, err := wt.Add("test.txt")
	require.NoError(t, err)
	commit, err := wt.Commit(message, &git.CommitOptions{
		Author:  &object.Signature{Name: "Test User", Email: "test@example.com"},
		Parents: parents,
	})
	require.NoError(t, err)
	return commit
}

func TestGitRepository_AddedFiles(t *testing.T) {
	t.Run("Should include introduced notes and exclude notes only moved inside the range", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		wt, err := repo.Worktree()
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".release-notes"), 0755))
		activePath := filepath.Join(dir, ".release-notes", "existing.md")
		require.NoError(t, os.WriteFile(activePath, []byte("existing note"), 0644))
		_, err = wt.Add(".release-notes/existing.md")
		require.NoError(t, err)
		baseCommit, err := wt.Commit("docs: add existing note", &git.CommitOptions{
			Author: &object.Signature{Name: "Test User", Email: "test@example.com"},
		})
		require.NoError(t, err)
		archiveDir := filepath.Join(dir, ".release-notes", "archive", "v1.1.0")
		require.NoError(t, os.MkdirAll(archiveDir, 0755))
		archivedPath := filepath.Join(archiveDir, "existing.md")
		require.NoError(t, os.Rename(activePath, archivedPath))
		_, err = wt.Remove(".release-notes/existing.md")
		require.NoError(t, err)
		_, err = wt.Add(".release-notes/archive/v1.1.0/existing.md")
		require.NoError(t, err)
		newPath := filepath.Join(archiveDir, "new.md")
		require.NoError(t, os.WriteFile(newPath, []byte("new note"), 0644))
		_, err = wt.Add(".release-notes/archive/v1.1.0/new.md")
		require.NoError(t, err)
		targetCommit, err := wt.Commit("docs: archive release notes", &git.CommitOptions{
			Author: &object.Signature{Name: "Test User", Email: "test@example.com"},
		})
		require.NoError(t, err)
		gitRepo := &gitRepository{repo: repo}
		paths, err := gitRepo.AddedFiles(
			t.Context(),
			baseCommit.String()+".."+targetCommit.String(),
			".release-notes",
		)
		require.NoError(t, err)
		assert.Equal(t, []string{".release-notes/archive/v1.1.0/new.md"}, paths)
	})
}

func TestGitRepository_RunReadOnlyGit(t *testing.T) {
	t.Run("Should preserve stderr when Git exits unsuccessfully", func(t *testing.T) {
		_, repo := setupTestRepo(t)
		gitRepo := &gitRepository{repo: repo}

		output, err := gitRepo.runReadOnlyGit(t.Context(), "rev-parse", "--verify", "missing")
		require.Error(t, err)
		assert.Empty(t, output)
		assert.ErrorContains(t, err, "stderr: fatal: Needed a single revision")
	})
}

func TestGitRepository_CreateBranch(t *testing.T) {
	t.Run("Should create branch successfully", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		oldPwd, _ := os.Getwd()
		err := os.Chdir(dir)
		require.NoError(t, err)
		defer os.Chdir(oldPwd)
		gitRepo := &gitRepository{repo: repo}
		err = gitRepo.CreateBranch(context.Background(), "feature/test")
		assert.NoError(t, err)
		// Verify branch was created
		ref, err := repo.Reference(plumbing.NewBranchReferenceName("feature/test"), false)
		assert.NoError(t, err)
		assert.NotNil(t, ref)
	})
	t.Run("Should return error for duplicate branch", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		oldPwd, _ := os.Getwd()
		err := os.Chdir(dir)
		require.NoError(t, err)
		defer os.Chdir(oldPwd)
		gitRepo := &gitRepository{repo: repo}
		err = gitRepo.CreateBranch(context.Background(), "feature/test")
		require.NoError(t, err)
		err = gitRepo.CreateBranch(context.Background(), "feature/test")
		assert.Error(t, err)
	})
}

func TestGitRepository_CommitsSinceTag(t *testing.T) {
	t.Run("Should count commits since tag", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		oldPwd, _ := os.Getwd()
		err := os.Chdir(dir)
		require.NoError(t, err)
		defer os.Chdir(oldPwd)
		// Create a tag
		head, err := repo.Head()
		require.NoError(t, err)
		_, err = repo.CreateTag("v1.0.0", head.Hash(), nil)
		require.NoError(t, err)
		// Add more commits
		wt, err := repo.Worktree()
		require.NoError(t, err)
		testFile := filepath.Join(dir, "test2.txt")
		err = os.WriteFile(testFile, []byte("test content 2"), 0644)
		require.NoError(t, err)
		_, err = wt.Add("test2.txt")
		require.NoError(t, err)
		_, err = wt.Commit("Second commit", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
			},
		})
		require.NoError(t, err)
		gitRepo := &gitRepository{repo: repo}
		count, err := gitRepo.CommitsSinceTag(context.Background(), "v1.0.0")
		assert.NoError(t, err)
		assert.Equal(t, 1, count)
	})
	t.Run("Should return error for non-existent tag", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		oldPwd, _ := os.Getwd()
		err := os.Chdir(dir)
		require.NoError(t, err)
		defer os.Chdir(oldPwd)
		gitRepo := &gitRepository{repo: repo}
		count, err := gitRepo.CommitsSinceTag(context.Background(), "v999.0.0")
		assert.Error(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestGitRepository_MoveFile(t *testing.T) {
	t.Run("Should move tracked file with git mv", func(t *testing.T) {
		dir, repo := setupTestRepo(t)
		oldPwd, _ := os.Getwd()
		err := os.Chdir(dir)
		require.NoError(t, err)
		defer os.Chdir(oldPwd)
		gitRepo := &gitRepository{repo: repo}
		err = os.MkdirAll(filepath.Join(dir, ".release-notes", "archive", "v1.0.0"), 0755)
		require.NoError(t, err)
		wt, err := repo.Worktree()
		require.NoError(t, err)
		notePath := filepath.Join(dir, ".release-notes", "note.md")
		err = os.WriteFile(notePath, []byte("note"), 0644)
		require.NoError(t, err)
		_, err = wt.Add(".release-notes/note.md")
		require.NoError(t, err)
		_, err = wt.Commit("Add release note", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
			},
		})
		require.NoError(t, err)
		err = gitRepo.MoveFile(
			context.Background(),
			".release-notes/note.md",
			".release-notes/archive/v1.0.0/note.md",
		)
		require.NoError(t, err)
		_, statErr := os.Stat(filepath.Join(dir, ".release-notes", "archive", "v1.0.0", "note.md"))
		assert.NoError(t, statErr)
		_, statErr = os.Stat(filepath.Join(dir, ".release-notes", "note.md"))
		assert.Error(t, statErr)
	})
}
