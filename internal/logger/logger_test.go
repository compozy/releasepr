// Contract: structured diagnostics never contaminate machine-readable command stdout.
// Owning layer: logger configuration.
// Canonical suite: this file; the logger package had no existing test suite.
// Rationale: plan and release-body expose stdout as a public automation contract.
package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildZapConfig(t *testing.T) {
	t.Parallel()

	t.Run("Should write structured logs and logger errors to stderr", func(t *testing.T) {
		t.Parallel()

		config, err := buildZapConfig(Config{Level: "info", Format: "json"})
		require.NoError(t, err)
		assert.Equal(t, []string{"stderr"}, config.OutputPaths)
		assert.Equal(t, []string{"stderr"}, config.ErrorOutputPaths)
	})
}
