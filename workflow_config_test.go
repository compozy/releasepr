package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCIWorkflowConfig(t *testing.T) {
	t.Run("Should define a runnable validation job without undefined dependencies", func(t *testing.T) {
		root := loadWorkflowRoot(t, ".github/workflows/ci.yml")
		jobs := mappingValue(root, "jobs")
		require.NotNil(t, jobs)
		testJob := mappingValue(jobs, "test")
		require.NotNil(t, testJob)
		assert.Nil(t, mappingValue(testJob, "needs"))
		ifNode := mappingValue(testJob, "if")
		if ifNode != nil {
			assert.NotContains(t, ifNode.Value, "needs.changes")
		}
		assert.True(t, hasJobWithoutNeeds(jobs), "workflow must include at least one job with no dependencies")
	})
}

func TestReleaseWorkflowConfig(t *testing.T) {
	t.Run("Should dry run newly opened and updated release pull requests", func(t *testing.T) {
		root := loadWorkflowRoot(t, ".github/workflows/release.yml")
		types := pullRequestTypes(t, root)
		assert.Contains(t, types, "opened")
		assert.Contains(t, types, "synchronize")
		assert.Contains(t, types, "reopened")
	})
}

func loadWorkflowRoot(t *testing.T, path string) *yaml.Node {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal(data, &doc))
	require.Len(t, doc.Content, 1)
	return doc.Content[0]
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func hasJobWithoutNeeds(jobs *yaml.Node) bool {
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return false
	}
	for i := 1; i < len(jobs.Content); i += 2 {
		if mappingValue(jobs.Content[i], "needs") == nil {
			return true
		}
	}
	return false
}

func pullRequestTypes(t *testing.T, root *yaml.Node) []string {
	t.Helper()
	onNode := mappingValue(root, "on")
	require.NotNil(t, onNode)
	pullRequest := mappingValue(onNode, "pull_request")
	require.NotNil(t, pullRequest)
	typesNode := mappingValue(pullRequest, "types")
	require.NotNil(t, typesNode)
	require.Equal(t, yaml.SequenceNode, typesNode.Kind)
	types := make([]string, 0, len(typesNode.Content))
	for _, item := range typesNode.Content {
		types = append(types, item.Value)
	}
	return types
}
