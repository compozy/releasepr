package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/compozy/releasepr/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturedCommand struct {
	name string
	args []string
}

func TestCliffService_GenerateChangelog(t *testing.T) {
	t.Run("Should use scoped unreleased changelog args for release mode", func(t *testing.T) {
		command := &capturedCommand{}
		svc := &cliffService{
			executor: func(_ context.Context, name string, args ...string) ([]byte, error) {
				command.name = name
				command.args = append([]string(nil), args...)
				return []byte("## 1.2.3"), nil
			},
		}
		changelog, err := svc.GenerateChangelog(t.Context(), "v1.2.3", "release")
		require.NoError(t, err)
		assert.Equal(t, "## 1.2.3", changelog)
		assert.Equal(t, "git-cliff", command.name)
		assert.Equal(t, []string{"--unreleased", "--tag", "v1.2.3", "--strip", "all"}, command.args)
	})
	t.Run("Should use unreleased args for update mode", func(t *testing.T) {
		command := &capturedCommand{}
		svc := &cliffService{
			executor: func(_ context.Context, name string, args ...string) ([]byte, error) {
				command.name = name
				command.args = append([]string(nil), args...)
				return []byte("## Unreleased"), nil
			},
		}
		changelog, err := svc.GenerateChangelog(t.Context(), "v1.2.3", "update")
		require.NoError(t, err)
		assert.Equal(t, "## Unreleased", changelog)
		assert.Equal(t, "git-cliff", command.name)
		assert.Equal(t, []string{"--unreleased"}, command.args)
	})
	t.Run("Should fail when release mode has no version", func(t *testing.T) {
		svc := &cliffService{}
		changelog, err := svc.GenerateChangelog(t.Context(), "", "release")
		require.Error(t, err)
		assert.Empty(t, changelog)
		assert.ErrorContains(t, err, "version required for release mode")
	})
	t.Run("Should fail when git cliff returns empty changelog", func(t *testing.T) {
		svc := &cliffService{
			executor: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				return []byte(" \n\t "), nil
			},
		}
		changelog, err := svc.GenerateChangelog(t.Context(), "v1.2.3", "release")
		require.Error(t, err)
		assert.Empty(t, changelog)
		assert.ErrorContains(t, err, "git-cliff returned empty changelog")
	})
}

func TestCliffService_GenerateChangelogIntegration(t *testing.T) {
	t.Run("Should exclude previous releases from release notes changelog", func(t *testing.T) {
		requireGitCliff(t)
		dir := t.TempDir()
		initChangelogFixture(t, dir)
		t.Chdir(dir)
		svc := NewCliffService()
		changelog, err := svc.GenerateChangelog(t.Context(), "v1.1.0", "release")
		require.NoError(t, err)
		assert.Contains(t, changelog, "## 1.1.0")
		assert.Contains(t, changelog, "Current release")
		assert.NotContains(t, changelog, "# Changelog")
		assert.NotContains(t, changelog, "## 1.0.0")
		assert.NotContains(t, changelog, "First release")
	})
}

func TestCliffService_GenerateFullChangelog(t *testing.T) {
	t.Run("Should render versioned full changelog for release branches", func(t *testing.T) {
		command := &capturedCommand{}
		svc := &cliffService{
			executor: func(_ context.Context, name string, args ...string) ([]byte, error) {
				command.name = name
				command.args = append([]string(nil), args...)
				return []byte("# Changelog\n\n## 1.2.3"), nil
			},
		}
		changelog, err := svc.GenerateFullChangelog(t.Context(), "v1.2.3")
		require.NoError(t, err)
		assert.Equal(t, "# Changelog\n\n## 1.2.3", changelog)
		assert.Equal(t, "git-cliff", command.name)
		assert.Equal(t, []string{"--tag", "v1.2.3", "-o", "-"}, command.args)
	})
	t.Run("Should render current full changelog when version is empty", func(t *testing.T) {
		command := &capturedCommand{}
		svc := &cliffService{
			executor: func(_ context.Context, name string, args ...string) ([]byte, error) {
				command.name = name
				command.args = append([]string(nil), args...)
				return []byte("# Changelog\n\n## Unreleased"), nil
			},
		}
		changelog, err := svc.GenerateFullChangelog(t.Context(), "")
		require.NoError(t, err)
		assert.Equal(t, "# Changelog\n\n## Unreleased", changelog)
		assert.Equal(t, "git-cliff", command.name)
		assert.Equal(t, []string{"-o", "-"}, command.args)
	})
}

