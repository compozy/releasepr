package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/compozy/releasepr/internal/config"
	"github.com/spf13/afero"
	"go.uber.org/zap"
)

const defaultReleaseArtifactCommandTimeout = 10 * time.Minute

type releaseArtifactCommandRunner func(
	ctx context.Context,
	command *config.ReleaseArtifactCommand,
	env map[string]string,
) error

type releaseArtifactResult struct {
	addPatterns   []string
	modifiedFiles []string
	createdFiles  []string
}

func defaultReleaseArtifactCommandRunner(
	ctx context.Context,
	command *config.ReleaseArtifactCommand,
	env map[string]string,
) error {
	workingDirectory, err := releaseArtifactWorkingDirectory()
	if err != nil {
		return err
	}
	timeout := defaultReleaseArtifactCommandTimeout
	if command.TimeoutSeconds > 0 {
		timeout = time.Duration(command.TimeoutSeconds) * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd, err := releaseArtifactExecCommand(commandCtx, command.Command, command.Args)
	if err != nil {
		return err
	}
	cmd.Dir = workingDirectory
	cmd.Env = releaseArtifactProcessEnv(env)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if commandCtx.Err() != nil {
			return fmt.Errorf("command timed out after %s: %w", timeout, commandCtx.Err())
		}
		return fmt.Errorf("command failed: %w (output: %s)", err, strings.TrimSpace(output.String()))
	}
	return nil
}

func releaseArtifactExecCommand(ctx context.Context, command string, args []string) (*exec.Cmd, error) {
	commandName, err := config.NormalizeReleaseArtifactCommand(command)
	if err != nil {
		return nil, err
	}
	switch commandName {
	case "bun":
		return exec.CommandContext(ctx, "bun", args...), nil
	case "go":
		return exec.CommandContext(ctx, "go", args...), nil
	case "make":
		return exec.CommandContext(ctx, "make", args...), nil
	case "node":
		return exec.CommandContext(ctx, "node", args...), nil
	case "npm":
		return exec.CommandContext(ctx, "npm", args...), nil
	case "npx":
		return exec.CommandContext(ctx, "npx", args...), nil
	case "pnpm":
		return exec.CommandContext(ctx, "pnpm", args...), nil
	case "yarn":
		return exec.CommandContext(ctx, "yarn", args...), nil
	}
	return nil, fmt.Errorf("unsupported release artifact command: %s", commandName)
}

func releaseArtifactWorkingDirectory() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	repoRoot := findRepoRoot(wd)
	if repoRoot == "" {
		return "", fmt.Errorf("repository root not found from %s", wd)
	}
	return repoRoot, nil
}

func releaseArtifactProcessEnv(overrides map[string]string) []string {
	env := os.Environ()
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+overrides[key])
	}
	return env
}

func (o *PRReleaseOrchestrator) runReleaseArtifactCommands(
	ctx context.Context,
	version string,
	branchName string,
	previousTag string,
) (*releaseArtifactResult, error) {
	cfg := config.FromContext(ctx)
	result := &releaseArtifactResult{
		addPatterns: releaseArtifactAddPatterns(cfg.ReleaseArtifacts),
	}
	if len(cfg.ReleaseArtifacts) == 0 {
		return result, nil
	}
	beforeFiles, err := o.releaseArtifactFiles(cfg.ReleaseArtifacts, false)
	if err != nil {
		return nil, err
	}
	before := make(map[string]struct{}, len(beforeFiles))
	for _, file := range beforeFiles {
		before[file] = struct{}{}
	}
	env := releaseArtifactEnvironment(cfg, version, branchName, previousTag)
	for index := range cfg.ReleaseArtifacts {
		command := &cfg.ReleaseArtifacts[index]
		name := strings.TrimSpace(command.Name)
		o.logger(ctx).Info("Running release artifact command", zap.String("name", name))
		if err := o.artifactRunner(ctx, command, env); err != nil {
			return nil, fmt.Errorf("release artifact %q failed: %w", name, err)
		}
	}
	afterFiles, err := o.releaseArtifactFiles(cfg.ReleaseArtifacts, true)
	if err != nil {
		return nil, err
	}
	for _, file := range afterFiles {
		if _, ok := before[file]; ok {
			result.modifiedFiles = append(result.modifiedFiles, file)
			continue
		}
		result.createdFiles = append(result.createdFiles, file)
	}
	return result, nil
}

func releaseArtifactEnvironment(
	cfg *config.Config,
	version string,
	branchName string,
	previousTag string,
) map[string]string {
	return map[string]string{
		"PR_RELEASE_VERSION":        version,
		"PR_RELEASE_VERSION_NUMBER": strings.TrimPrefix(version, "v"),
		"PR_RELEASE_BRANCH":         branchName,
		"PR_RELEASE_PREVIOUS_TAG":   previousTag,
		"PR_RELEASE_CHANGELOG_PATH": "CHANGELOG.md",
		"PR_RELEASE_BODY_PATH":      ReleaseBodyOutputFile,
		"PR_RELEASE_NOTES_PATH":     ReleaseNotesOutputFile,
		"PR_RELEASE_DATE":           time.Now().UTC().Format(time.RFC3339),
		"PR_RELEASE_GITHUB_OWNER":   cfg.GithubOwner,
		"PR_RELEASE_GITHUB_REPO":    cfg.GithubRepo,
	}
}

func (o *PRReleaseOrchestrator) releaseArtifactFiles(
	commands []config.ReleaseArtifactCommand,
	requireMatches bool,
) ([]string, error) {
	files := map[string]struct{}{}
	for _, command := range commands {
		for _, pattern := range command.Add {
			matches, err := afero.Glob(o.fsRepo, pattern)
			if err != nil {
				return nil, fmt.Errorf("release artifact %q add pattern %q is invalid: %w", command.Name, pattern, err)
			}
			if requireMatches && len(matches) == 0 {
				return nil, fmt.Errorf("release artifact %q add pattern %q did not match files", command.Name, pattern)
			}
			for _, match := range matches {
				files[filepath.ToSlash(match)] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(files))
	for file := range files {
		result = append(result, file)
	}
	sort.Strings(result)
	return result, nil
}

func releaseArtifactAddPatterns(commands []config.ReleaseArtifactCommand) []string {
	seen := map[string]struct{}{}
	patterns := make([]string, 0)
	for _, command := range commands {
		for _, pattern := range command.Add {
			if _, ok := seen[pattern]; ok {
				continue
			}
			seen[pattern] = struct{}{}
			patterns = append(patterns, pattern)
		}
	}
	return patterns
}
