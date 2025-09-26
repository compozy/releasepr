package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var goreleaserArgs = []string{"release", "--snapshot", "--skip=publish", "--clean"}

func toIface(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func TestDryRunOrchestrator_Execute(t *testing.T) {
	t.Run("Should successfully execute dry-run validation", func(t *testing.T) {
		ctx := context.Background()
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		goreleaserSvc := new(mockGoReleaserService)

		orch := NewDryRunOrchestrator(gitRepo, githubRepo, cliffSvc, goreleaserSvc, fsRepo)
		// Setup expectations
		goreleaserSvc.On("Run", append([]any{mock.Anything}, toIface(goreleaserArgs)...)...).Return(nil)
		// Setup test environment
		t.Setenv("GITHUB_HEAD_REF", "release/v1.1.0")
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_ISSUE_NUMBER", "123")
		// no tools validation
		// Create mock metadata file that GoReleaser would generate
		metadata := `{"version":"v1.1.0","artifacts":[{"type":"Archive","goos":"linux","goarch":"amd64"}]}`
		writeGoReleaserOutput(t, fsRepo, metadata, true)
		githubRepo.On("AddComment", mock.Anything, 123, mock.MatchedBy(func(body string) bool {
			return strings.Contains(body, "Dry-Run Completed Successfully")
		})).Return(nil)
		// Execute
		cfg := DryRunConfig{CIOutput: false}
		err := orch.Execute(ctx, cfg)
		require.NoError(t, err)
		goreleaserSvc.AssertExpectations(t)
		githubRepo.AssertExpectations(t)
	})

	t.Run("Should fail when GoReleaser dry-run fails", func(t *testing.T) {
		ctx := context.Background()
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		goreleaserSvc := new(mockGoReleaserService)
		orch := NewDryRunOrchestrator(gitRepo, githubRepo, cliffSvc, goreleaserSvc, fsRepo)
		t.Setenv("GITHUB_HEAD_REF", "release/v1.1.0")
		goreleaserSvc.On("Run", append([]any{mock.Anything}, toIface(goreleaserArgs)...)...).
			Return(errors.New("dry-run failed"))
		err := orch.Execute(ctx, DryRunConfig{})
		assert.ErrorContains(t, err, "GoReleaser dry-run failed")
	})

	t.Run("Should fail when no version found in branch name", func(t *testing.T) {
		ctx := context.Background()
		fsRepo := afero.NewMemMapFs()
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		goreleaserSvc := new(mockGoReleaserService)
		orch := NewDryRunOrchestrator(gitRepo, githubRepo, cliffSvc, goreleaserSvc, fsRepo)
		t.Setenv("GITHUB_HEAD_REF", "feature/no-version")
		goreleaserSvc.On("Run", append([]any{mock.Anything}, toIface(goreleaserArgs)...)...).Return(nil)
		err := orch.Execute(ctx, DryRunConfig{})
		assert.ErrorContains(t, err, "no version found in branch name")
	})

	t.Run("Should handle invalid metadata.json gracefully", func(t *testing.T) {
		ctx := context.Background()
		fsRepo := afero.NewMemMapFs()
		// no tools directory required
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		goreleaserSvc := new(mockGoReleaserService)
		orch := NewDryRunOrchestrator(gitRepo, githubRepo, cliffSvc, goreleaserSvc, fsRepo)
		t.Setenv("GITHUB_HEAD_REF", "release/v1.1.0")
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_ISSUE_NUMBER", "123")
		goreleaserSvc.On("Run", append([]any{mock.Anything}, toIface(goreleaserArgs)...)...).Return(nil)
		// Create dist directory and invalid metadata file
		writeGoReleaserOutput(t, fsRepo, "invalid json", true)
		// Execute should handle invalid JSON gracefully
		err := orch.Execute(ctx, DryRunConfig{CIOutput: false})
		assert.ErrorContains(t, err, "failed to parse metadata.json")
		// Should not post a comment on parse failure
		githubRepo.AssertNotCalled(t, "AddComment", mock.Anything, mock.Anything, mock.Anything)
		goreleaserSvc.AssertExpectations(t)
	})

	t.Run("Should post comment to PR when in CI with issue number", func(t *testing.T) {
		ctx := context.Background()
		fsRepo := afero.NewMemMapFs()
		// no tools directory required
		gitRepo := new(mockGitExtendedRepository)
		githubRepo := new(mockGithubExtendedRepository)
		cliffSvc := new(mockCliffService)
		goreleaserSvc := new(mockGoReleaserService)
		orch := NewDryRunOrchestrator(gitRepo, githubRepo, cliffSvc, goreleaserSvc, fsRepo)
		// Setup CI environment
		t.Setenv("GITHUB_HEAD_REF", "release/v2.0.0")
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("GITHUB_ISSUE_NUMBER", "456")
		t.Setenv("GITHUB_SHA", "abc123def456789")
		goreleaserSvc.On("Run", append([]any{mock.Anything}, toIface(goreleaserArgs)...)...).Return(nil)
		// Create metadata with multiple artifacts
		metadata := `{
            "version":"v2.0.0",
            "artifacts":[
                {"type":"Archive","goos":"linux","goarch":"amd64"},
                {"type":"Archive","goos":"darwin","goarch":"amd64"},
                {"type":"Archive","goos":"windows","goarch":"amd64"}
            ]
        }`
		writeGoReleaserOutput(t, fsRepo, metadata, true)
		// Expect comment with proper formatting
		githubRepo.On("AddComment", mock.Anything, 456, mock.MatchedBy(func(body string) bool {
			return strings.Contains(body, "Dry-Run Completed Successfully") &&
				strings.Contains(body, "v2.0.0") &&
				strings.Contains(body, "linux/amd64") &&
				strings.Contains(body, "darwin/amd64") &&
				strings.Contains(body, "windows/amd64")
		})).Return(nil)
		err := orch.Execute(ctx, DryRunConfig{CIOutput: false})
		require.NoError(t, err)
		goreleaserSvc.AssertExpectations(t)
		githubRepo.AssertExpectations(t)
	})

	// tools NPM validation removed from dry-run pipeline
}

func writeGoReleaserOutput(t *testing.T, fs afero.Fs, metadata string, withChecksums bool) {
	t.Helper()
	require.NoError(t, fs.MkdirAll("dist", 0755))
	require.NoError(t, afero.WriteFile(fs, "dist/metadata.json", []byte(metadata), 0644))
	if withChecksums {
		require.NoError(t, afero.WriteFile(fs, "dist/checksums.txt", []byte("checksums"), 0644))
	}
}
