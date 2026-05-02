package main

import (
	"encoding/json"
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

	t.Run("Should dispatch release pull request checks after automation updates", func(t *testing.T) {
		root := loadWorkflowRoot(t, ".github/workflows/release.yml")
		permissions := mappingValue(root, "permissions")
		require.NotNil(t, permissions)
		require.Equal(t, "write", mappingValue(permissions, "actions").Value)

		workflowDispatch := mappingValue(mappingValue(root, "on"), "workflow_dispatch")
		require.NotNil(t, workflowDispatch)
		inputs := mappingValue(workflowDispatch, "inputs")
		require.NotNil(t, inputs)
		assert.NotNil(t, mappingValue(inputs, "mode"))
		assert.NotNil(t, mappingValue(inputs, "head_ref"))
		assert.NotNil(t, mappingValue(inputs, "pr_number"))

		jobs := mappingValue(root, "jobs")
		releasePR := mappingValue(jobs, "release-pr")
		require.NotNil(t, releasePR)
		releasePRIf := mappingValue(releasePR, "if")
		require.NotNil(t, releasePRIf)
		assert.Contains(t, releasePRIf.Value, "inputs.mode == 'release-pr'")

		dispatchStep := findStepByName(t, releasePR, "Dispatch Release PR Checks")
		dispatchRun := mappingValue(dispatchStep, "run")
		require.NotNil(t, dispatchRun)
		assert.Contains(t, dispatchRun.Value, "gh workflow run ci.yml --ref \"$branch\"")
		assert.Contains(t, dispatchRun.Value, "gh workflow run release.yml")
		assert.Contains(t, dispatchRun.Value, "-f mode=dry-run")

		dryRun := mappingValue(jobs, "dry-run")
		require.NotNil(t, dryRun)
		dryRunIf := mappingValue(dryRun, "if")
		require.NotNil(t, dryRunIf)
		assert.Contains(t, dryRunIf.Value, "github.event_name == 'workflow_dispatch'")
		assert.Contains(t, dryRunIf.Value, "inputs.mode == 'dry-run'")

		checkoutStep := findStepByUses(t, dryRun, "actions/checkout@v4")
		checkoutWith := mappingValue(checkoutStep, "with")
		require.NotNil(t, checkoutWith)
		assert.Contains(t, mappingValue(checkoutWith, "ref").Value, "inputs.head_ref")

		dryRunStep := findStepByName(t, dryRun, "Run Dry-Run Orchestrator")
		dryRunEnv := mappingValue(dryRunStep, "env")
		require.NotNil(t, dryRunEnv)
		assert.Contains(t, mappingValue(dryRunEnv, "GITHUB_HEAD_REF").Value, "inputs.head_ref")
		assert.Contains(t, mappingValue(dryRunEnv, "GITHUB_ISSUE_NUMBER").Value, "inputs.pr_number")
	})

	t.Run("Should publish releases from current-version body artifact", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Clean(".github/workflows/release.yml"))
		require.NoError(t, err)
		workflow := string(data)
		assert.Contains(t, workflow, "--release-notes=RELEASE_BODY.md")
		assert.NotContains(t, workflow, "--release-notes=RELEASE_NOTES.md")
	})
}

func TestPackageManifestConfig(t *testing.T) {
	t.Run("Should keep Bun lockfile aligned with declared package dependencies", func(t *testing.T) {
		manifest := loadPackageManifest(t)
		_, err := os.Stat("bun.lock")
		if manifest.HasDependencies() {
			require.NoError(t, err)
			return
		}
		assert.True(t, os.IsNotExist(err), "bun.lock should not exist when package.json declares no dependencies")
	})
}

type packageManifest struct {
	Dependencies         map[string]string `json:"dependencies"`
	DevDependencies      map[string]string `json:"devDependencies"`
	OptionalDependencies map[string]string `json:"optionalDependencies"`
	PeerDependencies     map[string]string `json:"peerDependencies"`
}

func (m packageManifest) HasDependencies() bool {
	return len(m.Dependencies) > 0 ||
		len(m.DevDependencies) > 0 ||
		len(m.OptionalDependencies) > 0 ||
		len(m.PeerDependencies) > 0
}

func loadPackageManifest(t *testing.T) packageManifest {
	t.Helper()
	data, err := os.ReadFile("package.json")
	require.NoError(t, err)
	var manifest packageManifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	return manifest
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

func findStepByName(t *testing.T, job *yaml.Node, name string) *yaml.Node {
	t.Helper()
	for _, step := range workflowSteps(t, job) {
		nameNode := mappingValue(step, "name")
		if nameNode != nil && nameNode.Value == name {
			return step
		}
	}
	t.Fatalf("step %q not found", name)
	return nil
}

func findStepByUses(t *testing.T, job *yaml.Node, uses string) *yaml.Node {
	t.Helper()
	for _, step := range workflowSteps(t, job) {
		usesNode := mappingValue(step, "uses")
		if usesNode != nil && usesNode.Value == uses {
			return step
		}
	}
	t.Fatalf("step using %q not found", uses)
	return nil
}

func workflowSteps(t *testing.T, job *yaml.Node) []*yaml.Node {
	t.Helper()
	steps := mappingValue(job, "steps")
	require.NotNil(t, steps)
	require.Equal(t, yaml.SequenceNode, steps.Kind)
	return steps.Content
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
