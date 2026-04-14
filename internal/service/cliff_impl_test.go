package service

import (
	"context"
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
	t.Run("Should use tagged changelog args for release mode", func(t *testing.T) {
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
		assert.Equal(t, []string{"--tag", "v1.2.3"}, command.args)
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
