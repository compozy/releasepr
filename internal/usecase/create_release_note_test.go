package usecase

import (
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateReleaseNoteUseCase_Execute(t *testing.T) {
	t.Run("Should create release note with inline body", func(t *testing.T) {
		fsRepo := afero.NewMemMapFs()
		uc := &CreateReleaseNoteUseCase{
			FSRepo: fsRepo,
			Now: func() time.Time {
				return time.Unix(1713100800, 0)
			},
		}
		path, err := uc.Execute(t.Context(), CreateReleaseNoteInput{
			Title: "Shared layout package",
			Type:  "feature",
			Body:  "Supports full markdown.",
		})
		require.NoError(t, err)
		assert.Equal(t, ".release-notes/shared-layout-package-1713100800.md", path)
		data, readErr := afero.ReadFile(fsRepo, path)
		require.NoError(t, readErr)
		assert.Equal(
			t,
			"---\ntitle: Shared layout package\ntype: feature\n---\n\nSupports full markdown.\n",
			string(data),
		)
	})
	t.Run("Should create YAML-safe titles that round-trip through collection", func(t *testing.T) {
		fsRepo := afero.NewMemMapFs()
		uc := &CreateReleaseNoteUseCase{
			FSRepo: fsRepo,
			Now: func() time.Time {
				return time.Unix(1713100810, 0)
			},
		}
		title := "Fix: auth flow #123"
		path, err := uc.Execute(t.Context(), CreateReleaseNoteInput{
			Title: title,
			Type:  "fix",
			Body:  "Prevents token reuse.",
		})
		require.NoError(t, err)
		data, readErr := afero.ReadFile(fsRepo, path)
		require.NoError(t, readErr)
		assert.Contains(t, string(data), "title:")
		collection, collectErr := (&CollectReleaseNotesUseCase{FSRepo: fsRepo}).Execute(t.Context(), "")
		require.NoError(t, collectErr)
		require.Len(t, collection.Notes, 1)
		assert.Equal(t, title, collection.Notes[0].Title)
	})
	t.Run("Should write template body when body is empty", func(t *testing.T) {
		fsRepo := afero.NewMemMapFs()
		uc := &CreateReleaseNoteUseCase{
			FSRepo: fsRepo,
			Now: func() time.Time {
				return time.Unix(1713100900, 0)
			},
		}
		path, err := uc.Execute(t.Context(), CreateReleaseNoteInput{
			Title: "Auth changes",
			Type:  "breaking",
		})
		require.NoError(t, err)
		data, readErr := afero.ReadFile(fsRepo, path)
		require.NoError(t, readErr)
		assert.Contains(t, string(data), releaseNotesTemplateBody)
	})
	t.Run("Should reject invalid type", func(t *testing.T) {
		uc := &CreateReleaseNoteUseCase{
			FSRepo: afero.NewMemMapFs(),
		}
		path, err := uc.Execute(t.Context(), CreateReleaseNoteInput{
			Title: "Auth changes",
			Type:  "docs",
		})
		require.Error(t, err)
		assert.Empty(t, path)
		assert.ErrorContains(t, err, "invalid release note type")
	})
	t.Run("Should reject empty title", func(t *testing.T) {
		uc := &CreateReleaseNoteUseCase{
			FSRepo: afero.NewMemMapFs(),
		}
		path, err := uc.Execute(t.Context(), CreateReleaseNoteInput{
			Title: "   ",
			Type:  "feature",
		})
		require.Error(t, err)
		assert.Empty(t, path)
		assert.ErrorContains(t, err, "title cannot be empty")
	})
}
