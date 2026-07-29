// Contract: CLI diagnostics use stderr while command payloads remain clean on stdout.
// Owning layer: compiled CLI process boundary.
// Canonical suite: this file; no existing process-level CLI output suite exists.
// Rationale: plan and release-body stdout are consumed as public automation APIs.
package integration_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLIOutputContract(t *testing.T) {
	t.Parallel()

	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	binaryPath := filepath.Join(t.TempDir(), "pr-release")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binaryPath, ".")
	build.Dir = repositoryRoot
	buildOutput, err := build.CombinedOutput()
	require.NoError(t, err, string(buildOutput))

	t.Run("Should keep initialization logs out of command stdout", func(t *testing.T) {
		t.Parallel()

		command := exec.CommandContext(t.Context(), binaryPath, "version")
		command.Dir = repositoryRoot
		command.Env = []string{"GITHUB_REPOSITORY=compozy/releasepr"}
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		require.NoError(t, command.Run())
		assert.Contains(t, stdout.String(), "Version:")
		assert.NotContains(t, stdout.String(), `"logger":`)
		assert.Contains(t, stderr.String(), "cmd.container")
	})

	t.Run("Should expose release-body without GitHub repository configuration", func(t *testing.T) {
		t.Parallel()

		command := exec.CommandContext(t.Context(), binaryPath, "release-body", "--help")
		command.Dir = repositoryRoot
		command.Env = []string{}
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		require.NoError(t, command.Run(), stderr.String())
		assert.Contains(t, stdout.String(), "Render a release body from an explicit Git range")
	})
}
