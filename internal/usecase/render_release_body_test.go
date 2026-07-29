// Contract: an explicit release body contains only changelog entries and custom notes from its canonical range.
// Owning layer: release-body composition use case.
// Canonical suite: this file; no existing suite owns explicit range composition.
// Rationale: the rendered markdown is the artifact published to GitHub and the downstream site receipt.
package usecase

import (
	"context"
	"errors"
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
