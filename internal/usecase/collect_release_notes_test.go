package usecase

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectReleaseNotesUseCase_Execute(t *testing.T) {
	t.Run("Should render notes in type order and preserve markdown bodies", func(t *testing.T) {
		fsRepo := afero.NewMemMapFs()
		require.NoError(t, fsRepo.MkdirAll(releaseNotesArchiveDir("v0.1.0"), 0755))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/z-highlight.md", []byte(`---
title: Performance improvements
type: highlight
---

Build time dropped by **40%**.
`), 0644))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/a-breaking.md", []byte(`---
title: New auth middleware
type: breaking
---

Use the new token format.
`), 0644))
		featureNote := "---\n" +
			"title: Shared layout package\n" +
			"type: feature\n" +
			"---\n\n" +
			"Supports code fences:\n\n" +
			"```go\n" +
			"fmt.Println(\"ok\")\n" +
			"```\n\n" +
			"Literal template markers stay untouched: {{ .Value }}\n"
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/b-feature.md", []byte(featureNote), 0644))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/archive/v0.1.0/old.md", []byte(`---
title: Old note
type: fix
---

Archived note.
`), 0644))
		uc := &CollectReleaseNotesUseCase{
			FSRepo: fsRepo,
		}
		collection, err := uc.Execute(t.Context(), "v0.2.0")
		require.NoError(t, err)
		require.Len(t, collection.Notes, 3)
		rendered := collection.RenderMarkdown()
		assert.Contains(t, rendered, "### Release Notes")
		assert.True(t, orderedSubstrings(rendered,
			"#### Breaking Changes",
			"##### New auth middleware",
			"#### Features",
			"##### Shared layout package",
			"#### Highlights",
			"##### Performance improvements",
		))
		assert.Contains(t, rendered, "```go")
		assert.Contains(t, rendered, "{{ .Value }}")
		assert.NotContains(t, rendered, "Archived note.")
	})
	t.Run("Should include archived notes for the requested version on rerun", func(t *testing.T) {
		fsRepo := afero.NewMemMapFs()
		require.NoError(t, fsRepo.MkdirAll(releaseNotesArchiveDir("v1.2.3"), 0755))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/archive/v1.2.3/a-fix.md", []byte(`---
title: Fixed release branch rerun
type: fix
---

The archived note still belongs to this release.
`), 0644))
		uc := &CollectReleaseNotesUseCase{
			FSRepo: fsRepo,
		}
		collection, err := uc.Execute(t.Context(), "v1.2.3")
		require.NoError(t, err)
		require.Len(t, collection.Notes, 1)
		assert.Equal(t, "Fixed release branch rerun", collection.Notes[0].Title)
		assert.Contains(t, collection.RenderMarkdown(), "The archived note still belongs to this release.")
	})
	t.Run("Should skip invalid files and keep warnings", func(t *testing.T) {
		fsRepo := afero.NewMemMapFs()
		require.NoError(t, fsRepo.MkdirAll(releaseNotesDir, 0755))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/invalid.md", []byte("missing frontmatter"), 0644))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/empty.md", []byte(`---
title: Empty body
type: fix
---
`), 0644))
		require.NoError(t, afero.WriteFile(fsRepo, ".release-notes/valid.md", []byte(`---
title: Fixed startup race
type: fix
---

The worker now waits for readiness.
`), 0644))
		uc := &CollectReleaseNotesUseCase{
			FSRepo: fsRepo,
		}
		collection, err := uc.Execute(t.Context(), "")
		require.NoError(t, err)
		require.Len(t, collection.Notes, 1)
		assert.Len(t, collection.Warnings, 2)
		assert.Contains(t, collection.RenderMarkdown(), "##### Fixed startup race")
	})
	t.Run("Should return empty collection when directory does not exist", func(t *testing.T) {
		uc := &CollectReleaseNotesUseCase{
			FSRepo: afero.NewMemMapFs(),
		}
		collection, err := uc.Execute(t.Context(), "")
		require.NoError(t, err)
		assert.Empty(t, collection.Notes)
		assert.Empty(t, collection.RenderMarkdown())
	})
}

func orderedSubstrings(input string, items ...string) bool {
	index := 0
	for _, item := range items {
		position := strings.Index(input[index:], item)
		if position == -1 {
			return false
		}
		index += position + len(item)
	}
	return true
}
