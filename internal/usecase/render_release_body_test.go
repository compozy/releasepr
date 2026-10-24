// Contract: an explicit release body contains only changelog entries and custom notes from its canonical range.
// Owning layer: release-body composition use case.
// Canonical suite: this file; no existing suite owns explicit range composition.
// Rationale: the rendered markdown is the artifact published to GitHub and the downstream site receipt.
package usecase

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/compozy/releasepr/internal/config"
	"github.com/compozy/releasepr/internal/logger"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type releaseBodyCliffStub struct {
	tag       string
	gitRange  string
	initial   bool
	changelog string
}

func (s *releaseBodyCliffStub) GenerateReleaseBodyChangelog(
	_ context.Context,
	tag string,
	gitRange string,
	initial bool,
) (string, error) {
	s.tag = tag
	s.gitRange = gitRange
	s.initial = initial
	return s.changelog, nil
}

type releaseBodyGitStub struct {
	paths []string
	err   error
}

func (s *releaseBodyGitStub) AddedFiles(context.Context, string, string) ([]string, error) {
	return s.paths, s.err
}

func TestRenderReleaseBodyUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("Should render only custom notes introduced for the target core release", func(t *testing.T) {
		t.Parallel()

		fsRepo := afero.NewMemMapFs()
		require.NoError(t, fsRepo.MkdirAll(".release-notes/archive/v0.3.0", 0755))
		require.NoError(t, fsRepo.MkdirAll(".release-notes/archive/v0.2.15", 0755))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/archive/v0.3.0/the-os.md", []byte(`---
title: The OS release
type: highlight
---

Compozy now runs as an agent operating system.
`), 0644))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/archive/v0.2.15/legacy.md", []byte(`---
title: Legacy fix
type: fix
---

This belongs to the prior stable line.
`), 0644))
		cliff := &releaseBodyCliffStub{changelog: "## 0.3.0-beta.1\n\n### Features\n\n- Add beta releases"}
		gitRepo := &releaseBodyGitStub{paths: []string{
			".release-notes/archive/v0.2.15/legacy.md",
			".release-notes/archive/v0.3.0/the-os.md",
		}}
		uc := NewRenderReleaseBodyUseCase(gitRepo, fsRepo, cliff)
		body, err := uc.Execute(testRenderReleaseBodyContext(t), RenderReleaseBodyInput{
			Tag:      "v0.3.0-beta.1",
			GitRange: "v0.2.15..abcdef",
			Initial:  false,
		})
		require.NoError(t, err)
		assert.Equal(t, "v0.3.0-beta.1", cliff.tag)
		assert.Equal(t, "v0.2.15..abcdef", cliff.gitRange)
		assert.False(t, cliff.initial)
		assert.Contains(t, body, "Add beta releases")
		assert.Contains(t, body, "##### The OS release")
		assert.NotContains(t, body, "Legacy fix")
	})

	t.Run("Should consume pending notes regardless of their age and exclude historical archives", func(t *testing.T) {
		t.Parallel()
		fsRepo := afero.NewMemMapFs()
		files := map[string]string{
			".release-notes/stable.md":               "Pending stable warning",
			".release-notes/archive/v0.4.0/old.md":   "Already archived warning",
			".release-notes/archive/v0.3.0/other.md": "Other core warning",
		}
		for path, title := range files {
			require.NoError(t, fsRepo.MkdirAll(filepath.Dir(path), 0755))
			body := "---\ntitle: " + title + "\ntype: highlight\n---\nMigration guidance.\n"
			require.NoError(t, afero.WriteFile(fsRepo, path, []byte(body), 0644))
		}
		cliff := &releaseBodyCliffStub{changelog: "## 0.4.0\n\n- Current delta"}
		gitRepo := &releaseBodyGitStub{paths: []string{".release-notes/archive/v0.3.0/other.md"}}
		uc := NewRenderReleaseBodyUseCase(gitRepo, fsRepo, cliff)
		body, err := uc.Execute(testRenderReleaseBodyContext(t), RenderReleaseBodyInput{
			Tag: "v0.4.0", GitRange: "v0.3.0-beta.39..HEAD",
		})
		require.NoError(t, err)
		assert.Contains(t, body, "Pending stable warning")
		assert.Contains(t, body, "Current delta")
		assert.NotContains(t, body, "Already archived warning")
		assert.NotContains(t, body, "Other core warning")
		require.NoError(t, fsRepo.Rename(
			".release-notes/stable.md", ".release-notes/archive/v0.4.0/stable.md",
		))
		gitRepo.paths = append(gitRepo.paths, ".release-notes/archive/v0.4.0/stable.md")
		publishedBody, err := uc.Execute(testRenderReleaseBodyContext(t), RenderReleaseBodyInput{
			Tag: "v0.4.0", GitRange: "v0.3.0-beta.39..release-commit",
		})
		require.NoError(t, err)
		assert.Equal(t, body, publishedBody)
	})

	t.Run("Should preserve initial release notes after preparation archives them", func(t *testing.T) {
		t.Parallel()
		fsRepo := afero.NewMemMapFs()
		require.NoError(t, fsRepo.MkdirAll(".release-notes/archive/v1.0.0", 0755))
		require.NoError(t, fsRepo.MkdirAll(".release-notes/archive/v0.9.0", 0755))
		note := []byte("---\ntitle: First release\ntype: highlight\n---\nInitial guidance.\n")
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/first.md", note, 0644))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/archive/v0.9.0/other.md",
			[]byte("---\ntitle: Other core\ntype: highlight\n---\nOld guidance.\n"), 0644))
		cliff := &releaseBodyCliffStub{changelog: "## 1.0.0\n\n- Initial changes"}
		uc := NewRenderReleaseBodyUseCase(&releaseBodyGitStub{}, fsRepo, cliff)
		input := RenderReleaseBodyInput{Tag: "v1.0.0", Initial: true}
		before, err := uc.Execute(testRenderReleaseBodyContext(t), input)
		require.NoError(t, err)
		require.NoError(t, fsRepo.Rename(".release-notes/first.md", ".release-notes/archive/v1.0.0/first.md"))
		after, err := uc.Execute(testRenderReleaseBodyContext(t), input)
		require.NoError(t, err)
		assert.Contains(t, after, "Initial guidance.")
		assert.NotContains(t, after, "Old guidance.")
		assert.Equal(t, before, after)
	})

	t.Run("Should render only the changelog when the range adds no custom notes", func(t *testing.T) {
		t.Parallel()

		cliff := &releaseBodyCliffStub{changelog: "## 0.3.0-beta.2\n\n### Fixes\n\n- Fix recovery"}
		uc := NewRenderReleaseBodyUseCase(&releaseBodyGitStub{}, afero.NewMemMapFs(), cliff)
		body, err := uc.Execute(testRenderReleaseBodyContext(t), RenderReleaseBodyInput{
			Tag:      "v0.3.0-beta.2",
			GitRange: "v0.3.0-beta.1..fedcba",
			Initial:  false,
		})
		require.NoError(t, err)
		assert.Equal(t, cliff.changelog, body)
	})

	t.Run("Should preserve added-file discovery failures", func(t *testing.T) {
		t.Parallel()

		cliff := &releaseBodyCliffStub{changelog: "## 0.3.0-beta.2"}
		uc := NewRenderReleaseBodyUseCase(
			&releaseBodyGitStub{err: errors.New("git diff unavailable")},
			afero.NewMemMapFs(),
			cliff,
		)
		body, err := uc.Execute(testRenderReleaseBodyContext(t), RenderReleaseBodyInput{
			Tag:      "v0.3.0-beta.2",
			GitRange: "v0.3.0-beta.1..fedcba",
			Initial:  false,
		})
		require.Error(t, err)
		assert.Empty(t, body)
		assert.ErrorContains(t, err, "git diff unavailable")
	})
	t.Run("Should reject a missing range outside explicit initial mode", func(t *testing.T) {
		t.Parallel()

		cliff := &releaseBodyCliffStub{changelog: "## 0.3.0-beta.2"}
		uc := NewRenderReleaseBodyUseCase(&releaseBodyGitStub{}, afero.NewMemMapFs(), cliff)
		body, err := uc.Execute(testRenderReleaseBodyContext(t), RenderReleaseBodyInput{Tag: "v0.3.0-beta.2"})
		require.Error(t, err)
		assert.Empty(t, body)
		assert.ErrorContains(t, err, "initial release")
	})
}

func testRenderReleaseBodyContext(t *testing.T) context.Context {
	t.Helper()
	ctx := config.IntoContext(t.Context(), config.DefaultConfig())
	return logger.IntoContext(ctx, zap.NewNop())
}
