// Contract: explicit release inputs map to one canonical publication plan without normalization.
// Owning layer: release-plan domain policy.
// Canonical suite: this file; no existing suite owns channel policy.
// Rationale: this is a public release contract, not an implementation-detail snapshot.
package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReleasePlan(t *testing.T) {
	t.Parallel()

	t.Run("Should create the beta publication plan", func(t *testing.T) {
		t.Parallel()

		plan, err := NewReleasePlan("refs/heads/main", "abc123", "0.3.0-beta.1", ReleaseChannelBeta)
		require.NoError(t, err)
		assert.Equal(t, "refs/heads/main", plan.Ref)
		assert.Equal(t, "abc123", plan.Commit)
		assert.Equal(t, "0.3.0-beta.1", plan.Version)
		assert.Equal(t, "v0.3.0-beta.1", plan.Tag)
		assert.Equal(t, ReleaseChannelBeta, plan.Channel)
		assert.True(t, plan.GitHubPrerelease)
		assert.False(t, plan.GitHubMakeLatest)
		assert.Equal(t, "beta", plan.NPMTag)
		assert.True(t, plan.HomebrewSkipUpload)
	})

	for _, channel := range []ReleaseChannel{ReleaseChannelStable, ReleaseChannelLegacy} {
		t.Run("Should create the stable publication plan for "+string(channel), func(t *testing.T) {
			t.Parallel()

			plan, err := NewReleasePlan("HEAD", "abc123", "0.3.0", channel)
			require.NoError(t, err)
			assert.False(t, plan.GitHubPrerelease)
			assert.True(t, plan.GitHubMakeLatest)
			assert.Equal(t, "latest", plan.NPMTag)
			assert.False(t, plan.HomebrewSkipUpload)
		})
	}

	t.Run("Should reject a version with the tag prefix", func(t *testing.T) {
		t.Parallel()

		plan, err := NewReleasePlan("HEAD", "abc123", "v0.3.0-beta.1", ReleaseChannelBeta)
		require.Error(t, err)
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "must not start with v")
	})

	t.Run("Should reject a non-strict semantic version", func(t *testing.T) {
		t.Parallel()

		plan, err := NewReleasePlan("HEAD", "abc123", "0.3", ReleaseChannelStable)
		require.Error(t, err)
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "strict semantic version")
	})

	t.Run("Should require beta prerelease semantics for the beta channel", func(t *testing.T) {
		t.Parallel()

		plan, err := NewReleasePlan("HEAD", "abc123", "0.3.0", ReleaseChannelBeta)
		require.Error(t, err)
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "beta channel")
	})

	t.Run("Should reject a beta prerelease on a stable channel", func(t *testing.T) {
		t.Parallel()

		plan, err := NewReleasePlan("HEAD", "abc123", "0.3.0-beta.1", ReleaseChannelStable)
		require.Error(t, err)
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "stable channel")
	})

	t.Run("Should reject an unsupported channel", func(t *testing.T) {
		t.Parallel()

		channel, err := ParseReleaseChannel("preview")
		require.Error(t, err)
		assert.Empty(t, channel)
		assert.ErrorContains(t, err, "unsupported release channel")
	})

	t.Run("Should reject a ref containing an output control character", func(t *testing.T) {
		t.Parallel()

		plan, err := NewReleasePlan("main\nforged=true", "abc123", "0.3.0", ReleaseChannelStable)
		require.Error(t, err)
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "control characters")
	})
}