func requireGitCliff(t *testing.T) {
	t.Helper()
	_, err := exec.LookPath("git-cliff")
	require.NoError(t, err, "git-cliff must be installed for release changelog integration tests")
}

func initChangelogFixture(t *testing.T, dir string) {
	t.Helper()
	config := `[changelog]
header = """
# Changelog
"""
body = """
{% if version %}\
    ## {{ version | trim_start_matches(pat="v") }}
{% else %}\
    ## Unreleased
{% endif %}\
{% for group, commits in commits | group_by(attribute="group") %}
    ### {{ group | upper_first }}
    {% for commit in commits %}
        - {{ commit.message | upper_first }}
    {% endfor %}
{% endfor %}\n
"""
footer = """
---
generated
"""
trim = true

[git]
conventional_commits = true
filter_unconventional = true
filter_commits = false
tag_pattern = "v[0-9].*"
sort_commits = "oldest"
commit_parsers = [
  { message = "^feat", group = "Features" },
  { message = "^fix", group = "Bug Fixes" },
]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cliff.toml"), []byte(config), 0644))
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Release Test")
	runGit(t, dir, "config", "user.email", "release@example.com")
	writeFixtureFile(t, dir, "first")
	runGit(t, dir, "add", "fixture.txt", "cliff.toml")
	runGit(t, dir, "commit", "-m", "feat: first release")
	runGit(t, dir, "tag", "v1.0.0")
	writeFixtureFile(t, dir, "second")
	runGit(t, dir, "add", "fixture.txt")
	runGit(t, dir, "commit", "-m", "feat: current release")
}

func writeFixtureFile(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fixture.txt"), []byte(content), 0644))
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed:\n%s", args, string(output))
}

func TestCliffService_CalculateNextVersion(t *testing.T) {
	t.Run("Should calculate next version from bumped version output", func(t *testing.T) {
		command := &capturedCommand{}
		svc := &cliffService{
			executor: func(_ context.Context, name string, args ...string) ([]byte, error) {
				command.name = name
				command.args = append([]string(nil), args...)
				return []byte("v1.2.3\n"), nil
			},
		}
		version, err := svc.CalculateNextVersion(t.Context(), "v1.2.2")
		require.NoError(t, err)
		assert.Equal(t, "git-cliff", command.name)
		assert.Equal(t, []string{"--bumped-version"}, command.args)
		require.NotNil(t, version)
		assert.Equal(t, "v1.2.3", version.String())
	})
	t.Run("Should fail when bumped version output is invalid", func(t *testing.T) {
		svc := &cliffService{
			executor: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				return []byte("invalid"), nil
			},
		}
		version, err := svc.CalculateNextVersion(t.Context(), "v1.2.2")
		require.Error(t, err)
		assert.Nil(t, version)
		assert.ErrorContains(t, err, "git-cliff returned invalid version")
	})
}

func TestCliffService_CalculateNextVersion_Compatibility(t *testing.T) {
	t.Run("Should accept semantic version output with prerelease suffix", func(t *testing.T) {
		svc := &cliffService{
			executor: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				return []byte("v1.2.3-rc.1"), nil
			},
		}
		version, err := svc.CalculateNextVersion(t.Context(), "v1.2.2")
		require.NoError(t, err)
		require.NotNil(t, version)
		expected, expectedErr := domain.NewVersion("v1.2.3-rc.1")
		require.NoError(t, expectedErr)
		assert.Equal(t, expected.String(), version.String())
	})
}
