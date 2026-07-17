package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/compozy/releasepr/internal/config"
	"github.com/compozy/releasepr/internal/domain"
	"github.com/compozy/releasepr/internal/logger"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func testReleaseContext(t *testing.T) context.Context {
	t.Helper()
	return testReleaseContextWithConfig(t, testReleaseConfig())
}

func testReleaseContextWithConfig(t *testing.T, cfg *config.Config) context.Context {
	t.Helper()
	return config.IntoContext(t.Context(), cfg)
}

func testReleaseConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.GithubOwner = "compozy"
	cfg.GithubRepo = "releasepr"
	return cfg
}

func TestPRReleaseOrchestrator_generateChangelog(t *testing.T) {
	t.Run("Should write release body and preserve historical release notes", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)
		scopedChangelog := "## v1.1.0\n\n### Features\n- Current release"
		fullChangelog := "# Changelog\n\n" + scopedChangelog + "\n\n## v1.0.0\n\n### Features\n- Previous release"
		previousReleaseNotes := "## v1.0.0\n\n### Features\n- Previous release"
		require.NoError(t, afero.WriteFile(fsRepo, "RELEASE_NOTES.md", []byte(previousReleaseNotes), 0644))
		require.NoError(t, fsRepo.MkdirAll(".release-notes", 0755))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/manual.md", []byte(`---
title: Manual upgrade guide
type: highlight
---

Only this release needs these notes.
`), 0644))
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").Return(scopedChangelog, nil).Once()
		cliffSvc.On("GenerateFullChangelog", mock.Anything, "v1.1.0").Return(fullChangelog, nil).Once()
		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		artifacts, err := orch.generateChangelog(ctx, "v1.1.0")
		require.NoError(t, err)
		assert.Equal(t, scopedChangelog, artifacts.changelog)
		assert.Contains(t, artifacts.releaseNotes, "Only this release needs these notes.")
		changelogData, err := afero.ReadFile(fsRepo, "CHANGELOG.md")
		require.NoError(t, err)
		assert.Equal(t, fullChangelog, string(changelogData))
		releaseBodyData, err := afero.ReadFile(fsRepo, "RELEASE_BODY.md")
		require.NoError(t, err)
		releaseBodyDocument := string(releaseBodyData)
		assert.Contains(t, releaseBodyDocument, scopedChangelog)
		assert.Contains(t, releaseBodyDocument, "### Release Notes")
		assert.Contains(t, releaseBodyDocument, "Only this release needs these notes.")
		assert.NotContains(t, releaseBodyDocument, "## v1.0.0")
		assert.NotContains(t, releaseBodyDocument, "Previous release")
		releaseNotesData, err := afero.ReadFile(fsRepo, "RELEASE_NOTES.md")
		require.NoError(t, err)
		releaseNotesDocument := string(releaseNotesData)
		assert.True(t, strings.HasPrefix(releaseNotesDocument, releaseBodyDocument+"\n\n## v1.0.0"))
		assert.Contains(t, releaseNotesDocument, "### Release Notes")
		assert.Contains(t, releaseNotesDocument, "Only this release needs these notes.")
		assert.Contains(t, releaseNotesDocument, "Previous release")
		assert.NotContains(t, releaseNotesDocument, "# Changelog")
		cliffSvc.AssertExpectations(t)
	})

	t.Run("Should use scoped changelog when manual notes are absent", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)
		scopedChangelog := "## v2.0.0\n\n### Features\n- Current release"
		fullChangelog := "# Changelog\n\n" + scopedChangelog
		cliffSvc.On("GenerateChangelog", mock.Anything, "v2.0.0", "release").Return(scopedChangelog, nil).Once()
		cliffSvc.On("GenerateFullChangelog", mock.Anything, "v2.0.0").Return(fullChangelog, nil).Once()
		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		artifacts, err := orch.generateChangelog(ctx, "v2.0.0")
		require.NoError(t, err)
		assert.Equal(t, scopedChangelog, artifacts.changelog)
		assert.Empty(t, artifacts.releaseNotes)
		changelogData, err := afero.ReadFile(fsRepo, "CHANGELOG.md")
		require.NoError(t, err)
		assert.Equal(t, fullChangelog, string(changelogData))
		releaseBodyData, err := afero.ReadFile(fsRepo, "RELEASE_BODY.md")
		require.NoError(t, err)
		assert.Equal(t, scopedChangelog, string(releaseBodyData))
		releaseNotesData, err := afero.ReadFile(fsRepo, "RELEASE_NOTES.md")
		require.NoError(t, err)
		assert.Equal(t, scopedChangelog, string(releaseNotesData))
		cliffSvc.AssertExpectations(t)
	})

	t.Run("Should replace existing historical section for the same version", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)
		scopedChangelog := "## v2.0.0\n\n### Fixes\n- Correct release notes"
		fullChangelog := "# Changelog\n\n" + scopedChangelog
		previousReleaseNotes := "## v2.0.0\n\n### Fixes\n- Old content\n\n## v1.0.0\n\n### Features\n- Previous release"
		require.NoError(t, afero.WriteFile(fsRepo, "RELEASE_NOTES.md", []byte(previousReleaseNotes), 0644))
		cliffSvc.On("GenerateChangelog", mock.Anything, "v2.0.0", "release").Return(scopedChangelog, nil).Once()
		cliffSvc.On("GenerateFullChangelog", mock.Anything, "v2.0.0").Return(fullChangelog, nil).Once()
		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		_, err := orch.generateChangelog(ctx, "v2.0.0")
		require.NoError(t, err)
		releaseNotesData, err := afero.ReadFile(fsRepo, "RELEASE_NOTES.md")
		require.NoError(t, err)
		releaseNotesDocument := string(releaseNotesData)
		assert.Equal(t, 1, strings.Count(releaseNotesDocument, "## v2.0.0"))
		assert.Contains(t, releaseNotesDocument, "- Correct release notes")
		assert.Contains(t, releaseNotesDocument, "## v1.0.0")
		assert.NotContains(t, releaseNotesDocument, "- Old content")
		cliffSvc.AssertExpectations(t)
	})
}

func TestPRReleaseOrchestrator_releaseArtifactCommands(t *testing.T) {
	t.Run("Should run configured artifact command with release environment", func(t *testing.T) {
		cfg := testReleaseConfig()
		cfg.ReleaseArtifacts = []config.ReleaseArtifactCommand{
			{
				Name:    "site-changelog",
				Command: "bun",
				Args:    []string{"run", "release:site-changelog"},
				Add:     []string{"packages/site/content/blog/changelog/*.mdx"},
			},
		}
		ctx := testReleaseContextWithConfig(t, cfg)
		fsRepo := afero.NewMemMapFs()
		require.NoError(t, fsRepo.MkdirAll("packages/site/content/blog/changelog", 0755))
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)
		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		var gotEnv map[string]string
		orch.artifactRunner = func(
			_ context.Context,
			command *config.ReleaseArtifactCommand,
			env map[string]string,
		) error {
			assert.Equal(t, "site-changelog", command.Name)
			assert.Equal(t, []string{"run", "release:site-changelog"}, command.Args)
			gotEnv = env
			return afero.WriteFile(
				fsRepo,
				"packages/site/content/blog/changelog/v1.2.3.mdx",
				[]byte("---\nversion: \"v1.2.3\"\n---\n"),
				0644,
			)
		}

		result, err := orch.runReleaseArtifactCommands(ctx, "v1.2.3", "release/v1.2.3", "v1.2.2")

		require.NoError(t, err)
		assert.Equal(t, []string{"packages/site/content/blog/changelog/*.mdx"}, result.addPatterns)
		assert.Empty(t, result.modifiedFiles)
		assert.Equal(t, []string{"packages/site/content/blog/changelog/v1.2.3.mdx"}, result.createdFiles)
		assert.Equal(t, "v1.2.3", gotEnv["PR_RELEASE_VERSION"])
		assert.Equal(t, "1.2.3", gotEnv["PR_RELEASE_VERSION_NUMBER"])
		assert.Equal(t, "release/v1.2.3", gotEnv["PR_RELEASE_BRANCH"])
		assert.Equal(t, "v1.2.2", gotEnv["PR_RELEASE_PREVIOUS_TAG"])
		assert.Equal(t, "CHANGELOG.md", gotEnv["PR_RELEASE_CHANGELOG_PATH"])
		assert.Equal(t, "RELEASE_BODY.md", gotEnv["PR_RELEASE_BODY_PATH"])
		assert.Equal(t, "RELEASE_NOTES.md", gotEnv["PR_RELEASE_NOTES_PATH"])
		assert.NotEmpty(t, gotEnv["PR_RELEASE_DATE"])
	})

	t.Run("Should remove newly generated artifact files during rollback", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		path := "packages/site/content/blog/changelog/v1.2.3.mdx"
		require.NoError(t, fsRepo.MkdirAll("packages/site/content/blog/changelog", 0755))
		require.NoError(t, afero.WriteFile(fsRepo, path, []byte("generated"), 0644))
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		compensator := NewCompensatingActions(gitRepo, githubRepo, fsRepo)

		err := compensator.RestoreFiles(ctx, map[string]any{"created_files": []string{path}})

		require.NoError(t, err)
		exists, existsErr := afero.Exists(fsRepo, path)
		require.NoError(t, existsErr)
		assert.False(t, exists)
	})
}

func TestPRReleaseOrchestrator_ExecuteReleaseArtifacts(t *testing.T) {
	t.Run("Should run release artifacts during dry-run without committing", func(t *testing.T) {
		cfg := testReleaseConfig()
		cfg.ReleaseArtifacts = []config.ReleaseArtifactCommand{
			{
				Name:    "site-changelog",
				Command: "bun",
				Add:     []string{"packages/site/content/blog/changelog/*.mdx"},
			},
		}
		ctx := testReleaseContextWithConfig(t, cfg)
		fsRepo := afero.NewMemMapFs()
		require.NoError(t, fsRepo.MkdirAll("packages/site/content/blog/changelog", 0755))
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)
		t.Setenv("GITHUB_TOKEN", "test-token")
		gitRepo.On("LatestTag", mock.Anything).Return("v1.2.2", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.2.2").Return(1, nil).Once()
		nextVersion, err := domain.NewVersion("v1.2.3")
		require.NoError(t, err)
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.2.2").Return(nextVersion, nil).Times(2)
		gitRepo.On("CreateBranch", mock.Anything, "release/v1.2.3").Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, "release/v1.2.3").Return(nil).Once()
		changelog := "## v1.2.3\n\n### Features\n- Generate site changelog"
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.2.3", "release").Return(changelog, nil).Once()
		cliffSvc.On("GenerateFullChangelog", mock.Anything, "v1.2.3").Return("# Changelog\n\n"+changelog, nil).Once()
		artifactRuns := 0
		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		orch.artifactRunner = func(
			_ context.Context,
			_ *config.ReleaseArtifactCommand,
			_ map[string]string,
		) error {
			artifactRuns++
			return afero.WriteFile(
				fsRepo,
				"packages/site/content/blog/changelog/v1.2.3.mdx",
				[]byte("generated"),
				0644,
			)
		}

		err = orch.Execute(ctx, PRReleaseConfig{DryRun: true})

		require.NoError(t, err)
		assert.Equal(t, 1, artifactRuns)
		gitRepo.AssertExpectations(t)
		githubRepo.AssertExpectations(t)
		cliffSvc.AssertExpectations(t)
	})

	t.Run("Should stop the workflow when a release artifact command fails", func(t *testing.T) {
		cfg := testReleaseConfig()
		cfg.ReleaseArtifacts = []config.ReleaseArtifactCommand{
			{
				Name:    "site-changelog",
				Command: "bun",
				Add:     []string{"packages/site/content/blog/changelog/*.mdx"},
			},
		}
		ctx := testReleaseContextWithConfig(t, cfg)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)
		t.Setenv("GITHUB_TOKEN", "test-token")
		gitRepo.On("LatestTag", mock.Anything).Return("v1.2.2", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.2.2").Return(1, nil).Once()
		nextVersion, err := domain.NewVersion("v1.2.3")
		require.NoError(t, err)
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.2.2").Return(nextVersion, nil).Times(2)
		gitRepo.On("CreateBranch", mock.Anything, "release/v1.2.3").Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, "release/v1.2.3").Return(nil).Once()
		changelog := "## v1.2.3\n\n### Features\n- Generate site changelog"
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.2.3", "release").Return(changelog, nil).Once()
		cliffSvc.On("GenerateFullChangelog", mock.Anything, "v1.2.3").Return("# Changelog\n\n"+changelog, nil).Once()
		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		orch.artifactRunner = func(
			_ context.Context,
			_ *config.ReleaseArtifactCommand,
			_ map[string]string,
		) error {
			return errors.New("generator failed")
		}

		err = orch.Execute(ctx, PRReleaseConfig{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "release artifact \"site-changelog\" failed")
		gitRepo.AssertExpectations(t)
		githubRepo.AssertExpectations(t)
		cliffSvc.AssertExpectations(t)
	})
}

func TestPRReleaseOrchestrator_Execute(t *testing.T) {
	t.Run("Should successfully create a new release PR when changes exist", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		// Set required environment variables
		t.Setenv("GITHUB_TOKEN", "test-token")

		// Setup expectations for checkChanges
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()

		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Once()

		// Setup expectations for calculateVersion (called again in prepareRelease)
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Once()

		// Setup expectations for createReleaseBranch
		branchName := "release/v1.1.0"
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		// Setup expectations for generateChangelog
		changelog := "## v1.1.0\n\n### Features\n- New feature added\n### Bug Fixes\n- Fixed critical bug"
		fullChangelog := "# Changelog\n\n" + changelog + "\n\n## v1.0.0\n\n### Misc\n- Previous entry"
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").Return(changelog, nil).Once()
		cliffSvc.On("GenerateFullChangelog", mock.Anything, "v1.1.0").Return(fullChangelog, nil).Once()

		// Setup expectations for commitChanges
		gitRepo.On("ConfigureUser", mock.Anything, "github-actions[bot]", "github-actions[bot]@users.noreply.github.com").
			Return(nil).
			Once()
		gitRepo.On("AddFiles", mock.Anything, "CHANGELOG.md").Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, "RELEASE_BODY.md").Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, "RELEASE_NOTES.md").Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, "package.json").Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, "package-lock.json").Return(nil).Once()
		// tools/* updates removed
		gitRepo.On("Commit", mock.Anything, "release: prepare release v1.1.0").Return(nil).Once()
		gitRepo.On("PushBranch", mock.Anything, branchName).Return(nil).Once()
		githubRepo.On("CreateOrUpdatePR", mock.Anything, branchName, "main", "release: Release v1.1.0",
			mock.MatchedBy(func(body string) bool {
				return strings.Contains(body, "Release v1.1.0") && strings.Contains(body, "### Features")
			}),
			[]string{"release-pending", "automated"}).Return(nil).Once()

		// Create orchestrator and execute
		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{
			ForceRelease: false,
			DryRun:       false,
			CIOutput:     false,
			SkipPR:       false,
		}

		err := orch.Execute(ctx, cfg)
		require.NoError(t, err)

		// Verify all expectations were met
		gitRepo.AssertExpectations(t)
		githubRepo.AssertExpectations(t)
		cliffSvc.AssertExpectations(t)

		// Verify files were created
		changelogExists, _ := afero.Exists(fsRepo, "CHANGELOG.md")
		assert.True(t, changelogExists, "CHANGELOG.md should be created")
		if changelogExists {
			data, err := afero.ReadFile(fsRepo, "CHANGELOG.md")
			require.NoError(t, err)
			assert.Equal(t, fullChangelog, string(data))
		}
		releaseNotesExists, _ := afero.Exists(fsRepo, "RELEASE_NOTES.md")
		assert.True(t, releaseNotesExists, "RELEASE_NOTES.md should be created")
		if releaseNotesExists {
			data, err := afero.ReadFile(fsRepo, "RELEASE_NOTES.md")
			require.NoError(t, err)
			assert.Equal(t, changelog, string(data))
		}
	})

	t.Run("Should force push when release branch already exists remotely", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)
		stateRepo := new(mockStateRepository)

		t.Setenv("GITHUB_TOKEN", "test-token")
		stateRepo.On("Save", mock.Anything, mock.Anything).Return(nil).Maybe()
		gitRepo.On("GetCurrentBranch", mock.Anything).Return("main", nil).Once()
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()
		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Times(2)

		branchName := "release/v1.1.0"
		gitRepo.On("ListLocalBranches", mock.Anything).Return([]string{"main", branchName}, nil).Once()
		gitRepo.On("RemoteBranchExists", mock.Anything, branchName).Return(true, nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, "main").Return(nil).Once()
		gitRepo.On("DeleteBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		changelog := "## v1.1.0\n\n### Fixes\n- Refresh release automation"
		fullChangelog := "# Changelog\n\n" + changelog
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").Return(changelog, nil).Once()
		cliffSvc.On("GenerateFullChangelog", mock.Anything, "v1.1.0").Return(fullChangelog, nil).Once()

		gitRepo.On("ConfigureUser", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, mock.Anything).Return(nil).Times(5)
		gitRepo.On("Commit", mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("PushBranchForce", mock.Anything, branchName).Return(nil).Once()
		githubRepo.On(
			"CreateOrUpdatePR",
			mock.Anything,
			branchName,
			"main",
			"release: Release v1.1.0",
			mock.MatchedBy(func(body string) bool {
				return strings.Contains(body, "Release v1.1.0") && strings.Contains(body, "### Fixes")
			}),
			[]string{"release-pending", "automated"},
		).Return(nil).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		orch.stateRepo = stateRepo
		cfg := PRReleaseConfig{
			EnableRollback: true,
			ForceRelease:   true,
		}

		err := orch.Execute(ctx, cfg)
		require.NoError(t, err)

		gitRepo.AssertNotCalled(t, "DeleteRemoteBranch", mock.Anything, branchName)
		gitRepo.AssertNotCalled(t, "PushBranch", mock.Anything, branchName)
		gitRepo.AssertExpectations(t)
		githubRepo.AssertExpectations(t)
		cliffSvc.AssertExpectations(t)
	})

	t.Run("Should skip PR creation when no changes exist and force flag is false", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")

		// Setup expectations - no version bump means no changes
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(0, nil).Once()

		// Create orchestrator and execute
		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{
			ForceRelease: false,
			DryRun:       false,
			CIOutput:     false,
			SkipPR:       false,
		}

		err := orch.Execute(ctx, cfg)
		require.NoError(t, err) // No error, just skips

		// Verify no further operations were performed
		gitRepo.AssertExpectations(t)
		githubRepo.AssertNotCalled(t, "CreateOrUpdatePR")
		cliffSvc.AssertNotCalled(t, "GenerateChangelog")
	})

	t.Run("Should force PR creation when force flag is set despite no changes", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")

		// no tools directory setup required

		// Setup expectations - no changes but force is true
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(0, nil).Once()

		// Even with no changes, force should trigger the flow
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		nextVersion, _ := domain.NewVersion("v1.0.1")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Once()

		// Setup remaining expectations for forced release
		branchName := "release/v1.0.1"
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		changelog := "## v1.0.1\n\n### Maintenance\n- Forced release"
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.0.1", "release").Return(changelog, nil).Once()

		gitRepo.On("ConfigureUser", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, mock.Anything).Return(nil).Times(5)
		gitRepo.On("Commit", mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("PushBranch", mock.Anything, branchName).Return(nil).Once()
		githubRepo.On("CreateOrUpdatePR", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil).
			Once()

		// Create orchestrator and execute with force flag
		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{
			ForceRelease: true,
			DryRun:       false,
			CIOutput:     false,
			SkipPR:       false,
		}

		err := orch.Execute(ctx, cfg)
		require.NoError(t, err)

		gitRepo.AssertExpectations(t)
		githubRepo.AssertExpectations(t)
		cliffSvc.AssertExpectations(t)
	})

	t.Run("Should handle error when GITHUB_TOKEN is missing", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		// Explicitly unset GITHUB_TOKEN
		t.Setenv("GITHUB_TOKEN", "")

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{}

		err := orch.Execute(ctx, cfg)
		require.Error(t, err)
		assert.ErrorContains(t, err, "environment validation failed")
		assert.ErrorContains(t, err, "GITHUB_TOKEN")

		// Verify no operations were performed
		gitRepo.AssertNotCalled(t, "LatestTag")
		githubRepo.AssertNotCalled(t, "CreateOrUpdatePR")
	})

	t.Run("Should handle error in version calculation", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")

		// Setup expectations for checkChanges (use mock.Anything for context due to timeout wrapper)
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()
		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Once()

		// Setup expectations for calculateVersion to fail (use mock.Anything for context)
		gitRepo.On("LatestTag", mock.Anything).Return("", errors.New("failed to get tag")).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{}

		err := orch.Execute(ctx, cfg)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to calculate version")

		gitRepo.AssertExpectations(t)
		// Verify PR was not created
		githubRepo.AssertNotCalled(t, "CreateOrUpdatePR")
	})

	t.Run("Should handle error in changelog generation", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")

		// Create tools directory
		// no tools dir

		// Setup successful flow until changelog generation (use mock.Anything for context)
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()

		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Times(2)

		branchName := "release/v1.1.0"
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		// Fail on changelog generation (use mock.Anything for context)
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").
			Return("", errors.New("cliff failed")).
			Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{}

		err := orch.Execute(ctx, cfg)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to generate changelog")

		gitRepo.AssertExpectations(t)
		cliffSvc.AssertExpectations(t)
		// Verify PR was not created
		githubRepo.AssertNotCalled(t, "CreateOrUpdatePR")
	})

	t.Run("Should handle error in PR creation", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")

		// no tools directory setup required

		// Setup successful flow until PR creation (use mock.Anything for context)
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()

		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Times(2)

		branchName := "release/v1.1.0"
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		changelog := "## v1.1.0\n\n### Features\n- New feature"
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").Return(changelog, nil).Once()

		gitRepo.On("ConfigureUser", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, mock.Anything).Return(nil).Times(5)
		gitRepo.On("Commit", mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("PushBranch", mock.Anything, branchName).Return(nil).Once()

		// Fail on PR creation (use mock.Anything for context)
		// Note: The retry might not be happening for non-retryable errors
		githubRepo.On("CreateOrUpdatePR", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(errors.New("GitHub API error")).
			Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{}

		err := orch.Execute(ctx, cfg)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to create pull request")

		gitRepo.AssertExpectations(t)
		githubRepo.AssertExpectations(t)
		cliffSvc.AssertExpectations(t)
	})

	t.Run("Should skip PR creation when SkipPR flag is set", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")

		// no tools directory setup required

		// Setup expectations - normal flow but skip PR (use mock.Anything for context)
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()

		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Times(2)

		branchName := "release/v1.1.0"
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		changelog := "## v1.1.0\n\n### Features\n- New feature"
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").Return(changelog, nil).Once()

		gitRepo.On("ConfigureUser", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, mock.Anything).Return(nil).Times(5)
		gitRepo.On("Commit", mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("PushBranch", mock.Anything, branchName).Return(nil).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{
			SkipPR: true,
		}

		err := orch.Execute(ctx, cfg)
		require.NoError(t, err)

		gitRepo.AssertExpectations(t)
		// Verify PR was not created
		githubRepo.AssertNotCalled(t, "CreateOrUpdatePR")
	})

	t.Run("Should output CI format when CIOutput flag is set", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")

		// Configure logger to capture CI log output
		buf := &bytes.Buffer{}
		encoderCfg := zap.NewProductionEncoderConfig()
		encoderCfg.TimeKey = ""
		encoder := zapcore.NewJSONEncoder(encoderCfg)
		core := zapcore.NewCore(encoder, zapcore.AddSync(buf), zapcore.InfoLevel)
		testLogger := zap.New(core)
		ctx = logger.IntoContext(ctx, testLogger)
		t.Cleanup(func() {
			_ = logger.Sync(testLogger)
		})

		// Setup expectations - no changes for simplicity (use mock.Anything for context)
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(0, nil).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{
			CIOutput: true,
		}

		err := orch.Execute(ctx, cfg)
		require.NoError(t, err)
		output := buf.String()

		// Verify CI output format
		assert.Contains(t, output, "\"has_changes\":false")
		assert.Contains(t, output, "\"latest_tag\":\"v1.0.0\"")

		gitRepo.AssertExpectations(t)
	})

	t.Run("Should handle initial release when no tags exist", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")
		// tools env not required

		// no tools directory setup required

		// Setup expectations for initial release (no tags, use mock.Anything for context)
		gitRepo.On("LatestTag", mock.Anything).Return("", nil).Once() // No tags exist

		// For calculateVersion when no tag exists (use mock.Anything for context)
		gitRepo.On("LatestTag", mock.Anything).Return("", nil).Once()
		initialVersion, _ := domain.NewVersion("v0.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v0.0.0").Return(initialVersion, nil).Once()

		branchName := "release/v0.1.0"
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		changelog := "## v0.1.0\n\n### Features\n- Initial release"
		cliffSvc.On("GenerateChangelog", mock.Anything, "v0.1.0", "release").Return(changelog, nil).Once()

		gitRepo.On("ConfigureUser", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, mock.Anything).Return(nil).Times(5)
		gitRepo.On("Commit", mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("PushBranch", mock.Anything, branchName).Return(nil).Once()
		githubRepo.On("CreateOrUpdatePR", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil).
			Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{}

		err := orch.Execute(ctx, cfg)
		require.NoError(t, err)

		gitRepo.AssertExpectations(t)
		githubRepo.AssertExpectations(t)
		cliffSvc.AssertExpectations(t)
	})

	// NOTE: tools/ update tests removed (tools updates are no longer part of the pipeline)

	t.Run("Should handle error when creating release branch fails", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")

		// Setup expectations (use mock.Anything for context)
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()

		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Times(2)

		// Fail on branch creation (use mock.Anything for context)
		branchName := "release/v1.1.0"
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(errors.New("branch already exists")).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{}

		err := orch.Execute(ctx, cfg)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to create release branch")

		gitRepo.AssertExpectations(t)
		cliffSvc.AssertExpectations(t)
		// Verify no PR was created
		githubRepo.AssertNotCalled(t, "CreateOrUpdatePR")
	})

	t.Run("Should handle commit errors gracefully", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")
		// tools env not required

		// Create tools directory
		// no tools dir

		// Setup successful flow until commit (use mock.Anything for context)
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()

		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Times(2)

		branchName := "release/v1.1.0"
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		changelog := "## v1.1.0\n\n### Features\n- New feature"
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").Return(changelog, nil).Once()

		gitRepo.On("ConfigureUser", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, mock.Anything).Return(nil).Times(5)
		// Fail on commit (use mock.Anything for context)
		gitRepo.On("Commit", mock.Anything, mock.Anything).Return(errors.New("nothing to commit")).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{}

		err := orch.Execute(ctx, cfg)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to commit changes")

		gitRepo.AssertExpectations(t)
		// Verify no PR was created
		githubRepo.AssertNotCalled(t, "CreateOrUpdatePR")
	})

	t.Run("Should validate version format correctly", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")

		// Setup expectations for checkChanges (use mock.Anything for context)
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()
		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Once()

		// Setup expectations for calculateVersion to return nil version which will cause validation error
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		// Return nil to simulate an error case that will fail validation
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").
			Return(nil, errors.New("version calculation failed")).
			Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{}

		err := orch.Execute(ctx, cfg)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to calculate version")

		gitRepo.AssertExpectations(t)
		cliffSvc.AssertExpectations(t)
	})
}

func TestPRReleaseOrchestrator_RollbackOnFailure(t *testing.T) {
	t.Run("Should rollback branch creation when changelog generation fails", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)
		stateRepo := new(mockStateRepository)

		t.Setenv("GITHUB_TOKEN", "test-token")
		// tools env not required

		// Create tools directory
		// no tools dir

		// Setup expectations for initial saga setup and branch operations
		// GetCurrentBranch is called: initial setup, create branch, and during rollback
		gitRepo.On("GetCurrentBranch", mock.Anything).Return("main", nil).Times(3)

		// State saves - Allow any state saves during execution
		stateRepo.On("Save", mock.Anything, mock.Anything).Return(nil).Maybe()

		// Setup expectations for checkChanges step
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()
		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Once()

		// Setup expectations for calculateVersion step
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Once()

		// Setup expectations for createBranch step - successful
		branchName := "release/v1.1.0"
		// Mock ListLocalBranches to return branches WITHOUT the target branch (so it gets created)
		gitRepo.On("ListLocalBranches", mock.Anything).Return([]string{"main"}, nil).Once()
		gitRepo.On("RemoteBranchExists", mock.Anything, branchName).Return(false, nil).Once()
		// Once for create, once during rollback check
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		// Setup expectations for updatePackages step - successful

		// Setup GetFileStatus for rollback file restoration checks
		gitRepo.On("GetFileStatus", mock.Anything, mock.Anything).Return("modified", nil).Maybe()

		// Setup expectations for changelog generation - FAIL
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").
			Return("", errors.New("cliff failed")).Maybe() // May be called multiple times with retries

		// Rollback expectations
		gitRepo.On("RestoreFile", mock.Anything, mock.Anything).
			Return(nil).
			Maybe()
			// For file restoration during rollback
		gitRepo.On("ListLocalBranches", mock.Anything).
			Return([]string{"main", branchName}, nil).
			Maybe()
			// Check if branch exists locally
		gitRepo.On("RemoteBranchExists", mock.Anything, branchName).
			Return(true, nil).
			Maybe()
			// Check if branch exists remotely
		gitRepo.On("CheckoutBranch", mock.Anything, "main").
			Return(nil).
			Maybe()
			// Maybe because rollback might not always checkout
		gitRepo.On("DeleteBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("DeleteRemoteBranch", mock.Anything, branchName).Return(nil).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		orch.stateRepo = stateRepo
		cfg := PRReleaseConfig{
			EnableRollback: true,
		}

		err := orch.Execute(ctx, cfg)
		require.Error(t, err)
		assert.ErrorContains(t, err, "cliff failed")

		// Verify rollback was called
		gitRepo.AssertCalled(t, "DeleteBranch", mock.Anything, branchName)
		// Note: CheckoutBranch to main may or may not be called depending on rollback logic
	})

	t.Run("Should rollback all completed steps when PR creation fails", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)
		stateRepo := new(mockStateRepository)

		t.Setenv("GITHUB_TOKEN", "test-token")
		// tools env not required

		// Create tools directory with package.json
		// no tools dir

		// Setup expectations for initial saga setup
		gitRepo.On("GetCurrentBranch", mock.Anything).Return("main", nil).Once()

		// State saves
		stateRepo.On("Save", mock.Anything, mock.Anything).Return(nil).Maybe()

		// Setup all successful steps until PR creation
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()
		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Times(2)

		branchName := "release/v1.1.0"
		gitRepo.On("GetCurrentBranch", mock.Anything).Return("main", nil).Once()
		// Mock ListLocalBranches to return branches WITHOUT the target branch (so it gets created)
		gitRepo.On("ListLocalBranches", mock.Anything).Return([]string{"main"}, nil).Once()
		gitRepo.On("RemoteBranchExists", mock.Anything, branchName).Return(false, nil).Once()
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		changelog := "## v1.1.0\n\n### Features\n- New feature"
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").
			Return(changelog, nil).
			Maybe()
			// May be called multiple times with retries

		gitRepo.On("ConfigureUser", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, mock.Anything).Return(nil).Times(5)
		gitRepo.On("Commit", mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("PushBranch", mock.Anything, branchName).Return(nil).Once()

		// PR creation fails
		githubRepo.On("CreateOrUpdatePR", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(errors.New("GitHub API error")).
			Maybe()

			// May be called multiple times with retries

			// Retries

		// Rollback expectations - in reverse order
		gitRepo.On("GetFileStatus", mock.Anything, mock.Anything).
			Return("modified", nil).
			Maybe()
			// For file status checks during rollback
		gitRepo.On("ListLocalBranches", mock.Anything).
			Return([]string{"main", branchName}, nil).
			Maybe()
			// Check if branch exists
		gitRepo.On("RemoteBranchExists", mock.Anything, branchName).
			Return(true, nil).
			Maybe()
			// Check if branch exists remotely
		gitRepo.On("GetCurrentBranch", mock.Anything).
			Return(branchName, nil).
			Maybe()
			// Additional calls during rollback
		gitRepo.On("ResetHard", mock.Anything, "HEAD~1").Return(nil).Once()
		gitRepo.On("RestoreFile", mock.Anything, "CHANGELOG.md").Return(nil).Maybe()
		gitRepo.On("RestoreFile", mock.Anything, "RELEASE_NOTES.md").Return(nil).Maybe()
		gitRepo.On("RestoreFile", mock.Anything, "package.json").Return(nil).Maybe()
		gitRepo.On("RestoreFile", mock.Anything, "package-lock.json").Return(nil).Maybe()
		// tools restore no longer expected
		gitRepo.On("RestoreFile", mock.Anything, mock.Anything).
			Return(nil).
			Maybe()
			// Generic catch-all for any other files
		gitRepo.On("CheckoutBranch", mock.Anything, "main").Return(nil).Once()
		gitRepo.On("DeleteBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("DeleteRemoteBranch", mock.Anything, branchName).Return(nil).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		orch.stateRepo = stateRepo
		cfg := PRReleaseConfig{
			EnableRollback: true,
		}

		err := orch.Execute(ctx, cfg)
		require.Error(t, err)
		assert.ErrorContains(t, err, "GitHub API error")

		// Verify key rollback operations were called
		// Note: The specific compensations called depend on what operations completed successfully
		gitRepo.AssertCalled(t, "DeleteBranch", mock.Anything, branchName)
		// Other operations like ResetHard and CheckoutBranch depend on rollback execution order
	})

	t.Run("Should handle rollback failure gracefully", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)
		stateRepo := new(mockStateRepository)

		t.Setenv("GITHUB_TOKEN", "test-token")
		// tools env not required

		// Create tools directory
		// no tools dir

		// Setup expectations
		gitRepo.On("GetCurrentBranch", mock.Anything).Return("main", nil).Once()
		stateRepo.On("Save", mock.Anything, mock.Anything).Return(nil).Maybe()

		// Setup successful branch creation
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()
		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Times(2)

		branchName := "release/v1.1.0"
		// Mock ListLocalBranches to return branches WITHOUT the target branch (so it gets created)
		gitRepo.On("ListLocalBranches", mock.Anything).Return([]string{"main"}, nil).Once()
		gitRepo.On("RemoteBranchExists", mock.Anything, branchName).Return(false, nil).Once()
		gitRepo.On("GetCurrentBranch", mock.Anything).Return("main", nil).Once()
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		// Fail on changelog generation
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").
			Return("", errors.New("changelog failed")).Maybe() // May be called multiple times with retries

		// Add mocks for rollback operations
		gitRepo.On("GetFileStatus", mock.Anything, mock.Anything).Return("modified", nil).Maybe()
		gitRepo.On("ListLocalBranches", mock.Anything).Return([]string{"main", branchName}, nil).Maybe()
		gitRepo.On("RemoteBranchExists", mock.Anything, branchName).
			Return(true, nil).
			Maybe()
		gitRepo.On("GetCurrentBranch", mock.Anything).Return(branchName, nil).Times(2)
		gitRepo.On("RestoreFile", mock.Anything, mock.Anything).Return(nil).Maybe()

		// Rollback also fails - make checkout operations fail during rollback
		gitRepo.On("CheckoutBranch", mock.Anything, "main").
			Return(errors.New("checkout failed")).
			Maybe() // May be called multiple times due to retries
		gitRepo.On("CheckoutBranch", mock.Anything, "master").
			Return(errors.New("checkout failed")).
			Maybe() // May be called multiple times due to retries
		gitRepo.On("DeleteBranch", mock.Anything, branchName).
			Return(errors.New("delete branch failed")).
			Maybe() // This should cause rollback to fail
		gitRepo.On("DeleteRemoteBranch", mock.Anything, branchName).Return(nil).Maybe()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		orch.stateRepo = stateRepo
		cfg := PRReleaseConfig{
			EnableRollback: true,
		}

		err := orch.Execute(ctx, cfg)
		require.Error(t, err)
		assert.ErrorContains(t, err, "changelog failed")
		assert.ErrorContains(t, err, "rollback also failed")
	})
}

func TestPRReleaseOrchestrator_DisabledRollback(t *testing.T) {
	t.Run("Should not save state when rollback is disabled", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")
		// tools env not required

		// Create tools directory
		// no tools dir

		// Setup successful workflow
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()
		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Times(2)

		branchName := "release/v1.1.0"
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		changelog := "## v1.1.0\n\n### Features\n- New feature"
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").Return(changelog, nil).Once()

		gitRepo.On("ConfigureUser", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("AddFiles", mock.Anything, mock.Anything).Return(nil).Times(5)
		gitRepo.On("Commit", mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("PushBranch", mock.Anything, branchName).Return(nil).Once()

		githubRepo.On("CreateOrUpdatePR", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil).
			Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		// Don't set stateRepo - it should work with nil
		cfg := PRReleaseConfig{
			EnableRollback: false,
		}

		err := orch.Execute(ctx, cfg)
		require.NoError(t, err)

		// Verify state repository was not used
		// (no mock assertions for stateRepo since it wasn't created)
	})

	t.Run("Should not perform rollback when disabled even on failure", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		t.Setenv("GITHUB_TOKEN", "test-token")
		// tools env not required

		// Create tools directory
		// no tools dir

		// Setup expectations
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Times(2)
		gitRepo.On("CommitsSinceTag", mock.Anything, "v1.0.0").Return(10, nil).Once()
		nextVersion, _ := domain.NewVersion("v1.1.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(nextVersion, nil).Times(2)

		branchName := "release/v1.1.0"
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		// Fail on changelog generation
		cliffSvc.On("GenerateChangelog", mock.Anything, "v1.1.0", "release").
			Return("", errors.New("changelog failed")).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)
		cfg := PRReleaseConfig{
			EnableRollback: false,
		}

		err := orch.Execute(ctx, cfg)
		require.Error(t, err)
		assert.ErrorContains(t, err, "changelog failed")

		// Verify no rollback operations were performed
		gitRepo.AssertNotCalled(t, "DeleteBranch", mock.Anything, branchName)
		gitRepo.AssertNotCalled(t, "ResetHard", mock.Anything, mock.Anything)
	})
}

func TestPRReleaseOrchestrator_prepareRelease(t *testing.T) {
	t.Run("Should validate branch name format", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		// Setup expectations - test with a normal version (use mock.Anything for context)
		gitRepo.On("LatestTag", mock.Anything).Return("v1.0.0", nil).Once()
		validVersion, _ := domain.NewVersion("v1.0.0")
		cliffSvc.On("CalculateNextVersion", mock.Anything, "v1.0.0").Return(validVersion, nil).Once()

		// Setup branch creation expectations (use mock.Anything for context)
		branchName := "release/v1.0.0"
		gitRepo.On("CreateBranch", mock.Anything, branchName).Return(nil).Once()
		gitRepo.On("CheckoutBranch", mock.Anything, branchName).Return(nil).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)

		// This should succeed with valid branch name
		version, resultBranch, err := orch.prepareRelease(ctx, "v1.0.0", false)

		require.NoError(t, err)
		assert.Equal(t, "v1.0.0", version)
		assert.Equal(t, branchName, resultBranch)
		// Verify the branch name is within limits
		assert.LessOrEqual(t, len(resultBranch), 255)

		gitRepo.AssertExpectations(t)
		cliffSvc.AssertExpectations(t)
	})
}

func TestPRReleaseOrchestrator_commitChanges(t *testing.T) {
	t.Run("Should configure git user correctly", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		expectedUser := "github-actions[bot]"
		expectedEmail := "github-actions[bot]@users.noreply.github.com"

		gitRepo.On("ConfigureUser", ctx, expectedUser, expectedEmail).Return(nil).Once()
		gitRepo.On("AddFiles", ctx, "CHANGELOG.md").Return(nil).Once()
		gitRepo.On("AddFiles", ctx, "RELEASE_BODY.md").Return(nil).Once()
		gitRepo.On("AddFiles", ctx, "RELEASE_NOTES.md").Return(nil).Once()
		gitRepo.On("AddFiles", ctx, "package.json").Return(nil).Once()
		gitRepo.On("AddFiles", ctx, "package-lock.json").Return(nil).Once()
		// no tools files added
		gitRepo.On("Commit", ctx, "release: prepare release v1.2.0").Return(nil).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)

		err := orch.commitChanges(ctx, "v1.2.0", nil)
		require.NoError(t, err)

		gitRepo.AssertExpectations(t)
	})

	t.Run("Should add all required files in correct order", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		// Ensure files are added in the expected order
		var addedFiles []string
		gitRepo.On("ConfigureUser", ctx, mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("AddFiles", ctx, mock.Anything).Run(func(args mock.Arguments) {
			pattern := args.Get(1).(string)
			addedFiles = append(addedFiles, pattern)
		}).Return(nil).Times(5)
		gitRepo.On("Commit", ctx, mock.Anything).Return(nil).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)

		err := orch.commitChanges(ctx, "v1.2.0", nil)
		require.NoError(t, err)

		// Verify files were added in correct order
		assert.Equal(t, []string{
			"CHANGELOG.md",
			"RELEASE_BODY.md",
			"RELEASE_NOTES.md",
			"package.json",
			"package-lock.json",
			// tools removed
		}, addedFiles)

		gitRepo.AssertExpectations(t)
	})

	t.Run("Should stage configured release artifact paths after core release files", func(t *testing.T) {
		ctx := testReleaseContext(t)
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		npmSvc := new(mockNpmService)

		var addedFiles []string
		gitRepo.On("ConfigureUser", ctx, mock.Anything, mock.Anything).Return(nil).Once()
		gitRepo.On("AddFiles", ctx, mock.Anything).Run(func(args mock.Arguments) {
			pattern := args.Get(1).(string)
			addedFiles = append(addedFiles, pattern)
		}).Return(nil).Times(6)
		gitRepo.On("Commit", ctx, mock.Anything).Return(nil).Once()

		orch := NewPRReleaseOrchestrator(gitRepo, githubRepo, fsRepo, cliffSvc, npmSvc)

		err := orch.commitChanges(
			ctx,
			"v1.2.0",
			[]string{"packages/site/content/blog/changelog/*.mdx"},
		)
		require.NoError(t, err)

		assert.Equal(t, []string{
			"CHANGELOG.md",
			"RELEASE_BODY.md",
			"RELEASE_NOTES.md",
			"package.json",
			"package-lock.json",
			"packages/site/content/blog/changelog/*.mdx",
		}, addedFiles)

		gitRepo.AssertExpectations(t)
	})
}
