// Contract: release-body writes only the rendered markdown to stdout.
// Owning layer: Cobra command boundary.
// Canonical suite: this file; no existing command owns explicit release-body rendering.
// Rationale: stdout is consumed directly by release automation.
package cmd

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/compozy/releasepr/internal/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type releaseBodyRendererStub struct {
	input usecase.RenderReleaseBodyInput
	body  string
	err   error
}

func (s *releaseBodyRendererStub) Execute(
	_ context.Context,
	input usecase.RenderReleaseBodyInput,
) (string, error) {
	s.input = input
	return s.body, s.err
}

func TestNewReleaseBodyCmd(t *testing.T) {
	t.Parallel()

	t.Run("Should write rendered markdown and pass the exact range", func(t *testing.T) {
		t.Parallel()

		renderer := &releaseBodyRendererStub{body: "## 0.3.0-beta.2\n\n- Fix recovery"}
		output := new(bytes.Buffer)
		command := NewReleaseBodyCmd(renderer)
		command.SetOut(output)
		command.SetArgs([]string{
			"--tag", "v0.3.0-beta.2",
			"--range", "v0.3.0-beta.1..abcdef",
		})
		require.NoError(t, command.Execute())
		assert.Equal(t, usecase.RenderReleaseBodyInput{
			Tag:      "v0.3.0-beta.2",
			GitRange: "v0.3.0-beta.1..abcdef",
			Initial:  false,
		}, renderer.input)
		assert.Equal(t, "## 0.3.0-beta.2\n\n- Fix recovery\n", output.String())
	})

	t.Run("Should require the target tag", func(t *testing.T) {
		t.Parallel()

		command := NewReleaseBodyCmd(&releaseBodyRendererStub{})
		err := command.Execute()
		require.Error(t, err)
		assert.ErrorContains(t, err, "required flag")
	})

	t.Run("Should preserve rendering failures", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("render failed")
		command := NewReleaseBodyCmd(&releaseBodyRendererStub{err: expectedErr})
		command.SetArgs([]string{
			"--tag", "v0.3.0-beta.2",
			"--range", "v0.3.0-beta.1..abcdef",
		})
		err := command.Execute()
		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
	})
	t.Run("Should reject a missing selector instead of falling back", func(t *testing.T) {
		t.Parallel()

		command := NewReleaseBodyCmd(&releaseBodyRendererStub{})
		command.SetArgs([]string{"--tag", "v0.3.0-beta.2"})
		err := command.Execute()
		require.Error(t, err)
		assert.ErrorContains(t, err, "initial release")
	})
	t.Run("Should pass explicit initial-release mode to the renderer", func(t *testing.T) {
		t.Parallel()

		renderer := &releaseBodyRendererStub{body: "## 1.0.0"}
		command := NewReleaseBodyCmd(renderer)
		command.SetOut(new(bytes.Buffer))
		command.SetArgs([]string{"--tag", "v1.0.0", "--initial"})
		require.NoError(t, command.Execute())
		assert.Equal(t, usecase.RenderReleaseBodyInput{
			Tag:     "v1.0.0",
			Initial: true,
		}, renderer.input)
	})
}
