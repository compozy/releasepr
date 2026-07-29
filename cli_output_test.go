// Contract: CLI diagnostics use stderr while command payloads remain clean on stdout.
// Owning layer: compiled CLI process boundary.
// Canonical suite: this file; no existing process-level CLI output suite exists.
// Rationale: plan and release-body stdout are consumed as public automation APIs.
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLIOutputContract(t *testing.T) {
	t.Run("Should keep initialization logs out of command stdout", func(t *testing.T) {
		binaryPath := filepath.Join(t.TempDir(), "pr-release")
		build := exec.CommandContext(t.Context(), "go", "build", "-o", binaryPath, ".")
		buildOutput, err := build.CombinedOutput()
		require.NoError(t, err, string(buildOutput))
		command := exec.CommandContext(t.Context(), binaryPath, "version")
		command.Env = append(os.Environ(), "GITHUB_REPOSITORY=compozy/releasepr")
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		require.NoError(t, command.Run())
		assert.Contains(t, stdout.String(), "Version:")
		assert.NotContains(t, stdout.String(), `"logger":`)
		assert.Contains(t, stderr.String(), `"logger":"cmd.container"`)
	})
}
